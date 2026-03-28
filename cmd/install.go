package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"text/template"
	"time"

	"github.com/razqqm/fb-agent/cmd/ui"
	"github.com/razqqm/fb-agent/config"
	"github.com/razqqm/fb-agent/detect"
	"github.com/razqqm/fb-agent/embedded"
	"github.com/razqqm/fb-agent/network"
	"github.com/razqqm/fb-agent/pkg"
)

// Install is the entry point for the "install" subcommand. It ports the full
// logic of install.sh.
func Install() {
	// 1. Check root
	if os.Getuid() != 0 {
		ui.Err("Must be run as root: sudo fb-agent install")
		os.Exit(1)
	}

	// 2. Detect OS
	osInfo := detect.DetectOS()

	// 3. Detect environment
	job := envOr("FB_JOB", detect.DetectEnvironment())

	hostname := envOr("FB_HOSTNAME", mustHostname())
	vlHost := envOr("VL_HOST", "logs.ilia.ae")
	vlPortStr := envOr("VL_PORT", "443")
	vlPort := mustParsePort(vlPortStr)
	cfID := envOr("CF_CLIENT_ID", "")
	cfSecret := envOr("CF_CLIENT_SECRET", "")
	flushSec := envOrInt("FB_FLUSH", 5)

	// 4. Auto buffer size based on RAM
	bufferSize := envOr("FB_BUFFER_SIZE", autoBufferSize())

	// 5. Auto gzip
	gzip := false
	if gzipEnv := os.Getenv("FB_GZIP"); gzipEnv != "" {
		gzip = gzipEnv == "on" || gzipEnv == "1"
	} else if vlPort == 443 || job == "remote" || job == "vm" {
		gzip = true
	}

	ui.OK(fmt.Sprintf("Hostname: %s | Job: %s | OS: %s/%s", hostname, job, osInfo.ID, osInfo.Codename))
	ui.OK(fmt.Sprintf("Target: %s:%d | Buffer: %s | Gzip: %v", vlHost, vlPort, bufferSize, gzip))

	// 6. Detect services (unless FB_SKIP_DETECT=1)
	var fileInputs []detect.FileInput
	detectedServices := ""
	if os.Getenv("FB_SKIP_DETECT") != "1" {
		ui.Info("Auto-detecting services...")
		fileInputs, detectedServices = detect.DetectServices()
		if detectedServices != "" {
			ui.OK("Detected: " + detectedServices)
		} else {
			ui.OK("No additional services detected (journal only)")
		}
	} else {
		ui.Info("Service detection skipped (FB_SKIP_DETECT=1)")
	}

	// 7. Add manual paths from FB_LOG_PATHS env
	if logPaths := os.Getenv("FB_LOG_PATHS"); logPaths != "" {
		manualPaths := strings.Split(logPaths, ":")
		extraTags := strings.Split(os.Getenv("FB_EXTRA_TAGS"), ":")
		for i, path := range manualPaths {
			path = strings.TrimSpace(path)
			if path == "" {
				continue
			}
			// Dedup
			dup := false
			for _, fi := range fileInputs {
				if fi.Path == path {
					dup = true
					break
				}
			}
			if dup {
				continue
			}
			tag := fmt.Sprintf("manual_%d", i)
			if i < len(extraTags) && extraTags[i] != "" {
				tag = extraTags[i]
			}
			parser := ""
			switch {
			case strings.Contains(path, "kerio/mail") || strings.Contains(path, "maillog"):
				parser = "kerio_mail"
			case strings.Contains(path, "kerio/security"):
				parser = "kerio_security"
			case strings.Contains(path, "nginx/access") || strings.HasSuffix(path, "access.log"):
				parser = "nginx_access"
			case strings.Contains(path, "nginx/error") || strings.HasSuffix(path, "error.log"):
				parser = "nginx_error"
			}
			fileInputs = append(fileInputs, detect.FileInput{Path: path, Tag: tag, Parser: parser})
			ui.Info(fmt.Sprintf("  Manual: %s → tag:%s", path, tag))
		}
	}

	// 8. Install Fluent Bit
	installer := pkg.NewInstaller(osInfo.ID)
	if !installer.IsInstalled("fluent-bit") {
		ui.OK("Installing Fluent Bit...")
		repoCodename := detect.RepoCodename(osInfo)
		if repoCodename != osInfo.Codename {
			ui.Warn(fmt.Sprintf("FB repo: %s → fallback %s", osInfo.Codename, repoCodename))
		}
		if osInfo.ID == "debian" || osInfo.ID == "ubuntu" {
			if err := pkg.AddAptRepo(osInfo.ID, repoCodename); err != nil {
				ui.Err(fmt.Sprintf("Failed to add apt repo: %v", err))
				os.Exit(1)
			}
		}
		if err := installer.Install("fluent-bit"); err != nil {
			ui.Err(fmt.Sprintf("Failed to install fluent-bit: %v", err))
			os.Exit(1)
		}
	} else {
		out, _ := exec.Command("fluent-bit", "--version").Output()
		ui.OK("Fluent Bit already installed: " + strings.TrimSpace(string(out)))
	}

	// 9. Write embedded configs
	if err := os.MkdirAll("/etc/fluent-bit/parsers.d", 0755); err != nil {
		ui.Err("Failed to create config dirs: " + err.Error())
		os.Exit(1)
	}
	if err := os.MkdirAll("/var/lib/fluent-bit", 0755); err != nil {
		ui.Err("Failed to create var dir: " + err.Error())
		os.Exit(1)
	}

	// Write enrich.lua with HOSTNAME/JOB substitution
	ui.Info("Writing enrich.lua...")
	enrichContent := strings.ReplaceAll(string(embedded.EnrichLua), "FBHOST_PLACEHOLDER", hostname)
	enrichContent = strings.ReplaceAll(enrichContent, "FBJOB_PLACEHOLDER", job)
	if err := os.WriteFile("/etc/fluent-bit/enrich.lua", []byte(enrichContent), 0600); err != nil {
		ui.Err("Failed to write enrich.lua: " + err.Error())
		os.Exit(1)
	}

	// Write parsers-custom.conf
	ui.Info("Writing parsers-custom.conf...")
	if err := os.WriteFile("/etc/fluent-bit/parsers.d/parsers-custom.conf", embedded.ParsersConf, 0644); err != nil {
		ui.Err("Failed to write parsers: " + err.Error())
		os.Exit(1)
	}

	// 10. Auto-enroll mTLS cert
	tlsCfg := config.TLSConfig{Mode: "off"}

	// Check if certs already exist
	if fileExists("/etc/fluent-bit/certs/ca.crt") &&
		fileExists("/etc/fluent-bit/certs/client.crt") &&
		fileExists("/etc/fluent-bit/certs/client.key") {
		tlsCfg = config.TLSConfig{
			CA:   "/etc/fluent-bit/certs/ca.crt",
			Cert: "/etc/fluent-bit/certs/client.crt",
			Key:  "/etc/fluent-bit/certs/client.key",
			Mode: "mtls",
		}
		ui.OK("mTLS: existing certificates found")
	} else if os.Getenv("FB_SKIP_MTLS") != "1" && vlPort != 443 {
		ui.Info("mTLS: requesting certificate...")
		if err := network.EnrollCert(hostname, vlHost, vlPort, cfID, cfSecret); err != nil {
			ui.Warn("mTLS: enrollment failed (continuing without): " + err.Error())
		} else {
			tlsCfg = config.TLSConfig{
				CA:   "/etc/fluent-bit/certs/ca.crt",
				Cert: "/etc/fluent-bit/certs/client.crt",
				Key:  "/etc/fluent-bit/certs/client.key",
				Mode: "mtls",
			}
			ui.OK("mTLS: certificate enrolled")
		}
	} else if vlPort == 443 {
		ui.OK("TLS: CF Access (Bearer token via service token)")
		tlsCfg = config.TLSConfig{Mode: "cf-access"}
	}

	// Check for manually-provided TLS paths
	if tlsCA := os.Getenv("FB_TLS_CA"); tlsCA != "" {
		tlsCfg = config.TLSConfig{
			CA:   tlsCA,
			Cert: os.Getenv("FB_TLS_CERT"),
			Key:  os.Getenv("FB_TLS_KEY"),
			Mode: "mtls",
		}
	}

	// Detect journal availability
	journalInput := dirExists("/run/log/journal") || dirExists("/var/log/journal")

	// 11. Generate fluent-bit.conf
	ui.Info("Generating fluent-bit.conf...")
	cfg := config.Config{
		Hostname:     hostname,
		Job:          job,
		OSID:         osInfo.ID,
		OSCodename:   osInfo.Codename,
		Services:     detectedServices,
		FlushSec:     flushSec,
		BufferSize:   bufferSize,
		VLHost:       vlHost,
		VLPort:       vlPort,
		Gzip:         gzip,
		TLS:          tlsCfg,
		CFID:         cfID,
		CFSecret:     cfSecret,
		JournalInput: journalInput,
		FileInputs:   fileInputs,
		GeneratedAt:  time.Now().UTC().Format(time.RFC3339),
	}

	if !journalInput && len(fileInputs) == 0 {
		ui.Err("No inputs found (no journal, no log files). Fluent Bit cannot collect logs.")
		ui.Err("Set FB_LOG_PATHS or verify systemd journal is accessible.")
		os.Exit(1)
	}

	confContent, err := config.Generate(cfg)
	if err != nil {
		ui.Err("Failed to generate config: " + err.Error())
		os.Exit(1)
	}
	if err := os.WriteFile("/etc/fluent-bit/fluent-bit.conf", []byte(confContent), 0600); err != nil {
		ui.Err("Failed to write fluent-bit.conf: " + err.Error())
		os.Exit(1)
	}

	// 12. Write systemd hardening override
	ui.Info("Configuring systemd...")
	overrideDir := "/etc/systemd/system/fluent-bit.service.d"
	if err := os.MkdirAll(overrideDir, 0755); err != nil {
		ui.Err("Failed to create override dir: " + err.Error())
		os.Exit(1)
	}
	overrideContent := `[Service]
Restart=always
RestartSec=10
LimitNOFILE=65536
OOMScoreAdjust=-500
TimeoutStopSec=35
`
	if err := os.WriteFile(overrideDir+"/override.conf", []byte(overrideContent), 0644); err != nil {
		ui.Err("Failed to write systemd override: " + err.Error())
		os.Exit(1)
	}

	// 13. Write fb-agent.service from template
	execPath, err := os.Executable()
	if err != nil {
		execPath = "/usr/local/bin/fb-agent"
	}

	var envLines []string
	if cfID != "" {
		envLines = append(envLines, "CF_CLIENT_ID="+cfID)
		envLines = append(envLines, "CF_CLIENT_SECRET="+cfSecret)
	}

	svcTmpl, err := template.New("svc").Parse(string(embedded.FBAgentServiceTmpl))
	if err != nil {
		ui.Err("Failed to parse service template: " + err.Error())
		os.Exit(1)
	}
	type svcData struct {
		ExecPath string
		VLHost   string
		VLPort   int
		Env      []string
	}
	var svcBuf strings.Builder
	if err := svcTmpl.Execute(&svcBuf, svcData{
		ExecPath: execPath,
		VLHost:   vlHost,
		VLPort:   vlPort,
		Env:      envLines,
	}); err != nil {
		ui.Err("Failed to render service template: " + err.Error())
		os.Exit(1)
	}
	if err := os.WriteFile("/etc/systemd/system/fb-agent.service", []byte(svcBuf.String()), 0644); err != nil {
		ui.Err("Failed to write fb-agent.service: " + err.Error())
		os.Exit(1)
	}

	// 14. systemd daemon-reload, enable, start fluent-bit
	if err := pkg.DaemonReload(); err != nil {
		ui.Err("daemon-reload failed: " + err.Error())
		os.Exit(1)
	}
	if err := pkg.Enable("fluent-bit"); err != nil {
		ui.Warn("Enable fluent-bit failed: " + err.Error())
	}
	if err := pkg.Start("fluent-bit"); err != nil {
		ui.Warn("Start fluent-bit failed: " + err.Error())
	}

	// Verify fluent-bit is running
	if pkg.IsActiveQuiet("fluent-bit") {
		ui.OK("Fluent Bit is running")
	} else {
		ui.Err("Fluent Bit failed to start!")
		_ = exec.Command("journalctl", "-u", "fluent-bit", "--no-pager", "-n", "20").Run() //nolint:errcheck // best-effort diagnostic
		os.Exit(1)
	}

	// 15. Enable fb-agent.service (daemon mode)
	if err := pkg.Enable("fb-agent.service"); err != nil {
		ui.Warn("Enable fb-agent.service failed: " + err.Error())
	}
	if err := pkg.Start("fb-agent.service"); err != nil {
		ui.Warn("Start fb-agent.service failed: " + err.Error())
	}

	// 16. Run initial registration
	ui.OK("Running initial host registration...")
	if err := Register(); err != nil {
		ui.Warn("Initial registration failed (will retry via daemon): " + err.Error())
	}

	// Summary
	fmt.Println("")
	fmt.Println("================================================")
	fmt.Println("  Fluent Bit v3.0 — Production Ready")
	fmt.Println("================================================")
	fmt.Printf("  Host:     %s\n", hostname)
	fmt.Printf("  Job:      %s\n", job)
	fmt.Printf("  OS:       %s %s\n", osInfo.ID, osInfo.Codename)
	fmt.Printf("  Target:   %s:%d\n", vlHost, vlPort)
	fmt.Printf("  Gzip:     %v\n", gzip)
	fmt.Printf("  Buffer:   %s\n", bufferSize)
	if detectedServices != "" {
		fmt.Printf("  Services: %s\n", detectedServices)
	}
	fmt.Println("  Config:   /etc/fluent-bit/fluent-bit.conf")
	fmt.Println("  Lua:      /etc/fluent-bit/enrich.lua")
	fmt.Println("================================================")
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envOrInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		var n int
		if _, err := fmt.Sscan(v, &n); err == nil {
			return n
		}
	}
	return def
}

func mustParsePort(s string) int {
	var p int
	if _, err := fmt.Sscan(s, &p); err != nil || p <= 0 {
		return 443
	}
	return p
}

func mustHostname() string {
	h, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return h
}

func autoBufferSize() string {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return "30M"
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "MemTotal:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				var kb int
				if _, err := fmt.Sscan(fields[1], &kb); err != nil {
					return "30M" // fallback on parse error
				}
				mb := kb / 1024
				switch {
				case mb < 512:
					return "10M"
				case mb < 2048:
					return "30M"
				case mb < 8192:
					return "50M"
				default:
					return "100M"
				}
			}
		}
	}
	return "30M"
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func dirExists(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.IsDir()
}
