// Package auth builds the authorization headers a request to a protected
// source needs: Cookie, User-Agent, Referer.
package auth

import (
	"net/http"
	"net/url"
	"strings"

	"streamdrop/internal/engine"
)

// HeaderProvider mirrors the engine interface (declared there because Go wants
// interfaces at the consumer side). The alias keeps the auth API readable.
type HeaderProvider = engine.HeaderProvider

// HeaderSet is the resolved header set for one task.
type HeaderSet struct {
	Cookie    string            `json:"cookie,omitempty"`
	UserAgent string            `json:"user_agent,omitempty"`
	Referer   string            `json:"referer,omitempty"`
	Origin    string            `json:"origin,omitempty"`
	Extra     map[string]string `json:"extra,omitempty"`
}

// Apply writes the headers onto a request.
func (h HeaderSet) Apply(req *http.Request) {
	if h.Cookie != "" {
		req.Header.Set("Cookie", h.Cookie)
	}
	if h.UserAgent != "" {
		req.Header.Set("User-Agent", h.UserAgent)
	}
	if h.Referer != "" {
		req.Header.Set("Referer", h.Referer)
	}
	if h.Origin != "" {
		req.Header.Set("Origin", h.Origin)
	}
	for k, v := range h.Extra {
		if v != "" {
			req.Header.Set(k, v)
		}
	}
}

// Map returns the headers as a plain map for external tools.
func (h HeaderSet) Map() map[string]string {
	out := map[string]string{}
	if h.Cookie != "" {
		out["Cookie"] = h.Cookie
	}
	if h.UserAgent != "" {
		out["User-Agent"] = h.UserAgent
	}
	if h.Referer != "" {
		out["Referer"] = h.Referer
	}
	if h.Origin != "" {
		out["Origin"] = h.Origin
	}
	for k, v := range h.Extra {
		if v != "" {
			out[k] = v
		}
	}
	return out
}

// DefaultUserAgent is the pinned Desktop Chrome string used when a profile does
// not override it.
const DefaultUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36"

// RefererForSource derives Referer from the source URL (scheme://host/).
func RefererForSource(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return ""
	}
	return u.Scheme + "://" + u.Host + "/"
}

// hostMatches reports whether a cookie domain belongs to the source host:
// exact match or a parent domain (example.com matches cdn.example.com).
func hostMatches(domain, host string) bool {
	domain = strings.ToLower(strings.TrimPrefix(domain, "."))
	host = strings.ToLower(host)
	if domain == "" || host == "" {
		return false
	}
	if domain == host {
		return true
	}
	return strings.HasSuffix(host, "."+domain)
}
