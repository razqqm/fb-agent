# Contributing

Contributions are welcome! Here's how to get started.

## Development

```bash
# Clone
git clone https://github.com/razqqm/fb-agent.git
cd fb-agent

# Build
CGO_ENABLED=0 go build -o fb-agent .

# Run verification (requires golangci-lint + misspell)
python3 verify.py
```

## Requirements

- Go 1.21+
- `golangci-lint` — install via [golangci-lint.run](https://golangci-lint.run/welcome/install/)
- `misspell` — `go install github.com/client9/misspell/cmd/misspell@latest`
- Python 3.8+ (for `verify.py`)

## Guidelines

1. **All checks must pass** — run `python3 verify.py` before submitting
2. **No CGO** — the binary must remain statically compiled
3. **No external CLI frameworks** — stdlib `os.Args` + flags only
4. **Handle all errors** — `golangci-lint` enforces `errcheck`
5. **Use `log/slog`** for structured logging in library code
6. **`fmt.Print*`** is only allowed in `cmd/` (CLI output)
7. **Embed assets** — no external file dependencies at runtime

## Adding a New Service

To add auto-detection for a new service, edit `detect/services.go`:

```go
// In the defs slice within DetectServicesDetailed():
{"unit-name", "Display Name", []string{"binary", "--version"}},
```

Then add the corresponding log path and parser in `DetectServices()`.

## Pull Requests

- One feature per PR
- Clear description of what and why
- Verification must pass (`52 PASS / 0 FAIL`)
- Tested on at least one Linux host

## Reporting Issues

Open an issue with:
- OS and version
- `fb-agent version` output
- Steps to reproduce
- Expected vs actual behavior
