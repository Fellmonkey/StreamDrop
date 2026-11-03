package engine

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestInspectBasicHeaders(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodHead {
			t.Errorf("method=%s, want HEAD", r.Method)
		}
		w.Header().Set("Content-Type", "application/x-iso9660-image")
		w.Header().Set("Content-Length", "2610954240")
		w.Header().Set("Accept-Ranges", "bytes")
		w.Header().Set("Content-Disposition", `attachment; filename="ubuntu.iso"`)
	}))
	defer srv.Close()

	meta, err := Inspect(context.Background(), NewInspectClient(), srv.URL+"/download?id=1", nil)
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if meta.Filename != "ubuntu.iso" {
		t.Fatalf("filename=%q", meta.Filename)
	}
	if meta.SizeBytes != 2610954240 {
		t.Fatalf("size=%d", meta.SizeBytes)
	}
	if !meta.SupportsRange {
		t.Fatal("expected range support")
	}
	if meta.IsHTML || meta.EngineHint != "direct" {
		t.Fatalf("html=%v hint=%q", meta.IsHTML, meta.EngineHint)
	}
}

func TestInspectHTMLDetection(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte("<html></html>"))
	}))
	defer srv.Close()

	meta, err := Inspect(context.Background(), NewInspectClient(), srv.URL+"/page", nil)
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if !meta.IsHTML {
		t.Fatal("expected HTML detection")
	}
	if meta.EngineHint != "yt-dlp" {
		t.Fatalf("hint=%q", meta.EngineHint)
	}
}

func TestInspectRedirectChain(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/a":
			http.Redirect(w, r, srv.URL+"/b", http.StatusMovedPermanently)
		case "/b":
			http.Redirect(w, r, "/c", http.StatusFound) // relative
		case "/c":
			http.Redirect(w, r, srv.URL+"/file", http.StatusTemporaryRedirect)
		case "/file":
			w.Header().Set("Content-Disposition", "attachment; filename=final.bin")
			w.Header().Set("Content-Length", "42")
			w.Header().Set("Content-Type", "application/octet-stream")
		}
	}))
	defer srv.Close()

	meta, err := Inspect(context.Background(), NewInspectClient(), srv.URL+"/a", nil)
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if !strings.HasSuffix(meta.ResolvedURL, "/file") {
		t.Fatalf("resolved=%q", meta.ResolvedURL)
	}
	if meta.Filename != "final.bin" || meta.SizeBytes != 42 {
		t.Fatalf("filename=%q size=%d", meta.Filename, meta.SizeBytes)
	}
}

func TestInspectRedirectLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, r.URL.Path, http.StatusFound)
	}))
	defer srv.Close()

	_, err := Inspect(context.Background(), NewInspectClient(), srv.URL+"/loop", nil)
	if err == nil {
		t.Fatal("expected redirect limit error")
	}
	if !strings.Contains(err.Error(), "inspect_redirect_limit") {
		t.Fatalf("err=%v", err)
	}
}

func TestInspectHeadForbiddenFallsBackToRange(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		if r.Header.Get("Range") != "bytes=0-0" {
			t.Errorf("range=%q", r.Header.Get("Range"))
		}
		w.Header().Set("Content-Range", "bytes 0-0/987654321")
		w.Header().Set("Content-Type", "video/mp4")
		w.Header().Set("Content-Disposition", "attachment; filename=clip.mp4")
		w.WriteHeader(http.StatusPartialContent)
		w.Write([]byte{0})
	}))
	defer srv.Close()

	meta, err := Inspect(context.Background(), NewInspectClient(), srv.URL+"/clip", nil)
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	if meta.SizeBytes != 987654321 {
		t.Fatalf("size=%d (Content-Range must win over Content-Length=1)", meta.SizeBytes)
	}
	if meta.Filename != "clip.mp4" {
		t.Fatalf("filename=%q", meta.Filename)
	}
}

func TestInspectHeadNotImplementedFallback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusNotImplemented)
			return
		}
		w.Header().Set("Content-Length", "100")
		w.WriteHeader(http.StatusOK)
		w.Write(make([]byte, 100))
	}))
	defer srv.Close()

	meta, err := Inspect(context.Background(), NewInspectClient(), srv.URL+"/x.bin", nil)
	if err != nil {
		t.Fatalf("inspect: %v", err)
	}
	// Range ignored -> full response -> Content-Length is the size.
	if meta.SizeBytes != 100 {
		t.Fatalf("size=%d", meta.SizeBytes)
	}
	if meta.Filename != "x.bin" {
		t.Fatalf("filename=%q", meta.Filename)
	}
}

func TestInspectHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "gone", http.StatusNotFound)
	}))
	defer srv.Close()

	_, err := Inspect(context.Background(), NewInspectClient(), srv.URL+"/missing", nil)
	if err == nil || !strings.Contains(err.Error(), "inspect_http_error") {
		t.Fatalf("err=%v", err)
	}
}

func TestInspectAuthRequired(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()

	_, err := Inspect(context.Background(), NewInspectClient(), srv.URL+"/private", nil)
	if err == nil || !strings.Contains(err.Error(), "auth_required") {
		t.Fatalf("err=%v", err)
	}
}

func TestParseContentRangeTotal(t *testing.T) {
	cases := map[string]struct {
		want  int64
		found bool
	}{
		"bytes 0-0/123456": {123456, true},
		"bytes 0-0/*":      {0, false},
		"garbage":          {0, false},
		"bytes 5-10/99":    {99, true},
	}
	for in, want := range cases {
		got, ok := parseContentRangeTotal(in)
		if got != want.want || ok != want.found {
			t.Fatalf("%q => %d,%v want %d,%v", in, got, ok, want.want, want.found)
		}
	}
}

func TestParseContentDisposition(t *testing.T) {
	cases := map[string]string{
		`attachment; filename="report.pdf"`:                                     "report.pdf",
		`attachment; filename=simple.txt`:                                       "simple.txt",
		`attachment; filename*=UTF-8''%D0%98%D0%B2%D0%B0%D0%BD.doc`:             "Иван.doc",
		"attachment; filename*=utf-8''%D0%98%D0%B2%D0%B0%D0%BD.doc":             "Иван.doc",
		`attachment; filename*=UTF-8''%D0%98%D0%B2%D0%B0%D0%BD.doc; filename=x`: "Иван.doc",
		``: "",
	}
	for in, want := range cases {
		if got := parseContentDisposition(in); got != want {
			t.Fatalf("%q => %q, want %q", in, got, want)
		}
	}
}

func TestParseContentDispositionLatin1Transliterates(t *testing.T) {
	got := parseContentDisposition("attachment; filename=\"Документ 1.docx\"")
	if got != "Dokument_1.docx" {
		t.Fatalf("got %q", got)
	}
}

func TestFilenameFromURL(t *testing.T) {
	cases := map[string]string{
		"https://example.com/files/ubuntu.iso":                   "ubuntu.iso",
		"https://example.com/files/%D0%A4%D0%B0%D0%B9%D0%BB.zip": "Файл.zip",
		"https://example.com/":                                   "",
		"https://example.com/a/b/":                               "b",
		"https://example.com/download?file=x.iso":                "download",
	}
	for in, want := range cases {
		if got := filenameFromURL(in); got != want {
			t.Fatalf("%q => %q, want %q", in, got, want)
		}
	}
}

func TestTransliterateASCII(t *testing.T) {
	if got := transliterate("hello world.txt"); got != "hello_world.txt" {
		t.Fatalf("got %q", got)
	}
	if got := transliterate("файл-2.bin"); got != "fayl-2.bin" {
		t.Fatalf("got %q", got)
	}
}
