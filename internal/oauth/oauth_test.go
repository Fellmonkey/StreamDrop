package oauth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func testManager(t *testing.T, tokenURL string) (*Manager, *Registry) {
	t.Helper()
	reg := NewRegistry()
	// Give the test provider a deterministic client and endpoint.
	for i := range reg.providers {
		if reg.providers[i].ID == "yandex" {
			reg.providers[i].ClientID = "test-client"
			reg.providers[i].ClientSecret = "test-secret"
			reg.providers[i].NeedsClientID = false
			if tokenURL != "" {
				reg.providers[i].TokenURL = tokenURL
			}
		}
	}
	// Port 0 binds an ephemeral loopback port so parallel tests never fight
	// over the fixed production callback port.
	m := NewManager(reg, 0, nil)
	t.Cleanup(func() { m.Shutdown(context.Background()) })
	return m, reg
}

func TestAuthorizeURLContainsPKCEAndState(t *testing.T) {
	m, _ := testManager(t, "")
	view, authURL, err := m.Start(context.Background(), "yandex", "", "")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if view.Status != SessionPending {
		t.Fatalf("status=%s", view.Status)
	}
	u, err := url.Parse(authURL)
	if err != nil {
		t.Fatalf("auth url: %v", err)
	}
	q := u.Query()
	if q.Get("client_id") != "test-client" {
		t.Fatalf("client_id=%q", q.Get("client_id"))
	}
	if q.Get("response_type") != "code" {
		t.Fatalf("response_type=%q", q.Get("response_type"))
	}
	if q.Get("code_challenge_method") != "S256" || q.Get("code_challenge") == "" {
		t.Fatalf("pkce missing: %v", q)
	}
	if q.Get("state") == "" {
		t.Fatal("state missing")
	}
	if u.Host != "oauth.yandex.ru" {
		t.Fatalf("host=%q", u.Host)
	}
}

func TestCompleteExchangesCode(t *testing.T) {
	var gotForm url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotForm = r.PostForm
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "access", "refresh_token": "refresh",
			"token_type": "bearer", "expires_in": 3600, "scope": "disk",
		})
	}))
	defer srv.Close()

	var stored TokenSet
	m, _ := testManager(t, srv.URL)
	m.onTokens = func(ctx context.Context, providerID, clientID, clientSecret string, tok TokenSet) error {
		stored = tok
		return nil
	}
	_, authURL, err := m.Start(context.Background(), "yandex", "", "")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	u, _ := url.Parse(authURL)
	state := u.Query().Get("state")

	view, err := m.Complete(context.Background(), state, "the-code", "")
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if view.Status != SessionDone {
		t.Fatalf("status=%s err=%s", view.Status, view.ErrorCode)
	}
	if stored.AccessToken != "access" || stored.RefreshToken != "refresh" {
		t.Fatalf("tokens=%+v", stored)
	}
	if gotForm.Get("grant_type") != "authorization_code" || gotForm.Get("code") != "the-code" {
		t.Fatalf("form=%v", gotForm)
	}
	if gotForm.Get("code_verifier") == "" {
		t.Fatal("pkce verifier not sent")
	}
	if gotForm.Get("client_secret") != "test-secret" {
		t.Fatalf("client_secret=%q", gotForm.Get("client_secret"))
	}
}

func TestStateIsSingleUse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "a"})
	}))
	defer srv.Close()
	m, _ := testManager(t, srv.URL)
	_, authURL, err := m.Start(context.Background(), "yandex", "", "")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	state, _ := url.Parse(authURL)
	st := state.Query().Get("state")
	if _, err := m.Complete(context.Background(), st, "c", ""); err != nil {
		t.Fatalf("first complete: %v", err)
	}
	if _, err := m.Complete(context.Background(), st, "c", ""); err == nil {
		t.Fatal("second completion must be rejected")
	}
}

func TestUnknownStateRejected(t *testing.T) {
	m, _ := testManager(t, "")
	if _, err := m.Complete(context.Background(), "bogus", "code", ""); err == nil {
		t.Fatal("expected rejection")
	}
}

func TestProviderErrorMarksSessionFailed(t *testing.T) {
	m, _ := testManager(t, "")
	view, authURL, err := m.Start(context.Background(), "yandex", "", "")
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	u, _ := url.Parse(authURL)
	_, err = m.Complete(context.Background(), u.Query().Get("state"), "", "access_denied")
	if err == nil {
		t.Fatal("expected provider error")
	}
	got, _ := m.GetSession(view.ID)
	if got.Status != SessionFailed {
		t.Fatalf("status=%s", got.Status)
	}
	if !strings.Contains(got.ErrorCode, "auth_rejected") {
		t.Fatalf("code=%q", got.ErrorCode)
	}
}

func TestNeedsClientID(t *testing.T) {
	reg := NewRegistry()
	m := NewManager(reg, 0, nil)
	t.Cleanup(func() { m.Shutdown(context.Background()) })
	if _, _, err := m.Start(context.Background(), "drive", "", ""); err == nil {
		t.Fatal("drive without client id must be rejected")
	}
}

func TestTokenEndpointFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
	}))
	defer srv.Close()
	m, _ := testManager(t, srv.URL)
	_, authURL, _ := m.Start(context.Background(), "yandex", "", "")
	u, _ := url.Parse(authURL)
	if _, err := m.Complete(context.Background(), u.Query().Get("state"), "bad", ""); err == nil {
		t.Fatal("expected exchange failure")
	}
}

func TestRefresh(t *testing.T) {
	var form url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		form = r.PostForm
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "new-access", "expires_in": 60})
	}))
	defer srv.Close()
	m, _ := testManager(t, srv.URL)
	tok, err := m.Refresh(context.Background(), "yandex", "", "", "refresh-token")
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if tok.AccessToken != "new-access" {
		t.Fatalf("token=%+v", tok)
	}
	if form.Get("grant_type") != "refresh_token" || form.Get("refresh_token") != "refresh-token" {
		t.Fatalf("form=%v", form)
	}
}

func TestCodeChallengeIsS256(t *testing.T) {
	// RFC 7636 example verifier.
	if got := CodeChallenge("dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk"); got != "E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM" {
		t.Fatalf("challenge=%q", got)
	}
}
