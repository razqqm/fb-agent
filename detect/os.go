package detect

import (
	"bufio"
	"os"
	"strings"
)

// OSInfo holds parsed /etc/os-release fields.
type OSInfo struct {
	ID         string // debian, ubuntu, alpine, centos, etc.
	Codename   string // bookworm, noble, etc.
	PrettyName string
}

// DetectOS reads /etc/os-release (or /etc/alpine-release as fallback) and
// returns an OSInfo struct.
func DetectOS() OSInfo {
	info := OSInfo{ID: "unknown", Codename: "unknown", PrettyName: "unknown"}

	f, err := os.Open("/etc/os-release")
	if err != nil {
		// Alpine fallback
		if data, err2 := os.ReadFile("/etc/alpine-release"); err2 == nil {
			ver := strings.TrimSpace(string(data))
			return OSInfo{ID: "alpine", Codename: ver, PrettyName: "Alpine Linux " + ver}
		}
		return info
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	kv := make(map[string]string)
	for scanner.Scan() {
		line := scanner.Text()
		if idx := strings.IndexByte(line, '='); idx > 0 {
			key := line[:idx]
			val := strings.Trim(line[idx+1:], `"`)
			kv[key] = val
		}
	}

	if v, ok := kv["ID"]; ok {
		info.ID = v
	}
	if v, ok := kv["PRETTY_NAME"]; ok {
		info.PrettyName = v
	}
	// Prefer VERSION_CODENAME, fall back to VERSION_ID
	if v, ok := kv["VERSION_CODENAME"]; ok && v != "" {
		info.Codename = v
	} else if v, ok := kv["VERSION_ID"]; ok && v != "" {
		info.Codename = v
	}

	return info
}

// RepoCodename returns the Fluent Bit repository codename for the given OS,
// applying fallbacks for bleeding-edge distro releases that do not yet have
// a Fluent Bit repo entry.
func RepoCodename(o OSInfo) string {
	codename := o.Codename
	switch o.ID {
	case "debian":
		switch codename {
		case "trixie", "forky", "sid":
			codename = "bookworm"
		}
	case "ubuntu":
		switch codename {
		case "oracular", "plucky":
			codename = "noble"
		}
	}
	return codename
}
