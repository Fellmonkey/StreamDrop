package deps

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"streamdrop/internal/apierr"
	"streamdrop/internal/engine"
	"streamdrop/internal/store"
)

// Progress is emitted while a component downloads.
type Progress struct {
	ID       string `json:"id"`
	Received int64  `json:"received"`
	Total    int64  `json:"total"`
	SpeedBps int64  `json:"speed_bps"`
	State    string `json:"state"` // downloading | verifying | extracting | done | failed
	Code     string `json:"code,omitempty"`
}

// Manager owns installation, probing and resolution of components.
type Manager struct {
	paths  Paths
	store  *store.Store
	client *http.Client
	emit   func(Progress)
	logf   func(format string, args ...any)

	mu       sync.Mutex
	cache    map[string]probeResult
	cacheAt  time.Time
	binaryMu sync.Mutex
}

type probeResult struct {
	path     string
	status   Status
	sha      string
	verified bool
	note     string
}

// NewManager builds the dependency manager.
func NewManager(paths Paths, st *store.Store, emit func(Progress), logf func(string, ...any)) *Manager {
	return &Manager{
		paths:  paths,
		store:  st,
		client: &http.Client{Timeout: 0},
		emit:   emit,
		logf:   logf,
		cache:  map[string]probeResult{},
	}
}

// SetHTTPClient lets tests inject a client.
func (m *Manager) SetHTTPClient(c *http.Client) { m.client = c }

// Paths returns the resolution roots.
func (m *Manager) Paths() Paths { return m.paths }

// List returns the catalog with current states.
func (m *Manager) List() []Item {
	items := make([]Item, 0, len(Catalog))
	for _, c := range Catalog {
		items = append(items, m.item(c))
	}
	return items
}

func (m *Manager) item(c Component) Item {
	res := m.probe(c)
	item := Item{
		ID:          c.ID,
		Name:        c.ID,
		Version:     c.Version,
		Purpose:     c.Purpose,
		License:     c.License,
		SourceURL:   c.SourceURL,
		DownloadURL: DownloadURL(c, runtime.GOOS, runtime.GOARCH),
		SizeBytes:   c.SizeBytes,
		Required:    c.Required,
		Status:      res.status,
		Path:        res.path,
		SHA256:      res.sha,
		Verified:    res.verified,
		Note:        res.note,
		Platforms:   c.Platforms,
	}
	if res.status == StatusInstalled || res.status == StatusCorrupted {
		item.InstalledAt = c.Version
	}
	return item
}

func (m *Manager) probe(c Component) probeResult {
	m.mu.Lock()
	if time.Since(m.cacheAt) < 2*time.Second {
		if r, ok := m.cache[c.ID]; ok {
			m.mu.Unlock()
			return r
		}
	}
	m.mu.Unlock()

	p, st, sha, verified, note := probeComponent(c, m.paths)
	r := probeResult{path: p, status: st, sha: sha, verified: verified, note: note}

	m.mu.Lock()
	if time.Since(m.cacheAt) >= 2*time.Second {
		m.cache = map[string]probeResult{}
		m.cacheAt = time.Now()
	}
	m.cache[c.ID] = r
	m.mu.Unlock()
	return r
}

// Invalidate clears the probe cache after install/remove.
func (m *Manager) Invalidate() {
	m.mu.Lock()
	m.cache = map[string]probeResult{}
	m.cacheAt = time.Time{}
	m.mu.Unlock()
}

// BinaryPath returns the resolved executable path, or "" when missing.
func (m *Manager) BinaryPath(id string) string {
	c, ok := Find(id)
	if !ok {
		return ""
	}
	return m.probe(c).path
}

// Tools resolves the tool paths for the transfer pipeline.
func (m *Manager) Tools() engine.Tools {
	return engine.Tools{
		Rclone: m.BinaryPath("rclone"),
		Curl:   m.BinaryPath("curl"),
		YtDlp:  m.BinaryPath("yt-dlp"),
	}
}

// Install downloads, verifies and atomically installs a component.
//
// Order: download to .part -> verify archive SHA-256 -> extract to a temporary
// directory -> verify binary SHA-256 -> atomic rename into place. An
// interrupted install never leaves a half-installed component.
func (m *Manager) Install(ctx context.Context, id string) error {
	c, ok := Find(id)
	if !ok {
		return apierr.New(apierr.CodeValidationFailed, map[string]any{"field": "component"})
	}
	if !supportsPlatform(c, runtime.GOOS, runtime.GOARCH) {
		return apierr.New(apierr.CodeEngineUnavailable, map[string]any{"component": id, "platform": runtime.GOOS + "/" + runtime.GOARCH})
	}

	m.binaryMu.Lock()
	defer m.binaryMu.Unlock()

	if id == "curl" && runtime.GOOS != "windows" {
		// No portable curl archive is shipped for POSIX targets; system curl is
		// used when present, otherwise the built-in Go source handles "auto".
		return apierr.New(apierr.CodeEngineUnavailable, map[string]any{"component": id, "reason": "system_only"})
	}

	destRoot := m.paths.VersionDir(m.paths.DataDir, id, c.Version)
	tmpRoot := destRoot + ".tmp"
	_ = os.RemoveAll(tmpRoot)
	if err := os.MkdirAll(tmpRoot, 0o755); err != nil {
		return apierr.Wrap(apierr.CodeInternalError, err, nil)
	}
	defer os.RemoveAll(tmpRoot)

	url := DownloadURL(c, runtime.GOOS, runtime.GOARCH)
	ext := ".part"
	if c.ArchiveName != "" {
		if i := lastDot(c.ArchiveName); i >= 0 {
			ext = c.ArchiveName[i:]
		}
	}
	archivePath := filepath.Join(tmpRoot, "download"+ext)

	if err := m.download(ctx, c, url, archivePath); err != nil {
		m.emitState(c.ID, "failed", "")
		return err
	}

	if c.ArchiveHashFor(runtime.GOOS, runtime.GOARCH) != "" {
		m.emitEvent(Progress{ID: c.ID, State: "verifying"})
		sum, ok := VerifyArchive(archivePath, c)
		if !ok {
			m.emitState(c.ID, "failed", string(apierr.CodeDependencyCorrupted))
			return apierr.New(apierr.CodeDependencyCorrupted, map[string]any{
				"component": c.ID, "sha256": sum,
			})
		}
	}

	m.emitEvent(Progress{ID: c.ID, State: "extracting"})
	extractDir := filepath.Join(tmpRoot, "bin")
	binPath, err := ExtractComponent(archivePath, extractDir, c, runtime.GOOS)
	if err != nil {
		m.emitState(c.ID, "failed", "")
		return apierr.Wrap(apierr.CodeDependencyCorrupted, err, map[string]any{"component": c.ID})
	}
	if c.BinaryHashFor(runtime.GOOS, runtime.GOARCH) != "" {
		sum, ok := VerifyBinary(binPath, c)
		if !ok {
			m.emitState(c.ID, "failed", string(apierr.CodeDependencyCorrupted))
			return apierr.New(apierr.CodeDependencyCorrupted, map[string]any{"component": c.ID, "sha256": sum})
		}
	}

	// Atomic move of the prepared directory into the versioned location. The
	// whole directory moves so multi-file components (curl) keep their DLLs.
	parent := filepath.Dir(destRoot)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return apierr.Wrap(apierr.CodeInternalError, err, nil)
	}
	_ = os.RemoveAll(destRoot)
	if err := os.Rename(extractDir, destRoot); err != nil {
		return apierr.Wrap(apierr.CodeInternalError, err, nil)
	}

	installed := BinaryPath(destRoot, c)
	sha, _ := hashFile(installed)
	if m.store != nil {
		_ = m.store.UpsertDependency(store.Dependency{
			ID: id, CatalogVersion: c.Version, InstalledVersion: c.Version,
			InstallPath: installed, SHA256: sha, Status: string(StatusInstalled), Required: c.Required,
		})
	}
	m.Invalidate()
	m.emitState(c.ID, "done", "")
	m.logf("installed %s %s at %s", c.ID, c.Version, installed)
	return nil
}

func (m *Manager) download(ctx context.Context, c Component, url, dest string) error {
	if m.client == nil {
		m.client = &http.Client{Timeout: 0}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return apierr.Wrap(apierr.CodeSourceUnreachable, err, nil)
	}
	resp, err := m.client.Do(req)
	if err != nil {
		return apierr.Wrap(apierr.CodeSourceUnreachable, err, map[string]any{"component": c.ID})
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return apierr.New(apierr.CodeSourceUnreachable, map[string]any{"component": c.ID, "status_code": resp.StatusCode})
	}

	out, err := os.Create(dest)
	if err != nil {
		return apierr.Wrap(apierr.CodeInternalError, err, nil)
	}
	defer out.Close()

	total := resp.ContentLength
	if total <= 0 {
		total = c.SizeBytes
	}
	start := time.Now()
	var received int64
	buf := make([]byte, 128<<10)
	lastEmit := time.Now()
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := out.Write(buf[:n]); werr != nil {
				return apierr.Wrap(apierr.CodeInternalError, werr, nil)
			}
			received += int64(n)
			if time.Since(lastEmit) > 300*time.Millisecond {
				lastEmit = time.Now()
				elapsed := time.Since(start).Seconds()
				speed := int64(0)
				if elapsed > 0.2 {
					speed = int64(float64(received) / elapsed)
				}
				m.emitEvent(Progress{ID: c.ID, Received: received, Total: total, SpeedBps: speed, State: "downloading"})
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			if ctx.Err() != nil {
				return apierr.New(apierr.CodeTransferAborted, map[string]any{"component": c.ID})
			}
			return apierr.Wrap(apierr.CodeSourceInterrupted, readErr, map[string]any{"component": c.ID})
		}
	}
	return nil
}

func (m *Manager) emitState(id, state, code string) {
	m.emitEvent(Progress{ID: id, State: state, Code: code})
}

func (m *Manager) emitEvent(p Progress) {
	if m.emit != nil {
		m.emit(p)
	}
}

// Remove deletes an installed component.
func (m *Manager) Remove(id string) error {
	c, ok := Find(id)
	if !ok {
		return apierr.New(apierr.CodeValidationFailed, map[string]any{"field": "component"})
	}
	removed := false
	for _, dir := range m.paths.Candidates(c.ID, c.Version) {
		if _, err := os.Stat(dir); err == nil {
			if err := os.RemoveAll(dir); err != nil {
				return apierr.Wrap(apierr.CodeInternalError, err, nil)
			}
			removed = true
		}
	}
	if m.store != nil {
		_ = m.store.DeleteDependency(id)
	}
	m.Invalidate()
	if !removed {
		return apierr.New(apierr.CodeValidationFailed, map[string]any{"field": "component", "reason": "not_installed"})
	}
	return nil
}

// Summary reports installed/required counts for the header badge.
func (m *Manager) Summary() (installed, total int) {
	items := m.List()
	for _, it := range items {
		total++
		if it.Status == StatusInstalled {
			installed++
		}
	}
	return installed, total
}

func supportsPlatform(c Component, goos, goarch string) bool {
	key := goos + "/" + goarch
	for _, p := range c.Platforms {
		if p == key {
			return true
		}
	}
	return false
}

func lastDot(s string) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '.' {
			return i
		}
	}
	return -1
}

var _ = fmt.Sprintf
