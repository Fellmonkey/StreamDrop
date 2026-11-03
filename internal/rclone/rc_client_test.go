package rclone

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func mockDaemon(t *testing.T, handler func(method string, params map[string]any, w http.ResponseWriter)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var params map[string]any
		_ = json.NewDecoder(r.Body).Decode(&params)
		method := strings.TrimPrefix(r.URL.Path, "/")
		handler(method, params, w)
	}))
}

func TestListDirsUsesDirsOnlyAndMaxDepth(t *testing.T) {
	var gotParams map[string]any
	srv := mockDaemon(t, func(method string, params map[string]any, w http.ResponseWriter) {
		if method != "operations/list" {
			t.Fatalf("method=%s", method)
		}
		gotParams = params
		_ = json.NewEncoder(w).Encode(map[string]any{
			"list": []map[string]any{
				{"Path": "photos", "Name": "photos", "IsDir": true},
				{"Path": "photos/a.jpg", "Name": "a.jpg", "IsDir": false, "Size": 123},
			},
		})
	})
	defer srv.Close()

	c := NewClient(srv.URL)
	dirs, err := c.ListDirs(context.Background(), "sd_test:/")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(dirs) != 1 || dirs[0].Name != "photos" {
		t.Fatalf("dirs=%+v", dirs)
	}
	opt, _ := gotParams["opt"].(map[string]any)
	if opt["dirsOnly"] != true {
		t.Fatalf("dirsOnly not sent: %v", gotParams)
	}
	if opt["recurse"] != false {
		t.Fatalf("recurse must be false: %v", gotParams)
	}
	// operations/list requires fs and remote as separate keys; remote must be
	// relative (no leading slash) or empty for the root.
	if gotParams["fs"] != "sd_test:" {
		t.Fatalf("fs not sent: %v", gotParams)
	}
	if gotParams["remote"] != "" {
		t.Fatalf("remote not sent: %v", gotParams)
	}
	// The RC _filter parameter expects a JSON object, not a string; a bare
	// "max-depth=1" string makes rclone respond 400. Single-level listing is
	// achieved with dirsOnly + recurse=false, so no _filter must be sent.
	if _, ok := gotParams["_filter"]; ok {
		t.Fatalf("_filter must not be sent: %v", gotParams)
	}
}

func TestExplorerFiltersByName(t *testing.T) {
	srv := mockDaemon(t, func(method string, params map[string]any, w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"list": []map[string]any{
				{"Name": "Photos", "IsDir": true},
				{"Name": "Documents", "IsDir": true},
				{"Name": "photos-2024", "IsDir": true},
			},
		})
	})
	defer srv.Close()

	e := &Explorer{RC: NewClient(srv.URL)}
	dirs, err := e.List(context.Background(), "sd_test", "/backup", "photo")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(dirs) != 2 {
		t.Fatalf("dirs=%+v", dirs)
	}
	// The explorer sorts case-insensitively, so "Photos" precedes
	// "photos-2024" (a prefix sorts first).
	if dirs[0].Name != "Photos" || dirs[1].Name != "photos-2024" {
		t.Fatalf("order=%+v", dirs)
	}
}

func TestQuotaAndLatency(t *testing.T) {
	srv := mockDaemon(t, func(method string, params map[string]any, w http.ResponseWriter) {
		if method != "operations/about" {
			t.Fatalf("method=%s", method)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"total": 1000, "used": 400, "free": 600, "trashed": 10})
	})
	defer srv.Close()

	p := &Provisioner{RC: NewClient(srv.URL)}
	q, err := p.Quota(context.Background(), "sd_test:")
	if err != nil {
		t.Fatalf("quota: %v", err)
	}
	if q.Total != 1000 || q.Used != 400 || q.Free != 600 {
		t.Fatalf("quota=%+v", q)
	}
	if _, err := p.MeasureLatency(context.Background(), "sd_test:"); err != nil {
		t.Fatalf("latency: %v", err)
	}
}

func TestConfigCreateSendsParameters(t *testing.T) {
	var got map[string]any
	srv := mockDaemon(t, func(method string, params map[string]any, w http.ResponseWriter) {
		got = params
		_ = json.NewEncoder(w).Encode(map[string]any{})
	})
	defer srv.Close()

	c := NewClient(srv.URL)
	err := c.ConfigCreate(context.Background(), "sd_x", "webdav", map[string]string{"url": "https://example.com"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if got["name"] != "sd_x" || got["type"] != "webdav" {
		t.Fatalf("params=%v", got)
	}
	params, ok := got["parameters"].(map[string]any)
	if !ok || params["url"] != "https://example.com" {
		t.Fatalf("parameters=%v", got["parameters"])
	}
}

func TestMkdirAndDelete(t *testing.T) {
	var methods []string
	srv := mockDaemon(t, func(method string, params map[string]any, w http.ResponseWriter) {
		methods = append(methods, method)
		_ = json.NewEncoder(w).Encode(map[string]any{})
	})
	defer srv.Close()

	e := &Explorer{RC: NewClient(srv.URL)}
	if err := e.Mkdir(context.Background(), "sd_test", "/a", "b"); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	cleaner := &Cleaner{RC: NewClient(srv.URL)}
	if err := cleaner.DeleteObject(context.Background(), "sd_test", "/a/b", "file.iso"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if len(methods) != 2 || methods[0] != "operations/mkdir" || methods[1] != "operations/deletefile" {
		t.Fatalf("methods=%v", methods)
	}
}

func TestRCErrorFromBody(t *testing.T) {
	srv := mockDaemon(t, func(method string, params map[string]any, w http.ResponseWriter) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"error":"directory not found","status":500}`))
	})
	defer srv.Close()

	c := NewClient(srv.URL)
	if _, err := c.ListDirs(context.Background(), "sd_missing:/"); err == nil {
		t.Fatal("expected error")
	} else if !strings.Contains(err.Error(), "directory not found") {
		t.Fatalf("err=%v", err)
	}
}

func TestBuildParameters(t *testing.T) {
	typ, params, err := BuildParameters("mailru", map[string]string{"user": "u", "pass": "p"})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if typ != "webdav" || params["url"] != "https://webdav.cloud.mail.ru" || params["user"] != "u" {
		t.Fatalf("typ=%s params=%v", typ, params)
	}

	typ, params, err = BuildParameters("s3", map[string]string{
		"access_key_id": "AK", "secret_access_key": "SK", "endpoint": "https://s3.example.com", "region": "eu",
	})
	if err != nil || typ != "s3" || params["access_key_id"] != "AK" || params["endpoint"] != "https://s3.example.com" {
		t.Fatalf("s3 => %s %v %v", typ, params, err)
	}

	if _, _, err := BuildParameters("drive", map[string]string{}); err == nil {
		t.Fatal("drive without token must fail")
	}
	if _, _, err := BuildParameters("unknown", nil); err == nil {
		t.Fatal("unknown provider must fail")
	}
}

func TestJoinPath(t *testing.T) {
	cases := [][3]string{
		{"/iso/linux/", "file.iso", "iso/linux/file.iso"},
		{"", "file.iso", "file.iso"},
		{"/a", "", "a"},
	}
	for _, c := range cases {
		if got := JoinPath(c[0], c[1]); got != c[2] {
			t.Fatalf("JoinPath(%q,%q)=%q want %q", c[0], c[1], got, c[2])
		}
	}
}
