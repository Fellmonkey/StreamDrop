package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"streamdrop/internal/config"
)

// daemonFile is the discovery record written at startup so external clients
// (MCP server, future CLI/cron) can find the local API without scraping the
// UI. It contains the per-run port and token; it is removed on shutdown.
type daemonFile struct {
	PID       int       `json:"pid"`
	Port      int       `json:"port"`
	Token     string    `json:"token"`
	StartedAt time.Time `json:"started_at"`
	Version   string    `json:"version"`
}

// writeDaemonFile persists the discovery file with user-only permissions.
func writeDaemonFile(dataDir string, port int, token string) error {
	dir := filepath.Join(dataDir, "run")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(daemonFile{
		PID:       os.Getpid(),
		Port:      port,
		Token:     token,
		StartedAt: time.Now().UTC(),
		Version:   config.Version,
	}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "daemon.json"), raw, 0o600)
}

// removeDaemonFile clears the discovery record on shutdown.
func removeDaemonFile(dataDir string) {
	_ = os.Remove(filepath.Join(dataDir, "run", "daemon.json"))
}
