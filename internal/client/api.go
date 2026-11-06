package client

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"streamdrop/internal/apierr"
	"streamdrop/internal/auth"
	"streamdrop/internal/deps"
	"streamdrop/internal/engine"
	"streamdrop/internal/rclone"
	"streamdrop/internal/store"
	"streamdrop/internal/transfer"
	"streamdrop/internal/update"
)

// ---------------------------------------------------------------------------
// Status / stats / templates
// ---------------------------------------------------------------------------

// StatusView is GET /status.
type StatusView struct {
	Port            int       `json:"port"`
	Version         string    `json:"version"`
	EngineReady     bool      `json:"engine_ready"`
	DaemonReady     bool      `json:"daemon_ready"`
	Active          int       `json:"active"`
	Onboarded       bool      `json:"onboarded"`
	UptimeSeconds   int64     `json:"uptime_seconds"`
	ComponentsOK    int       `json:"components_ok"`
	ComponentsTotal int       `json:"components_total"`
	WSClients       int       `json:"ws_clients"`
	ServerTime      time.Time `json:"server_time"`
}

// Status returns daemon health.
func (c *Client) Status(ctx context.Context) (StatusView, error) {
	var v StatusView
	return v, c.get(ctx, "/status", "", &v)
}

// StatsView is GET /stats.
type StatsView struct {
	SavedBytes       int64          `json:"saved_bytes"`
	TimeSavedSeconds float64        `json:"time_saved_seconds"`
	Wear             map[string]any `json:"wear"`
}

// Stats returns the wear model and saved-byte totals.
func (c *Client) Stats(ctx context.Context) (StatsView, error) {
	var v StatsView
	return v, c.get(ctx, "/stats", "", &v)
}

// Template is a source preset.
type Template struct {
	ID       string `json:"id"`
	Category string `json:"category"`
	URL      string `json:"url"`
	Example  string `json:"example"`
}

// Templates lists source presets.
func (c *Client) Templates(ctx context.Context) ([]Template, error) {
	var out struct {
		Templates []Template `json:"templates"`
	}
	if err := c.get(ctx, "/templates", "", &out); err != nil {
		return nil, err
	}
	return out.Templates, nil
}

// ---------------------------------------------------------------------------
// Settings
// ---------------------------------------------------------------------------

// Settings returns all non-secret settings.
func (c *Client) Settings(ctx context.Context) (map[string]string, error) {
	var out struct {
		Settings map[string]string `json:"settings"`
	}
	if err := c.get(ctx, "/settings", "", &out); err != nil {
		return nil, err
	}
	return out.Settings, nil
}

// UpdateSettings applies validated settings and returns the new state.
func (c *Client) UpdateSettings(ctx context.Context, patch map[string]any) (map[string]string, error) {
	var out struct {
		Settings map[string]string `json:"settings"`
	}
	if err := c.put(ctx, "/settings", patch, &out); err != nil {
		return nil, err
	}
	return out.Settings, nil
}

// ---------------------------------------------------------------------------
// Inspection and transfers
// ---------------------------------------------------------------------------

// Inspect runs a pre-flight inspection of a source URL.
func (c *Client) Inspect(ctx context.Context, rawURL, profileID string) (*engine.FileMetadata, error) {
	body := map[string]any{"url": rawURL}
	if profileID != "" {
		body["auth_profile_id"] = profileID
	}
	var out struct {
		Metadata *engine.FileMetadata `json:"metadata"`
	}
	if err := c.post(ctx, "/inspect", body, &out); err != nil {
		return nil, err
	}
	return out.Metadata, nil
}

// CreateTransfers queues one URL or a batch. The call is atomic: any failing
// URL rejects the whole request.
func (c *Client) CreateTransfers(ctx context.Context, req transfer.CreateRequest) ([]store.Transfer, error) {
	var out struct {
		Transfers []store.Transfer `json:"transfers"`
	}
	if err := c.post(ctx, "/transfers", req, &out); err != nil {
		return nil, err
	}
	return out.Transfers, nil
}

// TransferPage is the paginated transfer list.
type TransferPage struct {
	Transfers []store.Transfer `json:"transfers"`
	Total     int              `json:"total"`
	Page      int              `json:"page"`
}

// ListTransfers lists transfers with optional status filter and pagination.
func (c *Client) ListTransfers(ctx context.Context, status string, page, limit int) (TransferPage, error) {
	q := ""
	if status != "" {
		q += "status=" + status
	}
	if page > 0 {
		if q != "" {
			q += "&"
		}
		q += "page=" + strconv.Itoa(page)
	}
	if limit > 0 {
		if q != "" {
			q += "&"
		}
		q += "limit=" + strconv.Itoa(limit)
	}
	var p TransferPage
	return p, c.get(ctx, "/transfers", q, &p)
}

// TransferDetail is a transfer plus its recent logs.
type TransferDetail struct {
	Transfer store.Transfer      `json:"transfer"`
	Logs     []store.TransferLog `json:"logs"`
}

// Transfer returns one transfer with its last logs.
func (c *Client) Transfer(ctx context.Context, id string) (TransferDetail, error) {
	var d TransferDetail
	return d, c.get(ctx, "/transfers/"+id, "", &d)
}

// TransferLogs returns raw logs for a transfer.
func (c *Client) TransferLogs(ctx context.Context, id string, tail int, level string) ([]store.TransferLog, error) {
	q := ""
	if tail > 0 {
		q = "tail=" + strconv.Itoa(tail)
	}
	if level != "" {
		if q != "" {
			q += "&"
		}
		q += "level=" + level
	}
	var out struct {
		Logs []store.TransferLog `json:"logs"`
	}
	if err := c.get(ctx, "/transfers/"+id+"/logs", q, &out); err != nil {
		return nil, err
	}
	return out.Logs, nil
}

// PauseTransfer pauses a running transfer.
func (c *Client) PauseTransfer(ctx context.Context, id string) (store.Transfer, error) {
	var t store.Transfer
	return t, c.post(ctx, "/transfers/"+id+"/pause", nil, &t)
}

// ResumeTransfer resumes a paused transfer.
func (c *Client) ResumeTransfer(ctx context.Context, id string) (store.Transfer, error) {
	var t store.Transfer
	return t, c.post(ctx, "/transfers/"+id+"/resume", nil, &t)
}

// AbortTransfer aborts a transfer.
func (c *Client) AbortTransfer(ctx context.Context, id string) (store.Transfer, error) {
	var t store.Transfer
	return t, c.post(ctx, "/transfers/"+id+"/abort", nil, &t)
}

// DeleteTransfer removes a finished transfer from history.
func (c *Client) DeleteTransfer(ctx context.Context, id string) error {
	return c.delete(ctx, "/transfers/"+id, nil)
}

// ---------------------------------------------------------------------------
// Queue
// ---------------------------------------------------------------------------

// QueueState returns the current queue view.
func (c *Client) QueueState(ctx context.Context) (transfer.QueueState, error) {
	var v transfer.QueueState
	return v, c.get(ctx, "/queue", "", &v)
}

// PauseQueue pauses the whole queue.
func (c *Client) PauseQueue(ctx context.Context) (transfer.QueueState, error) {
	var v transfer.QueueState
	return v, c.post(ctx, "/queue/pause", nil, &v)
}

// ResumeQueue resumes the queue.
func (c *Client) ResumeQueue(ctx context.Context) (transfer.QueueState, error) {
	var v transfer.QueueState
	return v, c.post(ctx, "/queue/resume", nil, &v)
}

// ReorderQueue sets the full queue order.
func (c *Client) ReorderQueue(ctx context.Context, ids []string) (transfer.QueueState, error) {
	var v transfer.QueueState
	return v, c.post(ctx, "/queue/reorder", map[string]any{"ids": ids}, &v)
}

// ---------------------------------------------------------------------------
// Remotes and explorer
// ---------------------------------------------------------------------------

// Remotes lists cloud connections (with best-effort quota).
func (c *Client) Remotes(ctx context.Context) ([]store.Remote, error) {
	var out struct {
		Remotes []store.Remote `json:"remotes"`
	}
	if err := c.get(ctx, "/remotes", "", &out); err != nil {
		return nil, err
	}
	return out.Remotes, nil
}

type remoteResponse struct {
	Remote store.Remote     `json:"remote"`
	Error  *apierr.Envelope `json:"error,omitempty"`
}

func (r remoteResponse) apiErr() error {
	if r.Error != nil && r.Error.Error.Code != "" {
		return apiErrorFromPayload(http.StatusOK, r.Error.Error)
	}
	return nil
}

// CreateRemote registers a cloud connection. Options carry credentials; they
// are never returned by the API.
func (c *Client) CreateRemote(ctx context.Context, name, provider, endpoint string, options map[string]string) (store.Remote, error) {
	body := map[string]any{"name": name, "provider": provider}
	if endpoint != "" {
		body["endpoint"] = endpoint
	}
	if options != nil {
		body["options"] = options
	}
	var resp remoteResponse
	if err := c.post(ctx, "/remotes", body, &resp); err != nil {
		return store.Remote{}, err
	}
	if err := resp.apiErr(); err != nil {
		return resp.Remote, err
	}
	return resp.Remote, nil
}

// UpdateRemote patches a remote's mutable fields (empty values are kept).
func (c *Client) UpdateRemote(ctx context.Context, id, name, endpoint string, options map[string]string) (store.Remote, error) {
	body := map[string]any{}
	if name != "" {
		body["name"] = name
	}
	if endpoint != "" {
		body["endpoint"] = endpoint
	}
	if options != nil {
		body["options"] = options
	}
	var resp remoteResponse
	if err := c.put(ctx, "/remotes/"+id, body, &resp); err != nil {
		return store.Remote{}, err
	}
	if err := resp.apiErr(); err != nil {
		return resp.Remote, err
	}
	return resp.Remote, nil
}

// DeleteRemote removes a cloud connection.
func (c *Client) DeleteRemote(ctx context.Context, id string) error {
	return c.delete(ctx, "/remotes/"+id, nil)
}

// TestResult is the outcome of a remote credential check.
type TestResult struct {
	OK    bool         `json:"ok"`
	RTTMS int64        `json:"rtt_ms"`
	Quota *store.Quota `json:"quota,omitempty"`
}

// TestRemote verifies a stored remote.
func (c *Client) TestRemote(ctx context.Context, id string) (TestResult, error) {
	var r TestResult
	if err := c.post(ctx, "/remotes/"+id+"/test", nil, &r); err != nil {
		return r, err
	}
	return r, nil
}

// TestRemoteCredentials verifies credentials without saving them.
func (c *Client) TestRemoteCredentials(ctx context.Context, provider string, options map[string]string) (TestResult, error) {
	var r TestResult
	body := map[string]any{"provider": provider, "options": options}
	if err := c.post(ctx, "/remotes/test", body, &r); err != nil {
		return r, err
	}
	return r, nil
}

// ExplorerView is one explorer listing.
type ExplorerView struct {
	Path        string            `json:"path"`
	Directories []rclone.DirEntry `json:"directories"`
	Quota       *store.Quota      `json:"quota,omitempty"`
}

// ExplorerList lists directories in a remote.
func (c *Client) ExplorerList(ctx context.Context, remoteID, path, q string) (ExplorerView, error) {
	query := "remote_id=" + remoteID
	if path != "" {
		query += "&path=" + path
	}
	if q != "" {
		query += "&q=" + q
	}
	var v ExplorerView
	return v, c.get(ctx, "/explorer", query, &v)
}

// Mkdir creates a directory in a remote and returns its full path.
func (c *Client) Mkdir(ctx context.Context, remoteID, path, name string) (string, error) {
	var out struct {
		Path string `json:"path"`
	}
	body := map[string]any{"remote_id": remoteID, "path": path, "name": name}
	if err := c.post(ctx, "/explorer/mkdir", body, &out); err != nil {
		return "", err
	}
	return out.Path, nil
}

// ---------------------------------------------------------------------------
// Auth profiles
// ---------------------------------------------------------------------------

// ProfilePayload mirrors the profile request schema. Secrets are never
// returned; they are only accepted as input.
type ProfilePayload struct {
	Name       string    `json:"name,omitempty"`
	Mode       string    `json:"mode,omitempty"`
	Kind       string    `json:"kind,omitempty"`
	ProfileDir string    `json:"profile_dir,omitempty"`
	CookieFile string    `json:"cookie_file,omitempty"`
	CookiesTxt string    `json:"cookies_txt,omitempty"`
	Cookie     string    `json:"cookie,omitempty"`
	UserAgent  string    `json:"user_agent,omitempty"`
	Referer    string    `json:"referer,omitempty"`
	Origin     string    `json:"origin,omitempty"`
	Extra      []auth.KV `json:"custom_headers,omitempty"`
}

// Profiles lists auth profiles.
func (c *Client) Profiles(ctx context.Context) ([]store.AuthProfile, error) {
	var out struct {
		Profiles []store.AuthProfile `json:"profiles"`
	}
	if err := c.get(ctx, "/auth/profiles", "", &out); err != nil {
		return nil, err
	}
	return out.Profiles, nil
}

// CreateProfile saves a new auth profile.
func (c *Client) CreateProfile(ctx context.Context, p ProfilePayload) (store.AuthProfile, error) {
	var out struct {
		Profile store.AuthProfile `json:"profile"`
	}
	if err := c.post(ctx, "/auth/profiles", p, &out); err != nil {
		return store.AuthProfile{}, err
	}
	return out.Profile, nil
}

// UpdateProfile patches a profile.
func (c *Client) UpdateProfile(ctx context.Context, id string, p ProfilePayload) (store.AuthProfile, error) {
	var out struct {
		Profile store.AuthProfile `json:"profile"`
	}
	if err := c.put(ctx, "/auth/profiles/"+id, p, &out); err != nil {
		return store.AuthProfile{}, err
	}
	return out.Profile, nil
}

// DeleteProfile removes a profile.
func (c *Client) DeleteProfile(ctx context.Context, id string) error {
	return c.delete(ctx, "/auth/profiles/"+id, nil)
}

// TestProfile verifies a profile against a target URL.
func (c *Client) TestProfile(ctx context.Context, id, targetURL string) (*engine.FileMetadata, error) {
	var out struct {
		OK       bool                 `json:"ok"`
		Metadata *engine.FileMetadata `json:"metadata"`
	}
	if err := c.post(ctx, "/auth/profiles/"+id+"/test", map[string]any{"url": targetURL}, &out); err != nil {
		return nil, err
	}
	return out.Metadata, nil
}

// ---------------------------------------------------------------------------
// OAuth
// ---------------------------------------------------------------------------

// OAuthProviders lists configured provider metadata.
func (c *Client) OAuthProviders(ctx context.Context) ([]map[string]any, string, error) {
	var out struct {
		Providers   []map[string]any `json:"providers"`
		RedirectURI string           `json:"redirect_uri"`
	}
	if err := c.get(ctx, "/oauth/providers", "", &out); err != nil {
		return nil, "", err
	}
	return out.Providers, out.RedirectURI, nil
}

// OAuthAuthorize starts a provider flow and returns the session handle and
// the auth URL the user must open.
func (c *Client) OAuthAuthorize(ctx context.Context, provider, clientID, clientSecret string) (map[string]any, error) {
	body := map[string]any{}
	if clientID != "" {
		body["client_id"] = clientID
	}
	if clientSecret != "" {
		body["client_secret"] = clientSecret
	}
	var out map[string]any
	if err := c.post(ctx, "/oauth/"+provider+"/authorize", body, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// OAuthSession polls the status of an authorization session.
func (c *Client) OAuthSession(ctx context.Context, sessionID string) (map[string]any, error) {
	var out map[string]any
	if err := c.get(ctx, "/oauth/session/"+sessionID, "", &out); err != nil {
		return nil, err
	}
	return out, nil
}

// OAuthSubmitCode completes a verification-code flow.
func (c *Client) OAuthSubmitCode(ctx context.Context, sessionID, code string) (map[string]any, error) {
	var out map[string]any
	if err := c.post(ctx, "/oauth/session/"+sessionID+"/code", map[string]any{"code": code}, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// OAuthRevoke revokes a provider's token and forgets its credentials.
func (c *Client) OAuthRevoke(ctx context.Context, provider string) error {
	return c.delete(ctx, "/oauth/"+provider, nil)
}

// ---------------------------------------------------------------------------
// Dependencies
// ---------------------------------------------------------------------------

// DepsView lists components with installed state.
type DepsView struct {
	Components []deps.Item `json:"components"`
	Installed  int         `json:"installed"`
	Total      int         `json:"total"`
}

// Deps returns the component registry.
func (c *Client) Deps(ctx context.Context) (DepsView, error) {
	var v DepsView
	return v, c.get(ctx, "/deps", "", &v)
}

// InstallDep starts a component install in the background. Progress is
// observed by polling Deps.
func (c *Client) InstallDep(ctx context.Context, id string) error {
	return c.post(ctx, "/deps/"+id+"/install", nil, nil)
}

// RemoveDep uninstalls a component.
func (c *Client) RemoveDep(ctx context.Context, id string) error {
	return c.delete(ctx, "/deps/"+id, nil)
}

// ---------------------------------------------------------------------------
// Notifications / UI
// ---------------------------------------------------------------------------

// TestNotifications sends a test notification through the configured channels.
func (c *Client) TestNotifications(ctx context.Context) error {
	return c.post(ctx, "/notifications/test", nil, nil)
}

// OpenUI opens the UI in the given mode ("browser" or "app").
func (c *Client) OpenUI(ctx context.Context, mode string) error {
	return c.post(ctx, "/ui/open", map[string]any{"mode": mode}, nil)
}

// OpenURL opens an http(s) URL in a browser.
func (c *Client) OpenURL(ctx context.Context, url, browser string) error {
	body := map[string]any{"url": url}
	if browser != "" {
		body["browser"] = browser
	}
	return c.post(ctx, "/ui/open-url", body, nil)
}

// ---------------------------------------------------------------------------
// Updates
// ---------------------------------------------------------------------------

// CheckUpdates asks the daemon for a newer release.
func (c *Client) CheckUpdates(ctx context.Context) (update.Info, error) {
	var info update.Info
	return info, c.get(ctx, "/update", "", &info)
}

// ApplyUpdate downloads and stages the update; it is applied on the next
// launch. Observation: poll CheckUpdates.
func (c *Client) ApplyUpdate(ctx context.Context) error {
	return c.post(ctx, "/update/apply", nil, nil)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------
