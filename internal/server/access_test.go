package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const testToken = "synthetic-test-credential-0123456789abcdef"

// A nil Store deliberately panics if a denied request reaches a route handler.
func TestAccessDeniesEveryAPISurfaceBeforeStore(t *testing.T) {
	srv, err := NewServerWithOptions(nil, Options{Token: testToken})
	if err != nil {
		t.Fatal(err)
	}
	for _, route := range []string{"GET /tasks", "POST /tasks", "GET /tasks/1", "PUT /tasks/1", "DELETE /tasks/1", "POST /tasks/1/done", "POST /tasks/1/start", "POST /tasks/stop", "GET /tasks/active", "POST /tasks/batch-update", "GET /projects", "POST /projects", "GET /projects/x", "PUT /projects/x", "DELETE /projects/x", "GET /dependencies", "POST /dependencies", "DELETE /dependencies/1/2", "GET /search", "OPTIONS /tasks", "GET /tasks/../projects"} {
		t.Run(route, func(t *testing.T) {
			method, path, _ := strings.Cut(route, " ")
			for _, credential := range []string{"", "Bearer wrong", "Basic " + testToken, "Bearer " + testToken + "extra"} {
				r := httptest.NewRequest(method, "http://localhost"+path, strings.NewReader(`{"title":"must not be written"}`))
				r.Header.Set("Authorization", credential)
				w := httptest.NewRecorder()
				srv.ServeHTTP(w, r)
				if w.Code != 401 || w.Header().Get("WWW-Authenticate") == "" || w.Header().Get("Cache-Control") != "no-store" {
					t.Fatalf("denial=%d headers=%v", w.Code, w.Header())
				}
			}
		})
	}
}

func TestAccessOriginsHostAndPreflight(t *testing.T) {
	for _, tt := range []struct {
		name, token, origin, host, peer, method, requestedMethod, headers, bearer string
		code                                                                      int
		cors                                                                      string
	}{
		{name: "local default", host: "localhost:8081", peer: "127.0.0.1:1234", method: "GET", code: 404},
		{name: "local ipv6", host: "[::1]:8081", peer: "[::1]:1234", method: "GET", code: 404},
		{name: "nonlocal peer", host: "localhost:8081", peer: "192.0.2.1:1234", method: "GET", code: 403},
		{name: "rebound host", host: "evil.example:8081", peer: "127.0.0.1:1234", method: "GET", code: 403},
		{name: "unlisted origin even with credential", token: testToken, origin: "https://evil.example", host: "localhost:8081", method: "GET", bearer: testToken, code: 403},
		{name: "null origin", token: testToken, origin: "null", method: "GET", bearer: testToken, code: 403},
		{name: "allowed preflight without secret", token: testToken, origin: "https://trusted.example", method: "OPTIONS", requestedMethod: "POST", headers: "content-type, Authorization", code: 204, cors: "https://trusted.example"},
		{name: "preflight rejects method", token: testToken, origin: "https://trusted.example", method: "OPTIONS", requestedMethod: "TRACE", code: 403, cors: "https://trusted.example"},
		{name: "preflight rejects header", token: testToken, origin: "https://trusted.example", method: "OPTIONS", requestedMethod: "POST", headers: "X-Secret", code: 403, cors: "https://trusted.example"},
		{name: "allowed origin still needs credential", token: testToken, origin: "https://trusted.example", method: "GET", code: 401, cors: "https://trusted.example"},
		{name: "valid credential", token: testToken, method: "GET", bearer: testToken, code: 404},
		{name: "valid origin and credential", token: testToken, origin: "https://trusted.example", method: "GET", bearer: testToken, code: 404, cors: "https://trusted.example"},
		{name: "same origin", host: "localhost:8081", peer: "127.0.0.1:1234", origin: "http://localhost:8081", method: "GET", code: 404, cors: "http://localhost:8081"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s, err := NewServerWithOptions(nil, Options{Token: tt.token, AllowedOrigins: []string{"https://trusted.example"}})
			if err != nil {
				t.Fatal(err)
			}
			r := httptest.NewRequest(tt.method, "http://localhost/missing", nil)
			r.Host = tt.host
			r.RemoteAddr = tt.peer
			if tt.origin != "" {
				r.Header.Set("Origin", tt.origin)
			}
			r.Header.Set("Access-Control-Request-Method", tt.requestedMethod)
			r.Header.Set("Access-Control-Request-Headers", tt.headers)
			if tt.bearer != "" {
				r.Header.Set("Authorization", "Bearer "+tt.bearer)
			}
			w := httptest.NewRecorder()
			s.ServeHTTP(w, r)
			if w.Code != tt.code || w.Header().Get("Access-Control-Allow-Origin") != tt.cors {
				t.Fatalf("got %d CORS=%q", w.Code, w.Header().Get("Access-Control-Allow-Origin"))
			}
		})
	}
}

func TestAccessRefusesAmbiguousHeadersAndBrowserBypasses(t *testing.T) {
	s, _ := NewServerWithOptions(nil, Options{Token: testToken, AllowedOrigins: []string{"https://trusted.example"}})
	for _, tt := range []struct {
		header, first, second string
		code                  int
	}{
		{"Authorization", "Bearer " + testToken, "Bearer " + testToken, 401},
		{"Origin", "https://trusted.example", "https://trusted.example", 403},
		{"Sec-Fetch-Site", "cross-site", "", 403},
	} {
		r := httptest.NewRequest("POST", "http://localhost/tasks?token="+testToken, nil)
		r.Header.Set("Authorization", "Bearer "+testToken)
		r.Header.Set(tt.header, tt.first)
		if tt.second != "" {
			r.Header.Add(tt.header, tt.second)
		}
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		if w.Code != tt.code {
			t.Fatalf("%s: got%d", tt.header, w.Code)
		}
	}
	r := httptest.NewRequest("GET", "http://localhost/tasks?token="+testToken, nil)
	r.AddCookie(&http.Cookie{Name: "Authorization", Value: testToken})
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatalf("query/cookie credentials bypass: %d", w.Code)
	}
}

func TestOptionsRejectInvalidOriginsAndTokens(t *testing.T) {
	for _, origin := range []string{"*", "null", "https://*.example", "https://example/", "https://user:pass@example", "https://example?x=y", "https://example?", "https://example#x", "file://example"} {
		if _, err := NewServerWithOptions(nil, Options{AllowedOrigins: []string{origin}}); err == nil {
			t.Errorf("accepted %q", origin)
		}
	}
	if _, err := NewServerWithOptions(nil, Options{Token: "short"}); err == nil {
		t.Fatal("accepted weak token")
	}
}
