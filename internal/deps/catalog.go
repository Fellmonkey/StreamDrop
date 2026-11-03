// Package deps manages the external utilities the engine orchestrates: rclone,
// yt-dlp and curl. It owns the catalog, integrity verification, portable and
// self-download layouts (back_spec.md §3.6).
package deps

import "runtime"

// Component describes one external dependency.
type Component struct {
	ID          string `json:"id"`
	Version     string `json:"version"`
	Purpose     string `json:"purpose"`
	License     string `json:"license"`
	SourceURL   string `json:"source_url"`
	ArchiveName string `json:"archive_name,omitempty"`
	BinaryName  string `json:"binary_name"`

	// ArchiveSHA256 is the checksum of the downloaded archive; it is verified
	// before extraction. Empty for single-file components.
	ArchiveSHA256 string `json:"archive_sha256,omitempty"`
	// BinarySHA256 is the checksum of the installed executable; it is verified
	// after extraction and on every probe.
	BinarySHA256 string `json:"binary_sha256,omitempty"`

	// Checksums differ per platform, so the pinned values live in maps keyed by
	// "os/arch". The scalar fields above remain as a fallback for tests and for
	// a build that pins a single platform.
	ArchiveSHA256ByPlatform map[string]string `json:"-"`
	BinarySHA256ByPlatform  map[string]string `json:"-"`

	SizeBytes int64    `json:"size_bytes"`
	Required  bool     `json:"required"`
	Platforms []string `json:"platforms"`
}

// Version pins. They are updated together with the checksums.
const (
	RcloneVersion = "v1.68.2"
	YtDlpVersion  = "2026.10.26"
	// curl 8.12.1's Windows build is no longer published; 8.23.0 is the
	// current build with a stable URL and pinned checksum.
	CurlVersion = "8.23.0"
)

// Catalog is the fixed dependency list.
//
// The checksum fields are intentionally part of the model and are verified by
// cmd/depcheck in CI. A blank checksum means "not pinned in this build": the
// probe then verifies presence and reports the component as installed but
// unverified instead of pretending the hash matched.
var Catalog = []Component{
	{
		ID:          "rclone",
		Version:     RcloneVersion,
		Purpose:     "cloud upload, folder explorer and quota",
		License:     "MIT",
		SourceURL:   "https://downloads.rclone.org/",
		ArchiveName: "rclone-" + RcloneVersion + "-{os}-{arch}.zip",
		BinaryName:  "rclone{ext}",
		SizeBytes:   25 * 1024 * 1024,
		Required:    true,
		Platforms:   []string{"windows/amd64", "windows/arm64", "linux/amd64", "linux/arm64", "darwin/amd64", "darwin/arm64"},
		ArchiveSHA256ByPlatform: map[string]string{
			"windows/amd64": "812bf76cc02c04cf6327f3683f3d5a88e47d36c39db84c1a745777496be7d993",
		},
		BinarySHA256ByPlatform: map[string]string{
			"windows/amd64": "dcbb5d188358df520b08a584df42a8e76161b30a90a62fefdd0001174d002122",
		},
	},
	{
		ID:         "yt-dlp",
		Version:    YtDlpVersion,
		Purpose:    "HTML page and media link resolution",
		License:    "Unlicense",
		SourceURL:  "https://github.com/yt-dlp/yt-dlp/releases",
		BinaryName: "yt-dlp{ext}",
		SizeBytes:  23 * 1024 * 1024,
		Platforms:  []string{"windows/amd64", "windows/arm64", "linux/amd64", "linux/arm64", "darwin/amd64", "darwin/arm64"},
		BinarySHA256ByPlatform: map[string]string{
			"windows/amd64": "f5389d3698ecde4ef06f4e6b1ce0294fe9ac1396228c8990b8d3416d6d177a7d",
			"linux/amd64":   "e51e69e89489f199530f13d32a4342da3ddb2cf34842f364603a066db05afbf3",
		},
	},
	{
		ID:          "curl",
		Version:     CurlVersion,
		Purpose:     "direct HTTP streaming and HEAD probes",
		License:     "curl",
		SourceURL:   "https://curl.se/windows/",
		ArchiveName: "curl-" + CurlVersion + "_8-win64-mingw.zip",
		BinaryName:  "curl{ext}",
		SizeBytes:   8 * 1024 * 1024,
		Platforms:   []string{"windows/amd64", "windows/arm64", "linux/amd64", "linux/arm64", "darwin/amd64", "darwin/arm64"},
		ArchiveSHA256ByPlatform: map[string]string{
			"windows/amd64": "ca5b0fd6e7a6dbde210281f545c73a89784f2e88104a2cdfe0b73ea3dfed5b3a",
		},
		BinarySHA256ByPlatform: map[string]string{
			"windows/amd64": "35121e9cb7c2ae16187f8c5311a840ce4191d6753525138f7fbfbe46511bc271",
		},
	},
}

// Find returns a catalog entry.
func Find(id string) (Component, bool) {
	for _, c := range Catalog {
		if c.ID == id {
			return c, true
		}
	}
	return Component{}, false
}

// DownloadURL resolves the release URL for the running platform.
func DownloadURL(c Component, goos, goarch string) string {
	if goos == "" {
		goos = runtime.GOOS
	}
	if goarch == "" {
		goarch = runtime.GOARCH
	}
	switch c.ID {
	case "rclone":
		if goos == "windows" || goos == "linux" || goos == "darwin" {
			return "https://downloads.rclone.org/" + c.Version + "/rclone-" + c.Version + "-" + goos + "-" + goarch + ".zip"
		}
	case "yt-dlp":
		asset := "yt-dlp"
		if goos == "windows" {
			asset = "yt-dlp.exe"
		}
		return "https://github.com/yt-dlp/yt-dlp/releases/download/" + c.Version + "/" + asset
	case "curl":
		if goos == "windows" {
			return "https://curl.se/windows/dl-" + c.Version + "_8/curl-" + c.Version + "_8-win64-mingw.zip"
		}
		return "https://curl.se/download/"
	}
	return c.SourceURL
}

// BinaryFileName returns the executable name for the platform.
func BinaryFileName(c Component, goos string) string {
	if goos == "" {
		goos = runtime.GOOS
	}
	name := c.BinaryName
	if goos == "windows" {
		name = replaceExt(name, ".exe")
	} else {
		name = replaceExt(name, "")
	}
	// curl ships inside bin/ in its Windows archive; Install matches by suffix.
	return name
}

func replaceExt(name, ext string) string {
	for i := len(name) - 1; i >= 0; i-- {
		if name[i] == '{' {
			return name[:i] + ext
		}
	}
	return name
}

// DisplayName is used in logs (never localized UI text).
func (c Component) DisplayName() string { return c.ID + " " + c.Version }

// ArchiveHashFor returns the pinned archive checksum for a platform.
func (c Component) ArchiveHashFor(goos, goarch string) string {
	if v := c.ArchiveSHA256ByPlatform[goos+"/"+goarch]; v != "" {
		return v
	}
	return c.ArchiveSHA256
}

// BinaryHashFor returns the pinned executable checksum for a platform.
func (c Component) BinaryHashFor(goos, goarch string) string {
	if v := c.BinarySHA256ByPlatform[goos+"/"+goarch]; v != "" {
		return v
	}
	return c.BinarySHA256
}

// ArchiveFileName resolves {os}/{arch}/{ext} placeholders in the archive name.
func ArchiveFileName(c Component, goos, goarch string) string {
	if goos == "" {
		goos = runtime.GOOS
	}
	if goarch == "" {
		goarch = runtime.GOARCH
	}
	name := c.ArchiveName
	name = replaceAll(name, "{os}", goos)
	name = replaceAll(name, "{arch}", goarch)
	ext := ""
	if goos == "windows" {
		ext = ".exe"
	}
	name = replaceAll(name, "{ext}", ext)
	if name == "" {
		name = BinaryFileName(c, goos)
	}
	return name
}

// CatalogVersionForLog summarises the pinned versions for CI output.
func CatalogVersionForLog() string {
	parts := make([]string, 0, len(Catalog))
	for _, c := range Catalog {
		parts = append(parts, c.ID+"="+c.Version)
	}
	return joinComma(parts)
}

func replaceAll(s, old, new string) string {
	for {
		idx := indexOf(s, old)
		if idx < 0 {
			return s
		}
		s = s[:idx] + new + s[idx+len(old):]
	}
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func joinComma(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += ", "
		}
		out += p
	}
	return out
}
