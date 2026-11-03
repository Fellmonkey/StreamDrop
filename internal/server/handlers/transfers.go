package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"streamdrop/internal/apierr"
	"streamdrop/internal/queue"
	"streamdrop/internal/store"
	"streamdrop/internal/transfer"
)

// Inspect implements POST /api/v1/inspect.
func (h *Handler) Inspect(w http.ResponseWriter, r *http.Request) {
	var body struct {
		URL           string `json:"url"`
		AuthProfileID string `json:"auth_profile_id"`
	}
	if err := decodeJSON(r, &body); err != nil {
		apierr.Write(w, err)
		return
	}
	if !isHTTPURL(body.URL) {
		apierr.Write(w, apierr.New(apierr.CodeValidationFailed, map[string]any{"field": "url"}))
		return
	}
	meta, err := h.Transfers.Inspect(r.Context(), body.URL, body.AuthProfileID)
	if err != nil {
		apierr.Write(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"metadata": meta})
}

// CreateTransfers implements POST /api/v1/transfers.
func (h *Handler) CreateTransfers(w http.ResponseWriter, r *http.Request) {
	var req transfer.CreateRequest
	if err := decodeJSON(r, &req); err != nil {
		apierr.Write(w, err)
		return
	}
	created, err := h.Transfers.Create(r.Context(), req)
	if err != nil {
		apierr.Write(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"transfers": created})
}

// ListTransfers implements GET /api/v1/transfers.
func (h *Handler) ListTransfers(w http.ResponseWriter, r *http.Request) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	status := r.URL.Query().Get("status")
	items, total, err := h.Store.ListTransfers(status, page, limit)
	if err != nil {
		apierr.Write(w, err)
		return
	}
	if items == nil {
		items = []store.Transfer{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"transfers": items,
		"total":     total,
		"page":      normalizePage(page),
	})
}

func normalizePage(p int) int {
	if p <= 0 {
		return 1
	}
	return p
}

// QueueState implements GET /api/v1/queue.
func (h *Handler) QueueState(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.Transfers.QueueState())
}

// PauseQueue implements POST /api/v1/queue/pause.
func (h *Handler) PauseQueue(w http.ResponseWriter, r *http.Request) {
	h.Transfers.PauseQueue()
	writeJSON(w, http.StatusOK, h.Transfers.QueueState())
}

// ResumeQueue implements POST /api/v1/queue/resume.
func (h *Handler) ResumeQueue(w http.ResponseWriter, r *http.Request) {
	h.Transfers.ResumeQueue()
	writeJSON(w, http.StatusOK, h.Transfers.QueueState())
}

// ReorderQueue implements POST /api/v1/queue/reorder.
func (h *Handler) ReorderQueue(w http.ResponseWriter, r *http.Request) {
	var body struct {
		IDs []string `json:"ids"`
	}
	if err := decodeJSON(r, &body); err != nil {
		apierr.Write(w, err)
		return
	}
	if len(body.IDs) == 0 {
		apierr.Write(w, apierr.New(apierr.CodeValidationFailed, map[string]any{"field": "ids"}))
		return
	}
	if err := h.Transfers.Reorder(body.IDs); err != nil {
		apierr.Write(w, err)
		return
	}
	writeJSON(w, http.StatusOK, h.Transfers.QueueState())
}

// PauseTransfer implements POST /api/v1/transfers/{id}/pause.
func (h *Handler) PauseTransfer(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.Transfers.Pause(id); err != nil {
		apierr.Write(w, err)
		return
	}
	tr, err := h.Store.GetTransfer(id)
	if err != nil {
		apierr.Write(w, err)
		return
	}
	writeJSON(w, http.StatusOK, tr)
}

// ResumeTransfer implements POST /api/v1/transfers/{id}/resume.
func (h *Handler) ResumeTransfer(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.Transfers.Resume(id); err != nil {
		apierr.Write(w, err)
		return
	}
	tr, err := h.Store.GetTransfer(id)
	if err != nil {
		apierr.Write(w, err)
		return
	}
	writeJSON(w, http.StatusOK, tr)
}

// AbortTransfer implements POST /api/v1/transfers/{id}/abort.
func (h *Handler) AbortTransfer(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.Transfers.Abort(id); err != nil {
		apierr.Write(w, err)
		return
	}
	tr, err := h.Store.GetTransfer(id)
	if err != nil {
		apierr.Write(w, err)
		return
	}
	writeJSON(w, http.StatusOK, tr)
}

// DeleteTransfer implements DELETE /api/v1/transfers/{id}.
func (h *Handler) DeleteTransfer(w http.ResponseWriter, r *http.Request) {
	if err := h.Transfers.Delete(chi.URLParam(r, "id")); err != nil {
		apierr.Write(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": true})
}

// TransferLogs implements GET /api/v1/transfers/{id}/logs.
func (h *Handler) TransferLogs(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if _, err := h.Store.GetTransfer(id); err != nil {
		apierr.Write(w, err)
		return
	}
	tail, _ := strconv.Atoi(r.URL.Query().Get("tail"))
	logs, err := h.Store.ListLogs(id, tail, r.URL.Query().Get("level"))
	if err != nil {
		apierr.Write(w, err)
		return
	}
	if logs == nil {
		logs = nil
	}
	writeJSON(w, http.StatusOK, map[string]any{"logs": logs})
}

// TransferByID implements GET /api/v1/transfers/{id} (details incl. SHA-256).
func (h *Handler) TransferByID(w http.ResponseWriter, r *http.Request) {
	tr, err := h.Store.GetTransfer(chi.URLParam(r, "id"))
	if err != nil {
		apierr.Write(w, err)
		return
	}
	logs, _ := h.Store.ListLogs(tr.ID, 50, "")
	writeJSON(w, http.StatusOK, map[string]any{"transfer": tr, "logs": logs})
}

func isHTTPURL(raw string) bool {
	raw = strings.TrimSpace(raw)
	return strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://")
}

var _ = json.Marshal
var _ = queue.StatusQueued
