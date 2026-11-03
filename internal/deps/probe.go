package deps

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Status is a component state.
type Status string

const (
	StatusInstalled Status = "installed"
	StatusMissing   Status = "missing"
	StatusOutdated  Status = "outdated"
	StatusCorrupted Status = "corrupted"
)

// Item is the API view of one component.
type Item struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	Purpose     string   `json:"purpose"`
	License     string   `json:"license"`
	SourceURL   string   `json:"source_url"`
	DownloadURL string   `json:"download_url"`
	SizeBytes   int64    `json:"size_bytes"`
	Required    bool     `json:"required"`
	Status      Status   `json:"status"`
	Path        string   `json:"path,omitempty"`
	InstalledAt string   `json:"installed_version,omitempty"`
	SHA256      string   `json:"sha256,omitempty"`
	Verified    bool     `json:"verified"`
	Note        string   `json:"note,omitempty"`
	Platforms   []string `json:"platforms"`
}

// Paths resolves the two allowed locations for a component version.
type Paths struct {
	DataDir string
	ExeDir  string
}

// VersionDir is the canonical layout: <root>/deps/<id>/<version>.
func (p Paths) VersionDir(root, id, version string) string {
	return filepath.Join(root, "deps", id, version)
}

// Candidates returns the portable directory first, then the user directory.
func (p Paths) Candidates(id, version string) []string {
	var out []string
	if p.ExeDir != "" {
		out = append(out, p.VersionDir(p.ExeDir, id, version))
	}
	out = append(out, p.VersionDir(p.DataDir, id, version))
	return out
}

// BinaryPath returns the executable path inside a version directory.
func BinaryPath(dir string, c Component) string {
	name := BinaryFileName(c, runtime.GOOS)
	// curl's Windows archive keeps the binary under bin/.
	if c.ID == "curl" {
		alt := filepath.Join(dir, "bin", name)
		if _, err := os.Stat(alt); err == nil {
			return alt
		}
	}
	return filepath.Join(dir, name)
}

// hashFile computes the SHA-256 of a file.
func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// probeComponent resolves a component and its integrity state.
func probeComponent(c Component, paths Paths) (path string, status Status, sha string, verified bool, note string) {
	for _, dir := range paths.Candidates(c.ID, c.Version) {
		bin := BinaryPath(dir, c)
		st, err := os.Stat(bin)
		if err != nil || st.IsDir() {
			continue
		}
		path = bin
		status = StatusInstalled
		expected := c.BinaryHashFor(runtime.GOOS, runtime.GOARCH)
		if expected == "" {
			note = "checksum_not_pinned"
			return path, status, "", false, note
		}
		sum, err := hashFile(bin)
		if err != nil {
			return path, StatusCorrupted, "", false, "unreadable"
		}
		sha = sum
		if !strings.EqualFold(sum, expected) {
			return path, StatusCorrupted, sha, false, "checksum_mismatch"
		}
		return path, status, sha, true, ""
	}
	return "", StatusMissing, "", false, ""
}

// ExtractComponent installs a component archive into destDir.
//
// curl's Windows archive is dynamically linked, so extracting only curl.exe
// would produce a binary that cannot start. Its whole archive is unpacked
// (with the single top-level directory stripped) and BinaryPath resolves
// bin/curl.exe. Single-file components are copied as-is.
func ExtractComponent(archivePath, destDir string, c Component, goos string) (string, error) {
	if c.ID == "curl" && strings.HasSuffix(strings.ToLower(archivePath), ".zip") {
		if err := extractZipTree(archivePath, destDir); err != nil {
			return "", err
		}
		return BinaryPath(destDir, c), nil
	}
	return ExtractBinary(archivePath, destDir, c, goos)
}

// extractZipTree unpacks every regular file, dropping the archive's single
// top-level directory when there is one.
func extractZipTree(archivePath, destDir string) error {
	zr, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer zr.Close()

	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		name := filepath.ToSlash(f.Name)
		if i := strings.IndexByte(name, '/'); i >= 0 {
			name = name[i+1:]
		}
		clean := filepath.Clean(name)
		if clean == "." || strings.HasPrefix(clean, "..") || filepath.IsAbs(clean) {
			continue
		}
		dst := filepath.Join(destDir, clean)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
		if err != nil {
			rc.Close()
			return err
		}
		if _, err := io.Copy(out, rc); err != nil {
			rc.Close()
			out.Close()
			return err
		}
		rc.Close()
		if err := out.Close(); err != nil {
			return err
		}
	}
	return nil
}

// ExtractBinary pulls the named executable out of an archive into destDir.
// ZIP and tar.gz are supported; a single-file component is copied as-is.
func ExtractBinary(archivePath, destDir string, c Component, goos string) (string, error) {
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return "", err
	}
	name := BinaryFileName(c, goos)
	lower := strings.ToLower(archivePath)
	switch {
	case strings.HasSuffix(lower, ".zip"):
		return extractFromZip(archivePath, destDir, name, c.ID)
	case strings.HasSuffix(lower, ".tar.gz"), strings.HasSuffix(lower, ".tgz"):
		return extractFromTarGz(archivePath, destDir, name, c.ID)
	default:
		dst := filepath.Join(destDir, name)
		if err := copyFileMode(archivePath, dst, 0o755); err != nil {
			return "", err
		}
		return dst, nil
	}
}

func extractFromZip(archivePath, destDir, binaryName, id string) (string, error) {
	zr, err := zip.OpenReader(archivePath)
	if err != nil {
		return "", err
	}
	defer zr.Close()

	target := ""
	for _, f := range zr.File {
		base := filepath.Base(f.Name)
		if base == binaryName || (id == "curl" && base == binaryName) {
			target = f.Name
			break
		}
	}
	if target == "" {
		return "", fmt.Errorf("deps: %s not found in archive", binaryName)
	}
	for _, f := range zr.File {
		if f.Name != target {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return "", err
		}
		defer rc.Close()
		dst := filepath.Join(destDir, binaryName)
		out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
		if err != nil {
			return "", err
		}
		if _, err := io.Copy(out, rc); err != nil {
			out.Close()
			return "", err
		}
		if err := out.Close(); err != nil {
			return "", err
		}
		return dst, nil
	}
	return "", errors.New("deps: archive entry disappeared")
}

func extractFromTarGz(archivePath, destDir, binaryName, id string) (string, error) {
	f, err := os.Open(archivePath)
	if err != nil {
		return "", err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return "", err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		if filepath.Base(hdr.Name) != binaryName {
			continue
		}
		dst := filepath.Join(destDir, binaryName)
		out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
		if err != nil {
			return "", err
		}
		if _, err := io.Copy(out, tr); err != nil {
			out.Close()
			return "", err
		}
		if err := out.Close(); err != nil {
			return "", err
		}
		return dst, nil
	}
	return "", fmt.Errorf("deps: %s not found in archive", binaryName)
}

func copyFileMode(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

// VerifyBinary checks the installed executable against the catalog for the
// running platform.
func VerifyBinary(path string, c Component) (string, bool) {
	return VerifyBinaryFor(path, c, runtime.GOOS, runtime.GOARCH)
}

// VerifyBinaryFor checks an executable against a platform's pinned checksum.
// depcheck uses it to inspect Windows artifacts from a Linux CI runner.
func VerifyBinaryFor(path string, c Component, goos, goarch string) (string, bool) {
	expected := c.BinaryHashFor(goos, goarch)
	if expected == "" {
		return "", false
	}
	sum, err := hashFile(path)
	if err != nil {
		return "", false
	}
	return sum, strings.EqualFold(sum, expected)
}

// VerifyArchive checks a downloaded archive against the catalog for the
// running platform.
func VerifyArchive(path string, c Component) (string, bool) {
	return VerifyArchiveFor(path, c, runtime.GOOS, runtime.GOARCH)
}

// VerifyArchiveFor checks an archive against a platform's pinned checksum.
func VerifyArchiveFor(path string, c Component, goos, goarch string) (string, bool) {
	sum, err := hashFile(path)
	if err != nil {
		return "", false
	}
	expected := c.ArchiveHashFor(goos, goarch)
	if expected == "" {
		return sum, false
	}
	return sum, strings.EqualFold(sum, expected)
}
