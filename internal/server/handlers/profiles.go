package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-chi/chi/v5"

	"streamdrop/internal/apierr"
	"streamdrop/internal/auth"
	"streamdrop/internal/store"
)

// Profiles implements GET /api/v1/auth/profiles.
func (h *Handler) Profiles(w http.ResponseWriter, r *http.Request) {
	profiles, err := h.Store.ListAuthProfiles()
	if err != nil {
		apierr.Write(w, err)
		return
	}
	if profiles == nil {
		profiles = []store.AuthProfile{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"profiles": profiles})
}

type profileBody struct {
	Name       string    `json:"name"`
	Mode       string    `json:"mode"`
	Kind       string    `json:"kind"`
	ProfileDir string    `json:"profile_dir"`
	CookieFile string    `json:"cookie_file"`
	CookiesTxt string    `json:"cookies_txt"`
	Cookie     string    `json:"cookie"`
	UserAgent  string    `json:"user_agent"`
	Referer    string    `json:"referer"`
	Origin     string    `json:"origin"`
	Extra      []auth.KV `json:"custom_headers"`
}

// CreateProfile implements POST /api/v1/auth/profiles.
func (h *Handler) CreateProfile(w http.ResponseWriter, r *http.Request) {
	var body profileBody
	if err := decodeJSON(r, &body); err != nil {
		apierr.Write(w, err)
		return
	}
	switch body.Mode {
	case auth.ModeBrowser, auth.ModeCookiesFile, auth.ModeManual:
	default:
		apierr.Write(w, apierr.New(apierr.CodeValidationFailed, map[string]any{"field": "mode"}))
		return
	}
	if strings.TrimSpace(body.Name) == "" {
		body.Name = defaultProfileName(body)
	}

	id := newUUID()
	supported, note := profileSupport(body.Mode, body.Kind, body.ProfileDir)

	cookieFile := body.CookieFile
	if body.Mode == auth.ModeCookiesFile && body.CookiesTxt != "" {
		dir := filepath.Join(h.DataDir, "auth")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			apierr.Write(w, apierr.Wrap(apierr.CodeInternalError, err, nil))
			return
		}
		path := filepath.Join(dir, "cookies-"+id+".txt")
		if err := os.WriteFile(path, []byte(body.CookiesTxt), 0o600); err != nil {
			apierr.Write(w, apierr.Wrap(apierr.CodeInternalError, err, nil))
			return
		}
		cookieFile = path
	}
	if body.Mode == auth.ModeCookiesFile && cookieFile == "" {
		apierr.Write(w, apierr.New(apierr.CodeValidationFailed, map[string]any{"field": "cookie_file"}))
		return
	}
	if body.Mode == auth.ModeManual && body.Cookie == "" {
		apierr.Write(w, apierr.New(apierr.CodeValidationFailed, map[string]any{"field": "cookie"}))
		return
	}

	headers := store.ProfileHeaders{
		Cookie:     body.Cookie,
		UserAgent:  body.UserAgent,
		Referer:    body.Referer,
		Origin:     body.Origin,
		Extra:      kvMap(body.Extra),
		CookieFile: cookieFile,
	}
	if headers.UserAgent == "" {
		headers.UserAgent = auth.DefaultUserAgent
	}

	profile := store.AuthProfile{
		ID:         id,
		Name:       body.Name,
		Kind:       body.Kind,
		Mode:       body.Mode,
		ProfileDir: body.ProfileDir,
		CookieFile: cookieFile,
		Supported:  supported,
		Note:       note,
	}
	if err := h.Store.CreateAuthProfile(profile, headers); err != nil {
		apierr.Write(w, err)
		return
	}
	createResponse(h, w, id)
}

// UpdateProfile implements PUT /api/v1/auth/profiles/{id}.
func (h *Handler) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	profile, headers, err := h.Store.GetAuthProfile(id)
	if err != nil {
		apierr.Write(w, err)
		return
	}
	var body profileBody
	if err := decodeJSON(r, &body); err != nil {
		apierr.Write(w, err)
		return
	}
	// Empty fields keep their stored value, so a rename never wipes a secret.
	if body.Name != "" {
		profile.Name = body.Name
	}
	if body.Mode != "" {
		profile.Mode = body.Mode
	}
	if body.Kind != "" {
		profile.Kind = body.Kind
	}
	if body.ProfileDir != "" {
		profile.ProfileDir = body.ProfileDir
	}
	if body.UserAgent != "" {
		headers.UserAgent = body.UserAgent
	}
	if body.Referer != "" {
		headers.Referer = body.Referer
	}
	if body.Origin != "" {
		headers.Origin = body.Origin
	}
	if body.Cookie != "" {
		headers.Cookie = body.Cookie
	}
	if len(body.Extra) > 0 {
		headers.Extra = kvMap(body.Extra)
	}
	if body.CookieFile != "" {
		profile.CookieFile = body.CookieFile
		headers.CookieFile = body.CookieFile
	}
	profile.Supported, profile.Note = profileSupport(profile.Mode, profile.Kind, profile.ProfileDir)

	if err := h.Store.UpdateAuthProfile(profile, &headers); err != nil {
		apierr.Write(w, err)
		return
	}
	createResponse(h, w, id)
}

// DeleteProfile implements DELETE /api/v1/auth/profiles/{id}.
func (h *Handler) DeleteProfile(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.Store.DeleteAuthProfile(id); err != nil {
		apierr.Write(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": true})
}

// TestProfile implements POST /api/v1/auth/profiles/{id}/test.
func (h *Handler) TestProfile(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var body struct {
		URL string `json:"url"`
	}
	if err := decodeJSON(r, &body); err != nil {
		apierr.Write(w, err)
		return
	}
	if !isHTTPURL(body.URL) {
		apierr.Write(w, apierr.New(apierr.CodeValidationFailed, map[string]any{"field": "url"}))
		return
	}
	meta, err := h.Transfers.ProbeProfile(r.Context(), id, body.URL)
	if err != nil {
		apierr.Write(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "metadata": meta})
}

func createResponse(h *Handler, w http.ResponseWriter, id string) {
	profiles, err := h.Store.ListAuthProfiles()
	if err != nil {
		apierr.Write(w, err)
		return
	}
	for _, p := range profiles {
		if p.ID == id {
			writeJSON(w, http.StatusCreated, map[string]any{"profile": p})
			return
		}
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": id})
}

func defaultProfileName(body profileBody) string {
	switch body.Mode {
	case auth.ModeManual:
		return "Manual cookies"
	case auth.ModeCookiesFile:
		return "cookies.txt"
	default:
		if body.Kind != "" {
			return body.Kind
		}
		return "browser"
	}
}

func profileSupport(mode, kind, dir string) (bool, string) {
	if mode != auth.ModeBrowser {
		return true, ""
	}
	for _, b := range auth.DetectBrowsers() {
		if dir != "" && strings.EqualFold(filepath.Clean(b.Dir), filepath.Clean(dir)) {
			return b.Supported, b.Note
		}
		if dir == "" && string(b.Browser) == kind {
			return b.Supported, b.Note
		}
	}
	// A profile that is not detected now may be detected later; do not hard
	// fail it, but make the limitation visible.
	return true, ""
}

func kvMap(list []auth.KV) map[string]string {
	if len(list) == 0 {
		return nil
	}
	out := map[string]string{}
	for _, kv := range list {
		if strings.TrimSpace(kv.Key) != "" {
			out[strings.TrimSpace(kv.Key)] = kv.Value
		}
	}
	return out
}

func newUUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "id-" + strings.ReplaceAll(defaultProfileName(profileBody{}), " ", "-")
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	hexs := hex.EncodeToString(b[:])
	return hexs[0:8] + "-" + hexs[8:12] + "-" + hexs[12:16] + "-" + hexs[16:20] + "-" + hexs[20:32]
}

var _ = randomToken
