package auth

import (
	"errors"
	"os"
	"runtime"
	"strings"
)

// BrowserKind identifies a supported browser.
type BrowserKind string

const (
	BrowserChrome  BrowserKind = "chrome"
	BrowserEdge    BrowserKind = "edge"
	BrowserFirefox BrowserKind = "firefox"
	BrowserBrave   BrowserKind = "brave"
	BrowserOpera   BrowserKind = "opera"
	BrowserManual  BrowserKind = "manual"
	BrowserFile    BrowserKind = "file"
)

// Profile modes.
const (
	ModeBrowser     = "browser"
	ModeCookiesFile = "cookies_file"
	ModeManual      = "manual"
)

// KV is a custom header pair.
type KV struct {
	Key   string `json:"key"`
	Value string `json:"value,omitempty"`
}

// Profile is the API representation of an authorization profile. Secrets are
// never serialized: only the presence flag and a mask are returned.
type Profile struct {
	ID              string      `json:"id"`
	Name            string      `json:"name"`
	Kind            BrowserKind `json:"kind"`
	ProfileDir      string      `json:"profile_dir,omitempty"`
	CookieFile      string      `json:"cookie_file,omitempty"`
	UserAgent       string      `json:"user_agent"`
	HasManualCookie bool        `json:"has_manual_cookie"`
	CookieMask      string      `json:"cookie_mask,omitempty"`
	CustomHeaders   []KV        `json:"custom_headers,omitempty"`
	Mode            string      `json:"mode"`
	Supported       bool        `json:"supported"`
	Note            string      `json:"note,omitempty"`
}

// DetectedProfile is one browser profile found on disk.
type DetectedProfile struct {
	Browser   BrowserKind `json:"browser"`
	Label     string      `json:"label"`
	Dir       string      `json:"dir"`
	Supported bool        `json:"supported"`
	Note      string      `json:"note,omitempty"`
	CookieDB  string      `json:"-"`
}

// ErrUnsupported reports that a browser profile cannot be read by the app.
// App-Bound Encryption (Chromium 127+) is the main case: the key is wrapped for
// the browser's own elevation service and cannot be unwrapped by a third-party
// process, not even with administrator rights.
var ErrUnsupported = errors.New("auth: profile encryption is not supported")

// UnsupportedNote is shown in the UI for Chromium 127+ profiles.
const UnsupportedNote = "Браузер использует шифрование, недоступное приложению. Импортируйте cookies.txt или введите Cookie вручную — это работает всегда."

// DetectBrowsers lists installed browsers and their profiles.
func DetectBrowsers() []DetectedProfile {
	if runtime.GOOS != "windows" {
		// The specification pins the Windows profile layout; other platforms
		// fall back to manual cookies / cookies.txt, which work everywhere.
		return nil
	}
	return detectBrowsersWindows()
}

// CookieHeaderFor resolves the Cookie header for a profile and host.
//
// Manual and cookies.txt modes carry their secret in the store, so the caller
// loads them; this helper only performs browser extraction.
func CookieHeaderFor(p Profile, host string) (string, error) {
	switch p.Mode {
	case ModeCookiesFile, ModeManual:
		return "", nil
	case ModeBrowser:
		dir := p.ProfileDir
		if dir == "" {
			return "", ErrUnsupported
		}
		switch p.Kind {
		case BrowserFirefox:
			return FirefoxCookieHeaderFor(dir, p.CookieFile, "", host)
		default:
			return ChromiumCookieHeaderFor(dir, host)
		}
	}
	return "", nil
}

// browserUserAgent returns the pinned Chrome UA string. It is used as a
// fallback so protected sources do not reject requests from unknown clients.
func browserUserAgent(kind BrowserKind) string {
	switch kind {
	case BrowserFirefox:
		return "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:140.0) Gecko/20100101 Firefox/140.0"
	default:
		return DefaultUserAgent
	}
}

func readFileTrim(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}
