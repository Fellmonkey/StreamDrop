package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"streamdrop/internal/apierr"
	"streamdrop/internal/deps"
)

// Deps implements GET /api/v1/deps.
func (h *Handler) DepsList(w http.ResponseWriter, r *http.Request) {
	if h.Deps == nil {
		writeJSON(w, http.StatusOK, map[string]any{"components": []any{}})
		return
	}
	items := h.Deps.List()
	installed, total := h.Deps.Summary()
	writeJSON(w, http.StatusOK, map[string]any{
		"components": items,
		"installed":  installed,
		"total":      total,
	})
}

// InstallDep implements POST /api/v1/deps/{id}/install. The download runs in
// the background; progress arrives through dep_progress events.
func (h *Handler) InstallDep(w http.ResponseWriter, r *http.Request) {
	if h.Deps == nil {
		apierr.Write(w, apierr.New(apierr.CodeDependencyMissing, nil))
		return
	}
	id := chi.URLParam(r, "id")
	if _, ok := deps.Find(id); !ok {
		apierr.Write(w, apierr.New(apierr.CodeValidationFailed, map[string]any{"field": "component"}))
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		if err := h.Deps.Install(ctx, id); err != nil && h.Log != nil {
			h.Log.Warn("install %s failed: %v", id, err)
		}
	}()
	writeJSON(w, http.StatusAccepted, map[string]any{"state": "downloading", "component": id})
}

// RemoveDep implements DELETE /api/v1/deps/{id}.
func (h *Handler) RemoveDep(w http.ResponseWriter, r *http.Request) {
	if h.Deps == nil {
		apierr.Write(w, apierr.New(apierr.CodeDependencyMissing, nil))
		return
	}
	if err := h.Deps.Remove(chi.URLParam(r, "id")); err != nil {
		apierr.Write(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": true})
}
