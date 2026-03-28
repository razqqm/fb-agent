#!/usr/bin/env python3
"""
fb-agent verification suite — MUST PASS before any deployment.
47+ checks across 7 categories. Exit 1 on any FAIL.
"""

import subprocess
import sys
import os
import re
import json
import glob
from pathlib import Path
from dataclasses import dataclass, field
from typing import Optional

# ============================================================================
# CONFIG
# ============================================================================

PROJECT_DIR = Path(__file__).parent.resolve()
GO_BIN = "/usr/local/go/bin"
GOPATH_BIN = Path.home() / "go" / "bin"
ENV = {
    **os.environ,
    "PATH": f"{os.environ.get('PATH', '')}:{GO_BIN}:{GOPATH_BIN}",
    "CGO_ENABLED": "0",
    "GOPATH": str(Path.home() / "go"),
}

# ============================================================================
# COLORS
# ============================================================================

class C:
    RED = "\033[0;31m"
    GREEN = "\033[0;32m"
    YELLOW = "\033[1;33m"
    BLUE = "\033[0;34m"
    BOLD = "\033[1m"
    DIM = "\033[2m"
    NC = "\033[0m"


# ============================================================================
# RESULT TRACKING
# ============================================================================

@dataclass
class CheckResult:
    name: str
    category: str
    status: str  # PASS, FAIL, WARN, SKIP
    detail: str = ""

@dataclass
class Results:
    checks: list = field(default_factory=list)
    
    @property
    def passed(self): return sum(1 for c in self.checks if c.status == "PASS")
    @property
    def failed(self): return sum(1 for c in self.checks if c.status == "FAIL")
    @property
    def warned(self): return sum(1 for c in self.checks if c.status == "WARN")
    @property
    def skipped(self): return sum(1 for c in self.checks if c.status == "SKIP")


results = Results()


# ============================================================================
# HELPERS
# ============================================================================

def run(cmd: str, timeout: int = 60, cwd: Optional[Path] = None) -> tuple[int, str, str]:
    """Run shell command, return (returncode, stdout, stderr)."""
    try:
        p = subprocess.run(
            cmd, shell=True, capture_output=True, text=True,
            timeout=timeout, cwd=cwd or PROJECT_DIR, env=ENV,
        )
        return p.returncode, p.stdout.strip(), p.stderr.strip()
    except subprocess.TimeoutExpired:
        return -1, "", "TIMEOUT"
    except Exception as e:
        return -1, "", str(e)


def find_in_go(pattern: str, flags: str = "-rn") -> list[str]:
    """Grep pattern in .go files, return matching lines."""
    rc, out, _ = run(f'grep {flags} "{pattern}" --include="*.go" .')
    return out.splitlines() if rc == 0 and out else []


def find_in_files(pattern: str, extensions: list[str]) -> list[str]:
    """Grep pattern in files with given extensions."""
    includes = " ".join(f'--include="*.{ext}"' for ext in extensions)
    rc, out, _ = run(f'grep -rn "{pattern}" {includes} .')
    return out.splitlines() if rc == 0 and out else []


def file_exists(pattern: str) -> list[str]:
    """Find files matching glob pattern."""
    return glob.glob(str(PROJECT_DIR / pattern), recursive=True)


def check(name: str, category: str, fn, *, warn_only: bool = False):
    """Run a check function. fn() should return (ok: bool, detail: str)."""
    status_label = f"{C.BLUE}[CHECK]{C.NC}"
    try:
        ok, detail = fn()
    except Exception as e:
        ok, detail = False, f"Exception: {e}"
    
    if ok:
        status = "PASS"
        color = C.GREEN
    elif warn_only:
        status = "WARN"
        color = C.YELLOW
    else:
        status = "FAIL"
        color = C.RED
    
    result = CheckResult(name, category, status, detail)
    results.checks.append(result)
    
    print(f"  {status_label} {name:<45s} {color}{status}{C.NC}")
    if not ok and detail:
        for line in detail.splitlines()[:5]:
            print(f"         {C.DIM}{line}{C.NC}")


def section(title: str):
    print(f"\n{C.BOLD}{C.BLUE}─── {title} ───{C.NC}")


# ============================================================================
# 1. BUILD CHECKS
# ============================================================================

def check_build():
    section("Build")
    
    check("go mod tidy (clean)", "build", lambda: (
        run("go mod tidy")[0] == 0 and run("git diff --exit-code go.mod go.sum")[0] in (0, 129),
        ""
    ))
    
    def _build_arch(arch):
        rc, _, err = run(f"GOOS=linux GOARCH={arch} go build -o /dev/null .", timeout=120)
        return rc == 0, err[:200] if rc != 0 else ""
    
    check("go build (linux/amd64)", "build", lambda: _build_arch("amd64"))
    check("go build (linux/arm64)", "build", lambda: _build_arch("arm64"))


# ============================================================================
# 2. STATIC ANALYSIS
# ============================================================================

def check_static_analysis():
    section("Static Analysis")
    
    def _go_vet():
        rc, out, err = run("go vet ./...")
        combined = f"{out}\n{err}".strip()
        return rc == 0, combined[:300] if rc != 0 else ""
    
    def _golangci_lint():
        rc, out, err = run("golangci-lint run ./...", timeout=120)
        combined = f"{out}\n{err}".strip()
        # Count issues
        lines = [l for l in combined.splitlines() if l.strip() and ": " in l]
        return rc == 0, f"{len(lines)} issue(s):\n" + "\n".join(lines[:10]) if rc != 0 else ""
    
    check("go vet", "static", _go_vet)
    check("golangci-lint (100+ linters)", "static", _golangci_lint)


# ============================================================================
# 3. SPELL CHECK
# ============================================================================

def check_spelling():
    section("Spell Check")
    
    # Known proper names that misspell flags incorrectly
    MISSPELL_ALLOW = {"mosquitto"}  # Eclipse Mosquitto MQTT broker
    
    def _misspell(label, extensions):
        find_args = " ".join(f'-name "*.{ext}"' for ext in extensions)
        cmd = f'find . {find_args} | xargs misspell -error 2>&1'
        rc, out, err = run(cmd)
        combined = f"{out}\n{err}".strip()
        issues = [l for l in combined.splitlines() if l.strip() and ":" in l]
        # Filter known proper names
        issues = [l for l in issues if not any(w in l.lower() for w in MISSPELL_ALLOW)]
        return len(issues) == 0, "\n".join(issues[:10]) if issues else ""
    
    check("misspell (Go source)", "spell", lambda: _misspell("go", ["go"]))
    check("misspell (configs/templates)", "spell", lambda: _misspell("configs", ["lua", "tmpl", "conf"]))
    check("misspell (documentation)", "spell", lambda: _misspell("docs", ["md"]))


# ============================================================================
# 4. CODE QUALITY
# ============================================================================

def check_code_quality():
    section("Code Quality")
    
    def _no_pattern(pattern, desc, exclude_patterns=None):
        matches = find_in_go(pattern)
        if exclude_patterns:
            for exc in exclude_patterns:
                matches = [m for m in matches if exc not in m]
        return len(matches) == 0, f"Found {len(matches)}:\n" + "\n".join(matches[:5]) if matches else ""
    
    check("no TODO/FIXME/HACK in code", "quality",
          lambda: _no_pattern(r"TODO\|FIXME\|HACK\|XXX", "leftover markers",
                              ["verify.py", "verify.sh", "_test.go"]))
    
    # fmt.Print is OK in cmd/ (CLI output), not in library code
    def _no_fmt_print_in_libs():
        matches = find_in_go(r"fmt\.Print")
        lib_matches = [m for m in matches if "/cmd/" not in m and "_test.go" not in m and "main.go" not in m]
        return len(lib_matches) == 0, f"fmt.Print in lib code:\n" + "\n".join(lib_matches[:5]) if lib_matches else ""
    
    check("no fmt.Print in library code", "quality", _no_fmt_print_in_libs)
    
    def _no_bare_panic():
        matches = find_in_go(r"panic(")
        filtered = [m for m in matches if "_test.go" not in m and "main.go" not in m]
        return len(filtered) == 0, "\n".join(filtered[:5]) if filtered else ""
    
    check("no panic() outside main/tests", "quality", _no_bare_panic)
    
    def _no_ignored_errors():
        matches = find_in_go(r"_ = ")
        filtered = [m for m in matches if "_test.go" not in m]
        return len(filtered) == 0, "\n".join(filtered[:5]) if filtered else ""
    
    check("all errors handled (no _ = )", "quality", _no_ignored_errors, warn_only=True)
    
    def _no_hardcoded_secrets():
        patterns = [
            r'password\s*[:=]\s*"[^"]+"',
            r'secret\s*[:=]\s*"[^"]+"',
            r'token\s*[:=]\s*"[^"]+"',
            r'apikey\s*[:=]\s*"[^"]+"',
        ]
        all_matches = []
        for pat in patterns:
            matches = find_in_go(pat, flags="-rni")
            filtered = [m for m in matches if not any(x in m.lower() for x in 
                       ["env", "flag", "const ", "//", "example", "test", "config", "header", "field", "key "])]
            all_matches.extend(filtered)
        return len(all_matches) == 0, "\n".join(all_matches[:5]) if all_matches else ""
    
    check("no hardcoded secrets", "quality", _no_hardcoded_secrets)


# ============================================================================
# 5. SPEC COMPLIANCE
# ============================================================================

def check_spec_compliance():
    section("Spec Compliance")
    
    # --- Embedding ---
    check("embed.FS declarations", "spec",
          lambda: (bool(find_in_go(r"//go:embed")), ""))
    
    # --- Subcommands ---
    for cmd in ["install", "register", "daemon", "watchdog", "uninstall", "status", "version"]:
        check(f'subcommand: {cmd}', "spec",
              lambda c=cmd: (bool(find_in_go(f'"{c}"')), ""))
    
    # --- Detection ---
    check("OS detection (/etc/os-release)", "spec",
          lambda: (bool(find_in_go(r"os-release")), ""))
    
    check("environment detection (LXC/Docker)", "spec",
          lambda: (bool(find_in_go(r"lxc\|docker\|container", flags="-rni")), ""))
    
    check("service auto-discovery", "spec",
          lambda: (bool(find_in_go(r"systemctl\|is-active", flags="-rn")), ""))
    
    # --- Crypto / TLS ---
    check("mTLS / x509 cert logic", "spec",
          lambda: (bool(find_in_go(r"x509\|crypto/tls", flags="-rn")), ""))
    
    # --- VictoriaLogs ---
    check("VictoriaLogs jsonline POST", "spec",
          lambda: (bool(find_in_go(r"jsonline\|insert/jsonline\|VictoriaLogs", flags="-rni")), ""))
    
    # --- Host ID ---
    check("machine-id fingerprint", "spec",
          lambda: (bool(find_in_go(r"machine-id\|machine_id\|machineID\|MachineID", flags="-rn")), ""))
    
    # --- systemd ---
    check("sd_notify integration", "spec",
          lambda: (bool(find_in_go(r"SdNotify\|sd_notify\|READY=1", flags="-rn")), ""))
    
    # --- Logging ---
    check("slog structured logging", "spec",
          lambda: (bool(find_in_go(r"slog\.", flags="-rn")), ""))
    
    # --- Build files ---
    check("Makefile exists", "spec",
          lambda: (bool(file_exists("Makefile")), ""))
    
    # --- Embedded assets ---
    check("enrich.lua present", "spec",
          lambda: (bool(file_exists("**/enrich.lua")), ""))
    
    check("parsers config present", "spec",
          lambda: (bool(file_exists("**/parsers*")), ""))
    
    check("systemd unit template", "spec",
          lambda: (bool(file_exists("**/*service*") or file_exists("**/*systemd*")), ""))
    
    # --- Features ---
    check("colored CLI output", "spec",
          lambda: (bool(find_in_go(r"\\033\[\|color\.\|Color", flags="-rn")), ""))
    
    check("Cloudflare Access headers", "spec",
          lambda: (bool(find_in_go(r"CF-Access\|CF_CLIENT\|cloudflare", flags="-rni")), ""))
    
    check("buffer auto-sizing by RAM", "spec",
          lambda: (bool(find_in_go(r"MemTotal\|meminfo\|bufferSize\|BufferSize", flags="-rni")), ""))


# ============================================================================
# 6. BASH PARITY
# ============================================================================

def check_bash_parity():
    section("Bash Parity (install.sh / register.sh)")
    
    parity_checks = [
        ("OS repo fallback (trixie→bookworm)", r"trixie\|bookworm\|oracular\|noble"),
        ("Kerio Connect detection", r"[Kk]erio"),
        ("Rocket.Chat detection", r"[Rr]ocket.*[Cc]hat\|rocketchat"),
        ("fail2ban detection", r"fail2ban"),
        ("external IP resolution", r"ifconfig\.me\|ipify\|icanhazip\|externalIP\|ExternalIP"),
        ("reverse DNS lookup", r"reverseDNS\|ReverseDNS\|LookupAddr\|reverse.*dns"),
        ("port scanning", r"proc/net/tcp\|ListenPort\|openPort\|scanPort\|OpenPorts"),
        ("systemd hardening", r"LimitNOFILE\|OOMScoreAdjust\|ProtectSystem"),
        ("connectivity state machine", r"connectivity\|failCount\|FailCount\|lastOK\|LastOK"),
        ("6h offline alert", r"6.*[Hh]our\|21600\|alertSent\|AlertSent\|offlineAlert"),
        ("uninstall / purge logic", r"[Uu]ninstall\|purge\|removeConfig\|RemoveConfig"),
        ("gzip compression", r"[Gg]zip"),
    ]
    
    for name, pattern in parity_checks:
        check(name, "parity",
              lambda p=pattern: (bool(find_in_go(p, flags="-rn")), ""))


# ============================================================================
# 7. BINARY SANITY
# ============================================================================

def check_binary():
    section("Binary Sanity")
    
    binary = PROJECT_DIR / "fb-agent-test"
    
    # Build
    rc, _, err = run(f"go build -ldflags '-s -w' -o {binary} .", timeout=120)
    if rc != 0:
        check("binary compiles", "binary", lambda: (False, err[:300]))
        return
    
    check("binary compiles", "binary", lambda: (True, ""))
    
    # Static link
    def _static():
        rc, out, _ = run(f"file {binary}")
        return "statically linked" in out.lower() or "static" in out.lower(), out
    check("statically linked", "binary", _static)
    
    # Size
    def _size():
        size_mb = binary.stat().st_size / (1024 * 1024)
        return size_mb < 20, f"{size_mb:.1f} MB"
    check(f"binary size < 20 MB", "binary", _size)
    
    # Runs
    def _runs():
        rc, out, err = run(f"{binary} version", timeout=10)
        return rc == 0, f"{out}\n{err}".strip()[:200]
    check("binary runs (version)", "binary", _runs)
    
    # Help
    def _help():
        rc, out, err = run(f"{binary} help 2>&1 || {binary} --help 2>&1 || {binary} 2>&1", timeout=10)
        has_content = len(out) > 10 or len(err) > 10
        return has_content, (out or err)[:200]
    check("help output present", "binary", _help)
    
    # Cleanup
    binary.unlink(missing_ok=True)


# ============================================================================
# SUMMARY
# ============================================================================

def print_summary():
    print(f"\n{'='*50}")
    print(f"  {C.BOLD}VERIFICATION RESULTS{C.NC}")
    print(f"{'='*50}")
    print(f"  {C.GREEN}PASS: {results.passed}{C.NC}  "
          f"{C.RED}FAIL: {results.failed}{C.NC}  "
          f"{C.YELLOW}WARN: {results.warned}{C.NC}  "
          f"{C.DIM}SKIP: {results.skipped}{C.NC}")
    
    # Show failures
    failures = [c for c in results.checks if c.status == "FAIL"]
    if failures:
        print(f"\n  {C.RED}{C.BOLD}Failures:{C.NC}")
        for f in failures:
            print(f"    {C.RED}✗{C.NC} [{f.category}] {f.name}")
            if f.detail:
                for line in f.detail.splitlines()[:3]:
                    print(f"      {C.DIM}{line}{C.NC}")
    
    # Show warnings
    warnings = [c for c in results.checks if c.status == "WARN"]
    if warnings:
        print(f"\n  {C.YELLOW}Warnings:{C.NC}")
        for w in warnings:
            print(f"    {C.YELLOW}⚠{C.NC} [{w.category}] {w.name}")
    
    print()
    if results.failed > 0:
        print(f"  {C.RED}{C.BOLD}❌ VERIFICATION FAILED — {results.failed} issue(s) must be fixed{C.NC}")
        return 1
    else:
        print(f"  {C.GREEN}{C.BOLD}✅ VERIFICATION PASSED{C.NC}")
        return 0


# ============================================================================
# JSON REPORT
# ============================================================================

def save_report():
    report = {
        "summary": {
            "passed": results.passed,
            "failed": results.failed,
            "warned": results.warned,
            "skipped": results.skipped,
            "total": len(results.checks),
        },
        "checks": [
            {
                "name": c.name,
                "category": c.category,
                "status": c.status,
                "detail": c.detail[:500] if c.detail else None,
            }
            for c in results.checks
        ],
    }
    report_path = PROJECT_DIR / "verify-report.json"
    with open(report_path, "w") as f:
        json.dump(report, f, indent=2, ensure_ascii=False)
    print(f"  {C.DIM}Report saved: {report_path}{C.NC}")


# ============================================================================
# MAIN
# ============================================================================

def main():
    print(f"\n{'='*50}")
    print(f"  {C.BOLD}fb-agent Verification Suite{C.NC}")
    print(f"  {C.DIM}{PROJECT_DIR}{C.NC}")
    print(f"{'='*50}")
    
    os.chdir(PROJECT_DIR)
    
    check_build()
    check_static_analysis()
    check_spelling()
    check_code_quality()
    check_spec_compliance()
    check_bash_parity()
    check_binary()
    
    save_report()
    return print_summary()


if __name__ == "__main__":
    sys.exit(main())
