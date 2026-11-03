package oauth

import "sync"

// Provider describes an OAuth-enabled cloud provider.
type Provider struct {
	ID            string            `json:"id"`
	Label         string            `json:"label"`
	RcloneType    string            `json:"rclone_type"`
	AuthURL       string            `json:"auth_url"`
	TokenURL      string            `json:"token_url"`
	Scopes        []string          `json:"scopes"`
	ClientID      string            `json:"client_id,omitempty"`
	ClientSecret  string            `json:"-"`
	NeedsClientID bool              `json:"needs_client_id"`
	ExtraParams   map[string]string `json:"-"`
}

// ProviderView is the API form. Client secrets never leave the process.
type ProviderView struct {
	ID            string   `json:"id"`
	Label         string   `json:"label"`
	RcloneType    string   `json:"rclone_type"`
	Scopes        []string `json:"scopes"`
	NeedsClientID bool     `json:"needs_client_id"`
}

// Registry holds the configured providers.
type Registry struct {
	mu        sync.RWMutex
	providers []Provider
}

// NewRegistry returns the built-in provider set.
//
// Google Drive and OneDrive require the user's own OAuth client: no client
// secret can be shipped in an open-source binary. Yandex also asks for one in
// this build, so the wizard shows the client fields for every provider that
// reports needs_client_id.
func NewRegistry() *Registry {
	r := &Registry{}
	r.providers = []Provider{
		{
			ID:            "drive",
			Label:         "Google Drive",
			RcloneType:    "drive",
			AuthURL:       "https://accounts.google.com/o/oauth2/v2/auth",
			TokenURL:      "https://oauth2.googleapis.com/token",
			Scopes:        []string{"https://www.googleapis.com/auth/drive"},
			NeedsClientID: true,
			ExtraParams:   map[string]string{"access_type": "offline", "prompt": "consent"},
		},
		{
			ID:            "onedrive",
			Label:         "Microsoft OneDrive",
			RcloneType:    "onedrive",
			AuthURL:       "https://login.microsoftonline.com/common/oauth2/v2.0/authorize",
			TokenURL:      "https://login.microsoftonline.com/common/oauth2/v2.0/token",
			Scopes:        []string{"Files.ReadWrite", "offline_access"},
			NeedsClientID: true,
			ExtraParams:   map[string]string{"response_mode": "query"},
		},
		{
			ID:            "yandex",
			Label:         "Яндекс.Диск",
			RcloneType:    "yandex",
			AuthURL:       "https://oauth.yandex.ru/authorize",
			TokenURL:      "https://oauth.yandex.ru/token",
			Scopes:        []string{"cloud_api:disk.read", "cloud_api:disk.write", "cloud_api:disk.info"},
			NeedsClientID: true,
		},
	}
	return r
}

// List returns a copy of the provider list.
func (r *Registry) List() []Provider {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Provider, len(r.providers))
	copy(out, r.providers)
	return out
}

// Views returns the API projection.
func (r *Registry) Views() []ProviderView {
	out := []ProviderView{}
	for _, p := range r.List() {
		out = append(out, ProviderView{
			ID: p.ID, Label: p.Label, RcloneType: p.RcloneType,
			Scopes: p.Scopes, NeedsClientID: p.NeedsClientID,
		})
	}
	return out
}

// Get returns one provider.
func (r *Registry) Get(id string) (Provider, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, p := range r.providers {
		if p.ID == id {
			return p, true
		}
	}
	return Provider{}, false
}

// SetCredentials stores a user-supplied OAuth client for a provider and reports
// whether a client id is still required.
func (r *Registry) SetCredentials(id, clientID, clientSecret string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := range r.providers {
		if r.providers[i].ID == id {
			if clientID != "" {
				r.providers[i].ClientID = clientID
			}
			if clientSecret != "" {
				r.providers[i].ClientSecret = clientSecret
			}
			r.providers[i].NeedsClientID = r.providers[i].ClientID == ""
			return true
		}
	}
	return false
}
