//go:build !windows

package auth

// Browser cookie auto-extraction is implemented for the Windows profile layout
// only. On other platforms manual cookies and cookies.txt imports are offered,
// which work everywhere and are the documented universal path.
func detectBrowsersWindows() []DetectedProfile { return nil }

// ChromiumCookieHeader is not available off Windows.
func ChromiumCookieHeader(profileDir string) (string, error) { return "", ErrUnsupported }

// FirefoxCookieHeader is not available off Windows.
func FirefoxCookieHeader(profileDir, cookieFile, keyFile string) (string, error) {
	return "", ErrUnsupported
}

// FirefoxCookieHeaderFor is not available off Windows.
func FirefoxCookieHeaderFor(profileDir, cookieFile, keyFile, host string) (string, error) {
	return "", ErrUnsupported
}
