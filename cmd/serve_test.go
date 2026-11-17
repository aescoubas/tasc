package cmd

import "testing"

func TestListenerRequiresExplicitProtectedRemoteAccess(t *testing.T) {
	for _, tt := range []struct {
		host, token, cert, key string
		port                   int
		valid                  bool
	}{
		{"127.0.0.1", "", "", "", 8081, true},
		{"::1", "", "", "", 8081, true},
		{"0.0.0.0", "", "", "", 8081, false},
		{"::", "token", "", "", 8081, false},
		{"192.0.2.1", "token", "cert", "key", 8081, true},
		{"0.0.0.0", "", "cert", "key", 8081, false},
		{"localhost", "", "", "", 8081, false},
		{"127.0.0.1", "", "cert", "", 8081, false},
		{"127.0.0.1", "", "", "", 65536, false},
	} {
		if err := validateListener(tt.host, tt.port, tt.token, tt.cert, tt.key); (err == nil) != tt.valid {
			t.Errorf("host=%s port=%d err=%v", tt.host, tt.port, err)
		}
	}
}
