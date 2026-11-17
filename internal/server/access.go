package server

import (
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/aescoubas/tasc/internal/auth"
)

// Options grants one owner credential access to the entire REST API. It does
// not affect local SQLite or MCP. Browser origins are opt-in, independently of
// authentication: CORS is not an authorization boundary.
type Options struct {
	Token          string
	AllowedOrigins []string
}

func validateOptions(options Options) error {
	if options.Token != "" {
		if err := auth.ValidateToken(options.Token); err != nil {
			return err
		}
	}
	for _, origin := range options.AllowedOrigins {
		u, err := url.Parse(origin)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.Path != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.ContainsAny(origin, "*\r\n") || u.String() != origin {
			return errors.New("allowed origins must be exact http(s) origins without paths, credentials or wildcards")
		}
	}
	return nil
}

func (s *Server) allowRequest(w http.ResponseWriter, r *http.Request) bool {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Add("Vary", "Origin")
	// Without a credential this handler is for loopback only. Host validation
	// prevents a browser reaching it through an attacker-controlled DNS name.
	if s.options.Token == "" {
		peer, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil || net.ParseIP(peer) == nil || !net.ParseIP(peer).IsLoopback() {
			http.Error(w, "Local access required", http.StatusForbidden)
			return false
		}
		host := r.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		if !auth.IsLoopbackHost(host) {
			http.Error(w, "Host not permitted", http.StatusForbidden)
			return false
		}
	}
	origin := r.Header.Get("Origin")
	if origin != "" {
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		allowed := origin == scheme+"://"+r.Host
		for _, candidate := range s.options.AllowedOrigins {
			allowed = allowed || origin == candidate
		}
		if len(r.Header.Values("Origin")) != 1 || !allowed {
			http.Error(w, "Origin not permitted", http.StatusForbidden)
			return false
		}
		w.Header().Set("Access-Control-Allow-Origin", origin)
		if r.Method == http.MethodOptions {
			method := r.Header.Get("Access-Control-Request-Method")
			if method != "GET" && method != "POST" && method != "PUT" && method != "DELETE" {
				http.Error(w, "Preflight method not permitted", http.StatusForbidden)
				return false
			}
			for _, header := range strings.Split(r.Header.Get("Access-Control-Request-Headers"), ",") {
				header = strings.TrimSpace(header)
				if header != "" && !strings.EqualFold(header, "Content-Type") && !strings.EqualFold(header, "Authorization") {
					http.Error(w, "Preflight header not permitted", http.StatusForbidden)
					return false
				}
			}
			w.Header().Add("Vary", "Access-Control-Request-Method")
			w.Header().Add("Vary", "Access-Control-Request-Headers")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			w.WriteHeader(http.StatusNoContent)
			return false
		}
	} else if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
		http.Error(w, "Cross-site request not permitted", http.StatusForbidden)
		return false
	}
	if s.options.Token != "" {
		values := r.Header.Values("Authorization")
		var supplied string
		if len(values) == 1 && len(values[0]) > 7 && strings.EqualFold(values[0][:7], "Bearer ") {
			supplied = values[0][7:]
		}
		expectedHash, suppliedHash := sha256.Sum256([]byte(s.options.Token)), sha256.Sum256([]byte(supplied))
		if subtle.ConstantTimeCompare(expectedHash[:], suppliedHash[:]) != 1 {
			w.Header().Set("WWW-Authenticate", `Bearer realm="tasc"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return false
		}
	}
	return true
}
