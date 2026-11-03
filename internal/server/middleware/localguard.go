// Package middleware contains the local API guard (token, Host, Origin) and
// the standard request middlewares.
package middleware

import (
	"crypto/subtle"
	"net"
	"net/http"
	"strings"

	"streamdrop/internal/apierr"
)

// LocalGuard protects the loopback API from drive-by requests and
// DNS-rebinding (back_spec.md §5 / PRD §5.2).
//
// Three checks:
//  1. Host must be a loopback host on the actual UI port.
//  2. Origin, when present, must be the local page.
//  3. Sec-Fetch-Site, when present, must be same-origin or none.
//
// The token itself is validated by RequireToken.
type LocalGuard struct {
	Port int
}

func (g LocalGuard) allowedHosts() []string {
	return []string{
		"127.0.0.1:" + itoa(g.Port),
		"localhost:" + itoa(g.Port),
		"[::1]:" + itoa(g.Port),
	}
}

func (g LocalGuard) allowedOrigins() []string {
	return []string{
		"http://127.0.0.1:" + itoa(g.Port),
		"http://localhost:" + itoa(g.Port),
		"http://[::1]:" + itoa(g.Port),
	}
}

// Handler wraps next.
func (g LocalGuard) Handler(next http.Handler) http.Handler {
	allowedHosts := g.allowedHosts()
	allowedOrigins := g.allowedOrigins()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := strings.ToLower(r.Host)
		if !contains(allowedHosts, host) {
			apierr.Write(w, apierr.New(apierr.CodeLocalAuthRequired, map[string]any{"reason": "host"}))
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" {
			if !contains(allowedOrigins, strings.ToLower(strings.TrimRight(origin, "/"))) {
				apierr.Write(w, apierr.New(apierr.CodeLocalAuthRequired, map[string]any{"reason": "origin"}))
				return
			}
		}
		if site := r.Header.Get("Sec-Fetch-Site"); site != "" && site != "same-origin" && site != "none" {
			apierr.Write(w, apierr.New(apierr.CodeLocalAuthRequired, map[string]any{"reason": "sec_fetch_site"}))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequireToken enforces the per-run X-StreamDrop-Token header. WebSocket
// connections pass the token in the query string and are handled by the hub.
func RequireToken(token string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			got := r.Header.Get("X-StreamDrop-Token")
			if got == "" || subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 {
				apierr.Write(w, apierr.New(apierr.CodeLocalAuthRequired, map[string]any{"reason": "token"}))
				return
			}
			w.Header().Set("Cache-Control", "no-store")
			next.ServeHTTP(w, r)
		})
	}
}

// CORS answers preflight for the local page only and never uses a wildcard.
func CORS(port int) func(http.Handler) http.Handler {
	origins := LocalGuard{Port: port}.allowedOrigins()
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := strings.ToLower(strings.TrimRight(r.Header.Get("Origin"), "/"))
			if origin != "" && contains(origins, origin) {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Vary", "Origin")
				w.Header().Set("Access-Control-Allow-Headers", "X-StreamDrop-Token, Content-Type")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			}
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// Recover converts a panic into a 500 instead of killing the process.
func Recover(logf func(string, ...any)) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					if logf != nil {
						logf("panic serving %s %s: %v", r.Method, r.URL.Path, rec)
					}
					apierr.Write(w, apierr.New(apierr.CodeInternalError, nil).WithDetail("panic"))
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// IsLoopbackAddr reports whether a listen address stayed on loopback.
func IsLoopbackAddr(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	return ip != nil && ip.IsLoopback()
}

func contains(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := []byte{}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}
