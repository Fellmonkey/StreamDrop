package auth

import (
	"bufio"
	"strings"
)

// ParseNetscapeCookies parses a Netscape cookies.txt export and returns the
// Cookie header value for the entries that belong to host.
func ParseNetscapeCookies(content string, host string) (string, error) {
	return NetscapeCookieHeader(strings.NewReader(content), host)
}

// NetscapeCookieHeader is the reader-based implementation.
func NetscapeCookieHeader(r interface{ Read([]byte) (int, error) }, host string) (string, error) {
	var sb strings.Builder
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 4<<20)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#HttpOnly_") {
			// A valid directive: strip the marker and keep the entry.
			line = strings.TrimPrefix(line, "#HttpOnly_")
		} else if strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 7 {
			continue
		}
		domain := strings.TrimPrefix(fields[0], ".")
		if !hostMatches(domain, host) {
			continue
		}
		name := fields[5]
		value := fields[6]
		if name == "" {
			continue
		}
		sb.WriteString(name)
		sb.WriteString("=")
		sb.WriteString(value)
		sb.WriteString("; ")
	}
	// Drop the trailing separator so the header has one canonical shape.
	return strings.TrimSpace(strings.TrimRight(sb.String(), "; ")), scanner.Err()
}

// CookieDomains lists the distinct domains present in a Netscape export. The UI
// shows them before the file is applied so the user can tell whether the export
// fits the source.
func CookieDomains(content string) []string {
	seen := map[string]bool{}
	var out []string
	scanner := bufio.NewScanner(strings.NewReader(content))
	scanner.Buffer(make([]byte, 64*1024), 4<<20)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#HttpOnly_") {
			line = strings.TrimPrefix(line, "#HttpOnly_")
		} else if strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 7 {
			continue
		}
		d := strings.TrimPrefix(fields[0], ".")
		if d == "" || seen[d] {
			continue
		}
		seen[d] = true
		out = append(out, d)
	}
	return out
}
