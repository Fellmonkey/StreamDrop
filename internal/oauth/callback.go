package oauth

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"streamdrop/internal/apierr"
)

// ensureCallbackServer starts the fixed-port callback listener once.
//
// The listener is intentionally separate from the UI server: redirect URIs are
// registered at the providers for 127.0.0.1:35357, so the UI port can change
// freely.
func (m *Manager) ensureCallbackServer() error {
	m.mu.Lock()
	if m.server != nil {
		m.mu.Unlock()
		return nil
	}
	if m.serverErr != nil {
		err := m.serverErr
		m.mu.Unlock()
		return err
	}
	m.mu.Unlock()

	addr := fmt.Sprintf("127.0.0.1:%d", m.callbackPort)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		m.mu.Lock()
		m.serverErr = apierr.Wrap(apierr.CodeSourceUnreachable, err, map[string]any{"port": m.callbackPort})
		m.mu.Unlock()
		return apierr.Wrap(apierr.CodeSourceUnreachable, err, map[string]any{"port": m.callbackPort})
	}

	// With an ephemeral port the registered redirect URI must follow the
	// actual listener address, otherwise the provider would redirect nowhere.
	if m.callbackPort <= 0 {
		m.mu.Lock()
		m.redirectURI = fmt.Sprintf("http://%s%s", ln.Addr().String(), CallbackPath)
		m.mu.Unlock()
	}

	mux := http.NewServeMux()
	mux.HandleFunc(CallbackPath, m.handleCallback)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, CallbackPath, http.StatusFound)
	})

	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	m.mu.Lock()
	m.server = srv
	m.mu.Unlock()

	go func() {
		if serveErr := srv.Serve(ln); serveErr != nil && serveErr != http.ErrServerClosed {
			m.mu.Lock()
			m.serverErr = apierr.Wrap(apierr.CodeSourceUnreachable, serveErr, nil)
			m.server = nil
			m.mu.Unlock()
		}
	}()
	return nil
}

// handleCallback is the provider redirect endpoint.
func (m *Manager) handleCallback(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	state := query.Get("state")
	code := query.Get("code")
	providerErr := query.Get("error")

	if state == "" {
		writeCallbackPage(w, http.StatusBadRequest, false)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	if _, err := m.Complete(ctx, state, code, providerErr); err != nil {
		writeCallbackPage(w, http.StatusBadRequest, false)
		return
	}
	writeCallbackPage(w, http.StatusOK, true)
}

// writeCallbackPage serves the tiny browser landing page. It is deliberately
// bilingual: it is rendered by the Go process, not by the localized UI.
func writeCallbackPage(w http.ResponseWriter, status int, ok bool) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	title := "Authorization complete"
	body := "Авторизация завершена. Вернитесь в приложение."
	if !ok {
		title = "Authorization failed"
		body = "Авторизация не удалась. Вернитесь в приложение и попробуйте снова."
	}
	fmt.Fprintf(w, `<!doctype html><html lang="ru"><head><meta charset="utf-8"><title>StreamDrop — %s</title>
<style>body{background:#090A0F;color:#e5e7eb;font-family:system-ui,Segoe UI,sans-serif;display:flex;align-items:center;justify-content:center;min-height:100vh;margin:0}
.card{background:#11131B;border:1px solid rgba(255,255,255,.1);border-radius:16px;padding:40px 48px;max-width:420px;text-align:center}
h1{font-size:18px;margin:0 0 12px}p{color:#9ca3af;font-size:14px;line-height:1.6;margin:0}
.dot{width:10px;height:10px;border-radius:50%%;background:%s;display:inline-block;margin-right:8px;box-shadow:0 0 12px %s}</style>
</head><body><div class="card"><h1><span class="dot"></span>%s</h1><p>%s</p><p style="margin-top:16px">You can close this tab.</p></div></body></html>`,
		title, dotColor(ok), dotColor(ok), title, body)
}

func dotColor(ok bool) string {
	if ok {
		return "#34D399"
	}
	return "#F87171"
}
