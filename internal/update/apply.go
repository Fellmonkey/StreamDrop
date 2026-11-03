package update

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// pendingUpdate is the marker written after a successful download.
type pendingUpdate struct {
	AssetPath string    `json:"asset_path"`
	Version   string    `json:"version"`
	SHA256    string    `json:"sha256"`
	CreatedAt time.Time `json:"created_at"`
}

// ApplyPending installs a staged update. It must run before the main loop, at
// which point the new archive can safely replace the executable.
func (m *Manager) ApplyPending() error {
	markerPath := filepath.Join(m.DataDir, "update", "pending.json")
	raw, err := os.ReadFile(markerPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	var marker pendingUpdate
	if err := json.Unmarshal(raw, &marker); err != nil {
		_ = os.Remove(markerPath)
		return err
	}
	if marker.AssetPath == "" {
		_ = os.Remove(markerPath)
		return nil
	}
	if err := verifyFile(marker.AssetPath, marker.SHA256); err != nil {
		_ = os.Remove(markerPath)
		return err
	}

	exe, err := os.Executable()
	if err != nil {
		return err
	}
	exe, _ = filepath.Abs(exe)
	old := exe + ".old"
	_ = os.Remove(old)
	if err := os.Rename(exe, old); err != nil {
		return err
	}
	if err := moveFile(marker.AssetPath, exe); err != nil {
		// Roll back so the user is never left without a binary.
		_ = os.Rename(old, exe)
		return err
	}
	if runtime.GOOS != "windows" {
		_ = os.Chmod(exe, 0o755)
	}
	_ = os.Remove(markerPath)
	if m.Log != nil {
		m.Log.Info("update %s applied; previous binary kept at %s", marker.Version, old)
	}
	return nil
}

func verifyFile(path, want string) error {
	if want == "" {
		return nil
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return err
	}
	if hex.EncodeToString(h.Sum(nil)) != want {
		return errors.New("update: checksum mismatch")
	}
	return nil
}

// moveFile copies across volumes when rename is not possible.
func moveFile(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	_ = os.Remove(src)
	return nil
}
