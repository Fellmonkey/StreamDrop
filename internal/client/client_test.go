package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"streamdrop/internal/apierr"
)

func writeDaemonFixture(t *testing.T, dir string, port int, token string) {
	t.Helper()
	runDir := filepath.Join(dir, "run")
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatal(err)
	}
	df := DaemonFile{PID: 123, Port: port, Token: token, StartedAt: time.Now(), Version: "1.0.0"}
	raw, _ := json.Marshal(df)
	if err := os.WriteFile(filepath.Join(runDir, "daemon.json"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestDiscover(t *testing.T) {
	dir := t.TempDir()
	writeDaemonFixture(t, dir, 8085, "tok123")

	ep, err := Discover(dir)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if ep.BaseURL != "http://127.0.0.1:8085" {
		t.Errorf("BaseURL = %q", ep.BaseURL)
	}
	if ep.Token != "tok123" {
		t.Errorf("Token = %q", ep.Token)
	}
}

func TestDiscoverMissing(t *testing.T) {
	if _, err := Discover(t.TempDir()); err == nil {
		t.Fatal("expected error for missing daemon file")
	}
}

func TestDiscoverEnvOverrides(t *testing.T) {
	dir := t.TempDir()
	writeDaemonFixture(t, dir, 8085, "tok123")
	t.Setenv("STREAMDROP_PORT", "9090")
	t.Setenv("STREAMDROP_TOKEN", "envtok")

	ep, err := Discover(dir)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if want := "http://127.0.0.1:9090"; ep.BaseURL != want {
		t.Errorf("BaseURL = %q, want %q", ep.BaseURL, want)
	}
	if ep.Token != "envtok" {
		t.Errorf("Token = %q", ep.Token)
	}
}

func TestAPIErrorMapping(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apierr.Write(w, apierr.New(apierr.CodeSourceUnreachable, map[string]any{"status_code": 404}))
	}))
	defer srv.Close()

	c := New(Endpoint{BaseURL: srv.URL, Token: "tok"})
	_, err := c.Status(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if apiErr.Code != apierr.CodeSourceUnreachable {
		t.Errorf("Code = %q", apiErr.Code)
	}
	if apiErr.StatusCode != http.StatusBadGateway {
		t.Errorf("StatusCode = %d", apiErr.StatusCode)
	}
	if apiErr.Kind != apierr.KindNetwork {
		t.Errorf("Kind = %q", apiErr.Kind)
	}
	if !apiErr.IsCode(apierr.CodeSourceUnreachable) {
		t.Error("IsCode failed")
	}
}

func TestTransportError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close() // now unreachable

	c := New(Endpoint{BaseURL: url, Token: "tok"})
	_, err := c.Status(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if _, ok := err.(*TransportError); !ok {
		t.Fatalf("expected *TransportError, got %T", err)
	}
}

func TestStatusRoundTrip(t *testing.T) {
	var gotToken string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotToken = r.Header.Get("X-StreamDrop-Token")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"port":8085,"version":"1.0.0","engine_ready":true,"active":2,"components_ok":3,"components_total":3}`))
	}))
	defer srv.Close()

	c := New(Endpoint{BaseURL: srv.URL, Token: "tok"})
	st, err := c.Status(context.Background())
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if gotToken != "tok" {
		t.Errorf("token header = %q", gotToken)
	}
	if !st.EngineReady || st.Active != 2 || st.ComponentsTotal != 3 {
		t.Errorf("unexpected status view: %+v", st)
	}
}
