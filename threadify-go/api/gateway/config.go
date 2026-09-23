// Package gateway serves stateless, license-authenticated model inference.
package gateway

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config contains deployment settings only, never customer or conversation state.
type Config struct {
	Port          int
	RegistryURL   string
	UpstreamURL   string
	UpstreamKey   string
	Model         string
	UpstreamModel string
	MaxConcurrent int
	Timeout       time.Duration
	CAFile        string
	CertFile      string
	KeyFile       string
}

// LoadConfig reads hosted-service settings independently of the Engine's configuration.
func LoadConfig() (Config, error) {
	c := Config{RegistryURL: os.Getenv("THREADIFY_AI_REGISTRY_URL"), UpstreamURL: os.Getenv("THREADIFY_AI_UPSTREAM_URL"), UpstreamKey: os.Getenv("THREADIFY_AI_UPSTREAM_KEY"), Model: os.Getenv("THREADIFY_AI_MODEL"), UpstreamModel: os.Getenv("THREADIFY_AI_UPSTREAM_MODEL"), CAFile: os.Getenv("THREADIFY_AI_CA_FILE"), CertFile: os.Getenv("THREADIFY_AI_CERT_FILE"), KeyFile: os.Getenv("THREADIFY_AI_KEY_FILE")}
	if c.Model == "" {
		c.Model = "threadify-agent"
	}
	var err error
	if c.Port, err = positiveEnv("THREADIFY_AI_PORT", 8081, 65535); err != nil {
		return c, err
	}
	if c.MaxConcurrent, err = positiveEnv("THREADIFY_AI_MAX_CONCURRENT", 16, 4096); err != nil {
		return c, err
	}
	seconds, err := positiveEnv("THREADIFY_AI_TIMEOUT_SECONDS", 180, 3600)
	c.Timeout = time.Duration(seconds) * time.Second
	return c, err
}

func positiveEnv(name string, fallback, maximum int) (int, error) {
	raw := os.Getenv(name)
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 1 || value > maximum {
		return 0, errors.New("invalid " + name)
	}
	return value, nil
}

// endpoint permits cleartext only on loopback, including local Ollama deployments.
func endpoint(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, errors.New("invalid gateway endpoint")
	}
	ip := net.ParseIP(u.Hostname())
	local := u.Hostname() == "localhost" || (ip != nil && ip.IsLoopback())
	if u.Scheme != "https" && !(u.Scheme == "http" && local) {
		return nil, errors.New("gateway endpoints require HTTPS except on loopback")
	}
	u.Path = strings.TrimRight(u.Path, "/")
	return u, nil
}

func upstreamTransport(c Config) (*http.Transport, error) {
	config := &tls.Config{MinVersion: tls.VersionTLS12}
	if (c.CertFile == "") != (c.KeyFile == "") {
		return nil, errors.New("both upstream TLS certificate and key are required")
	}
	if c.CAFile != "" {
		pem, err := os.ReadFile(c.CAFile)
		if err != nil {
			return nil, errors.New("unable to read upstream CA file")
		}
		config.RootCAs = x509.NewCertPool()
		if !config.RootCAs.AppendCertsFromPEM(pem) {
			return nil, errors.New("invalid upstream CA file")
		}
	}
	if c.CertFile != "" {
		cert, err := tls.LoadX509KeyPair(c.CertFile, c.KeyFile)
		if err != nil {
			return nil, errors.New("invalid upstream client certificate or key")
		}
		config.Certificates = []tls.Certificate{cert}
	}
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.Proxy = nil // No ambient proxy receives licenses or prompts.
	t.TLSClientConfig = config
	t.ResponseHeaderTimeout = 30 * time.Second
	return t, nil
}
