package handlers

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"streamdrop/internal/apierr"
)

// OAuthProviders implements GET /api/v1/oauth/providers.
func (h *Handler) OAuthProviders(w http.ResponseWriter, r *http.Request) {
	if h.OAuth == nil {
		writeJSON(w, http.StatusOK, map[string]any{"providers": []any{}})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"providers": h.OAuth.Providers()})
}

// OAuthAuthorize implements POST /api/v1/oauth/{provider}/authorize.
func (h *Handler) OAuthAuthorize(w http.ResponseWriter, r *http.Request) {
	if h.OAuth == nil {
		apierr.Write(w, apierr.New(apierr.CodeAuthOAuthRequired, nil))
		return
	}
	var body struct {
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
	}
	// An empty body is valid: providers with a built-in client need nothing.
	if r.ContentLength > 0 {
		if err := decodeJSON(r, &body); err != nil {
			apierr.Write(w, err)
			return
		}
	}
	session, authURL, err := h.OAuth.Start(r.Context(), chi.URLParam(r, "provider"), body.ClientID, body.ClientSecret)
	if err != nil {
		apierr.Write(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"session_id":   session.ID,
		"auth_url":     authURL,
		"redirect_uri": h.OAuth.RedirectURI(),
		"expires_at":   session.ExpiresAt,
	})
}

// OAuthSession implements GET /api/v1/oauth/session/{id}.
func (h *Handler) OAuthSession(w http.ResponseWriter, r *http.Request) {
	if h.OAuth == nil {
		apierr.Write(w, apierr.New(apierr.CodeValidationFailed, map[string]any{"field": "session"}))
		return
	}
	view, err := h.OAuth.GetSession(chi.URLParam(r, "id"))
	if err != nil {
		apierr.Write(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}

// OAuthRevoke implements DELETE /api/v1/oauth/{provider}.
func (h *Handler) OAuthRevoke(w http.ResponseWriter, r *http.Request) {
	provider := chi.URLParam(r, "provider")
	if err := h.Store.DeleteOAuthToken(provider); err != nil {
		apierr.Write(w, err)
		return
	}
	// Remove the provider's remotes from the daemon configuration; rows stay so
	// history remains readable, but the daemon forgets the credentials.
	if h.Provisioner != nil {
		remotes, err := h.Store.ListRemotes()
		if err == nil {
			for _, remote := range remotes {
				if remote.Provider == provider {
					_ = h.Provisioner.Remove(r.Context(), remote.RcloneName)
					_ = h.Store.SetRemoteStatus(remote.ID, "unavailable", "auth_token_expired", 0)
				}
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"revoked": true})
}
