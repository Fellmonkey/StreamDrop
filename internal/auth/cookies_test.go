package auth

import (
	"net/http"
	"net/url"
	"testing"
)

func TestHostMatches(t *testing.T) {
	cases := []struct {
		domain, host string
		want         bool
	}{
		{"example.com", "example.com", true},
		{".example.com", "example.com", true},
		{"example.com", "cdn.example.com", true},
		{"cdn.example.com", "example.com", false},
		{"other.com", "example.com", false},
		{"", "example.com", false},
	}
	for _, c := range cases {
		if got := hostMatches(c.domain, c.host); got != c.want {
			t.Fatalf("%q/%q => %v, want %v", c.domain, c.host, got, c.want)
		}
	}
}

func TestParseNetscapeCookies(t *testing.T) {
	content := `# Netscape HTTP Cookie File
# comment line
.example.com	TRUE	/	FALSE	0	sid	abc123
#HttpOnly_.example.com	TRUE	/	FALSE	0	tok	secret
other.org	TRUE	/	FALSE	0	other	nope
malformed line
`
	got, err := ParseNetscapeCookies(content, "cdn.example.com")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	want := "sid=abc123; tok=secret"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestCookieDomains(t *testing.T) {
	content := "# comment\n.example.com\tTRUE\t/\tFALSE\t0\ta\t1\nother.org\tTRUE\t/\tFALSE\t0\tb\t2\n"
	domains := CookieDomains(content)
	if len(domains) != 2 || domains[0] != "example.com" || domains[1] != "other.org" {
		t.Fatalf("domains=%v", domains)
	}
}

func TestHeaderSetApply(t *testing.T) {
	h := HeaderSet{
		Cookie:    "sid=1",
		UserAgent: "UA",
		Referer:   "https://example.com/",
		Extra:     map[string]string{"X-Test": "yes"},
	}
	req, _ := http.NewRequest("GET", "https://example.com/file", nil)
	h.Apply(req)
	if req.Header.Get("Cookie") != "sid=1" || req.Header.Get("User-Agent") != "UA" ||
		req.Header.Get("Referer") != "https://example.com/" || req.Header.Get("X-Test") != "yes" {
		t.Fatalf("headers=%v", req.Header)
	}
	m := h.Map()
	if m["Cookie"] != "sid=1" || m["X-Test"] != "yes" {
		t.Fatalf("map=%v", m)
	}
}

func TestRefererForSource(t *testing.T) {
	if got := RefererForSource("https://cdn.example.com/a/b.zip"); got != "https://cdn.example.com/" {
		t.Fatalf("got %q", got)
	}
	if got := RefererForSource("not a url"); got != "" {
		t.Fatalf("got %q", got)
	}
	// Ensure url parsing stays consistent for the helper.
	u, _ := url.Parse("https://example.com/x")
	if u.Host != "example.com" {
		t.Fatal("sanity")
	}
}
