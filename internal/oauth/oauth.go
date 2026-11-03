// Package oauth implements the provider authorization flow: one-time sessions
// with state and PKCE, a fixed loopback callback port, token exchange, refresh
// and revocation (back_spec.md §3.7).
package oauth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"streamdrop/internal/apierr"
)

// CallbackPort is the fixed loopback port providers redirect to. It does not
// depend on the UI port, so changing 8085 -> 8086 never breaks authorization.
const CallbackPort = 35357

// CallbackPath is the callback route.
const CallbackPath = "/api/v1/oauth/callback"

// sessionTTL bounds how long a started authorization stays valid.
const sessionTTL = 10 * time.Minute

// TokenSet is a provider token response.
type TokenSet struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
	Scope        string `json:"scope"`
}

// OnTokens is called after a successful exchange so the application can persist
// encrypted tokens and materialise the rclone remote.
type OnTokens func(ctx context.Context, providerID string, tok TokenSet) error

// SessionState is the lifecycle state of an authorization session.
type SessionState string

const (
	SessionPending SessionState = "pending"
	SessionDone    SessionState = "done"
	SessionFailed  SessionState = "failed"
)

// Session is a one-time authorization context.
type Session struct {
	ID           string
	ProviderID   string
	State        string
	CodeVerifier string
	RedirectURI  string
	ClientID     string
	ClientSecret string
	ExpiresAt    time.Time
	Provider     Provider

	Status    SessionState
	ErrorCode string
	RemoteID  string
	Token     TokenSet
}

// SessionView is the API projection of a session.
type SessionView struct {
	ID         string       `json:"id"`
	ProviderID string       `json:"provider_id"`
	Status     SessionState `json:"status"`
	RemoteID   string       `json:"remote_id,omitempty"`
	ErrorCode  string       `json:"error_code,omitempty"`
	ExpiresAt  time.Time    `json:"expires_at"`
}

// Manager owns the sessions and the callback server.
type Manager struct {
	mu           sync.Mutex
	sessions     map[string]*Session
	byState      map[string]*Session
	registry     *Registry
	http         *http.Client
	redirectURI  string
	callbackPort int
	onTokens     OnTokens
	server       *http.Server
	serverErr    error
	now          func() time.Time
}

// NewManager builds an OAuth manager.
//
// callbackPort 0 binds an ephemeral loopback port (used by tests); a negative
// value selects the standard CallbackPort. Production always passes the
// configured fixed port so the registered redirect URI stays stable.
func NewManager(reg *Registry, callbackPort int, onTokens OnTokens) *Manager {
	if callbackPort < 0 {
		callbackPort = CallbackPort
	}
	return &Manager{
		sessions:     map[string]*Session{},
		byState:      map[string]*Session{},
		registry:     reg,
		http:         &http.Client{Timeout: 30 * time.Second},
		redirectURI:  fmt.Sprintf("http://127.0.0.1:%d%s", callbackPort, CallbackPath),
		callbackPort: callbackPort,
		onTokens:     onTokens,
		now:          time.Now,
	}
}

// Providers returns the provider list for the wizard.
func (m *Manager) Providers() []ProviderView { return m.registry.Views() }

// RedirectURI returns the registered loopback redirect URI.
func (m *Manager) RedirectURI() string { return m.redirectURI }

// Registry exposes the provider registry (used by tests and handlers).
func (m *Manager) Registry() *Registry { return m.registry }

// Start begins an authorization session and returns its id plus the URL the
// browser must open.
func (m *Manager) Start(ctx context.Context, providerID, clientID, clientSecret string) (SessionView, string, error) {
	provider, ok := m.registry.Get(providerID)
	if !ok {
		return SessionView{}, "", apierr.New(apierr.CodeValidationFailed, map[string]any{"field": "provider"})
	}
	if clientID != "" || clientSecret != "" {
		m.registry.SetCredentials(providerID, clientID, clientSecret)
	}
	if clientID == "" {
		clientID = provider.ClientID
	}
	if clientSecret == "" {
		clientSecret = provider.ClientSecret
	}
	if provider.NeedsClientID && clientID == "" {
		return SessionView{}, "", apierr.New(apierr.CodeAuthOAuthRequired, map[string]any{"provider": providerID, "reason": "client_id"})
	}

	if err := m.ensureCallbackServer(); err != nil {
		return SessionView{}, "", err
	}

	state, err := randomToken(32)
	if err != nil {
		return SessionView{}, "", apierr.Wrap(apierr.CodeInternalError, err, nil)
	}
	verifier, err := randomToken(48)
	if err != nil {
		return SessionView{}, "", apierr.Wrap(apierr.CodeInternalError, err, nil)
	}
	sessionID, err := randomToken(18)
	if err != nil {
		return SessionView{}, "", apierr.Wrap(apierr.CodeInternalError, err, nil)
	}

	s := &Session{
		ID:           sessionID,
		ProviderID:   providerID,
		State:        state,
		CodeVerifier: verifier,
		RedirectURI:  m.redirectURI,
		ClientID:     clientID,
		ClientSecret: clientSecret,
		ExpiresAt:    m.now().Add(sessionTTL),
		Provider:     provider,
		Status:       SessionPending,
	}

	m.mu.Lock()
	m.sessions[sessionID] = s
	m.byState[state] = s
	m.cleanupLocked()
	m.mu.Unlock()

	authURL, err := m.authorizeURL(s)
	if err != nil {
		return SessionView{}, "", err
	}
	return s.view(), authURL, nil
}

func (m *Manager) authorizeURL(s *Session) (string, error) {
	u, err := url.Parse(s.Provider.AuthURL)
	if err != nil {
		return "", apierr.Wrap(apierr.CodeInternalError, err, nil)
	}
	q := u.Query()
	q.Set("response_type", "code")
	q.Set("client_id", s.ClientID)
	q.Set("redirect_uri", s.RedirectURI)
	q.Set("state", s.State)
	q.Set("code_challenge", CodeChallenge(s.CodeVerifier))
	q.Set("code_challenge_method", "S256")
	if len(s.Provider.Scopes) > 0 {
		q.Set("scope", strings.Join(s.Provider.Scopes, " "))
	}
	for k, v := range s.Provider.ExtraParams {
		q.Set(k, v)
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// GetSession returns the state of an authorization session.
func (m *Manager) GetSession(id string) (SessionView, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[id]
	if !ok {
		return SessionView{}, apierr.New(apierr.CodeValidationFailed, map[string]any{"field": "session"})
	}
	if s.Status == SessionPending && m.now().After(s.ExpiresAt) {
		s.Status = SessionFailed
		s.ErrorCode = string(apierr.CodeAuthTokenExpired)
	}
	return s.view(), nil
}

// SetSessionRemote records which connection a session produced.
func (m *Manager) SetSessionRemote(sessionID, remoteID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.sessions[sessionID]; ok {
		s.RemoteID = remoteID
	}
}

// Complete handles the provider redirect: it validates state, exchanges the
// code and stores the result.
func (m *Manager) Complete(ctx context.Context, state, code, providerError string) (SessionView, error) {
	m.mu.Lock()
	s, ok := m.byState[state]
	if !ok {
		m.mu.Unlock()
		return SessionView{}, apierr.New(apierr.CodeAuthRejected, map[string]any{"reason": "state"})
	}
	if s.Status != SessionPending {
		m.mu.Unlock()
		return s.view(), apierr.New(apierr.CodeAuthRejected, map[string]any{"reason": "state_used"})
	}
	if m.now().After(s.ExpiresAt) {
		s.Status = SessionFailed
		s.ErrorCode = string(apierr.CodeAuthTokenExpired)
		m.mu.Unlock()
		return s.view(), apierr.New(apierr.CodeAuthTokenExpired, nil)
	}
	// State is single-use regardless of the outcome.
	s.State = ""
	delete(m.byState, state)
	m.mu.Unlock()

	if providerError != "" {
		m.fail(s, string(apierr.CodeAuthRejected))
		return s.view(), apierr.New(apierr.CodeAuthRejected, map[string]any{"reason": providerError})
	}
	if code == "" {
		m.fail(s, string(apierr.CodeAuthRejected))
		return s.view(), apierr.New(apierr.CodeAuthRejected, map[string]any{"reason": "code"})
	}

	tokens, err := m.exchange(ctx, s, code)
	if err != nil {
		m.fail(s, string(apierr.CodeAuthRejected))
		return s.view(), err
	}

	if m.onTokens != nil {
		if err := m.onTokens(ctx, s.ProviderID, tokens); err != nil {
			m.fail(s, string(apierr.CodeInternalError))
			return s.view(), err
		}
	}

	m.mu.Lock()
	s.Token = tokens
	s.Status = SessionDone
	s.CodeVerifier = ""
	m.mu.Unlock()
	return s.view(), nil
}

func (m *Manager) fail(s *Session, code string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s.Status = SessionFailed
	s.ErrorCode = code
	s.CodeVerifier = ""
}

func (m *Manager) exchange(ctx context.Context, s *Session, code string) (TokenSet, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", s.RedirectURI)
	form.Set("client_id", s.ClientID)
	form.Set("code_verifier", s.CodeVerifier)
	if s.ClientSecret != "" {
		form.Set("client_secret", s.ClientSecret)
	}
	return m.tokenRequest(ctx, s.Provider.TokenURL, form)
}

// Refresh exchanges a refresh token for a new access token.
func (m *Manager) Refresh(ctx context.Context, providerID, clientSecret, refreshToken string) (TokenSet, error) {
	provider, ok := m.registry.Get(providerID)
	if !ok {
		return TokenSet{}, apierr.New(apierr.CodeValidationFailed, map[string]any{"field": "provider"})
	}
	if clientSecret == "" {
		clientSecret = provider.ClientSecret
	}
	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", refreshToken)
	form.Set("client_id", provider.ClientID)
	if clientSecret != "" {
		form.Set("client_secret", clientSecret)
	}
	return m.tokenRequest(ctx, provider.TokenURL, form)
}

func (m *Manager) tokenRequest(ctx context.Context, endpoint string, form url.Values) (TokenSet, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return TokenSet{}, apierr.Wrap(apierr.CodeInternalError, err, nil)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := m.http.Do(req)
	if err != nil {
		return TokenSet{}, apierr.Wrap(apierr.CodeSourceUnreachable, err, map[string]any{"provider": endpoint})
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	if resp.StatusCode >= 400 {
		return TokenSet{}, apierr.New(apierr.CodeAuthRejected, map[string]any{
			"status_code": resp.StatusCode,
		}).WithDetail(string(body))
	}
	var tok TokenSet
	if err := json.Unmarshal(body, &tok); err != nil {
		return TokenSet{}, apierr.Wrap(apierr.CodeAuthRejected, err, nil)
	}
	if tok.AccessToken == "" {
		return TokenSet{}, apierr.New(apierr.CodeAuthRejected, map[string]any{"reason": "empty_token"}).WithDetail(string(body))
	}
	if tok.TokenType == "" {
		tok.TokenType = "Bearer"
	}
	return tok, nil
}

// Shutdown stops the callback server.
func (m *Manager) Shutdown(ctx context.Context) {
	m.mu.Lock()
	srv := m.server
	m.server = nil
	m.mu.Unlock()
	if srv != nil {
		_ = srv.Shutdown(ctx)
	}
}

func (m *Manager) cleanupLocked() {
	now := m.now()
	for id, s := range m.sessions {
		if s.Status != SessionPending && now.Sub(s.ExpiresAt) > time.Hour {
			delete(m.sessions, id)
		}
	}
}

func (s *Session) view() SessionView {
	return SessionView{
		ID: s.ID, ProviderID: s.ProviderID, Status: s.Status,
		RemoteID: s.RemoteID, ErrorCode: s.ErrorCode, ExpiresAt: s.ExpiresAt,
	}
}

// CodeChallenge computes the S256 PKCE challenge for a verifier.
func CodeChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func randomToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
