package handlers

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"streamdrop/internal/apierr"
	"streamdrop/internal/rclone"
	"streamdrop/internal/store"
	"streamdrop/internal/transfer"
)

var validProviders = map[string]bool{
	"mailru": true, "webdav": true, "yandex": true, "drive": true,
	"onedrive": true, "s3": true, "ftp": true, "sftp": true,
}

func brandFor(provider string) string {
	switch provider {
	case "mailru":
		return "mailru"
	case "yandex":
		return "yandex"
	case "drive":
		return "google"
	case "onedrive":
		return "microsoft"
	default:
		return provider
	}
}

// Remotes implements GET /api/v1/remotes.
func (h *Handler) Remotes(w http.ResponseWriter, r *http.Request) {
	remotes, err := h.Store.ListRemotes()
	if err != nil {
		apierr.Write(w, err)
		return
	}
	if remotes == nil {
		remotes = []store.Remote{}
	}
	// Quota is best effort: a slow or unreachable cloud must not delay the
	// whole list, so each probe gets a short budget.
	if h.Provisioner != nil && h.Daemon != nil && h.Daemon.Ready() {
		for i := range remotes {
			ctx, cancel := context.WithTimeout(r.Context(), 1500*time.Millisecond)
			quota, qerr := h.Provisioner.Quota(ctx, remotes[i].RcloneName+":")
			cancel()
			if qerr == nil {
				remotes[i].Quota = &store.Quota{
					Total: quota.Total, Used: quota.Used, Free: quota.Free, Trashed: quota.Trashed,
				}
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"remotes": remotes})
}

// CreateRemote implements POST /api/v1/remotes.
func (h *Handler) CreateRemote(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name     string            `json:"name"`
		Provider string            `json:"provider"`
		Endpoint string            `json:"endpoint"`
		Options  map[string]string `json:"options"`
	}
	if err := decodeJSON(r, &body); err != nil {
		apierr.Write(w, err)
		return
	}
	body.Name = strings.TrimSpace(body.Name)
	if body.Name == "" {
		apierr.Write(w, apierr.New(apierr.CodeValidationFailed, map[string]any{"field": "name"}))
		return
	}
	if !validProviders[body.Provider] {
		apierr.Write(w, apierr.New(apierr.CodeValidationFailed, map[string]any{"field": "provider"}))
		return
	}
	if body.Options == nil {
		body.Options = map[string]string{}
	}

	id := newUUID()
	remote := store.Remote{
		ID:         id,
		Name:       body.Name,
		Provider:   body.Provider,
		Brand:      brandFor(body.Provider),
		Endpoint:   body.Endpoint,
		RcloneName: "sd_" + strings.ReplaceAll(id, "-", "")[:12],
		Status:     "unknown",
	}
	if err := h.Store.CreateRemote(remote, body.Options); err != nil {
		apierr.Write(w, err)
		return
	}

	// Materialise immediately so the wizard's saved connection is usable.
	if _, err := h.Transfers.EnsureRemote(r.Context(), id); err != nil {
		apiErr := apierr.From(err)
		remote.Status = "error"
		remote.LastError = string(apiErr.Code)
		_ = h.Store.SetRemoteStatus(id, "error", string(apiErr.Code), 0)
		writeJSON(w, http.StatusCreated, map[string]any{"remote": remote, "error": apierr.EnvelopeOf(apiErr)})
		return
	}
	_ = h.Store.SetRemoteStatus(id, "online", "", 0)
	remote.Status = "online"
	writeJSON(w, http.StatusCreated, map[string]any{"remote": remote})
}

// UpdateRemote implements PUT /api/v1/remotes/{id}.
func (h *Handler) UpdateRemote(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	remote, err := h.Store.GetRemote(id)
	if err != nil {
		apierr.Write(w, err)
		return
	}
	var body struct {
		Name     string            `json:"name"`
		Endpoint string            `json:"endpoint"`
		Options  map[string]string `json:"options"`
	}
	if err := decodeJSON(r, &body); err != nil {
		apierr.Write(w, err)
		return
	}
	if strings.TrimSpace(body.Name) != "" {
		remote.Name = strings.TrimSpace(body.Name)
	}
	if body.Endpoint != "" {
		remote.Endpoint = body.Endpoint
	}
	if err := h.Store.UpdateRemote(remote); err != nil {
		apierr.Write(w, err)
		return
	}
	if len(body.Options) > 0 {
		if err := h.Store.UpdateRemoteOptions(id, body.Options); err != nil {
			apierr.Write(w, err)
			return
		}
	}
	if _, err := h.Transfers.EnsureRemote(r.Context(), id); err != nil {
		apiErr := apierr.From(err)
		remote.Status = "error"
		remote.LastError = string(apiErr.Code)
		writeJSON(w, http.StatusOK, map[string]any{"remote": remote, "error": apierr.EnvelopeOf(apiErr)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"remote": remote})
}

// DeleteRemote implements DELETE /api/v1/remotes/{id}.
func (h *Handler) DeleteRemote(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	remote, err := h.Store.GetRemote(id)
	if err != nil {
		apierr.Write(w, err)
		return
	}
	if h.Provisioner != nil {
		_ = h.Provisioner.Remove(r.Context(), remote.RcloneName)
	}
	if err := h.Store.DeleteRemote(id); err != nil {
		apierr.Write(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": true})
}

// TestRemoteCredentials implements POST /api/v1/remotes/test (no record yet).
func (h *Handler) TestRemoteCredentials(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Provider string            `json:"provider"`
		Options  map[string]string `json:"options"`
	}
	if err := decodeJSON(r, &body); err != nil {
		apierr.Write(w, err)
		return
	}
	if !validProviders[body.Provider] {
		apierr.Write(w, apierr.New(apierr.CodeValidationFailed, map[string]any{"field": "provider"}))
		return
	}
	quota, rtt, err := h.Transfers.TestCredentials(r.Context(), body.Provider, body.Options)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": apierr.EnvelopeOf(err)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "rtt_ms": rtt, "quota": quota})
}

// TestRemote implements POST /api/v1/remotes/{id}/test.
func (h *Handler) TestRemote(w http.ResponseWriter, r *http.Request) {
	quota, rtt, err := h.Transfers.CheckRemote(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": apierr.EnvelopeOf(err)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "rtt_ms": rtt, "quota": quota})
}

// ExplorerList implements GET /api/v1/explorer.
func (h *Handler) ExplorerList(w http.ResponseWriter, r *http.Request) {
	remoteID := r.URL.Query().Get("remote_id")
	path := r.URL.Query().Get("path")
	q := r.URL.Query().Get("q")
	remote, err := h.Store.GetRemote(remoteID)
	if err != nil {
		apierr.Write(w, err)
		return
	}
	if h.Explorer == nil {
		apierr.Write(w, apierr.New(apierr.CodeDependencyMissing, map[string]any{"component": "rclone"}))
		return
	}
	// The remote may not be materialised yet (fresh start, daemon restart).
	if _, err := h.Transfers.EnsureRemote(r.Context(), remoteID); err != nil {
		apierr.Write(w, err)
		return
	}
	dirs, err := h.Explorer.List(r.Context(), remote.RcloneName, path, q)
	if err != nil {
		apierr.Write(w, err)
		return
	}
	quota, qerr := h.Explorer.Quota(r.Context(), remote.RcloneName)
	resp := map[string]any{
		"path":        normalizeRemotePath(path),
		"directories": dirs,
	}
	if qerr == nil {
		resp["quota"] = quota
	}
	writeJSON(w, http.StatusOK, resp)
}

// Mkdir implements POST /api/v1/explorer/mkdir.
func (h *Handler) Mkdir(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RemoteID string `json:"remote_id"`
		Path     string `json:"path"`
		Name     string `json:"name"`
	}
	if err := decodeJSON(r, &body); err != nil {
		apierr.Write(w, err)
		return
	}
	remote, err := h.Store.GetRemote(body.RemoteID)
	if err != nil {
		apierr.Write(w, err)
		return
	}
	if h.Explorer == nil {
		apierr.Write(w, apierr.New(apierr.CodeDependencyMissing, map[string]any{"component": "rclone"}))
		return
	}
	if _, err := h.Transfers.EnsureRemote(r.Context(), body.RemoteID); err != nil {
		apierr.Write(w, err)
		return
	}
	if err := h.Explorer.Mkdir(r.Context(), remote.RcloneName, body.Path, body.Name); err != nil {
		apierr.Write(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"path": rclone.JoinPath(body.Path, strings.TrimSpace(body.Name)),
	})
}

func normalizeRemotePath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return "/"
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return p
}

var _ = transfer.Settings{}
