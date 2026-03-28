package config

import (
	"bytes"
	"fmt"
	"text/template"

	"github.com/razqqm/fb-agent/embedded"
)

// ServiceTmplData is the data model for fb-agent.service.tmpl.
type ServiceTmplData struct {
	ExecPath string
	VLHost   string
	VLPort   int
	Env      []string // extra Environment= lines, e.g. CF_CLIENT_ID=...
}

// RenderServiceUnit renders the fb-agent.service systemd unit from the
// embedded template.
func RenderServiceUnit(data ServiceTmplData) (string, error) {
	tmpl, err := template.New("fb-agent.service").Parse(string(embedded.FBAgentServiceTmpl))
	if err != nil {
		return "", fmt.Errorf("templates: parse service unit: %w", err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("templates: execute service unit: %w", err)
	}
	return buf.String(), nil
}
