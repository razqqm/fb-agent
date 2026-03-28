package health

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// StateFile is the path where connectivity state is persisted.
const StateFile = "/var/lib/fluent-bit/connectivity.state"

// State tracks Fluent Bit connectivity over time.
type State struct {
	LastOK    int64 `json:"last_ok"`    // unix timestamp of last successful check
	FailCount int   `json:"fail_count"` // consecutive failures
	AlertSent bool  `json:"alert_sent"` // whether the >6h alert has been written
}

// LoadState reads the state file and returns the parsed State.
// If the file does not exist or is unreadable, a zero State is returned.
func LoadState() State {
	var s State
	data, err := os.ReadFile(StateFile)
	if err != nil {
		return s
	}
	_ = json.Unmarshal(data, &s)
	return s
}

// SaveState persists the State to StateFile atomically (write to tmp, rename).
func SaveState(s State) error {
	data, err := json.Marshal(s)
	if err != nil {
		return fmt.Errorf("health: marshal state: %w", err)
	}
	dir := filepath.Dir(StateFile)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("health: create state dir: %w", err)
	}
	tmp := StateFile + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return fmt.Errorf("health: write state tmp: %w", err)
	}
	if err := os.Rename(tmp, StateFile); err != nil {
		return fmt.Errorf("health: rename state file: %w", err)
	}
	return nil
}
