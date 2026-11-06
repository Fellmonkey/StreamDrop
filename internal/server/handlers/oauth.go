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
	writeJSON(w, http.StatusOK, map[string]any{
		"providers":    h.OAuth.Providers(),
		"redirect_uri": h.OAuth.RedirectURI(),
	})
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
	// OneDrive: honor the account-type preference (personal accounts use the
	// "consumers" endpoint; otherwise the "common" endpoint).
	if chi.URLParam(r, "provider") == "onedrive" && h.Store != nil {
		if h.Store.GetSetting("onedrive_account_type", "") == "consumers" {
			h.OAuth.SetEndpointOverride("onedrive",
				"https://login.microsoftonline.com/consumers/oauth2/v2.0/authorize",
				"https://login.microsoftonline.com/consumers/oauth2/v2.0/token")
		} else {
			h.OAuth.SetEndpointOverride("onedrive", "", "")
		}
	}
	session, authURL, err := h.OAuth.Start(r.Context(), chi.URLParam(r, "provider"), body.ClientID, body.ClientSecret)
	if err != nil {
		apierr.Write(w, err)
		return
	}
	// Open the provider page in the system browser from the backend, so the
	// flow works even if the frontend call is blocked. The browser choice set
	// in Settings is honored (defaults to the system browser); errors are
	// logged and the UI still shows a manual link plus a retry button.
	if h.OpenURL != nil {
		browser := "default"
		if h.Store != nil {
			browser = h.Store.GetSetting("oauth_browser", "default")
		}
		go func() {
			if err := h.OpenURL(authURL, browser); err != nil && h.Log != nil {
				h.Log.Warn("oauth browser open failed: %v", err)
			}
		}()
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

// OAuthSubmitCode implements POST /api/v1/oauth/session/{id}/code — completes a
// provider authorization whose code is copied from the provider's verification
// page (Yandex).
func (h *Handler) OAuthSubmitCode(w http.ResponseWriter, r *http.Request) {
	if h.OAuth == nil {
		apierr.Write(w, apierr.New(apierr.CodeValidationFailed, map[string]any{"field": "session"}))
		return
	}
	var body struct {
		Code string `json:"code"`
	}
	if err := decodeJSON(r, &body); err != nil {
		apierr.Write(w, err)
		return
	}
	if body.Code == "" {
		apierr.Write(w, apierr.New(apierr.CodeValidationFailed, map[string]any{"field": "code"}))
		return
	}
	view, err := h.OAuth.CompleteWithCode(r.Context(), chi.URLParam(r, "id"), body.Code)
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
