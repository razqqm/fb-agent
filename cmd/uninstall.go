package cmd

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/razqqm/fb-agent/cmd/ui"
	"github.com/razqqm/fb-agent/pkg"
)

// Uninstall stops and removes Fluent Bit and the fb-agent from the system.
// Pass purge=true to also remove the package from the OS package manager.
func Uninstall(purge bool) {
	if os.Getuid() != 0 {
		ui.Err("Must be run as root: sudo fb-agent uninstall")
		os.Exit(1)
	}

	units := []string{"fb-agent.service", "fluent-bit"}

	// Stop and disable services
	for _, unit := range units {
		ui.Info(fmt.Sprintf("Stopping %s...", unit))
		if err := pkg.Stop(unit); err != nil {
			ui.Warn(fmt.Sprintf("Stop %s: %v", unit, err))
		}
		if err := pkg.Disable(unit); err != nil {
			ui.Warn(fmt.Sprintf("Disable %s: %v", unit, err))
		}
	}

	// Remove systemd units and overrides
	unitFiles := []string{
		"/etc/systemd/system/fb-agent.service",
		"/etc/systemd/system/fluent-bit.service.d/override.conf",
		// Legacy timer units from old bash install
		"/etc/systemd/system/fb-watchdog.service",
		"/etc/systemd/system/fb-watchdog.timer",
		"/etc/systemd/system/fb-register.service",
		"/etc/systemd/system/fb-register.timer",
		"/etc/systemd/system/fb-cert-renew.service",
		"/etc/systemd/system/fb-cert-renew.timer",
	}
	for _, f := range unitFiles {
		if err := os.Remove(f); err != nil && !os.IsNotExist(err) {
			ui.Warn(fmt.Sprintf("Remove %s: %v", f, err))
		}
	}
	// Remove override directory if empty
	_ = os.Remove("/etc/systemd/system/fluent-bit.service.d")

	if err := pkg.DaemonReload(); err != nil {
		ui.Warn("daemon-reload: " + err.Error())
	}

	// Remove config and data directories
	dirs := []string{
		"/etc/fluent-bit",
		"/var/lib/fluent-bit",
	}
	for _, d := range dirs {
		ui.Info(fmt.Sprintf("Removing %s...", d))
		if err := os.RemoveAll(d); err != nil {
			ui.Warn(fmt.Sprintf("Remove %s: %v", d, err))
		}
	}

	// Optionally purge the package
	if purge {
		ui.Info("Purging fluent-bit package...")
		osInfo := detectOSForUninstall()
		switch osInfo {
		case "debian", "ubuntu":
			if err := exec.Command("apt-get", "purge", "-y", "fluent-bit").Run(); err != nil {
				ui.Warn("apt-get purge fluent-bit: " + err.Error())
			}
		case "centos", "rhel", "rocky", "almalinux", "fedora":
			if err := exec.Command("yum", "remove", "-y", "fluent-bit").Run(); err != nil {
				ui.Warn("yum remove fluent-bit: " + err.Error())
			}
		case "alpine":
			if err := exec.Command("apk", "del", "fluent-bit").Run(); err != nil {
				ui.Warn("apk del fluent-bit: " + err.Error())
			}
		}
	}

	ui.OK("Uninstall complete")
}

func detectOSForUninstall() string {
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return "unknown"
	}
	for _, line := range splitLines(string(data)) {
		if len(line) > 3 && line[:3] == "ID=" {
			return trimQuotes(line[3:])
		}
	}
	return "unknown"
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i, c := range s {
		if c == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

func trimQuotes(s string) string {
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	return s
}
