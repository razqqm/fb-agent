package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/razqqm/fb-agent/cmd/ui"
	"github.com/razqqm/fb-agent/health"
	"github.com/razqqm/fb-agent/network"
	"github.com/razqqm/fb-agent/pkg"
)

// Status displays the current operational status of the agent.
func Status() {
	fmt.Println("=== fb-agent status ===")
	fmt.Println()

	// Fluent Bit running status
	active, _ := pkg.IsActive("fluent-bit")
	if active {
		ui.OK("Fluent Bit:     running")
	} else {
		ui.Err("Fluent Bit:     STOPPED")
	}

	// Uptime via FB API
	uptimeSec := getFBUptimeSeconds()
	if uptimeSec > 0 {
		d := time.Duration(uptimeSec) * time.Second
		ui.Info(fmt.Sprintf("FB uptime:      %v", d.Round(time.Second)))
	}

	// Connectivity state
	state := health.LoadState()
	if state.LastOK > 0 {
		lastOK := time.Unix(state.LastOK, 0)
		offlineDur := time.Since(lastOK)
		if offlineDur < 10*time.Minute {
			ui.OK(fmt.Sprintf("Connectivity:   OK (last: %s)", lastOK.Format("2006-01-02 15:04:05")))
		} else {
			ui.Warn(fmt.Sprintf("Connectivity:   DEGRADED (last OK: %s, %.0fh ago)",
				lastOK.Format("2006-01-02 15:04:05"), offlineDur.Hours()))
		}
	} else {
		ui.Warn("Connectivity:   unknown (no check data)")
	}

	if state.FailCount > 0 {
		ui.Warn(fmt.Sprintf("Fail count:     %d consecutive", state.FailCount))
	}
	if state.AlertSent {
		ui.Err("Alert:          ACTIVE (>6h offline)")
		if data, err := os.ReadFile("/var/lib/fluent-bit/ALERT_NO_CONNECTION"); err == nil {
			fmt.Print(string(data))
		}
	}

	// Cert expiry
	certPath := "/etc/fluent-bit/certs/client.crt"
	if _, err := os.Stat(certPath); err == nil {
		if network.CertExpiresIn(certPath) < 30*24*time.Hour {
			ui.Warn(fmt.Sprintf("Cert:           expires in %v", network.CertExpiresIn(certPath).Round(time.Hour)))
		} else {
			ui.OK(fmt.Sprintf("Cert:           valid (%v remaining)", network.CertExpiresIn(certPath).Round(24*time.Hour)))
		}
	} else {
		ui.Info("Cert:           not present (CF Access mode)")
	}

	fmt.Println()
}

func getFBUptimeSeconds() int64 {
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("http://127.0.0.1:2020/api/v1/uptime")
	if err != nil {
		return 0
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	var data struct {
		UptimeSec int64 `json:"uptime_sec"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return 0
	}
	return data.UptimeSec
}
