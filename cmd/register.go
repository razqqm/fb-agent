package cmd

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/razqqm/fb-agent/cmd/ui"
	"github.com/razqqm/fb-agent/detect"
	"github.com/razqqm/fb-agent/network"
)

const registrationFile = "/var/lib/fluent-bit/host-registration.json"

// registrationRecord is the full host registration payload.
type registrationRecord struct {
	// Identity
	HostID   string `json:"host_id"`
	Hostname string `json:"hostname"`
	FQDN     string `json:"fqdn"`

	// Network
	InternalIP string `json:"internal_ip"`
	ExternalIP string `json:"external_ip"`
	ReverseDNS string `json:"reverse_dns"`
	OpenPorts  string `json:"open_ports"`

	// System
	OS          string `json:"os"`
	Kernel      string `json:"kernel"`
	Arch        string `json:"arch"`
	CPU         string `json:"cpu"`
	RAMMB       int    `json:"ram_mb"`
	Disk        string `json:"disk"`
	Environment string `json:"environment"`
	Uptime      string `json:"uptime"`

	// Services
	Services []detect.ServiceInfo `json:"services"`

	// Fluent Bit
	FluentBit fbStatus `json:"fluent_bit"`

	// Meta
	RegisteredAt string `json:"registered_at"`
	Version      string `json:"version"`
}

type fbStatus struct {
	Running   bool  `json:"running"`
	UptimeSec int64 `json:"uptime_sec"`
}

// vlEntry is the VictoriaLogs-compatible jsonline entry.
type vlEntry struct {
	Hostname   string `json:"hostname"`
	App        string `json:"app"`
	Level      string `json:"level"`
	Job        string `json:"job"`
	Msg        string `json:"_msg"`
	Time       string `json:"_time"`
	HostID     string `json:"host_id"`
	FQDN       string `json:"fqdn"`
	InternalIP string `json:"internal_ip"`
	ExternalIP string `json:"external_ip"`
	ReverseDNS string `json:"reverse_dns"`
	OpenPorts  string `json:"open_ports"`
	OS         string `json:"os"`
	Kernel     string `json:"kernel"`
	Arch       string `json:"arch"`
	CPU        string `json:"cpu"`
	RAMMB      string `json:"ram_mb"`
	Disk       string `json:"disk"`
	Env        string `json:"environment"`
	Services   string `json:"services"`
	FluentBit  string `json:"fluent_bit"`
	RegVersion string `json:"reg_version"`
}

// Register collects host metadata and sends a registration record to
// VictoriaLogs. It also saves a local snapshot.
func Register() error {
	vlHost := envOr("VL_HOST", "localhost")
	vlPortStr := envOr("VL_PORT", "443")
	vlPort := mustParsePort(vlPortStr)
	cfID := envOr("CF_CLIENT_ID", "")
	cfSecret := envOr("CF_CLIENT_SECRET", "")

	// 1. Collect host fingerprint
	fp := network.GetFingerprint()

	// 2. Collect system info
	osInfo := detect.DetectOS()
	cpuInfo := getCPUInfo()
	ramMB := getRAMMB()
	diskInfo := getDiskInfo()
	kernel := getKernel()
	arch := runtime.GOARCH
	uptimeStr := getUptime()
	environment := detect.DetectEnvironment()

	// 3. Detect services detailed
	services := detect.DetectServicesDetailed()

	// 4. Get FB status
	fbStat := getFBStatus()

	// 5. Scan open ports via /proc/net/tcp
	openPorts := network.ScanOpenPorts()

	// 6. Build registration record
	now := time.Now().UTC().Format(time.RFC3339)
	rec := registrationRecord{
		HostID:       fp.HostID,
		Hostname:     fp.Hostname,
		FQDN:         fp.FQDN,
		InternalIP:   fp.InternalIP,
		ExternalIP:   fp.ExternalIP,
		ReverseDNS:   fp.ReverseDNS,
		OpenPorts:    openPorts,
		OS:           osInfo.PrettyName,
		Kernel:       kernel,
		Arch:         arch,
		CPU:          cpuInfo,
		RAMMB:        ramMB,
		Disk:         diskInfo,
		Environment:  environment,
		Uptime:       uptimeStr,
		Services:     services,
		FluentBit:    fbStat,
		RegisteredAt: now,
		Version:      "3.0",
	}

	// 7. Save to local file
	if err := os.MkdirAll("/var/lib/fluent-bit", 0755); err != nil {
		return fmt.Errorf("register: create var dir: %w", err)
	}
	recJSON, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return fmt.Errorf("register: marshal record: %w", err)
	}
	if err := os.WriteFile(registrationFile, recJSON, 0644); err != nil {
		return fmt.Errorf("register: write local file: %w", err)
	}

	// 8. POST to VictoriaLogs as jsonline
	svcJSON, _ := json.Marshal(rec.Services)
	fbJSON, _ := json.Marshal(rec.FluentBit)

	entry := vlEntry{
		Hostname:   rec.Hostname,
		App:        "host-registry",
		Level:      "info",
		Job:        rec.Environment,
		Msg:        fmt.Sprintf("Host registration: %s (%s)", rec.Hostname, rec.ExternalIP),
		Time:       now,
		HostID:     rec.HostID,
		FQDN:       rec.FQDN,
		InternalIP: rec.InternalIP,
		ExternalIP: rec.ExternalIP,
		ReverseDNS: rec.ReverseDNS,
		OpenPorts:  rec.OpenPorts,
		OS:         rec.OS,
		Kernel:     rec.Kernel,
		Arch:       rec.Arch,
		CPU:        rec.CPU,
		RAMMB:      fmt.Sprintf("%d", rec.RAMMB),
		Disk:       rec.Disk,
		Env:        rec.Environment,
		Services:   string(svcJSON),
		FluentBit:  string(fbJSON),
		RegVersion: rec.Version,
	}

	entryJSON, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("register: marshal entry: %w", err)
	}

	vlURI := "/insert/jsonline?_stream_fields=hostname,app,level&_msg_field=_msg&_time_field=_time"

	scheme := "https"
	if vlPort == 9428 {
		scheme = "http"
	}
	url := fmt.Sprintf("%s://%s:%d%s", scheme, vlHost, vlPort, vlURI)

	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(entryJSON))
	if err != nil {
		return fmt.Errorf("register: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if vlPort == 443 && cfID != "" {
		req.Header.Set("CF-Access-Client-Id", cfID)
		req.Header.Set("CF-Access-Client-Secret", cfSecret)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		ui.Warn("Registration HTTP error (saved locally): " + err.Error())
		return nil
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == 200 || resp.StatusCode == 204 {
		ui.OK(fmt.Sprintf("Registered: %s (%s...)", fp.Hostname, fp.HostID[:12]))
		ui.Info(fmt.Sprintf("  IP: %s / %s", fp.InternalIP, fp.ExternalIP))
		ui.Info(fmt.Sprintf("  Ports: %s", openPorts))
	} else {
		ui.Warn(fmt.Sprintf("Registration sent but HTTP %d (saved locally: %s)", resp.StatusCode, registrationFile))
	}

	return nil
}

// ---------------------------------------------------------------------------
// system info helpers
// ---------------------------------------------------------------------------

func getCPUInfo() string {
	data, err := os.ReadFile("/proc/cpuinfo")
	if err != nil {
		return "unknown"
	}
	cores := 0
	model := "unknown"
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "processor") {
			cores++
		}
		if strings.HasPrefix(line, "model name") && model == "unknown" {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				model = strings.TrimSpace(parts[1])
			}
		}
	}
	if cores == 0 {
		cores = runtime.NumCPU()
	}
	return fmt.Sprintf("%dx %s", cores, model)
}

func getRAMMB() int {
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "MemTotal:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				var kb int
				if _, err := fmt.Sscan(fields[1], &kb); err == nil {
					return kb / 1024
				}
			}
		}
	}
	return 0
}

func getDiskInfo() string {
	out, err := exec.Command("df", "-h", "/").Output()
	if err != nil {
		return "unknown"
	}
	lines := strings.Split(string(out), "\n")
	if len(lines) >= 2 {
		fields := strings.Fields(lines[1])
		if len(fields) >= 5 {
			return fmt.Sprintf("%s total, %s used (%s)", fields[1], fields[2], fields[4])
		}
	}
	return "unknown"
}

func getKernel() string {
	out, err := exec.Command("uname", "-r").Output()
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(out))
}

func getUptime() string {
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return "unknown"
	}
	fields := strings.Fields(string(data))
	if len(fields) == 0 {
		return "unknown"
	}
	var secs float64
	if _, err := fmt.Sscan(fields[0], &secs); err != nil {
		return "unknown"
	}
	d := time.Duration(secs) * time.Second
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	mins := int(d.Minutes()) % 60
	if days > 0 {
		return fmt.Sprintf("up %d days, %d hours, %d minutes", days, hours, mins)
	}
	return fmt.Sprintf("up %d hours, %d minutes", hours, mins)
}

func getFBStatus() fbStatus {
	if err := exec.Command("systemctl", "is-active", "--quiet", "fluent-bit").Run(); err != nil {
		return fbStatus{Running: false}
	}
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get("http://127.0.0.1:2020/api/v1/uptime")
	if err != nil {
		return fbStatus{Running: true}
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	var data struct {
		UptimeSec int64 `json:"uptime_sec"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return fbStatus{Running: true}
	}
	return fbStatus{Running: true, UptimeSec: data.UptimeSec}
}
