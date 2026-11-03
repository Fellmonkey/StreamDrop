package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const token = "test-token"

func guarded() http.Handler {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	return LocalGuard{Port: 8085}.Handler(RequireToken(token)(next))
}

func request(method, target, host, origin, tok string) *http.Request {
	r := httptest.NewRequest(method, target, nil)
	r.Host = host
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	if tok != "" {
		r.Header.Set("X-StreamDrop-Token", tok)
	}
	return r
}

func TestValidRequestPasses(t *testing.T) {
	w := httptest.NewRecorder()
	guarded().ServeHTTP(w, request("GET", "http://127.0.0.1:8085/api/v1/status", "127.0.0.1:8085", "http://127.0.0.1:8085", token))
	if w.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
	}
}

func TestMissingTokenRejected(t *testing.T) {
	w := httptest.NewRecorder()
	guarded().ServeHTTP(w, request("POST", "http://127.0.0.1:8085/api/v1/transfers", "127.0.0.1:8085", "", ""))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d", w.Code)
	}
	var env struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil || env.Error.Code != "local_auth_required" {
		t.Fatalf("body=%s", w.Body.String())
	}
}

func TestWrongTokenRejected(t *testing.T) {
	w := httptest.NewRecorder()
	guarded().ServeHTTP(w, request("GET", "http://127.0.0.1:8085/api/v1/status", "127.0.0.1:8085", "", "nope"))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d", w.Code)
	}
}

func TestForeignHostRejected(t *testing.T) {
	// A DNS-rebound host must not pass even with a token.
	w := httptest.NewRecorder()
	guarded().ServeHTTP(w, request("GET", "http://evil.example.com/api/v1/status", "evil.example.com", "", token))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d", w.Code)
	}
}

func TestForeignOriginRejected(t *testing.T) {
	w := httptest.NewRecorder()
	guarded().ServeHTTP(w, request("POST", "http://127.0.0.1:8085/api/v1/transfers", "127.0.0.1:8085", "https://evil.example.com", token))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d", w.Code)
	}
}

func TestCrossSiteFetchRejected(t *testing.T) {
	r := request("POST", "http://127.0.0.1:8085/api/v1/transfers", "127.0.0.1:8085", "", token)
	r.Header.Set("Sec-Fetch-Site", "cross-site")
	w := httptest.NewRecorder()
	guarded().ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d", w.Code)
	}
}

func TestCORSWildcardNeverSent(t *testing.T) {
	handler := CORS(8085)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	w := httptest.NewRecorder()
	r := httptest.NewRequest("OPTIONS", "http://127.0.0.1:8085/api/v1/status", nil)
	r.Header.Set("Origin", "https://evil.example.com")
	handler.ServeHTTP(w, r)
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("foreign origin got CORS header %q", got)
	}

	w2 := httptest.NewRecorder()
	r2 := httptest.NewRequest("OPTIONS", "http://127.0.0.1:8085/api/v1/status", nil)
	r2.Header.Set("Origin", "http://127.0.0.1:8085")
	handler.ServeHTTP(w2, r2)
	if got := w2.Header().Get("Access-Control-Allow-Origin"); got != "http://127.0.0.1:8085" {
		t.Fatalf("local origin CORS=%q", got)
	}
}

func TestIsLoopbackAddr(t *testing.T) {
	if !IsLoopbackAddr("127.0.0.1") || !IsLoopbackAddr("::1") || IsLoopbackAddr("0.0.0.0") {
		t.Fatal("loopback detection is wrong")
	}
}

func TestTokenComparisonIsConstantTimeShape(t *testing.T) {
	// Guards against accidental prefix acceptance.
	w := httptest.NewRecorder()
	guarded().ServeHTTP(w, request("GET", "http://127.0.0.1:8085/x", "127.0.0.1:8085", "", token+"x"))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("prefix token accepted: %d", w.Code)
	}
	if strings.Contains(w.Body.String(), token) {
		t.Fatal("token leaked into response")
	}
}
