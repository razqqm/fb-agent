# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.1.0] - 2026-03-28

### Added
- Initial release
- Subcommands: `install`, `register`, `watchdog`, `daemon`, `uninstall`, `status`, `version`
- Auto-detection: OS (Debian, Ubuntu, Alpine, RHEL), environment (LXC, Docker, VM, bare-metal)
- Service discovery: SSH, Nginx, PostgreSQL, Redis, Kerio Connect, Rocket.Chat, Fail2Ban, Docker, HAProxy, and 15+ more
- Config generation from embedded Go templates
- Embedded assets via `embed.FS`: enrich.lua, parsers, config templates, systemd unit
- Host registration with fingerprint (IPs, ports, hardware, services) → VictoriaLogs
- mTLS certificate enrollment and renewal via pure Go crypto
- Daemon mode replacing 4 systemd timers (watchdog, register, cert renewal)
- Connectivity state machine with 6h offline alerting
- Adaptive filesystem buffer sizing by available RAM
- Cloudflare Access header support for remote hosts
- Gzip compression (auto-enabled for remote targets)
- Systemd hardening (LimitNOFILE, ProtectSystem, OOMScoreAdjust)
- Cross-compilation for linux/amd64 and linux/arm64
- Verification suite (`verify.py`): 52 automated checks
- Bilingual README (English + Russian)
