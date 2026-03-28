package config

import (
	"bytes"
	"fmt"
	"text/template"

	"github.com/razqqm/fb-agent/detect"
	"github.com/razqqm/fb-agent/embedded"
)

// TLSConfig holds TLS/mTLS settings for the Fluent Bit output.
type TLSConfig struct {
	CA   string
	Cert string
	Key  string
	// Mode is one of: "mtls", "cf-access", "off"
	Mode string
}

// Config holds all parameters needed to render fluent-bit.conf.
type Config struct {
	Hostname     string
	Job          string
	OSID         string
	OSCodename   string
	Services     string
	FlushSec     int
	BufferSize   string
	VLHost       string
	VLPort       int
	Gzip         bool
	TLS          TLSConfig
	CFID         string
	CFSecret     string
	JournalInput bool
	FileInputs   []detect.FileInput
	GeneratedAt  string
}

// Generate renders fluent-bit.conf from the embedded template and the
// provided Config. It returns the rendered config as a string.
func Generate(cfg Config) (string, error) {
	tmpl, err := template.New("fluent-bit.conf").Parse(string(embedded.FluentBitConfTmpl))
	if err != nil {
		return "", fmt.Errorf("config: parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, cfg); err != nil {
		return "", fmt.Errorf("config: execute template: %w", err)
	}
	return buf.String(), nil
}
