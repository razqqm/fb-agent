package main

import (
	"fmt"
	"os"

	"github.com/razqqm/fb-agent/cmd"
	"github.com/razqqm/fb-agent/cmd/ui"
)

// version and buildTime are injected at build time via -ldflags.
var version = "dev"
var buildTime = "unknown"

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "install":
		cmd.Install()
	case "register":
		if err := cmd.Register(); err != nil {
			ui.Err(fmt.Sprintf("Registration failed: %v", err))
			os.Exit(1)
		}
	case "watchdog":
		cmd.Watchdog()
	case "daemon":
		cmd.Daemon()
	case "uninstall":
		purge := len(os.Args) > 2 && os.Args[2] == "--purge"
		cmd.Uninstall(purge)
	case "status":
		cmd.Status()
	case "version", "--version", "-v":
		cmd.Version(version, buildTime)
	case "help", "--help", "-h":
		printUsage()
	default:
		ui.Err(fmt.Sprintf("Unknown subcommand: %s", os.Args[1]))
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	const (
		colorReset = "\033[0m"
		colorBlue  = "\033[0;34m"
		colorGreen = "\033[0;32m"
		colorBold  = "\033[1m"
	)
	fmt.Printf("%s%sfb-agent%s %s — Fluent Bit infrastructure agent\n\n", colorBold, colorBlue, colorReset, version)
	fmt.Printf("%sUsage:%s\n", colorBold, colorReset)
	fmt.Printf("  fb-agent %s<subcommand>%s [flags]\n\n", colorGreen, colorReset)
	fmt.Printf("%sSubcommands:%s\n", colorBold, colorReset)
	cmds := [][2]string{
		{"install", "Install and configure Fluent Bit (requires root)"},
		{"uninstall [--purge]", "Stop, disable and remove Fluent Bit"},
		{"register", "Register/update this host in VictoriaLogs"},
		{"watchdog", "One-shot connectivity health check"},
		{"daemon", "Run as a long-lived daemon (sd_notify aware)"},
		{"status", "Show current agent and connectivity status"},
		{"version", "Print version information"},
	}
	for _, c := range cmds {
		fmt.Printf("  %s%-30s%s %s\n", colorGreen, c[0], colorReset, c[1])
	}
	fmt.Printf("\n%sEnvironment variables:%s\n", colorBold, colorReset)
	envVars := [][2]string{
		{"FB_HOSTNAME", "Override hostname (default: os.Hostname)"},
		{"FB_JOB", "Environment label: lxc|remote|docker|vm"},
		{"VL_HOST", "VictoriaLogs host (default: localhost)"},
		{"VL_PORT", "VictoriaLogs port (default: 443)"},
		{"CF_CLIENT_ID", "Cloudflare Access client ID"},
		{"CF_CLIENT_SECRET", "Cloudflare Access client secret"},
		{"FB_LOG_PATHS", "Colon-separated extra log file paths"},
		{"FB_EXTRA_TAGS", "Colon-separated tags for FB_LOG_PATHS"},
		{"FB_BUFFER_SIZE", "Fluent Bit filesystem buffer (default: auto)"},
		{"FB_GZIP", "Enable gzip compression: on|off (default: auto)"},
		{"FB_FLUSH", "Flush interval in seconds (default: 5)"},
		{"FB_SKIP_DETECT", "Set to 1 to skip service auto-detection"},
		{"FB_SKIP_MTLS", "Set to 1 to skip mTLS certificate enrollment"},
		{"FB_TLS_CA", "Path to CA certificate for mTLS"},
		{"FB_TLS_CERT", "Path to client certificate for mTLS"},
		{"FB_TLS_KEY", "Path to client private key for mTLS"},
	}
	for _, e := range envVars {
		fmt.Printf("  %s%-25s%s %s\n", colorGreen, e[0], colorReset, e[1])
	}
	fmt.Println()
}
