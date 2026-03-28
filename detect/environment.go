package detect

import (
	"os"
	"os/exec"
	"strings"
)

// DetectEnvironment returns the execution environment as one of:
// "lxc", "docker", "vm", "bare-metal".
//
// Detection order:
//  1. systemd-detect-virt (most reliable on systemd hosts)
//  2. /run/container_type exists → lxc
//  3. /proc/1/environ contains container=lxc → lxc
//  4. /.dockerenv exists → docker
//  5. /proc/1/cgroup contains "docker" or "lxc" → docker/lxc
//  6. /proc/cpuinfo has "hypervisor" in flags → vm
//  7. Otherwise → bare-metal
func DetectEnvironment() string {
	// Best method: systemd-detect-virt (works in unprivileged LXC)
	if out, err := exec.Command("systemd-detect-virt", "-c").Output(); err == nil {
		virt := strings.TrimSpace(string(out))
		switch virt {
		case "lxc", "lxc-libvirt":
			return "lxc"
		case "docker":
			return "docker"
		case "podman":
			return "docker"
		case "openvz":
			return "lxc"
		}
		// "none" means not a container — fall through to VM check
	}

	// LXC check 1: container_type file (Proxmox sets this in some configs)
	if data, err := os.ReadFile("/run/container_type"); err == nil {
		if strings.TrimSpace(string(data)) == "lxc" {
			return "lxc"
		}
	}

	// LXC check 2: /proc/1/environ (requires privileged container)
	if environ, err := os.ReadFile("/proc/1/environ"); err == nil {
		envStr := string(environ)
		if strings.Contains(envStr, "container=lxc") || strings.Contains(envStr, "container=proxmox") {
			return "lxc"
		}
	}

	// Docker check 1: .dockerenv sentinel file
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return "docker"
	}

	// Docker/LXC check via cgroup
	if cgroup, err := os.ReadFile("/proc/1/cgroup"); err == nil {
		cgStr := string(cgroup)
		if strings.Contains(cgStr, "docker") {
			return "docker"
		}
		if strings.Contains(cgStr, "lxc") {
			return "lxc"
		}
	}

	// VM check via systemd-detect-virt (full virtualization)
	if out, err := exec.Command("systemd-detect-virt").Output(); err == nil {
		virt := strings.TrimSpace(string(out))
		switch virt {
		case "kvm", "qemu", "vmware", "oracle", "xen", "bochs", "uml", "parallels", "bhyve", "hyper-v":
			return "vm"
		}
	}

	// VM check fallback: hypervisor flag in /proc/cpuinfo
	if cpuinfo, err := os.ReadFile("/proc/cpuinfo"); err == nil {
		for _, line := range strings.Split(string(cpuinfo), "\n") {
			if strings.HasPrefix(line, "flags") && strings.Contains(line, "hypervisor") {
				return "vm"
			}
		}
	}

	return "bare-metal"
}
