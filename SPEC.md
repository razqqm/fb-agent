# fb-agent — Fluent Bit Infrastructure Agent (Go)

## Goal
Single static Go binary that replaces install.sh + register.sh + 4 systemd timers.
Manages Fluent Bit lifecycle: install, configure, monitor, register, cert renewal.

## CLI Interface
```
fb-agent install    # Install FB, detect services, generate config, start
fb-agent register   # Collect host fingerprint, send to VictoriaLogs
fb-agent watchdog   # One-shot connectivity check
fb-agent daemon     # Long-running: watchdog + register + cert-renew on schedule
fb-agent uninstall  # Stop FB, remove configs, optionally purge
fb-agent status     # Show FB health, connectivity, cert expiry
fb-agent version    # Show version + build info
```

No external CLI frameworks. Use `os.Args` for subcommands + simple flag parsing.

## Architecture

### Package Layout
```
fb-agent/
├── main.go              # Entry point, subcommand dispatch
├── cmd/
│   ├── install.go       # Install subcommand
│   ├── register.go      # Register subcommand
│   ├── watchdog.go      # Watchdog subcommand
│   ├── daemon.go        # Daemon (scheduler) subcommand
│   ├── uninstall.go     # Uninstall subcommand
│   ├── status.go        # Status subcommand
│   └── version.go       # Version subcommand
├── detect/
│   ├── os.go            # OS detection (Debian/Ubuntu/Alpine/RHEL)
│   ├── environment.go   # Environment (LXC/Docker/VM/bare-metal)
│   └── services.go      # Service auto-discovery
├── config/
│   ├── generate.go      # Config generation from templates
│   └── templates.go     # Template loading from embed
├── network/
│   ├── fingerprint.go   # Host ID, IPs, reverse DNS
│   └── mtls.go          # mTLS enrollment + renewal
├── health/
│   ├── watchdog.go      # FB health + VL connectivity checks
│   └── state.go         # Persistent state (connectivity.state)
├── pkg/
│   ├── installer.go     # Package manager abstraction (apt/yum/apk)
│   └── systemd.go       # Systemd unit management
├── embedded/
│   ├── embed.go         # go:embed declarations
│   ├── enrich.lua       # Lua enrichment script
│   ├── parsers-custom.conf
│   ├── fluent-bit.conf.tmpl
│   └── fb-agent.service.tmpl
├── go.mod
├── go.sum
├── Makefile
└── README.md
```

### Key Design Decisions

1. **Pure Go, no CGO** → easy cross-compile: `GOOS=linux GOARCH=amd64|arm64`
2. **`embed.FS`** for all templates/configs → single binary, no external files at runtime
3. **`text/template`** for config generation → no regex sed gymnastics
4. **`crypto/tls` + `crypto/x509`** for mTLS → no shelling out to openssl
5. **`coreos/go-systemd/v22/daemon`** for sd_notify → proper systemd integration
6. **Minimal deps**: only go-systemd + stdlib. No cobra, no viper, no logrus.
7. **Structured logging**: `log/slog` (stdlib since Go 1.21)

### Daemon Mode (replaces 4 systemd timers)
Single systemd unit `fb-agent.service` that runs:
- **Watchdog**: every 5 min — check FB health endpoint + output retries
- **Register**: every 24h — host fingerprint → VL
- **Cert renew**: every 7d — check cert expiry, re-enroll if <30d
- **Alert**: if offline >6h → write alert file + syslog

Use `time.Ticker` for scheduling, `context.Context` for shutdown.
sd_notify(READY=1) after startup, WATCHDOG=1 on each tick.

### Environment Variables (same as bash, backward compatible)
```
FB_HOSTNAME      # Override hostname
FB_JOB           # lxc|remote|docker|k8s
VL_HOST          # VictoriaLogs host (default: logs.ilia.ae)
VL_PORT          # 443=HTTPS+CF, 9428=direct, 9429=mTLS
FB_LOG_PATHS     # Extra log files (colon-separated)
FB_EXTRA_TAGS    # Tags for extra files (colon-separated)
FB_BUFFER_SIZE   # Filesystem buffer (auto by RAM)
FB_GZIP          # on|off (auto: on for remote)
FB_FLUSH         # Flush interval seconds (default 5)
FB_SKIP_DETECT   # Skip service auto-detection
FB_SKIP_MTLS     # Skip mTLS enrollment
FB_TLS_CA        # CA cert path
FB_TLS_CERT      # Client cert path
FB_TLS_KEY       # Client key path
CF_CLIENT_ID     # Cloudflare Access service token ID
CF_CLIENT_SECRET # Cloudflare Access secret
```

### Service Auto-Detection (port from bash)
Detect via: systemctl is-active, file existence, docker ps
Services: Kerio Connect, Nginx, Apache, Rocket.Chat, PostgreSQL, MySQL/MariaDB,
MongoDB, Redis, MinIO, Fail2Ban, Docker, Cloudflared, HAProxy, Grafana, ioBroker

For each: log paths, parser name, tag name.

### Config Generation
Use `text/template` with struct:
```go
type Config struct {
    Hostname     string
    Job          string
    OSID         string
    OSCodename   string
    Services     string
    FlushSec     int
    BufferSize   string
    VLHost       string
    VLPort       int
    Gzip         bool
    TLS          TLSConfig
    JournalInput bool
    FileInputs   []FileInput
    GeneratedAt  string
}
```

### mTLS Flow (pure Go)
1. Generate RSA 2048 key → `crypto/rsa`
2. Create CSR with CN=hostname → `crypto/x509`
3. POST CSR to signing API → `net/http`
4. Save cert/key to `/etc/fluent-bit/certs/`
5. Verify with `x509.Certificate.NotAfter`

### Host Registration (VictoriaLogs)
Collect and POST as jsonline:
- host_id (machine-id / product_uuid / hash)
- hostname, fqdn, internal_ip, external_ip, reverse_dns
- os, kernel, arch, cpu, ram_mb, disk
- environment (lxc/docker/vm/bare-metal)
- open_ports (from /proc/net/tcp or net.Listen probe)
- services (detailed with versions)
- fluent_bit status

### Build
```makefile
VERSION := $(shell git describe --tags --always 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION) -X main.buildTime=$(shell date -u +%Y-%m-%dT%H:%M:%SZ)

build:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o fb-agent-linux-amd64
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o fb-agent-linux-arm64
```

### Error Handling
- All errors wrapped with `fmt.Errorf("context: %w", err)`
- `install` exits non-zero on failure with clear message
- `daemon` logs errors via slog but keeps running
- Colored output: same color scheme as bash (green/yellow/red/blue)

### What NOT to include
- No web UI
- No config file for fb-agent itself (env vars only)
- No automatic updates
- No Windows/macOS support
- No Prometheus metrics endpoint (FB already has one)

## Reference: existing bash scripts
The full install.sh and register.sh are in /home/openclaw/.openclaw/workspace/fluent-bit/
Port ALL logic from there, including:
- enrich.lua (embed verbatim)
- parsers-custom.conf (embed verbatim)
- OS repo fallback (trixie→bookworm, oracular→noble)
- Systemd hardening (LimitNOFILE, OOMScoreAdjust, TimeoutStopSec)
- Uninstall script logic
- Connectivity state machine (last_ok, fail_count, alert_sent)
- CF Access headers for remote hosts
- Auto buffer sizing by RAM
- Watchdog alert after 6h offline
