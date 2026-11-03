// Package handlers implements the REST contract from back_spec.md §5.1 and
// front_spec.md §8. Every error leaves as a machine-readable code; the client
// builds the localized text.
package handlers

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"streamdrop/internal/apierr"
	"streamdrop/internal/auth"
	"streamdrop/internal/deps"
	"streamdrop/internal/logging"
	"streamdrop/internal/notify"
	"streamdrop/internal/oauth"
	"streamdrop/internal/queue"
	"streamdrop/internal/rclone"
	"streamdrop/internal/security"
	"streamdrop/internal/stats"
	"streamdrop/internal/store"
	"streamdrop/internal/transfer"
	"streamdrop/internal/update"
	"streamdrop/internal/ws"
)

// Handler holds every dependency the REST layer needs.
type Handler struct {
	Store       *store.Store
	Transfers   *transfer.Manager
	Queue       *queue.Manager
	Deps        *deps.Manager
	OAuth       *oauth.Manager
	Explorer    *rclone.Explorer
	Daemon      *rclone.Daemon
	Provisioner *rclone.Provisioner
	Notifier    *notify.Notifier
	Secret      security.SecretStore
	Update      *update.Manager
	Hub         *ws.Hub
	Snapshot    func() any
	Settings    func() transfer.Settings
	// EnsureDaemon starts the rclone RC daemon after rclone is installed at
	// runtime, so a fresh install never needs a manual restart.
	EnsureDaemon func() error
	// OpenBrowser opens the UI in the default browser (normal tab).
	OpenBrowser func() error
	// OpenApp opens/refocuses the desktop-style app window.
	OpenApp func() error
	// OpenURL opens an arbitrary http(s) URL in the requested browser
	// ("default", "edge", "chrome", "firefox"). Used by OAuth flows.
	OpenURL func(url, browser string) error
	Version string
	Port    int
	DataDir string
	Started time.Time
	Log     *logging.Logger
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if v != nil {
		_ = json.NewEncoder(w).Encode(v)
	}
}

// decodeJSON reads a bounded body.
func decodeJSON(r *http.Request, dst any) error {
	body, err := io.ReadAll(io.LimitReader(r.Body, 4<<20))
	if err != nil {
		return apierr.Wrap(apierr.CodeValidationFailed, err, nil)
	}
	if len(strings.TrimSpace(string(body))) == 0 {
		return apierr.New(apierr.CodeValidationFailed, map[string]any{"reason": "empty_body"})
	}
	if err := json.Unmarshal(body, dst); err != nil {
		return apierr.Wrap(apierr.CodeValidationFailed, err, nil)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Status, stats, templates
// ---------------------------------------------------------------------------// Status implements GET /api/v1/status.
func (h *Handler) Status(w http.ResponseWriter, r *http.Request) {
	active, _ := h.Store.ActiveTransfers()
	installed, total := 0, 0
	if h.Deps != nil {
		installed, total = h.Deps.Summary()
	}
	daemonReady := h.Daemon != nil && h.Daemon.Ready()
	engineReady := daemonReady && installed > 0
	writeJSON(w, http.StatusOK, map[string]any{
		"port":             h.Port,
		"version":          h.Version,
		"engine_ready":     engineReady,
		"daemon_ready":     daemonReady,
		"active":           len(active),
		"onboarded":        h.Store.SettingBool("onboarded", false),
		"uptime_seconds":   int64(time.Since(h.Started).Seconds()),
		"components_ok":    installed,
		"components_total": total,
		"ws_clients":       h.Hub.ClientCount(),
		"server_time":      time.Now().UTC(),
	})
}

// Stats implements GET /api/v1/stats: saved bytes plus the wear model.
func (h *Handler) Stats(w http.ResponseWriter, r *http.Request) {
	saved, err := h.Store.SavedBytes()
	if err != nil {
		apierr.Write(w, err)
		return
	}
	report := stats.Calculate(saved)
	writeJSON(w, http.StatusOK, map[string]any{
		"saved_bytes": saved,
		"wear":        report,
	})
}

// Template is a source preset. Labels are localized client-side by id.
type Template struct {
	ID       string `json:"id"`
	Category string `json:"category"`
	URL      string `json:"url"`
	Example  string `json:"example"`
}

// Templates implements GET /api/v1/templates.
func (h *Handler) Templates(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"templates": []Template{
		{ID: "github_release", Category: "code", URL: "https://github.com/owner/repo/releases/latest", Example: "https://github.com/owner/repo/releases/download/v1.0.0/app.zip"},
		{ID: "ubuntu_iso", Category: "iso", URL: "https://releases.ubuntu.com/", Example: "https://releases.ubuntu.com/24.04/ubuntu-24.04-live-server-amd64.iso"},
		{ID: "debian_iso", Category: "iso", URL: "https://cdimage.debian.org/debian-cd/current/amd64/iso-cd/", Example: "https://cdimage.debian.org/debian-cd/current/amd64/iso-cd/debian-12.5.0-amd64-netinst.iso"},
		{ID: "fedora_iso", Category: "iso", URL: "https://fedoraproject.org/workstation/download", Example: "https://download.fedoraproject.org/pub/fedora/linux/releases/41/Workstation/x86_64/iso/Fedora-Workstation-Live-x86_64-41-1.4.iso"},
		{ID: "node_dist", Category: "code", URL: "https://nodejs.org/dist/", Example: "https://nodejs.org/dist/v22.11.0/node-v22.11.0-win-x64.zip"},
		{ID: "archive_org", Category: "media", URL: "https://archive.org/download/", Example: "https://archive.org/download/item/file.mp4"},
	}},
	)
}

// ---------------------------------------------------------------------------
// Settings
// ---------------------------------------------------------------------------

const telegramTokenKey = "telegram_token"
const telegramChatKey = "telegram_chat_id"

// Settings implements GET /api/v1/settings.
func (h *Handler) SettingsGET(w http.ResponseWriter, r *http.Request) {
	values, err := h.Store.Settings()
	if err != nil {
		apierr.Write(w, err)
		return
	}
	// Secrets never leave the process in clear text or at all; only presence
	// is reported.
	_, hasToken := values[telegramTokenKey]
	delete(values, telegramTokenKey)
	delete(values, telegramChatKey)
	values["has_telegram_token"] = boolStr(hasToken)
	writeJSON(w, http.StatusOK, map[string]any{"settings": values})
}

// SettingsPUT implements PUT /api/v1/settings.
func (h *Handler) SettingsPUT(w http.ResponseWriter, r *http.Request) {
	var body map[string]json.RawMessage
	if err := decodeJSON(r, &body); err != nil {
		apierr.Write(w, err)
		return
	}

	updates := map[string]string{}
	asString := func(raw json.RawMessage) (string, bool) {
		var s string
		if err := json.Unmarshal(raw, &s); err == nil {
			return s, true
		}
		var n int64
		if err := json.Unmarshal(raw, &n); err == nil {
			return strconv.FormatInt(n, 10), true
		}
		var b bool
		if err := json.Unmarshal(raw, &b); err == nil {
			if b {
				return "1", true
			}
			return "0", true
		}
		return "", false
	}

	for key, raw := range body {
		val, ok := asString(raw)
		if !ok {
			continue
		}
		switch key {
		case "parallel_limit":
			n, err := strconv.Atoi(val)
			if err != nil || n < 1 || n > 5 {
				apierr.Write(w, apierr.New(apierr.CodeValidationFailed, map[string]any{"field": key}))
				return
			}
			if h.Queue != nil {
				if err := h.Queue.SetLimit(n); err != nil {
					apierr.Write(w, err)
					return
				}
			}
		case "default_mode":
			if val != "pure_stream" && val != "ring_buffer" {
				apierr.Write(w, apierr.New(apierr.CodeValidationFailed, map[string]any{"field": key}))
				return
			}
		case "default_buffer_bytes":
			n, err := strconv.ParseInt(val, 10, 64)
			if err != nil || !validBuffer(n) {
				apierr.Write(w, apierr.New(apierr.CodeValidationFailed, map[string]any{"field": key}))
				return
			}
		case "locale":
			if val != "ru" && val != "en" {
				apierr.Write(w, apierr.New(apierr.CodeValidationFailed, map[string]any{"field": key}))
				return
			}
		case "retry_max_attempts":
			n, err := strconv.Atoi(val)
			if err != nil || n < 0 || n > 10 {
				apierr.Write(w, apierr.New(apierr.CodeValidationFailed, map[string]any{"field": key}))
				return
			}
		case "bandwidth_down_bps", "bandwidth_up_bps":
			n, err := strconv.ParseInt(val, 10, 64)
			if err != nil || n < 0 {
				apierr.Write(w, apierr.New(apierr.CodeValidationFailed, map[string]any{"field": key}))
				return
			}
		case "telegram_token":
			if val == "" {
				updates[telegramTokenKey] = ""
				continue
			}
			enc, err := h.encryptSecret(val)
			if err != nil {
				apierr.Write(w, err)
				return
			}
			updates[telegramTokenKey] = enc
			continue
		case "telegram_chat_id":
			if val == "" {
				updates[telegramChatKey] = ""
				continue
			}
			enc, err := h.encryptSecret(val)
			if err != nil {
				apierr.Write(w, err)
				return
			}
			updates[telegramChatKey] = enc
			continue
		case "checksum_enabled", "onboarded", "crash_reporting_enabled", "telegram_enabled", "notifications_toast":
			if val != "0" && val != "1" && val != "true" && val != "false" {
				apierr.Write(w, apierr.New(apierr.CodeValidationFailed, map[string]any{"field": key}))
				return
			}
			if val == "true" {
				val = "1"
			}
			if val == "false" {
				val = "0"
			}
		default:
			// Unknown keys are ignored so an older UI keeps working.
			continue
		}
		updates[key] = val
	}

	if err := h.Store.SetSettings(updates); err != nil {
		apierr.Write(w, err)
		return
	}
	h.SettingsGET(w, r)
}

func validBuffer(n int64) bool {
	for _, mib := range []int{32, 64, 128, 256, 512, 1024} {
		if n == int64(mib)<<20 {
			return true
		}
	}
	return false
}

func boolStr(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

// encryptSecret seals a settings secret as "enc:<base64>".
func (h *Handler) encryptSecret(plain string) (string, error) {
	if h.Secret == nil {
		return "", apierr.New(apierr.CodeInternalError, nil)
	}
	enc, err := h.Secret.Encrypt([]byte(plain))
	if err != nil {
		return "", apierr.Wrap(apierr.CodeInternalError, err, nil)
	}
	return "enc:" + base64.StdEncoding.EncodeToString(enc), nil
}

func (h *Handler) decryptSecret(value string) string {
	if !strings.HasPrefix(value, "enc:") || h.Secret == nil {
		return ""
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(value, "enc:"))
	if err != nil {
		return ""
	}
	plain, err := h.Secret.Decrypt(raw)
	if err != nil {
		return ""
	}
	return string(plain)
}

// telegramSettings resolves the notification configuration.
func (h *Handler) telegramSettings() notify.Settings {
	return notify.Settings{
		ToastEnabled:    h.Store.SettingBool("notifications_toast", true),
		TelegramEnabled: h.Store.SettingBool("telegram_enabled", false),
		TelegramToken:   h.decryptSecret(h.Store.GetSetting(telegramTokenKey, "")),
		TelegramChatID:  h.decryptSecret(h.Store.GetSetting(telegramChatKey, "")),
	}
}

// NotificationsTest implements POST /api/v1/notifications/test.
func (h *Handler) NotificationsTest(w http.ResponseWriter, r *http.Request) {
	if h.Notifier == nil {
		apierr.Write(w, apierr.New(apierr.CodeValidationFailed, map[string]any{"field": "notifications"}))
		return
	}
	cfg := h.telegramSettings()
	if cfg.TelegramEnabled && cfg.TelegramChatID != "" && !isNumericChatID(cfg.TelegramChatID) {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error": map[string]any{
				"code":        apierr.CodeTelegramChatInvalid,
				"kind":        "auth",
				"http_status": http.StatusBadRequest,
				"params":      map[string]any{"field": "telegram_chat_id"},
			},
		})
		return
	}
	if err := h.Notifier.TestMessage(r.Context(), cfg); err != nil {
		apierr.Write(w, apierr.Wrap(apierr.CodeValidationFailed, err, map[string]any{"field": "notifications"}))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// isNumericChatID accepts telegram chat ids (positive users/groups and
// negative supergroup ids). Bot @usernames are never valid chat ids.
func isNumericChatID(s string) bool {
	if s == "" {
		return false
	}
	trimmed := strings.TrimPrefix(s, "-")
	if trimmed == "" {
		return false
	}
	for _, ch := range trimmed {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return true
}

// UIOpen implements POST /api/v1/ui/open — opens the UI either in the default
// browser (mode=browser) or in the desktop app window (mode=app).
func (h *Handler) UIOpen(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Mode string `json:"mode"`
	}
	if err := decodeJSON(r, &body); err != nil {
		apierr.Write(w, err)
		return
	}
	switch body.Mode {
	case "browser":
		if h.OpenBrowser != nil {
			if err := h.OpenBrowser(); err != nil {
				apierr.Write(w, apierr.Wrap(apierr.CodeInternalError, err, nil))
				return
			}
		}
	case "app":
		if h.OpenApp != nil {
			if err := h.OpenApp(); err != nil {
				apierr.Write(w, apierr.Wrap(apierr.CodeInternalError, err, nil))
				return
			}
		}
	default:
		apierr.Write(w, apierr.New(apierr.CodeValidationFailed, map[string]any{"field": "mode"}))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// UIOpenURL implements POST /api/v1/ui/open-url — opens an http(s) URL in the
// selected browser (used by OAuth and external links).
func (h *Handler) UIOpenURL(w http.ResponseWriter, r *http.Request) {
	if h.OpenURL == nil {
		apierr.Write(w, apierr.New(apierr.CodeInternalError, nil))
		return
	}
	var body struct {
		URL     string `json:"url"`
		Browser string `json:"browser"`
	}
	if err := decodeJSON(r, &body); err != nil {
		apierr.Write(w, err)
		return
	}
	u, err := url.Parse(body.URL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		apierr.Write(w, apierr.New(apierr.CodeValidationFailed, map[string]any{"field": "url"}))
		return
	}
	if body.Browser == "" {
		body.Browser = "default"
	}
	if err := h.OpenURL(u.String(), body.Browser); err != nil {
		apierr.Write(w, apierr.Wrap(apierr.CodeInternalError, err, nil))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

// UpdateCheck implements GET /api/v1/update.
func (h *Handler) UpdateCheck(w http.ResponseWriter, r *http.Request) {
	if h.Update == nil {
		writeJSON(w, http.StatusOK, map[string]any{"available": false, "current_version": h.Version})
		return
	}
	info, err := h.Update.Check(r.Context(), false)
	if err != nil {
		apiErr := apierr.From(err)
		writeJSON(w, http.StatusOK, map[string]any{
			"available":       false,
			"current_version": h.Version,
			"error_code":      apiErr.Code,
		})
		return
	}
	writeJSON(w, http.StatusOK, info)
}

// UpdateApply implements POST /api/v1/update/apply.
func (h *Handler) UpdateApply(w http.ResponseWriter, r *http.Request) {
	if h.Update == nil {
		apierr.Write(w, apierr.New(apierr.CodeValidationFailed, map[string]any{"field": "update"}))
		return
	}
	go func() {
		if _, err := h.Update.Apply(context.WithoutCancel(r.Context())); err != nil && h.Log != nil {
			h.Log.Warn("update apply failed: %v", err)
		}
	}()
	writeJSON(w, http.StatusAccepted, map[string]any{"state": "downloading"})
}

// ---------------------------------------------------------------------------
// Misc
// ---------------------------------------------------------------------------

// randomToken returns a URL-safe random token.
func randomToken(bytes int) string {
	b := make([]byte, bytes)
	if _, err := rand.Read(b); err != nil {
		return base64.RawURLEncoding.EncodeToString([]byte(time.Now().String()))
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// Browsers implements GET /api/v1/auth/browsers.
func (h *Handler) Browsers(w http.ResponseWriter, r *http.Request) {
	profiles := auth.DetectBrowsers()
	if profiles == nil {
		profiles = []auth.DetectedProfile{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"browsers": profiles})
}

var (
	_ = errors.New
	_ = store.Remote{}
)
