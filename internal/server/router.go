// Package server registers the REST routes and composes the middleware stack.
package server

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"streamdrop/internal/server/handlers"
	"streamdrop/internal/server/middleware"
	"streamdrop/internal/ws"
)

// New builds the HTTP handler.
//
// LocalGuard wraps the whole handler in the app (it needs the actual port);
// the token requirement applies to the REST subtree only, because the WebSocket
// passes its token as a query parameter.
func New(h *handlers.Handler, token string, port int, hub *ws.Hub, static http.Handler) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Recover(func(format string, args ...any) { h.Log.Error(format, args...) }))
	r.Use(middleware.CORS(port))

	r.Handle("/api/v1/ws", hub)

	r.Route("/api/v1", func(api chi.Router) {
		api.Use(middleware.RequireToken(token))

		api.Post("/inspect", h.Inspect)

		api.Post("/transfers", h.CreateTransfers)
		api.Get("/transfers", h.ListTransfers)
		api.Get("/transfers/{id}", h.TransferByID)
		api.Delete("/transfers/{id}", h.DeleteTransfer)
		api.Post("/transfers/{id}/pause", h.PauseTransfer)
		api.Post("/transfers/{id}/resume", h.ResumeTransfer)
		api.Post("/transfers/{id}/abort", h.AbortTransfer)
		api.Get("/transfers/{id}/logs", h.TransferLogs)

		api.Get("/queue", h.QueueState)
		api.Post("/queue/pause", h.PauseQueue)
		api.Post("/queue/resume", h.ResumeQueue)
		api.Post("/queue/reorder", h.ReorderQueue)

		api.Get("/remotes", h.Remotes)
		api.Post("/remotes", h.CreateRemote)
		api.Post("/remotes/test", h.TestRemoteCredentials)
		api.Put("/remotes/{id}", h.UpdateRemote)
		api.Delete("/remotes/{id}", h.DeleteRemote)
		api.Post("/remotes/{id}/test", h.TestRemote)

		api.Get("/explorer", h.ExplorerList)
		api.Post("/explorer/mkdir", h.Mkdir)

		api.Get("/oauth/providers", h.OAuthProviders)
		api.Post("/oauth/{provider}/authorize", h.OAuthAuthorize)
		api.Get("/oauth/session/{id}", h.OAuthSession)
		api.Delete("/oauth/{provider}", h.OAuthRevoke)

		api.Get("/auth/browsers", h.Browsers)
		api.Get("/auth/profiles", h.Profiles)
		api.Post("/auth/profiles", h.CreateProfile)
		api.Put("/auth/profiles/{id}", h.UpdateProfile)
		api.Delete("/auth/profiles/{id}", h.DeleteProfile)
		api.Post("/auth/profiles/{id}/test", h.TestProfile)

		api.Get("/deps", h.DepsList)
		api.Post("/deps/{id}/install", h.InstallDep)
		api.Delete("/deps/{id}", h.RemoveDep)

		api.Get("/settings", h.SettingsGET)
		api.Put("/settings", h.SettingsPUT)
		api.Post("/notifications/test", h.NotificationsTest)
		api.Post("/ui/open", h.UIOpen)
		api.Post("/ui/open-url", h.UIOpenURL)

		api.Get("/stats", h.Stats)
		api.Get("/status", h.Status)
		api.Get("/templates", h.Templates)

		api.Get("/update", h.UpdateCheck)
		api.Post("/update/apply", h.UpdateApply)

		api.NotFound(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":{"code":"not_found","kind":"source","http_status":404}}`))
		})
	})

	r.Handle("/*", static)
	return r
}
