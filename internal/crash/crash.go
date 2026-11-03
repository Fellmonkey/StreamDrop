// Package crash writes a local diagnostic bundle after a fatal error. The
// bundle is never sent anywhere and never contains secrets (back_spec.md §3.12).
package crash

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// Bundle is the metadata written next to the stack trace.
type Bundle struct {
	Timestamp string   `json:"timestamp"`
	Version   string   `json:"version"`
	OS        string   `json:"os"`
	Arch      string   `json:"arch"`
	Error     string   `json:"error"`
	Remotes   []string `json:"remotes"` // names only, no credentials
	LogTail   []string `json:"log_tail"`
}

// Write stores crash information under <dataDir>/crash. It returns the path of
// the stack file.
func Write(dataDir, version, errText string, remotes []string, logTail []string, stack []byte) (string, error) {
	dir := filepath.Join(dataDir, "crash")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	ts := time.Now().UTC()
	base := filepath.Join(dir, "crash-"+ts.Format("20060102-150405"))

	bundle := Bundle{
		Timestamp: ts.Format(time.RFC3339),
		Version:   version,
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
		Error:     errText,
		Remotes:   remotes,
		LogTail:   logTail,
	}
	raw, err := json.MarshalIndent(bundle, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(base+".json", raw, 0o644); err != nil {
		return "", err
	}
	if err := os.WriteFile(base+".stack.txt", stack, 0o644); err != nil {
		return "", err
	}
	return base + ".json", nil
}

// Describe formats the recovery value.
func Describe(v any) string { return fmt.Sprint(v) }
