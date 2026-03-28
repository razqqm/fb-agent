package health

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

const alertFile = "/var/lib/fluent-bit/ALERT_NO_CONNECTION"

// RunWatchdog performs a one-shot health check of Fluent Bit:
//  1. GET http://127.0.0.1:2020/api/v1/health
//  2. Parse fluentbit_output_retries_total from Prometheus metrics
//  3. Update State and persist it
//  4. If offline >6h: write ALERT_NO_CONNECTION file
func RunWatchdog(vlHost string, vlPort int) error {
	state := LoadState()
	now := time.Now().Unix()

	healthy := checkFBHealth()

	retries := 0
	if healthy {
		retries = getOutputRetries()
	}

	if healthy && retries == 0 {
		state.LastOK = now
		state.FailCount = 0
		state.AlertSent = false
		_ = os.Remove(alertFile)
	} else {
		state.FailCount++
	}

	// Calculate offline duration
	offlineSec := int64(0)
	if state.LastOK > 0 {
		offlineSec = now - state.LastOK
	}
	offlineHours := offlineSec / 3600

	if offlineHours >= 6 && !state.AlertSent {
		msg := fmt.Sprintf(
			"ALERT: Fluent Bit has been offline for %dh (since %s)\n",
			offlineHours,
			time.Unix(state.LastOK, 0).UTC().Format("2006-01-02 15:04"),
		)
		if err := os.WriteFile(alertFile, []byte(msg), 0644); err == nil {
			slog.Warn("connectivity alert written", "offline_hours", offlineHours)
			state.AlertSent = true
		}
	} else if offlineHours < 6 {
		_ = os.Remove(alertFile)
	}

	if err := SaveState(state); err != nil {
		return fmt.Errorf("watchdog: save state: %w", err)
	}

	slog.Info("watchdog check",
		"healthy", healthy,
		"retries", retries,
		"fail_count", state.FailCount,
		"offline_hours", offlineHours,
	)

	return nil
}

// checkFBHealth returns true if the Fluent Bit health endpoint returns HTTP 200.
func checkFBHealth() bool {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get("http://127.0.0.1:2020/api/v1/health")
	if err != nil {
		return false
	}
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode == http.StatusOK
}

// getOutputRetries fetches Prometheus metrics from Fluent Bit and returns the
// sum of fluentbit_output_retries_total across all outputs.
func getOutputRetries() int {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get("http://127.0.0.1:2020/api/v1/metrics/prometheus")
	if err != nil {
		return 0
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return 0
	}

	total := 0
	for _, line := range strings.Split(string(body), "\n") {
		if strings.HasPrefix(line, "fluentbit_output_retries_total") && !strings.HasPrefix(line, "#") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				v, err := strconv.ParseFloat(parts[len(parts)-1], 64)
				if err == nil {
					total += int(v)
				}
			}
		}
	}
	return total
}
