//go:build !windows

package app

import (
	"os/exec"
	"regexp"
	"runtime"
	"strings"
)

// BrowserInfo describes one installed browser.
type BrowserInfo struct {
	Name    string
	Path    string
	Default bool
}

// InstalledBrowsers enumerates browsers via OS conventions. On Linux we query
// xdg for the default and check PATH for the common engines; on macOS we scan
// /Applications.
func InstalledBrowsers() []BrowserInfo {
	if runtime.GOOS == "darwin" {
		return macBrowsers()
	}
	return linuxBrowsers()
}

func linuxBrowsers() []BrowserInfo {
	// Resolve the default browser via xdg.
	defName := ""
	if out, err := exec.Command("xdg-settings", "get", "default-web-browser").Output(); err == nil {
		s := strings.TrimSpace(string(out))
		// e.g. "firefox.desktop", "google-chrome.desktop"
		s = strings.TrimSuffix(s, ".desktop")
		if s != "" {
			defName = s
		}
	}
	engines := []struct{ name, exe string }{
		{"google-chrome", "google-chrome"},
		{"chromium", "chromium"},
		{"chromium-browser", "chromium-browser"},
		{"firefox", "firefox"},
		{"brave", "brave-browser"},
		{"microsoft-edge", "microsoft-edge"},
	}
	var out []BrowserInfo
	for _, e := range engines {
		if p, err := exec.LookPath(e.exe); err == nil {
			out = append(out, BrowserInfo{Name: e.name, Path: p, Default: defName == e.name})
		}
	}
	return out
}

func macBrowsers() []BrowserInfo {
	apps := []struct{ name, bundle, exe string }{
		{"safari", "Safari", "Safari"},
		{"chrome", "Google Chrome", "Google Chrome"},
		{"firefox", "Firefox", "Firefox"},
		{"edge", "Microsoft Edge", "Microsoft Edge"},
		{"brave", "Brave Browser", "Brave Browser"},
	}
	// Resolve the actual default via LaunchServices (LSHandler for public.url).
	defName := ""
	if out, err := exec.Command("defaults", "read", "com.apple.LaunchServices/com.apple.launchservices.secure").Output(); err == nil {
		if m := macDefaultBundleRe.FindStringSubmatch(string(out)); len(m) > 1 {
			for _, a := range apps {
				if m[1] == macBundleIDs[a.name] {
					defName = a.name
					break
				}
			}
		}
	}
	var out []BrowserInfo
	for _, a := range apps {
		p := "/Applications/" + a.bundle + ".app/Contents/MacOS/" + a.exe
		if fileExists(p) {
			out = append(out, BrowserInfo{Name: a.name, Path: p, Default: a.name == defName})
		}
	}
	return out
}

// macBundleIDs maps browser names to LaunchServices bundle identifiers.
var macBundleIDs = map[string]string{
	"safari":  "com.apple.Safari",
	"chrome":  "com.google.Chrome",
	"firefox": "org.mozilla.firefox",
	"edge":    "com.microsoft.edgemac",
	"brave":   "com.brave.Browser",
}

var macDefaultBundleRe = regexp.MustCompile(`LSHandlerContentType\s*=\s*"public\.url";[\s\S]*?LSHandlerRoleAll\s*=\s*"([^"]+)"`)

// FindBrowserPath resolves a browser by friendly name.
func FindBrowserPath(name string) string {
	for _, b := range InstalledBrowsers() {
		if b.Name == name {
			return b.Path
		}
	}
	return ""
}
