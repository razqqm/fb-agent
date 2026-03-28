package cmd

import (
	"fmt"
	"os"

	"github.com/razqqm/fb-agent/cmd/ui"
	"github.com/razqqm/fb-agent/health"
)

// Watchdog performs a one-shot watchdog health check.
func Watchdog() {
	vlHost := envOr("VL_HOST", "localhost")
	vlPortStr := envOr("VL_PORT", "443")
	vlPort := mustParsePort(vlPortStr)

	if err := health.RunWatchdog(vlHost, vlPort); err != nil {
		ui.Err(fmt.Sprintf("Watchdog error: %v", err))
		os.Exit(1)
	}
}
