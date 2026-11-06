package mcpserver

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDataDirForExe(t *testing.T) {
	root := t.TempDir()
	bin := filepath.Join(root, "bin")
	run := filepath.Join(root, "run")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(run, 0o755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(run, "daemon.json")
	if err := os.WriteFile(marker, []byte(`{"port":8085,"token":"x"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	if got := dataDirForExe(filepath.Join(bin, "streamdrop-mcp.exe")); got != root {
		t.Errorf("dataDirForExe = %q, want %q", got, root)
	}
	if got := dataDirForExe(filepath.Join(root, "streamdrop.exe")); got != "" {
		t.Errorf("non-bin layout: got %q, want empty", got)
	}

	if err := os.Remove(marker); err != nil {
		t.Fatal(err)
	}
	if got := dataDirForExe(filepath.Join(bin, "streamdrop-mcp.exe")); got != "" {
		t.Errorf("missing marker: got %q, want empty", got)
	}
}
