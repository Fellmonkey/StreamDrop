// Package client is the typed API client for the StreamDrop daemon. It is
// the single programmatic surface for external consumers (the MCP server,
// future CLI / cron jobs) and always talks to the local REST API.
package client

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// Endpoint is a resolved daemon address.
type Endpoint struct {
	BaseURL string
	Token   string
}

// DaemonFile is the discovery record the daemon writes at startup so that
// clients can find it without scraping the UI. It is strictly local: port,
// token and pid change on every run.
type DaemonFile struct {
	PID       int       `json:"pid"`
	Port      int       `json:"port"`
	Token     string    `json:"token"`
	StartedAt time.Time `json:"started_at"`
	Version   string    `json:"version"`
}

// DaemonPath is the discovery file location.
func DaemonPath(dataDir string) string {
	return filepath.Join(dataDir, "run", "daemon.json")
}

// ReadDaemonFile loads the discovery record.
func ReadDaemonFile(dataDir string) (*DaemonFile, error) {
	raw, err := os.ReadFile(DaemonPath(dataDir))
	if err != nil {
		return nil, fmt.Errorf("streamdrop daemon not running (read %s): %w", DaemonPath(dataDir), err)
	}
	var df DaemonFile
	if err := json.Unmarshal(raw, &df); err != nil {
		return nil, fmt.Errorf("streamdrop daemon file invalid: %w", err)
	}
	if df.Port == 0 || df.Token == "" {
		return nil, fmt.Errorf("streamdrop daemon file incomplete (port=%d, token=%q)", df.Port, df.Token)
	}
	return &df, nil
}

// Discover resolves the endpoint from the daemon file with environment
// overrides (STREAMDROP_HOST, STREAMDROP_PORT, STREAMDROP_TOKEN).
func Discover(dataDir string) (*Endpoint, error) {
	if dataDir == "" {
		return nil, fmt.Errorf("data directory is empty")
	}
	df, err := ReadDaemonFile(dataDir)
	if err != nil {
		return nil, err
	}

	host := "127.0.0.1"
	if v := os.Getenv("STREAMDROP_HOST"); v != "" {
		host = v
	}
	port := df.Port
	if v := os.Getenv("STREAMDROP_PORT"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 65535 {
			return nil, fmt.Errorf("STREAMDROP_PORT invalid: %q", v)
		}
		port = n
	}
	token := df.Token
	if v := os.Getenv("STREAMDROP_TOKEN"); v != "" {
		token = v
	}
	return &Endpoint{BaseURL: fmt.Sprintf("http://%s:%d", host, port), Token: token}, nil
}
