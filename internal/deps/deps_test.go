package deps

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestCatalogIntegrity(t *testing.T) {
	if len(Catalog) != 3 {
		t.Fatalf("catalog size=%d", len(Catalog))
	}
	for _, id := range []string{"rclone", "yt-dlp", "curl"} {
		c, ok := Find(id)
		if !ok {
			t.Fatalf("%s missing", id)
		}
		if c.Version == "" || c.Purpose == "" || c.SourceURL == "" {
			t.Fatalf("%s incomplete: %+v", id, c)
		}
		if url := DownloadURL(c, "windows", "amd64"); url == "" {
			t.Fatalf("%s has no download url", id)
		}
	}
	rclone, _ := Find("rclone")
	if !rclone.Required {
		t.Fatal("rclone must be required")
	}
	ytdlp, _ := Find("yt-dlp")
	if ytdlp.Required {
		t.Fatal("yt-dlp must be optional")
	}
}

func TestDownloadURLPerPlatform(t *testing.T) {
	rclone, _ := Find("rclone")
	windows := DownloadURL(rclone, "windows", "amd64")
	if windows != "https://downloads.rclone.org/v1.68.2/rclone-v1.68.2-windows-amd64.zip" {
		t.Fatalf("windows url=%s", windows)
	}
	linux := DownloadURL(rclone, "linux", "arm64")
	if linux != "https://downloads.rclone.org/v1.68.2/rclone-v1.68.2-linux-arm64.zip" {
		t.Fatalf("linux url=%s", linux)
	}
}

func TestBinaryFileName(t *testing.T) {
	rclone, _ := Find("rclone")
	if got := BinaryFileName(rclone, "windows"); got != "rclone.exe" {
		t.Fatalf("windows=%s", got)
	}
	if got := BinaryFileName(rclone, "linux"); got != "rclone" {
		t.Fatalf("linux=%s", got)
	}
	ytdlp, _ := Find("yt-dlp")
	if got := BinaryFileName(ytdlp, "windows"); got != "yt-dlp.exe" {
		t.Fatalf("ytdlp=%s", got)
	}
}

func TestPathsResolutionOrder(t *testing.T) {
	p := Paths{DataDir: filepath.Join("data"), ExeDir: filepath.Join("app")}
	cands := p.Candidates("rclone", "v1.68.2")
	if len(cands) != 2 {
		t.Fatalf("candidates=%v", cands)
	}
	if cands[0] != filepath.Join("app", "deps", "rclone", "v1.68.2") {
		t.Fatalf("portable first: %v", cands)
	}
	if cands[1] != filepath.Join("data", "deps", "rclone", "v1.68.2") {
		t.Fatalf("user dir second: %v", cands)
	}

	// Without a portable root only the user directory is considered.
	p2 := Paths{DataDir: "data"}
	if got := p2.Candidates("rclone", "v"); len(got) != 1 || got[0] != filepath.Join("data", "deps", "rclone", "v") {
		t.Fatalf("candidates=%v", got)
	}
}

func TestProbeMissingAndInstalled(t *testing.T) {
	dir := t.TempDir()
	// Synthetic component without pinned hashes: the catalog itself now pins
	// Windows binaries, so it cannot exercise the unpinned branch.
	c := Component{ID: "rclone", Version: "v-test", BinaryName: "rclone{ext}"}
	p := Paths{DataDir: dir}
	if _, st, _, _, _ := probeComponent(c, p); st != StatusMissing {
		t.Fatalf("status=%s", st)
	}

	target := p.VersionDir(dir, c.ID, c.Version)
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}
	bin := BinaryPath(target, c)
	if err := os.WriteFile(bin, []byte("fake binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	path, st, _, verified, note := probeComponent(c, p)
	if st != StatusInstalled || path != bin {
		t.Fatalf("status=%s path=%s", st, path)
	}
	if verified {
		t.Fatal("an unpinned checksum must not report verified")
	}
	if note != "checksum_not_pinned" {
		t.Fatalf("note=%q", note)
	}
}

func TestExtractBinaryFromZip(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "rclone.zip")
	writeZip(t, archive, map[string]string{
		"rclone-v1.68.2-windows-amd64/rclone.exe": "binary-data",
		"rclone-v1.68.2-windows-amd64/README.txt": "readme",
	})
	c, _ := Find("rclone")
	out := filepath.Join(dir, "out")
	path, err := ExtractBinary(archive, out, c, "windows")
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "binary-data" {
		t.Fatalf("data=%q err=%v", data, err)
	}
}

// TestExtractCurlKeepsDLLs locks the multi-file install: a dynamically linked
// curl.exe is useless without the DLLs shipped next to it.
func TestExtractCurlKeepsDLLs(t *testing.T) {
	dir := t.TempDir()
	archive := filepath.Join(dir, "curl.zip")
	writeZip(t, archive, map[string]string{
		"curl-8.23.0_8-win64-mingw/bin/curl.exe":    "exe",
		"curl-8.23.0_8-win64-mingw/bin/libcurl.dll": "dll",
		"curl-8.23.0_8-win64-mingw/COPYING.txt":     "license",
	})
	c, _ := Find("curl")
	out := filepath.Join(dir, "out")
	bin, err := ExtractComponent(archive, out, c, "windows")
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if filepath.Base(bin) != "curl.exe" {
		t.Fatalf("bin=%s", bin)
	}
	if _, err := os.Stat(filepath.Join(out, "bin", "libcurl.dll")); err != nil {
		t.Fatalf("dll missing: %v", err)
	}
}

func TestVerifyBinaryWithPinnedHash(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "x.bin")
	if err := os.WriteFile(bin, []byte("payload"), 0o644); err != nil {
		t.Fatal(err)
	}
	// sha256("payload")
	const want = "239f59ed55e737c77147cf55ad0c1b030b6d7ee748a7426952f9b852d5a935e5"
	sum, ok := VerifyBinary(bin, Component{BinarySHA256: want})
	if !ok || sum != want {
		t.Fatalf("sum=%s ok=%v", sum, ok)
	}
	_, bad := VerifyBinary(bin, Component{BinarySHA256: "deadbeef"})
	if bad {
		t.Fatal("mismatched hash must fail")
	}
}

func TestWindowsChecksumsPinned(t *testing.T) {
	for _, id := range []string{"rclone", "yt-dlp", "curl"} {
		c, _ := Find(id)
		if c.BinaryHashFor("windows", "amd64") == "" {
			t.Fatalf("%s has no pinned Windows binary checksum", id)
		}
	}
	rclone, _ := Find("rclone")
	if rclone.ArchiveHashFor("windows", "amd64") == "" {
		t.Fatal("rclone has no pinned Windows archive checksum")
	}
	curl, _ := Find("curl")
	if curl.ArchiveHashFor("windows", "amd64") == "" {
		t.Fatal("curl has no pinned Windows archive checksum")
	}
}

func TestSupportsPlatform(t *testing.T) {
	c, _ := Find("rclone")
	if !supportsPlatform(c, runtime.GOOS, runtime.GOARCH) {
		t.Skipf("current platform %s/%s is not in the catalog", runtime.GOOS, runtime.GOARCH)
	}
	if supportsPlatform(c, "plan9", "mips") {
		t.Fatal("unsupported platform accepted")
	}
}
