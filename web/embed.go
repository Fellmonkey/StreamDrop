// Package web embeds the compiled SPA and serves it with the per-run API token
// injected into index.html (back_spec.md §5, §6).
package web

import (
	"bytes"
	"embed"
	"fmt"
	"html"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed all:dist
var content embed.FS

// dist returns the compiled asset filesystem.
func dist() (fs.FS, error) {
	return fs.Sub(content, "dist")
}

// Handler serves the SPA. index.html gets a meta tag carrying the token that
// the local API requires; the token is never exposed anywhere else.
func Handler(token string) http.Handler {
	sub, err := dist()
	if err != nil {
		panic(err)
	}
	fileServer := http.FileServer(http.FS(sub))

	index, _ := fs.ReadFile(sub, "index.html")
	index = injectToken(index, token)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The token-authenticated API and the WebSocket are handled by the
		// router; this handler only serves the shell and static assets.
		p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if p == "" || p == "." || p == "index.html" {
			serveIndex(w, index)
			return
		}
		if _, statErr := fs.Stat(sub, p); statErr == nil {
			setAssetHeaders(w, p)
			fileServer.ServeHTTP(w, r)
			return
		}
		// SPA fallback: unknown extension-less paths render the app shell.
		if !strings.Contains(path.Base(p), ".") {
			serveIndex(w, index)
			return
		}
		http.NotFound(w, r)
	})
}

func serveIndex(w http.ResponseWriter, index []byte) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy",
		"default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self' data:; connect-src 'self' ws://127.0.0.1:* ws://localhost:*; frame-ancestors 'none'; base-uri 'none'; form-action 'none'")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	_, _ = w.Write(index)
}

func setAssetHeaders(w http.ResponseWriter, p string) {
	switch {
	case strings.HasSuffix(p, ".js"):
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	case strings.HasSuffix(p, ".css"):
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
	case strings.HasSuffix(p, ".svg"):
		w.Header().Set("Content-Type", "image/svg+xml")
	case strings.HasSuffix(p, ".woff2"):
		w.Header().Set("Content-Type", "font/woff2")
	}
	// Vite emits content-hashed asset names; they can be cached for a year.
	if strings.HasPrefix(p, "assets/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	}
}

func injectToken(index []byte, token string) []byte {
	meta := fmt.Sprintf("<meta name=\"streamdrop-token\" content=\"%s\">", html.EscapeString(token))
	if idx := bytes.Index(index, []byte("</head>")); idx >= 0 {
		out := make([]byte, 0, len(index)+len(meta))
		out = append(out, index[:idx]...)
		out = append(out, meta...)
		out = append(out, index[idx:]...)
		return out
	}
	return append(index, []byte(meta)...)
}
