package remote

import (
	"fmt"
	"net/http"
	"net/url"

	"github.com/aescoubas/tasc/internal/auth"
)

type bearerTransport struct {
	token string
}

func (t bearerTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	copy := request.Clone(request.Context())
	copy.Header.Set("Authorization", "Bearer "+t.token)
	return http.DefaultTransport.RoundTrip(copy)
}

// NewAuthenticatedClient refuses plaintext credential transport except over a
// loopback endpoint (e.g. an existing SSH tunnel). Redirects never receive it.
func NewAuthenticatedClient(baseURL, token string) (*Client, error) {
	if err := auth.ValidateToken(token); err != nil {
		return nil, err
	}
	u, err := url.Parse(baseURL)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, fmt.Errorf("authenticated remote URL must be an http(s) URL without credentials, query or fragment")
	}
	if u.Scheme == "http" && !auth.IsLoopbackHost(u.Hostname()) {
		return nil, fmt.Errorf("authenticated remote access requires HTTPS or a loopback SSH tunnel")
	}
	c := NewClient(baseURL)
	c.http.Transport = bearerTransport{token: token}
	c.http.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }
	return c, nil
}
