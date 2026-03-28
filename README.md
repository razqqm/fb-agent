# fb-agent

[![Go](https://img.shields.io/badge/Go-1.24+-00ADD8?style=flat&logo=go&logoColor=white)](https://go.dev/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Linux](https://img.shields.io/badge/OS-Linux-FCC624?style=flat&logo=linux&logoColor=black)](https://kernel.org)
[![Static Binary](https://img.shields.io/badge/Binary-Static_(CGO__off)-success)](https://github.com/razqqm/fb-agent)
[![Fluent Bit](https://img.shields.io/badge/Fluent_Bit-3.x-49BDA5?style=flat&logo=fluentbit&logoColor=white)](https://fluentbit.io/)
[![VictoriaLogs](https://img.shields.io/badge/VictoriaLogs-compatible-6C2DC7?style=flat)](https://docs.victoriametrics.com/victorialogs/)
[![Arch: amd64](https://img.shields.io/badge/arch-amd64-blue)](https://github.com/razqqm/fb-agent)
[![Arch: arm64](https://img.shields.io/badge/arch-arm64-blue)](https://github.com/razqqm/fb-agent)

Single-binary infrastructure agent for [Fluent Bit](https://fluentbit.io/) lifecycle management. Installs, configures, monitors, and registers hosts — all from one static Go binary.

**[Документация на русском →](README.ru.md)**

---

## Features

- **Single binary** — no runtime dependencies, no interpreters, one file to deploy
- **Auto-detection** — OS, environment (LXC/Docker/VM/bare-metal), and services with versions
- **Config generation** — Fluent Bit configs from templates with detected services as inputs
- **Service discovery** — SSH, Nginx, PostgreSQL, Redis, Kerio Connect, Rocket.Chat, Fail2Ban, Docker, HAProxy, and 15+ more
- **Host registration** — collects fingerprint (IPs, ports, hardware, services) and sends to [VictoriaLogs](https://docs.victoriametrics.com/victorialogs/)
- **mTLS enrollment** — automatic certificate generation and signing via pure Go crypto
- **Daemon mode** — replaces 4 separate systemd timers with one service (watchdog + registration + cert renewal)
- **Connectivity monitoring** — state machine with configurable offline alerting (default: 6 hours)
- **Cross-platform** — builds for `linux/amd64` and `linux/arm64`

## Quick Start

```bash
# Build
make build

# Or manually
CGO_ENABLED=0 go build -o fb-agent .

# Install Fluent Bit on a host (requires root)
sudo ./fb-agent install

# Check status
./fb-agent status

# Register host in VictoriaLogs
sudo ./fb-agent register

# Run as daemon (replaces cron/timers)
sudo ./fb-agent daemon
```

## Subcommands

| Command | Description |
|---------|-------------|
| `install` | Install Fluent Bit, detect services, generate config, start |
| `register` | Collect host fingerprint, send to VictoriaLogs |
| `watchdog` | One-shot connectivity and health check |
| `daemon` | Long-running mode: watchdog + register + cert renewal |
| `uninstall` | Stop and remove Fluent Bit (add `--purge` for full cleanup) |
| `status` | Show agent health, connectivity, certificates |
| `version` | Print version and build info |

## Configuration

All configuration is via environment variables — no config files for the agent itself.

| Variable | Default | Description |
|----------|---------|-------------|
| `VL_HOST` | `localhost` | VictoriaLogs host |
| `VL_PORT` | `443` | VictoriaLogs port (443=HTTPS, 9428=HTTP, 9429=mTLS) |
| `FB_HOSTNAME` | OS hostname | Override hostname |
| `FB_JOB` | auto-detect | Environment label: `lxc`, `remote`, `docker`, `vm` |
| `FB_LOG_PATHS` | — | Extra log files (colon-separated) |
| `FB_EXTRA_TAGS` | — | Tags for extra log files (colon-separated) |
| `FB_BUFFER_SIZE` | auto by RAM | Filesystem buffer size |
| `FB_GZIP` | auto | Compression: `on`/`off` (auto: on for remote) |
| `FB_FLUSH` | `5` | Flush interval in seconds |
| `FB_SKIP_DETECT` | — | Set to `1` to skip service auto-detection |
| `FB_SKIP_MTLS` | — | Set to `1` to skip mTLS enrollment |
| `CF_CLIENT_ID` | — | Cloudflare Access service token ID |
| `CF_CLIENT_SECRET` | — | Cloudflare Access service token secret |

## How It Works

### Install Flow

1. Detect OS (Debian, Ubuntu, Alpine, RHEL, etc.)
2. Add Fluent Bit package repository (with codename fallbacks: trixie→bookworm, oracular→noble)
3. Install Fluent Bit via package manager
4. Detect environment (LXC, Docker, VM, bare-metal)
5. Auto-discover running services and their log paths
6. Generate `fluent-bit.conf` from embedded templates
7. Deploy embedded `enrich.lua` and custom parsers
8. Optionally enroll mTLS certificates
9. Configure systemd with hardened unit (LimitNOFILE, ProtectSystem, OOMScoreAdjust)
10. Start Fluent Bit and the fb-agent daemon

### Daemon Mode

Replaces separate systemd timers with a single service:

- **Every 5 min** — watchdog: check Fluent Bit health endpoint + output retries
- **Every 24h** — register: update host fingerprint in VictoriaLogs
- **Every 7d** — certificate renewal check (re-enroll if <30 days remaining)
- **Alert** — if offline >6 hours, write alert file + syslog

### Registration Data

The `register` command collects and sends:

```json
{
  "host_id": "machine-id-based fingerprint",
  "hostname": "myhost",
  "internal_ip": "10.0.1.3",
  "external_ip": "203.0.113.1",
  "os": "Debian GNU/Linux 13 (trixie)",
  "environment": "lxc",
  "cpu": "4x AMD EPYC",
  "ram_mb": 4096,
  "open_ports": "22,80,443,5432",
  "services": [{"Name": "SSH", "Status": "active", "Version": "OpenSSH_10.0"}]
}
```

## Building

```bash
# Both architectures
make build

# Single architecture
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "-s -w" -o fb-agent .

# With version info
go build -ldflags "-s -w -X main.version=1.0.0 -X main.buildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)" -o fb-agent .
```

## Verification

```bash
python3 verify.py
```

Runs 52 automated checks: build, linting (golangci-lint), spell check, code quality, spec compliance, bash parity, and binary sanity.

## Requirements

- **Build**: Go 1.21+
- **Runtime**: Linux (systemd-based), root for install/register/daemon
- **Target**: Fluent Bit 3.x, VictoriaLogs

## License

MIT
