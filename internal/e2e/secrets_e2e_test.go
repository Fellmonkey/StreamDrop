package e2e

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestAPIDoesNotLeakSecrets guards the API contract: encrypted fields must
// never come back in plaintext.
func TestAPIDoesNotLeakSecrets(t *testing.T) {
	if getenv("MAILRU_USER", "") == "" {
		t.Skip("MAILRU_USER/MAILRU_PASS not set")
	}
	base := getenv("STREAMDROP_URL", "http://127.0.0.1:8085")
	api := newAPIClient(t, base)
	pass := getenv("MAILRU_PASS", "")
	user := getenv("MAILRU_USER", "")

	_, _ = ensureRemoteE2E(t, api)

	// Store a known telegram secret via the API (server encrypts it).
	const token = "dummy-telegram-token-abc123"
	const chatID = "424242"
	status, data := api.do(t, "PUT", "/api/v1/settings", map[string]any{
		"telegram_token":   token,
		"telegram_chat_id": chatID,
	})
	requireOK(t, status, data, "put secrets")

	checks := []struct {
		method string
		path   string
	}{
		{"GET", "/api/v1/settings"},
		{"GET", "/api/v1/remotes"},
		{"GET", "/api/v1/auth/profiles"},
		{"GET", "/api/v1/transfers?limit=50"},
	}
	for _, c := range checks {
		status, data = api.do(t, c.method, c.path, nil)
		requireOK(t, status, data, c.path)
		body := string(data)
		for _, secret := range []string{token, chatID, pass, user} {
			if secret == "" {
				continue
			}
			if strings.Contains(body, secret) {
				t.Fatalf("%s leaks secret value %q", c.path, secret)
			}
		}
	}
	_ = json.Valid(data)
}
