//go:build !windows

package auth

// ChromiumCookieHeaderFor is unavailable on non-Windows builds; manual
// cookies / cookies.txt work everywhere.
func ChromiumCookieHeaderFor(profileDir, host string) (string, error) {
	return "", ErrUnsupported
}
