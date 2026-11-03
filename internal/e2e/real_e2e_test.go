// Package e2e contains environment-gated end-to-end smoke tests against a
// running StreamDrop instance and real cloud providers.
//
// BACK §8.3: these tests are NOT executed in CI. They require a locally
// running service (default http://127.0.0.1:8085) with rclone installed, and
// real credentials supplied via a .env file in the repository root:
//
//	cp e2e.env.example .env   # then fill in MAILRU_USER / MAILRU_PASS
//
// Without credentials the tests skip with a clear message.
package e2e

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Environment and helpers
// ---------------------------------------------------------------------------

type envMap map[string]string

// loadEnv reads a KEY=VALUE file (e.g. .env) and returns the merged values.
// It looks for the file relative to the current working directory and up to
// two parent directories, so `go test ./internal/e2e` works from the repo
// root and from web/.
func loadEnv(file string) (envMap, error) {
	env := envMap{}
	dir, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	for i := 0; i < 3; i++ {
		path := filepath.Join(dir, file)
		if data, err := os.ReadFile(path); err == nil {
			scanner := bufio.NewScanner(strings.NewReader(string(data)))
			for scanner.Scan() {
				line := strings.TrimSpace(scanner.Text())
				if line == "" || strings.HasPrefix(line, "#") {
					continue
				}
				key, value, ok := strings.Cut(line, "=")
				if !ok {
					continue
				}
				env[strings.TrimSpace(key)] = strings.Trim(strings.TrimSpace(value), `"'`)
			}
			return env, nil
		}
		dir = filepath.Dir(dir)
	}
	return env, nil
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	if env, err := loadEnv(".env"); err == nil {
		if v := env[key]; v != "" {
			return v
		}
	}
	return fallback
}

type apiClient struct {
	base  string
	token string
	hc    *http.Client
}

func newAPIClient(t *testing.T, base string) *apiClient {
	t.Helper()
	c := &apiClient{base: strings.TrimRight(base, "/"), hc: &http.Client{Timeout: 30 * time.Second}}
	c.token = c.fetchToken(t)
	return c
}

var tokenRe = regexp.MustCompile(`<meta name="streamdrop-token" content="([^"]+)"`)

// fetchToken reads the per-run local token from the served index.html, the
// same way the frontend does.
func (c *apiClient) fetchToken(t *testing.T) string {
	t.Helper()
	resp, err := c.hc.Get(c.base + "/")
	if err != nil {
		t.Fatalf("GET %s/: %v", c.base, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read index: %v", err)
	}
	m := tokenRe.FindSubmatch(body)
	if m == nil {
		t.Fatalf("meta[name=streamdrop-token] not found in index.html (is the server running?)")
	}
	return string(m[1])
}

func (c *apiClient) do(t *testing.T, method, path string, body any) (int, []byte) {
	t.Helper()
	var rd io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal %s %s: %v", method, path, err)
		}
		rd = strings.NewReader(string(raw))
	}
	req, err := http.NewRequestWithContext(context.Background(), method, c.base+path, rd)
	if err != nil {
		t.Fatalf("new request %s %s: %v", method, path, err)
	}
	req.Header.Set("X-StreamDrop-Token", c.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.hc.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read %s %s: %v", method, path, err)
	}
	return resp.StatusCode, data
}

func requireOK(t *testing.T, status int, data []byte, what string) {
	t.Helper()
	if status < 200 || status >= 300 {
		t.Fatalf("%s: HTTP %d: %s", what, status, strings.TrimSpace(string(data)))
	}
}

// ---------------------------------------------------------------------------
// REST shapes (subset of front_spec.md §8)
// ---------------------------------------------------------------------------

type statusPayload struct {
	EngineReady bool `json:"engine_ready"`
}

type remotePayload struct {
	Remote struct {
		ID string `json:"id"`
	} `json:"remote"`
	Error *struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	} `json:"error"`
}

type testRemotePayload struct {
	OK    bool `json:"ok"`
	RTTMS int  `json:"rtt_ms"`
}

type explorerPayload struct {
	Path        string `json:"path"`
	Directories []struct {
		Name   string `json:"name"`
		Path   string `json:"path"`
		IsDir  bool   `json:"is_dir"`
	} `json:"directories"`
}

type mkdirPayload struct {
	Path string `json:"path"`
}

// ---------------------------------------------------------------------------
// E2E smoke: Mail.ru via WebDAV
// ---------------------------------------------------------------------------

// TestMailRuSmoke runs the connection -> quota/test -> explorer -> mkdir flow
// against a real Mail.ru account. It requires:
//   - a running StreamDrop on STREAMDROP_URL (default 127.0.0.1:8085),
//   - MAILRU_USER / MAILRU_PASS in .env (see e2e.env.example).
//
// The created folder is intentionally left in place (the API has no delete
// endpoint for directories); remove it manually after a run.
func TestMailRuSmoke(t *testing.T) {
	user := getenv("MAILRU_USER", "")
	pass := getenv("MAILRU_PASS", "")
	mailruURL := getenv("MAILRU_URL", "https://webdav.cloud.mail.ru")
	base := getenv("STREAMDROP_URL", "http://127.0.0.1:8085")

	if user == "" || pass == "" {
		t.Skip("MAILRU_USER/MAILRU_PASS not set — copy e2e.env.example to .env and fill in credentials")
	}

	c := newAPIClient(t, base)

	// 1. Health: engine must be ready (rclone daemon up).
	status, data := c.do(t, "GET", "/api/v1/status", nil)
	requireOK(t, status, data, "status")
	var st statusPayload
	if err := json.Unmarshal(data, &st); err != nil {
		t.Fatalf("status decode: %v", err)
	}
	if !st.EngineReady {
		t.Skip("engine_ready=false — install rclone or start the daemon first")
	}

	suffix := fmt.Sprintf("%d", time.Now().Unix())
	remoteName := "e2e-mailru-" + suffix

	// 2. Create the remote (exactly what AddCloudWizard step 1-4 posts).
	status, data = c.do(t, "POST", "/api/v1/remotes", map[string]any{
		"name":     remoteName,
		"provider": "mailru",
		"endpoint": mailruURL,
		"options": map[string]string{
			"url":  mailruURL,
			"user": user,
			"pass": pass,
		},
	})
	requireOK(t, status, data, "create remote")
	var created remotePayload
	if err := json.Unmarshal(data, &created); err != nil {
		t.Fatalf("remote decode: %v", err)
	}
	if created.Error != nil {
		t.Fatalf("create remote error: %s", created.Error.Error.Code)
	}
	remoteID := created.Remote.ID
	if remoteID == "" {
		t.Fatal("create remote returned empty id")
	}
	t.Cleanup(func() {
		if _, data := c.do(t, "DELETE", "/api/v1/remotes/"+url.PathEscape(remoteID), nil); len(data) == 0 {
			t.Log("cleanup: remote deleted")
		}
	})

	// 3. Test connection: RTT and quota come from operations/about.
	status, data = c.do(t, "POST", "/api/v1/remotes/"+url.PathEscape(remoteID)+"/test", nil)
	requireOK(t, status, data, "test remote")
	var tested testRemotePayload
	if err := json.Unmarshal(data, &tested); err != nil {
		t.Fatalf("test remote decode: %v", err)
	}
	if !tested.OK {
		t.Fatalf("remote test failed: %s", string(data))
	}
	t.Logf("remote test ok, rtt=%dms", tested.RTTMS)

	// 4. Explorer: list root.
	status, data = c.do(t, "GET", "/api/v1/explorer?remote_id="+url.QueryEscape(remoteID)+"&path=%2F&q=", nil)
	requireOK(t, status, data, "explorer root")
	var exp explorerPayload
	if err := json.Unmarshal(data, &exp); err != nil {
		t.Fatalf("explorer decode: %v", err)
	}
	t.Logf("explorer root: %d directories", len(exp.Directories))

	// 5. mkdir: verify the folder appears in the listing.
	folder := "sd-e2e-" + suffix
	status, data = c.do(t, "POST", "/api/v1/explorer/mkdir", map[string]any{
		"remote_id": remoteID,
		"path":      "/",
		"name":      folder,
	})
	requireOK(t, status, data, "mkdir")
	var mk mkdirPayload
	if err := json.Unmarshal(data, &mk); err != nil {
		t.Fatalf("mkdir decode: %v", err)
	}
	if mk.Path == "" {
		t.Fatal("mkdir returned empty path")
	}

	status, data = c.do(t, "GET", "/api/v1/explorer?remote_id="+url.QueryEscape(remoteID)+"&path=%2F&q=", nil)
	requireOK(t, status, data, "explorer after mkdir")
	if err := json.Unmarshal(data, &exp); err != nil {
		t.Fatalf("explorer decode: %v", err)
	}
	found := false
	for _, d := range exp.Directories {
		if d.Name == folder {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("folder %q not found after mkdir", folder)
	}
	t.Logf("e2e mail.ru smoke ok (remote %s, folder %s)", remoteName, folder)
}
