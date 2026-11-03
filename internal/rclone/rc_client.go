// Package rclone talks to the local rclone Remote Control daemon: explorer,
// quota, folder creation, remote provisioning and fail-safe cleanup.
package rclone

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client is an rclone RC HTTP client.
type Client struct {
	BaseURL string
	HTTP    *http.Client
}

// NewClient builds a client for http://127.0.0.1:5572.
func NewClient(baseURL string) *Client {
	return &Client{BaseURL: strings.TrimRight(baseURL, "/"), HTTP: &http.Client{Timeout: 60 * time.Second}}
}

// RCError is a daemon-level error response.
type RCError struct {
	Status  int
	Message string
	Method  string
}

func (e *RCError) Error() string {
	return fmt.Sprintf("rclone %s: %s (status %d)", e.Method, e.Message, e.Status)
}

// Call invokes one RC method.
func (c *Client) Call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	body := []byte("{}")
	if params != nil {
		raw, err := json.Marshal(params)
		if err != nil {
			return nil, err
		}
		body = raw
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/"+method, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("rclone %s: %w", method, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, &RCError{Status: resp.StatusCode, Message: strings.TrimSpace(string(raw)), Method: method}
	}
	var probe struct {
		Error  string `json:"error"`
		Status int    `json:"status"`
	}
	if json.Unmarshal(raw, &probe) == nil && probe.Error != "" {
		status := probe.Status
		if status == 0 {
			status = http.StatusBadGateway
		}
		return nil, &RCError{Status: status, Message: probe.Error, Method: method}
	}
	return json.RawMessage(raw), nil
}

// Ping verifies the daemon is alive.
func (c *Client) Ping(ctx context.Context) error {
	_, err := c.Call(ctx, "rc/noop", map[string]any{})
	return err
}

// ListRemotes returns remote names registered in the daemon.
func (c *Client) ListRemotes(ctx context.Context) ([]string, error) {
	raw, err := c.Call(ctx, "config/listremotes", nil)
	if err != nil {
		return nil, err
	}
	var out struct {
		Remotes []string `json:"remotes"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out.Remotes, nil
}

// ConfigCreate creates or replaces a remote in the in-memory configuration.
//
// rclone's RC API expects {"name","type","parameters"}; passing parameters
// flattened would be ignored.
func (c *Client) ConfigCreate(ctx context.Context, name, providerType string, parameters map[string]string) error {
	params := map[string]any{
		"name":       name,
		"type":       providerType,
		"parameters": parameters,
	}
	_, err := c.Call(ctx, "config/create", params)
	return err
}

// RemoteConfig returns an INI block for one remote section from the running
// daemon. The daemon keeps configuration in memory only; separate rclone
// subprocesses (e.g. rcat) need a materialised config file to see a remote.
func (c *Client) RemoteConfig(ctx context.Context, name string) (string, error) {
	raw, err := c.Call(ctx, "config/dump", map[string]any{})
	if err != nil {
		return "", err
	}
	var all map[string]map[string]string
	if err := json.Unmarshal(raw, &all); err != nil {
		return "", err
	}
	sec, ok := all[name]
	if !ok {
		return "", fmt.Errorf("section %q not in daemon config", name)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "[%s]\n", name)
	for k, v := range sec {
		fmt.Fprintf(&b, "%s = %s\n", k, v)
	}
	return b.String(), nil
}

// ConfigUpdate updates an existing remote.
func (c *Client) ConfigUpdate(ctx context.Context, name string, parameters map[string]string) error {
	_, err := c.Call(ctx, "config/update", map[string]any{
		"name":       name,
		"parameters": parameters,
	})
	return err
}

// ConfigDelete removes a remote from the daemon configuration.
func (c *Client) ConfigDelete(ctx context.Context, name string) error {
	_, err := c.Call(ctx, "config/delete", map[string]any{"name": name})
	return err
}

// DirEntry is a directory entry returned by the explorer. The JSON tags are
// the API shape (snake_case), not the rclone RC shape.
type DirEntry struct {
	Path    string    `json:"path"`
	Name    string    `json:"name"`
	Size    int64     `json:"size,omitempty"`
	ModTime time.Time `json:"mod_time,omitempty"`
	IsDir   bool      `json:"is_dir"`
}

// rcListEntry mirrors rclone's RC response, whose fields are PascalCase. It is
// separate from DirEntry so the public API keeps a stable snake_case contract.
type rcListEntry struct {
	Path    string
	Name    string
	Size    int64
	ModTime time.Time
	IsDir   bool
}

// splitRemoteFS splits an rclone filesystem string "name:path" into the fs
// ("name:") and the remote path (relative, no leading slash — rclone RC's
// operations/list and operations/mkdir return empty results or errors when the
// remote carries a leading slash, at least for WebDAV backends).
func splitRemoteFS(fs string) (remote, root string) {
	if i := strings.Index(fs, ":"); i >= 0 {
		root = fs[:i+1]
		remote = fs[i+1:]
	} else {
		root = fs + ":"
		remote = ""
	}
	return strings.Trim(remote, "/"), root
}

// ListDirs lists only the directories at one level. dirsOnly without recurse
// returns the current level, so file metadata never reaches the browser.
func (c *Client) ListDirs(ctx context.Context, fs string) ([]DirEntry, error) {
	remote, root := splitRemoteFS(fs)
	raw, err := c.Call(ctx, "operations/list", map[string]any{
		"fs":     root,
		"remote": remote,
		"opt":    map[string]any{"dirsOnly": true, "recurse": false},
	})
	if err != nil {
		return nil, err
	}
	var out struct {
		List []rcListEntry `json:"list"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	dirs := make([]DirEntry, 0, len(out.List))
	for _, e := range out.List {
		if !e.IsDir {
			continue
		}
		dirs = append(dirs, DirEntry{Path: e.Path, Name: e.Name, Size: e.Size, ModTime: e.ModTime, IsDir: true})
	}
	return dirs, nil
}

// About returns the storage quota.
func (c *Client) About(ctx context.Context, fs string) (Quota, error) {
	raw, err := c.Call(ctx, "operations/about", map[string]any{"fs": fs})
	if err != nil {
		return Quota{}, err
	}
	var q Quota
	if err := json.Unmarshal(raw, &q); err != nil {
		return Quota{}, err
	}
	return q, nil
}

// Mkdir creates a folder.
func (c *Client) Mkdir(ctx context.Context, fs string) error {
	remote, root := splitRemoteFS(fs)
	_, err := c.Call(ctx, "operations/mkdir", map[string]any{"fs": root, "remote": remote})
	return err
}

// DeleteFile removes one object (fail-safe cleanup).
func (c *Client) DeleteFile(ctx context.Context, fs string) error {
	_, err := c.Call(ctx, "operations/deletefile", map[string]any{"fs": fs})
	return err
}

// Quota is a cloud quota snapshot.
type Quota struct {
	Total   int64 `json:"total"`
	Used    int64 `json:"used"`
	Free    int64 `json:"free"`
	Trashed int64 `json:"trashed,omitempty"`
}
