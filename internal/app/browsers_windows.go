//go:build windows

package app

import (
	"os"

	"golang.org/x/sys/windows/registry"
)

// BrowserInfo describes one installed browser.
type BrowserInfo struct {
	Name    string
	Path    string
	Default bool
}

var browserCandidates = []struct {
	name      string
	progID    string
	exe       string
	wellKnown []string
}{
	{name: "edge", progID: "MSEdgeHTM", exe: "msedge.exe", wellKnown: []string{
		`Microsoft\Edge\Application\msedge.exe`,
	}},
	{name: "chrome", progID: "ChromeHTML", exe: "chrome.exe", wellKnown: []string{
		`Google\Chrome\Application\chrome.exe`,
	}},
	{name: "firefox", progID: "FirefoxURL", exe: "firefox.exe", wellKnown: []string{
		`Mozilla Firefox\firefox.exe`,
	}},
	{name: "brave", progID: "BraveHTML", exe: "brave.exe", wellKnown: []string{
		`BraveSoftware\Brave-Browser\Application\brave.exe`,
	}},
	{name: "opera", progID: "OperaStable", exe: "launcher.exe", wellKnown: []string{
		`Opera\launcher.exe`,
	}},
}

func sysRoots() []string {
	var roots []string
	for _, v := range []string{"ProgramFiles", "ProgramFiles(x86)", "LOCALAPPDATA"} {
		if dir := os.Getenv(v); dir != "" {
			roots = append(roots, dir)
		}
	}
	return roots
}

func appPath(exe string) string {
	keys := []registry.Key{registry.LOCAL_MACHINE, registry.CURRENT_USER}
	for _, root := range keys {
		k, err := registry.OpenKey(root, `Software\Microsoft\Windows\CurrentVersion\App Paths\`+exe, registry.QUERY_VALUE)
		if err == nil {
			path, _, err := k.GetStringValue("")
			_ = k.Close()
			if err == nil && fileExists(path) {
				return path
			}
		}
	}
	for _, base := range sysRoots() {
		p := base + `\` + wellKnownName(exe)
		if fileExists(p) {
			return p
		}
	}
	return ""
}

func wellKnownName(exe string) string {
	for _, c := range browserCandidates {
		if c.exe == exe {
			return c.wellKnown[0]
		}
	}
	return exe
}

func defaultProgID() string {
	k, err := registry.OpenKey(registry.CURRENT_USER,
		`Software\Microsoft\Windows\Shell\Associations\UrlAssociations\http\UserChoice`,
		registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer k.Close()
	id, _, err := k.GetStringValue("ProgId")
	if err != nil {
		return ""
	}
	return id
}

// InstalledBrowsers returns browsers found via the registry / App Paths and
// marks the OS default.
func InstalledBrowsers() []BrowserInfo {
	defaultID := defaultProgID()
	var out []BrowserInfo
	seen := map[string]bool{}
	for _, c := range browserCandidates {
		path := appPath(c.exe)
		if path == "" || seen[c.name] {
			continue
		}
		seen[c.name] = true
		out = append(out, BrowserInfo{Name: c.name, Path: path, Default: defaultID == c.progID})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// FindBrowserPath resolves a browser by friendly name.
func FindBrowserPath(name string) string {
	for _, b := range InstalledBrowsers() {
		if b.Name == name {
			return b.Path
		}
	}
	return ""
}
