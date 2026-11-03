//go:build windows

package auth

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// chromRoot maps a browser to its user-data directory.
func chromRoot(kind BrowserKind) string {
	local := os.Getenv("LOCALAPPDATA")
	roaming := os.Getenv("APPDATA")
	switch kind {
	case BrowserChrome:
		return filepath.Join(local, "Google", "Chrome", "User Data")
	case BrowserEdge:
		return filepath.Join(local, "Microsoft", "Edge", "User Data")
	case BrowserBrave:
		return filepath.Join(local, "BraveSoftware", "Brave-Browser", "User Data")
	case BrowserOpera:
		return filepath.Join(roaming, "Opera Software", "Opera Stable")
	}
	return ""
}

func firefoxRoot() string {
	return filepath.Join(os.Getenv("APPDATA"), "Mozilla", "Firefox", "Profiles")
}

// localState is the subset of Chromium's Local State we care about.
type localState struct {
	OSCrypt struct {
		EncryptedKey      string `json:"encrypted_key"`
		AppBoundEncrypted string `json:"app_bound_encrypted_key"`
	} `json:"os_crypt"`
}

// detectBrowsersWindows enumerates installed Chromium and Firefox profiles.
func detectBrowsersWindows() []DetectedProfile {
	var out []DetectedProfile
	for _, kind := range []BrowserKind{BrowserChrome, BrowserEdge, BrowserBrave, BrowserOpera} {
		root := chromRoot(kind)
		if root == "" {
			continue
		}
		if _, err := os.Stat(root); err != nil {
			continue
		}
		state := readLocalState(filepath.Join(root, "Local State"))
		appBound := state.OSCrypt.AppBoundEncrypted != "" || state.OSCrypt.EncryptedKey == ""

		dirs := chromiumProfileDirs(root)
		for _, dir := range dirs {
			p := DetectedProfile{
				Browser:   kind,
				Label:     string(kind) + " — " + filepath.Base(dir),
				Dir:       dir,
				Supported: !appBound,
				CookieDB:  filepath.Join(dir, "Cookies"),
			}
			if _, err := os.Stat(p.CookieDB); err != nil {
				// Newer Chromium keeps cookies under Network/Cookies.
				alt := filepath.Join(dir, "Network", "Cookies")
				if _, err := os.Stat(alt); err == nil {
					p.CookieDB = alt
				} else {
					continue
				}
			}
			if appBound {
				p.Note = UnsupportedNote
			}
			out = append(out, p)
		}
	}

	// Firefox
	if root := firefoxRoot(); root != "" {
		entries, err := os.ReadDir(root)
		if err == nil {
			for _, e := range entries {
				if !e.IsDir() {
					continue
				}
				dir := filepath.Join(root, e.Name())
				db := filepath.Join(dir, "cookies.sqlite")
				if _, err := os.Stat(db); err != nil {
					continue
				}
				out = append(out, DetectedProfile{
					Browser:   BrowserFirefox,
					Label:     "Firefox — " + e.Name(),
					Dir:       dir,
					Supported: true,
					CookieDB:  db,
				})
			}
		}
	}
	return out
}

// chromiumProfileDirs returns "Default" plus "Profile N" directories.
func chromiumProfileDirs(root string) []string {
	var dirs []string
	defaultDir := filepath.Join(root, "Default")
	if st, err := os.Stat(defaultDir); err == nil && st.IsDir() {
		dirs = append(dirs, defaultDir)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return dirs
	}
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), "Profile ") {
			dirs = append(dirs, filepath.Join(root, e.Name()))
		}
	}
	return dirs
}

func readLocalState(path string) localState {
	var s localState
	raw, err := os.ReadFile(path)
	if err != nil {
		return s
	}
	_ = json.Unmarshal(raw, &s)
	return s
}
