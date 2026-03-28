package network

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

const certsDir = "/etc/fluent-bit/certs"

type signRequest struct {
	CSR      string `json:"csr"`
	Hostname string `json:"hostname"`
}

type signResponse struct {
	Cert  string `json:"cert"`
	CA    string `json:"ca"`
	Error string `json:"error"`
}

// EnrollCert generates a private key and CSR, sends them to the signing API,
// and stores the resulting certificate and CA in /etc/fluent-bit/certs/.
func EnrollCert(hostname, vlHost string, vlPort int, cfID, cfSecret string) error {
	if err := os.MkdirAll(certsDir, 0700); err != nil {
		return fmt.Errorf("mtls: create certs dir: %w", err)
	}

	// Check if existing cert is still valid for >7 days
	certPath := certsDir + "/client.crt"
	if _, err := os.Stat(certPath); err == nil {
		if isCertValid(certPath, 7*24*time.Hour) {
			return nil // still valid
		}
	}

	signURL := buildSignURL(vlHost, vlPort)
	if signURL == "" {
		return fmt.Errorf("mtls: cannot determine signing API URL for %s:%d", vlHost, vlPort)
	}

	// Generate key if not present
	keyPath := certsDir + "/client.key"
	key, err := ensurePrivateKey(keyPath)
	if err != nil {
		return fmt.Errorf("mtls: private key: %w", err)
	}

	// Generate CSR
	csrPEM, err := generateCSR(key, hostname)
	if err != nil {
		return fmt.Errorf("mtls: generate csr: %w", err)
	}

	// POST CSR
	resp, err := postCSR(signURL, csrPEM, hostname, vlPort, cfID, cfSecret)
	if err != nil {
		return fmt.Errorf("mtls: sign request: %w", err)
	}

	// Save cert
	if err := os.WriteFile(certPath, []byte(resp.Cert), 0600); err != nil {
		return fmt.Errorf("mtls: write cert: %w", err)
	}
	// Save CA
	if resp.CA != "" {
		if err := os.WriteFile(certsDir+"/ca.crt", []byte(resp.CA), 0644); err != nil {
			return fmt.Errorf("mtls: write ca: %w", err)
		}
	}

	return nil
}

// RenewCertIfExpiring checks whether the client certificate expires within 30
// days; if so it re-enrolls.
func RenewCertIfExpiring(hostname, vlHost string, vlPort int, cfID, cfSecret string) error {
	certPath := certsDir + "/client.crt"
	if _, err := os.Stat(certPath); os.IsNotExist(err) {
		// No cert at all — nothing to renew
		return nil
	}
	if isCertValid(certPath, 30*24*time.Hour) {
		return nil // more than 30 days remaining
	}
	// Remove old key so EnrollCert generates a fresh one
	_ = os.Remove(certsDir + "/client.key")
	return EnrollCert(hostname, vlHost, vlPort, cfID, cfSecret)
}

// CertExpiresIn returns the time remaining until the certificate at path
// expires. Returns 0 if the cert cannot be read or parsed.
func CertExpiresIn(path string) time.Duration {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return 0
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return 0
	}
	return time.Until(cert.NotAfter)
}

// isCertValid returns true if the cert at path expires no sooner than
// threshold from now.
func isCertValid(path string, threshold time.Duration) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return false
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return false
	}
	return time.Until(cert.NotAfter) > threshold
}

// buildSignURL determines the mTLS signing API URL.
func buildSignURL(vlHost string, vlPort int) string {
	switch vlPort {
	case 443:
		return fmt.Sprintf("https://%s:%d/sign", vlHost, vlPort)
	case 9429:
		return fmt.Sprintf("https://%s:%d/sign", vlHost, vlPort)
	default:
		// Try default mTLS port
		testURL := fmt.Sprintf("https://%s:9429/health", vlHost)
		client := &http.Client{Timeout: 3 * time.Second}
		resp, err := client.Get(testURL) //nolint:noctx
		if err == nil {
			_ = resp.Body.Close()
			return fmt.Sprintf("https://%s:9429/sign", vlHost)
		}
		return ""
	}
}

// ensurePrivateKey loads an existing RSA private key or generates a new 2048-
// bit one and saves it.
func ensurePrivateKey(path string) (*rsa.PrivateKey, error) {
	if data, err := os.ReadFile(path); err == nil {
		block, _ := pem.Decode(data)
		if block != nil {
			key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
			if err == nil {
				return key, nil
			}
		}
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})
	if err := os.WriteFile(path, keyPEM, 0600); err != nil {
		return nil, err
	}
	return key, nil
}

// generateCSR creates a PKCS#10 certificate signing request.
func generateCSR(key *rsa.PrivateKey, hostname string) ([]byte, error) {
	template := &x509.CertificateRequest{
		Subject: pkix.Name{
			Organization: []string{"InfraLogs"},
			CommonName:   hostname,
		},
	}
	csrDER, err := x509.CreateCertificateRequest(rand.Reader, template, key)
	if err != nil {
		return nil, err
	}
	csrPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrDER})
	return csrPEM, nil
}

// postCSR sends the PEM-encoded CSR to the signing API and returns the
// parsed response.
func postCSR(signURL string, csrPEM []byte, hostname string, vlPort int, cfID, cfSecret string) (*signResponse, error) {
	payload, err := json.Marshal(signRequest{
		CSR:      string(csrPEM),
		Hostname: hostname,
	})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(http.MethodPost, signURL, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if vlPort == 443 && cfID != "" {
		req.Header.Set("CF-Access-Client-Id", cfID)
		req.Header.Set("CF-Access-Client-Secret", cfSecret)
	}

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var result signResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse signing response: %w", err)
	}
	if result.Error != "" {
		return nil, fmt.Errorf("signing API error: %s", result.Error)
	}
	if result.Cert == "" {
		return nil, fmt.Errorf("signing API returned empty cert")
	}
	return &result, nil
}
