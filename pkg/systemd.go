package pkg

import (
	"fmt"
	"os/exec"
	"strings"
)

// DaemonReload runs "systemctl daemon-reload".
func DaemonReload() error {
	if err := exec.Command("systemctl", "daemon-reload").Run(); err != nil {
		return fmt.Errorf("systemd: daemon-reload: %w", err)
	}
	return nil
}

// Enable runs "systemctl enable <unit>".
func Enable(unit string) error {
	if err := exec.Command("systemctl", "enable", unit).Run(); err != nil {
		return fmt.Errorf("systemd: enable %s: %w", unit, err)
	}
	return nil
}

// Start runs "systemctl start <unit>".
func Start(unit string) error {
	if err := exec.Command("systemctl", "start", unit).Run(); err != nil {
		return fmt.Errorf("systemd: start %s: %w", unit, err)
	}
	return nil
}

// Stop runs "systemctl stop <unit>".
func Stop(unit string) error {
	if err := exec.Command("systemctl", "stop", unit).Run(); err != nil {
		return fmt.Errorf("systemd: stop %s: %w", unit, err)
	}
	return nil
}

// Disable runs "systemctl disable <unit>".
func Disable(unit string) error {
	if err := exec.Command("systemctl", "disable", unit).Run(); err != nil {
		return fmt.Errorf("systemd: disable %s: %w", unit, err)
	}
	return nil
}

// IsActive returns (true, nil) if the unit is in the "active" state.
func IsActive(unit string) (bool, error) {
	out, err := exec.Command("systemctl", "is-active", unit).Output()
	status := strings.TrimSpace(string(out))
	if err != nil {
		// is-active returns exit code 3 when inactive — not a fatal error
		return false, nil
	}
	return status == "active", nil
}

// IsActiveQuiet returns true if the unit is active, suppressing any errors.
func IsActiveQuiet(unit string) bool {
	active, _ := IsActive(unit)
	return active
}
