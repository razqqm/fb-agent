package cmd

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	sdnotify "github.com/coreos/go-systemd/v22/daemon"

	"github.com/razqqm/fb-agent/health"
	"github.com/razqqm/fb-agent/network"
)

// Daemon runs the long-running agent daemon mode.
//
// Responsibilities:
//   - sd_notify(READY=1) on startup
//   - Every 5min: RunWatchdog + sd_notify(WATCHDOG=1)
//   - Every 24h: RunRegister
//   - Every 7d: RenewCertIfExpiring
//   - Graceful shutdown on SIGTERM/SIGINT
func Daemon() {
	slog.Info("fb-agent daemon starting")

	vlHost := envOr("VL_HOST", "logs.ilia.ae")
	vlPortStr := envOr("VL_PORT", "443")
	vlPort := mustParsePort(vlPortStr)
	cfID := envOr("CF_CLIENT_ID", "")
	cfSecret := envOr("CF_CLIENT_SECRET", "")

	// Notify systemd that we are ready
	if _, err := sdnotify.SdNotify(false, sdnotify.SdNotifyReady); err != nil {
		slog.Warn("sd_notify READY failed", "err", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Signal handling
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

	watchdogTicker := time.NewTicker(5 * time.Minute)
	registerTicker := time.NewTicker(24 * time.Hour)
	certTicker := time.NewTicker(7 * 24 * time.Hour)
	defer watchdogTicker.Stop()
	defer registerTicker.Stop()
	defer certTicker.Stop()

	// Run watchdog immediately on startup
	runWatchdog(vlHost, vlPort)

	for {
		select {
		case <-ctx.Done():
			slog.Info("daemon context cancelled, exiting")
			return

		case sig := <-sigCh:
			slog.Info("received signal, shutting down", "signal", sig)
			return

		case <-watchdogTicker.C:
			runWatchdog(vlHost, vlPort)
			if _, err := sdnotify.SdNotify(false, sdnotify.SdNotifyWatchdog); err != nil {
				slog.Warn("sd_notify WATCHDOG failed", "err", err)
			}

		case <-registerTicker.C:
			slog.Info("running scheduled registration")
			if err := Register(); err != nil {
				slog.Error("scheduled registration failed", "err", err)
			}

		case <-certTicker.C:
			slog.Info("checking cert expiry")
			hostname := envOr("FB_HOSTNAME", mustHostname())
			if err := network.RenewCertIfExpiring(hostname, vlHost, vlPort, cfID, cfSecret); err != nil {
				slog.Error("cert renewal failed", "err", err)
			}
		}
	}
}

func runWatchdog(vlHost string, vlPort int) {
	if err := health.RunWatchdog(vlHost, vlPort); err != nil {
		slog.Error("watchdog check failed", "err", err)
	}
}
