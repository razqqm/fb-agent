package network

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Fingerprint holds persistent host identity and network information.
type Fingerprint struct {
	HostID     string
	Hostname   string
	FQDN       string
	InternalIP string
	ExternalIP string
	ReverseDNS string
}

// GetFingerprint collects all fingerprint data for the current host.
func GetFingerprint() Fingerprint {
	hostname, _ := os.Hostname()
	fqdn := hostname

	// Try to get FQDN
	addrs, err := net.LookupHost(hostname)
	if err == nil && len(addrs) > 0 {
		names, err := net.LookupAddr(addrs[0])
		if err == nil && len(names) > 0 {
			fqdn = strings.TrimSuffix(names[0], ".")
		}
	}

	internalIP := GetInternalIP()
	externalIP := GetExternalIP()

	return Fingerprint{
		HostID:     getHostID(hostname),
		Hostname:   hostname,
		FQDN:       fqdn,
		InternalIP: internalIP,
		ExternalIP: externalIP,
		ReverseDNS: GetReverseDNS(externalIP),
	}
}

// getHostID returns a stable host identifier.
// Priority: /etc/machine-id → /sys/class/dmi/id/product_uuid → hash(hostname+mac)
func getHostID(hostname string) string {
	if data, err := os.ReadFile("/etc/machine-id"); err == nil {
		id := strings.TrimSpace(string(data))
		if id != "" {
			return id
		}
	}

	if data, err := os.ReadFile("/sys/class/dmi/id/product_uuid"); err == nil {
		id := strings.TrimSpace(strings.ToLower(string(data)))
		if id != "" {
			return id
		}
	}

	// Fallback: SHA-256 of hostname + first real MAC
	mac := getMACAddress()
	h := sha256.Sum256([]byte(hostname + "-" + mac))
	return hex.EncodeToString(h[:])
}

// getMACAddress returns the first non-loopback, non-virtual MAC address found
// by reading /sys/class/net/*/address.
func getMACAddress() string {
	entries, err := os.ReadDir("/sys/class/net")
	if err != nil {
		return "00:00:00:00:00:00"
	}

	for _, e := range entries {
		name := e.Name()
		if name == "lo" {
			continue
		}
		// Skip obvious virtual interfaces
		if strings.HasPrefix(name, "veth") || strings.HasPrefix(name, "docker") ||
			strings.HasPrefix(name, "br-") || strings.HasPrefix(name, "virbr") {
			continue
		}
		data, err := os.ReadFile("/sys/class/net/" + name + "/address")
		if err != nil {
			continue
		}
		mac := strings.TrimSpace(string(data))
		if mac != "" && mac != "00:00:00:00:00:00" {
			return mac
		}
	}
	return "00:00:00:00:00:00"
}

// GetInternalIP returns the first non-loopback IPv4 address with global scope.
func GetInternalIP() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "unknown"
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.IsLoopback() {
				continue
			}
			if ip4 := ip.To4(); ip4 != nil {
				return ip4.String()
			}
		}
	}
	return "unknown"
}

// GetExternalIP probes three public IP echo services and returns the first
// valid IPv4 address. Returns "unknown" if all probes fail.
func GetExternalIP() string {
	urls := []string{
		"https://ifconfig.me/ip",
		"https://api.ipify.org",
		"https://icanhazip.com",
	}

	client := &http.Client{Timeout: 5 * time.Second}
	for _, url := range urls {
		resp, err := client.Get(url)
		if err != nil {
			continue
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 64))
		_ = resp.Body.Close()
		if err != nil {
			continue
		}
		ip := strings.TrimSpace(string(body))
		if net.ParseIP(ip) != nil && strings.Contains(ip, ".") {
			return ip
		}
	}
	return "unknown"
}

// GetReverseDNS performs a reverse DNS lookup on the given IP address.
func GetReverseDNS(ip string) string {
	if ip == "unknown" || ip == "" {
		return ""
	}
	names, err := net.LookupAddr(ip)
	if err != nil || len(names) == 0 {
		return ""
	}
	return strings.TrimSuffix(names[0], ".")
}

// ScanOpenPorts reads /proc/net/tcp and /proc/net/tcp6, parses listening
// sockets, and returns a comma-separated sorted list of port numbers.
func ScanOpenPorts() string {
	ports := make(map[int]bool)

	for _, path := range []string{"/proc/net/tcp", "/proc/net/tcp6"} {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		lines := strings.Split(string(data), "\n")
		for i, line := range lines {
			if i == 0 {
				continue // skip header
			}
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			fields := strings.Fields(line)
			if len(fields) < 4 {
				continue
			}
			// State field: "0A" = TCP_LISTEN
			state := fields[3]
			if state != "0A" {
				continue
			}
			// Local address is fields[1]: hex_ip:hex_port
			localAddr := fields[1]
			colonIdx := strings.LastIndex(localAddr, ":")
			if colonIdx < 0 {
				continue
			}
			portHex := localAddr[colonIdx+1:]
			portNum, err := strconv.ParseInt(portHex, 16, 32)
			if err != nil || portNum <= 0 || portNum >= 65536 {
				continue
			}
			ports[int(portNum)] = true
		}
	}

	if len(ports) == 0 {
		return ""
	}

	list := make([]int, 0, len(ports))
	for p := range ports {
		list = append(list, p)
	}
	sort.Ints(list)

	if len(list) > 50 {
		list = list[:50]
	}

	strs := make([]string, len(list))
	for i, p := range list {
		strs[i] = fmt.Sprintf("%d", p)
	}
	return strings.Join(strs, ",")
}
