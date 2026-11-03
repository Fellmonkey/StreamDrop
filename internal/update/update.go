// Package update checks GitHub Releases, downloads a newer build, verifies its
// SHA-256 and applies it on the next start (back_spec.md §3.11).
package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"streamdrop/internal/apierr"
	"streamdrop/internal/logging"
	"streamdrop/internal/store"
)

// Info is the GET /api/v1/update payload.
type Info struct {
	Available        bool   `json:"available"`
	CurrentVersion   string `json:"current_version"`
	Version          string `json:"version,omitempty"`
	SizeBytes        int64  `json:"size_bytes,omitempty"`
	SHA256           string `json:"sha256,omitempty"`
	URL              string `json:"url,omitempty"`
	AssetName        string `json:"asset_name,omitempty"`
	Notes            string `json:"notes,omitempty"`
	PublishedAt      string `json:"published_at,omitempty"`
	SignatureChecked bool   `json:"signature_checked"`
}

// Broadcaster receives update_progress events.
type Broadcaster interface {
	Broadcast(event string, data any)
}

// Manager checks for and stages updates.
type Manager struct {
	Repo           string
	CurrentVersion string
	DataDir        string
	Store          *store.Store
	Hub            Broadcaster
	Log            *logging.Logger
	Client         *http.Client
	// CheckInterval is the minimum time between automatic checks.
	CheckInterval time.Duration
}

// NewManager builds an update manager.
func NewManager(repo, version, dataDir string, st *store.Store, hub Broadcaster, log *logging.Logger) *Manager {
	return &Manager{
		Repo:           repo,
		CurrentVersion: version,
		DataDir:        dataDir,
		Store:          st,
		Hub:            hub,
		Log:            log,
		Client:         &http.Client{Timeout: 60 * time.Second},
		CheckInterval:  24 * time.Hour,
	}
}

type releaseAsset struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
	URL  string `json:"browser_download_url"`
}

type release struct {
	TagName     string         `json:"tag_name"`
	Name        string         `json:"name"`
	Body        string         `json:"body"`
	Draft       bool           `json:"draft"`
	Prerelease  bool           `json:"prerelease"`
	PublishedAt string         `json:"published_at"`
	Assets      []releaseAsset `json:"assets"`
}

// Check looks for a newer release. force bypasses the daily throttle.
func (m *Manager) Check(ctx context.Context, force bool) (Info, error) {
	info := Info{CurrentVersion: m.CurrentVersion}
	if m.Repo == "" {
		return info, nil
	}
	if !force {
		if last := m.Store.GetSetting("last_update_check", ""); last != "" {
			if t, err := time.Parse(time.RFC3339, last); err == nil && time.Since(t) < m.CheckInterval {
				return info, nil
			}
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("https://api.github.com/repos/%s/releases?per_page=20", m.Repo), nil)
	if err != nil {
		return info, apierr.Wrap(apierr.CodeInternalError, err, nil)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "StreamDrop/"+m.CurrentVersion)

	resp, err := m.Client.Do(req)
	if err != nil {
		return info, apierr.Wrap(apierr.CodeSourceUnreachable, err, nil)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return info, apierr.New(apierr.CodeSourceUnreachable, map[string]any{"status_code": resp.StatusCode})
	}
	var releases []release
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&releases); err != nil {
		return info, apierr.Wrap(apierr.CodeSourceUnreachable, err, nil)
	}

	_ = m.Store.SetSetting("last_update_check", time.Now().UTC().Format(time.RFC3339))

	var best *release
	for i := range releases {
		r := &releases[i]
		if r.Draft || r.Prerelease {
			continue
		}
		if !semverGreater(r.TagName, m.CurrentVersion) {
			continue
		}
		if best == nil || semverGreater(r.TagName, best.TagName) {
			best = r
		}
	}
	if best == nil {
		return info, nil
	}

	asset := pickAsset(best.Assets)
	if asset == nil {
		return info, nil
	}
	info.Available = true
	info.Version = strings.TrimPrefix(best.TagName, "v")
	info.SizeBytes = asset.Size
	info.URL = asset.URL
	info.AssetName = asset.Name
	info.Notes = best.Body
	info.PublishedAt = best.PublishedAt
	info.SHA256 = m.findChecksum(ctx, best.Assets, asset.Name)
	return info, nil
}

// pickAsset chooses the archive for this OS and architecture.
func pickAsset(assets []releaseAsset) *releaseAsset {
	wantOS := runtime.GOOS
	wantArch := runtime.GOARCH
	archAliases := map[string][]string{
		"amd64": {"amd64", "x86_64", "x64"},
		"arm64": {"arm64", "aarch64"},
	}
	aliases := archAliases[wantArch]
	if aliases == nil {
		aliases = []string{wantArch}
	}
	var fallback *releaseAsset
	for i := range assets {
		name := strings.ToLower(assets[i].Name)
		if !strings.Contains(name, wantOS) {
			continue
		}
		matchedArch := false
		for _, a := range aliases {
			if strings.Contains(name, a) {
				matchedArch = true
				break
			}
		}
		if matchedArch {
			if strings.HasSuffix(name, ".zip") || strings.HasSuffix(name, ".tar.gz") {
				return &assets[i]
			}
			if fallback == nil {
				fallback = &assets[i]
			}
		}
	}
	return fallback
}

// findChecksum looks for SHA256SUMS / checksums.txt / <asset>.sha256.
func (m *Manager) findChecksum(ctx context.Context, assets []releaseAsset, assetName string) string {
	for _, a := range assets {
		lower := strings.ToLower(a.Name)
		if lower != "sha256sums" && lower != "sha256sums.txt" && lower != "checksums.txt" && lower != assetName+".sha256" {
			continue
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.URL, nil)
		if err != nil {
			continue
		}
		resp, err := m.Client.Do(req)
		if err != nil {
			continue
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		resp.Body.Close()
		if lower == assetName+".sha256" {
			return strings.Fields(string(body))[0]
		}
		for _, line := range strings.Split(string(body), "\n") {
			fields := strings.Fields(line)
			if len(fields) >= 2 && strings.Contains(fields[len(fields)-1], assetName) {
				return fields[0]
			}
		}
	}
	return ""
}

// Apply downloads and stages the update. It never touches the running binary.
func (m *Manager) Apply(ctx context.Context) (Info, error) {
	info, err := m.Check(ctx, true)
	if err != nil {
		return info, err
	}
	if !info.Available || info.URL == "" {
		return info, apierr.New(apierr.CodeValidationFailed, map[string]any{"field": "update"})
	}

	staging := filepath.Join(m.DataDir, "update", "staging")
	if err := os.MkdirAll(staging, 0o755); err != nil {
		return info, apierr.Wrap(apierr.CodeInternalError, err, nil)
	}
	target := filepath.Join(staging, info.AssetName)
	part := target + ".part"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, info.URL, nil)
	if err != nil {
		return info, apierr.Wrap(apierr.CodeInternalError, err, nil)
	}
	req.Header.Set("User-Agent", "StreamDrop/"+m.CurrentVersion)
	resp, err := m.Client.Do(req)
	if err != nil {
		return info, apierr.Wrap(apierr.CodeSourceUnreachable, err, nil)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return info, apierr.New(apierr.CodeSourceUnreachable, map[string]any{"status_code": resp.StatusCode})
	}

	out, err := os.Create(part)
	if err != nil {
		return info, apierr.Wrap(apierr.CodeInternalError, err, nil)
	}
	hasher := sha256.New()
	total := resp.ContentLength
	if total <= 0 {
		total = info.SizeBytes
	}
	start := time.Now()
	var received int64
	last := time.Now()
	buf := make([]byte, 128<<10)
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := out.Write(buf[:n]); werr != nil {
				out.Close()
				return info, apierr.Wrap(apierr.CodeInternalError, werr, nil)
			}
			hasher.Write(buf[:n])
			received += int64(n)
			if time.Since(last) > 300*time.Millisecond {
				last = time.Now()
				speed := int64(0)
				if elapsed := time.Since(start).Seconds(); elapsed > 0.2 {
					speed = int64(float64(received) / elapsed)
				}
				m.emit(Progress{Version: info.Version, Received: received, Total: total, SpeedBps: speed, State: "downloading"})
			}
		}
		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			out.Close()
			return info, apierr.Wrap(apierr.CodeSourceInterrupted, readErr, nil)
		}
	}
	if err := out.Close(); err != nil {
		return info, apierr.Wrap(apierr.CodeInternalError, err, nil)
	}

	sum := hex.EncodeToString(hasher.Sum(nil))
	if info.SHA256 != "" && !strings.EqualFold(sum, info.SHA256) {
		_ = os.Remove(part)
		return info, apierr.New(apierr.CodeDependencyCorrupted, map[string]any{"sha256": sum})
	}
	info.SHA256 = sum
	info.SignatureChecked = false // no signing key is configured in this build

	if err := os.Rename(part, target); err != nil {
		return info, apierr.Wrap(apierr.CodeInternalError, err, nil)
	}
	marker := pendingUpdate{
		AssetPath: target,
		Version:   info.Version,
		SHA256:    sum,
		CreatedAt: time.Now().UTC(),
	}
	raw, _ := json.MarshalIndent(marker, "", "  ")
	if err := os.WriteFile(filepath.Join(m.DataDir, "update", "pending.json"), raw, 0o644); err != nil {
		return info, apierr.Wrap(apierr.CodeInternalError, err, nil)
	}
	m.emit(Progress{Version: info.Version, Received: received, Total: total, State: "done"})
	if m.Log != nil {
		m.Log.Info("update %s staged at %s", info.Version, target)
	}
	return info, nil
}

func (m *Manager) emit(p Progress) {
	if m.Hub != nil {
		m.Hub.Broadcast("update_progress", p)
	}
}

// Progress is an update_progress event.
type Progress struct {
	Version  string `json:"version"`
	Received int64  `json:"received"`
	Total    int64  `json:"total"`
	SpeedBps int64  `json:"speed_bps"`
	State    string `json:"state"`
}

// semverGreater reports whether a > b for "v1.2.3" style tags.
func semverGreater(a, b string) bool {
	pa := parseSemver(a)
	pb := parseSemver(b)
	for i := 0; i < 3; i++ {
		if pa[i] != pb[i] {
			return pa[i] > pb[i]
		}
	}
	return false
}

func parseSemver(v string) [3]int {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if idx := strings.IndexAny(v, "-+"); idx >= 0 {
		v = v[:idx]
	}
	parts := strings.Split(v, ".")
	var out [3]int
	for i := 0; i < 3 && i < len(parts); i++ {
		n, _ := strconv.Atoi(parts[i])
		out[i] = n
	}
	return out
}
