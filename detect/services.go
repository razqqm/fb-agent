package detect

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
)

// FileInput describes a Fluent Bit tail INPUT section.
type FileInput struct {
	Path      string
	Tag       string
	Parser    string
	Multiline bool
}

// ServiceInfo holds detailed service metadata collected during registration.
type ServiceInfo struct {
	Name    string
	Unit    string
	Status  string
	Version string
}

// isServiceActive returns true if the named systemd unit is active, or if the
// OpenRC service is started.
func isServiceActive(name string) bool {
	if exec.Command("systemctl", "is-active", "--quiet", name).Run() == nil {
		return true
	}
	out, err := exec.Command("rc-service", name, "status").CombinedOutput()
	if err == nil && bytes.Contains(out, []byte("started")) {
		return true
	}
	return false
}

// addInput appends a FileInput to the slice if the given path (or glob) looks
// usable (file exists, or it contains a wildcard).
func addInput(inputs []FileInput, path, tag, parser string) []FileInput {
	// Accept glob patterns (contain *) without stat check
	if strings.Contains(path, "*") {
		return append(inputs, FileInput{Path: path, Tag: tag, Parser: parser})
	}
	if _, err := os.Stat(path); err == nil {
		return append(inputs, FileInput{Path: path, Tag: tag, Parser: parser})
	}
	return inputs
}

// DetectServices auto-detects running services and returns:
//   - a slice of FileInput structs describing log files to tail
//   - a space-separated string of detected service names
func DetectServices() ([]FileInput, string) {
	var inputs []FileInput
	var services []string

	// --- Kerio Connect ---
	kerioBase := "/var/log/kerio"
	if _, err := os.Stat("/opt/kerio/mailserver/store/logs"); err == nil {
		kerioBase = "/opt/kerio/mailserver/store/logs"
	}
	if _, err := os.Stat("/opt/kerio/mailserver"); err == nil || isServiceActive("kerio-connect") {
		services = append(services, "kerio")
		kFiles := []struct{ f, tag, parser string }{
			{"mail.log", "kerio_mail", "kerio_mail"},
			{"security.log", "kerio_security", "kerio_security"},
			{"error.log", "kerio_error", ""},
			{"warning.log", "kerio_warning", ""},
			{"spam.log", "kerio_spam", ""},
			{"debug.log", "kerio_debug", ""},
		}
		for _, kf := range kFiles {
			inputs = addInput(inputs, kerioBase+"/"+kf.f, kf.tag, kf.parser)
		}
	}

	// --- Nginx ---
	_, nginxErr := exec.LookPath("nginx")
	if nginxErr == nil || isServiceActive("nginx") {
		services = append(services, "nginx")
		for _, d := range []string{"/var/log/nginx", "/var/log/httpd"} {
			inputs = addInput(inputs, d+"/access.log", "nginx_access", "nginx_access")
			inputs = addInput(inputs, d+"/error.log", "nginx_error", "nginx_error")
		}
	}

	// --- Apache ---
	_, apacheErr := exec.LookPath("apache2")
	_, httpdErr := exec.LookPath("httpd")
	if apacheErr == nil || httpdErr == nil {
		services = append(services, "apache")
		for _, d := range []string{"/var/log/apache2", "/var/log/httpd"} {
			inputs = addInput(inputs, d+"/access.log", "apache_access", "nginx_access")
			inputs = addInput(inputs, d+"/error.log", "apache_error", "")
		}
	}

	// --- Rocket.Chat ---
	if isServiceActive("rocketchat") {
		services = append(services, "rocketchat")
	} else {
		// Check Docker container name containing "rocket"
		out, err := exec.Command("docker", "ps", "--format", "{{.Names}}").Output()
		if err == nil && strings.Contains(strings.ToLower(string(out)), "rocket") {
			services = append(services, "rocketchat")
		}
	}

	// --- PostgreSQL ---
	if isServiceActive("postgresql") {
		services = append(services, "postgresql")
		for _, d := range []string{"/var/log/postgresql", "/var/lib/pgsql/data/log"} {
			glob := d + "/*.log"
			if _, err := os.Stat(d); err == nil {
				inputs = append(inputs, FileInput{Path: glob, Tag: "postgresql", Parser: ""})
				break
			}
		}
	}

	// --- MySQL/MariaDB ---
	if isServiceActive("mysql") || isServiceActive("mariadb") {
		services = append(services, "mysql")
		for _, f := range []string{
			"/var/log/mysql/error.log",
			"/var/log/mariadb/mariadb.log",
			"/var/log/mysqld.log",
		} {
			if _, err := os.Stat(f); err == nil {
				inputs = addInput(inputs, f, "mysql", "")
				break
			}
		}
	}

	// --- MongoDB ---
	if isServiceActive("mongod") {
		services = append(services, "mongodb")
		inputs = addInput(inputs, "/var/log/mongodb/mongod.log", "mongodb", "")
	}

	// --- Redis ---
	if isServiceActive("redis-server") || isServiceActive("redis") {
		services = append(services, "redis")
		for _, f := range []string{
			"/var/log/redis/redis-server.log",
			"/var/log/redis.log",
		} {
			if _, err := os.Stat(f); err == nil {
				inputs = addInput(inputs, f, "redis", "")
				break
			}
		}
	}

	// --- MinIO ---
	if isServiceActive("minio") {
		services = append(services, "minio")
		// MinIO logs via journal/systemd; no file tail needed
	}

	// --- Fail2Ban ---
	if isServiceActive("fail2ban") {
		services = append(services, "fail2ban")
		inputs = addInput(inputs, "/var/log/fail2ban.log", "fail2ban", "")
	}

	// --- Syslog fallback (when no journal) ---
	_, runJournalErr := os.Stat("/run/log/journal")
	_, varJournalErr := os.Stat("/var/log/journal")
	if runJournalErr != nil && varJournalErr != nil {
		for _, f := range []string{"/var/log/syslog", "/var/log/messages"} {
			if _, err := os.Stat(f); err == nil {
				inputs = addInput(inputs, f, "syslog", "syslog_rfc3164")
				break
			}
		}
	}

	// --- Auth log ---
	for _, f := range []string{"/var/log/auth.log", "/var/log/secure"} {
		if _, err := os.Stat(f); err == nil {
			inputs = addInput(inputs, f, "auth", "")
			break
		}
	}

	return inputs, strings.Join(services, " ")
}

// DetectServicesDetailed collects detailed service info for host registration.
func DetectServicesDetailed() []ServiceInfo {
	type svcDef struct {
		unit    string
		display string
		verCmd  []string
	}

	defs := []svcDef{
		// SSH — try both units
		{"sshd", "SSH", []string{"ssh", "-V"}},
		{"ssh", "SSH", []string{"ssh", "-V"}},
		{"fail2ban", "Fail2Ban", []string{"fail2ban-client", "version"}},
		{"ufw", "UFW", []string{"ufw", "version"}},
		{"firewalld", "Firewalld", []string{"firewall-cmd", "--version"}},
		{"nginx", "Nginx", []string{"nginx", "-v"}},
		{"apache2", "Apache", []string{"apache2", "-v"}},
		{"httpd", "Apache", []string{"httpd", "-v"}},
		{"kerio-connect", "Kerio Connect", nil},
		{"postfix", "Postfix", []string{"postconf", "mail_version"}},
		{"dovecot", "Dovecot", []string{"dovecot", "--version"}},
		{"exim4", "Exim", []string{"exim4", "--version"}},
		{"rocketchat", "Rocket.Chat", nil},
		{"postgresql", "PostgreSQL", []string{"psql", "--version"}},
		{"mysql", "MySQL", []string{"mysql", "--version"}},
		{"mariadb", "MariaDB", []string{"mariadb", "--version"}},
		{"mongod", "MongoDB", []string{"mongod", "--version"}},
		{"redis-server", "Redis", []string{"redis-server", "--version"}},
		{"redis", "Redis", []string{"redis-server", "--version"}},
		{"minio", "MinIO", nil},
		{"grafana-server", "Grafana", nil},
		{"fluent-bit", "Fluent Bit", []string{"fluent-bit", "--version"}},
		{"docker", "Docker", []string{"docker", "--version"}},
		{"containerd", "Containerd", nil},
		{"cloudflared", "Cloudflared", []string{"cloudflared", "--version"}},
		{"haproxy", "HAProxy", []string{"haproxy", "-v"}},
		{"iobroker", "ioBroker", nil},
		{"zigbee2mqtt", "Zigbee2MQTT", nil},
		{"mosquitto", "Mosquitto", []string{"mosquitto", "-h"}},
	}

	seen := make(map[string]bool)
	var result []ServiceInfo

	for _, def := range defs {
		// Avoid duplicate display names (e.g. sshd vs ssh, redis-server vs redis)
		if seen[def.display] {
			continue
		}
		if !isServiceActive(def.unit) {
			continue
		}
		seen[def.display] = true

		status := "active"
		out, err := exec.Command("systemctl", "is-active", def.unit).Output()
		if err == nil {
			status = strings.TrimSpace(string(out))
		}

		ver := ""
		if len(def.verCmd) > 0 {
			vout, verr := exec.Command(def.verCmd[0], def.verCmd[1:]...).CombinedOutput()
			if verr == nil && len(vout) > 0 {
				lines := strings.SplitN(string(vout), "\n", 2)
				ver = strings.TrimSpace(lines[0])
			}
		}

		result = append(result, ServiceInfo{
			Name:    def.display,
			Unit:    def.unit,
			Status:  status,
			Version: ver,
		})
	}

	return result
}
