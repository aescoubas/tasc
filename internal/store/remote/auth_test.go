package remote

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

const syntheticToken = "synthetic-remote-credential-0123456789abcdef"

func TestAuthenticatedClientSendsTokenForEveryHTTPMethod(t *testing.T) {
	var calls atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("Authorization") != "Bearer "+syntheticToken {
			t.Error("missing credential")
		}
		w.WriteHeader(200)
	}))
	defer s.Close()
	c, err := NewAuthenticatedClient(s.URL, syntheticToken)
	if err != nil {
		t.Fatal(err)
	}
	for _, method := range []string{"GET", "POST", "PUT", "DELETE"} {
		r, err := http.NewRequest(method, s.URL+"/tasks", nil)
		if err != nil {
			t.Fatal(err)
		}
		response, err := c.http.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if r.Header.Get("Authorization") != "" {
			t.Fatal("transport mutated caller request")
		}
	}
	if calls.Load() != 4 {
		t.Fatalf("got%d calls", calls.Load())
	}
}

func TestAuthenticatedClientRefusesRedirectsAndUnsafeURLs(t *testing.T) {
	var leaked atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { leaked.Add(1) }))
	defer other.Close()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, other.URL, http.StatusFound) }))
	defer s.Close()
	c, err := NewAuthenticatedClient(s.URL, syntheticToken)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.ListProjects(); err == nil {
		t.Fatal("redirect accepted as API success")
	}
	if leaked.Load() != 0 {
		t.Fatal("redirect received a request")
	}
	for _, url := range []string{"http://192.0.2.1:8081", "http://example.com", "https://user:pass@example.com", "https://example.com?x=y", "https://example.com#fragment", "file:///tasks"} {
		if _, err := NewAuthenticatedClient(url, syntheticToken); err == nil {
			t.Errorf("accepted %s", url)
		}
	}
}
