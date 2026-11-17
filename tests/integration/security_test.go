package integration

import (
	"bufio"
	"context"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func startSecurityServer(t *testing.T, args ...string) string {
	t.Helper()
	dir := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	cmd := exec.CommandContext(ctx, binaryPath, append([]string{"serve", "--port", "0"}, args...)...)
	cmd.Env = append(os.Environ(), "HOME="+dir, "TASC_DB_PATH="+filepath.Join(dir, "server.db"), "TASC_REMOTE=", "TASC_API_TOKEN_FILE=")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cancel(); _ = cmd.Wait() })
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "Starting tasc server on ") {
			address := strings.TrimSuffix(strings.TrimPrefix(line, "Starting tasc server on "), "...")
			host, _, err := net.SplitHostPort(address)
			if err != nil || host != "127.0.0.1" {
				t.Fatalf("actual listener is not IPv4 loopback: %s", address)
			}
			scheme := "http://"
			for _, arg := range args {
				if arg == "--tls-cert" {
					scheme = "https://"
				}
			}
			return scheme + address
		}
	}
	t.Fatalf("server did not start: %v", scanner.Err())
	return ""
}

func TestServeBinaryTLSAndBearerTogether(t *testing.T) {
	// Use an ephemeral fixture identity trusted only by this test client.
	fixture := httptest.NewTLSServer(http.NotFoundHandler())
	certificate := fixture.TLS.Certificates[0]
	client := fixture.Client()
	fixture.Close()
	key, err := x509.MarshalPKCS8PrivateKey(certificate.PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	certPath, keyPath, tokenPath := filepath.Join(dir, "cert.pem"), filepath.Join(dir, "key.pem"), filepath.Join(dir, "token")
	for path, data := range map[string][]byte{
		certPath:  pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate.Certificate[0]}),
		keyPath:   pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: key}),
		tokenPath: []byte("synthetic-tls-token-0123456789abcdef0123456789"),
	} {
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	base := startSecurityServer(t, "--api-token-file", tokenPath, "--tls-cert", certPath, "--tls-key", keyPath)
	for _, token := range []string{"", "synthetic-tls-token-0123456789abcdef0123456789"} {
		r, _ := http.NewRequest("GET", base+"/tasks", nil)
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		response, err := client.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		want := 200
		if token == "" {
			want = 401
		}
		if response.TLS == nil || response.StatusCode != want {
			t.Fatalf("TLS status=%d, want%d", response.StatusCode, want)
		}
	}
}

func TestServeBinaryDefaultsAndBrowserRejection(t *testing.T) {
	base := startSecurityServer(t)
	client := &http.Client{Timeout: 5 * time.Second}
	for _, tt := range []struct {
		name, host, origin string
		status             int
	}{
		{"local", "", "", 200},
		{"rebound host", "evil.example", "", 403},
		{"cross origin", "", "https://evil.example", 403},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r, _ := http.NewRequest("GET", base+"/tasks", nil)
			if tt.host != "" {
				r.Host = tt.host
			}
			if tt.origin != "" {
				r.Header.Set("Origin", tt.origin)
			}
			response, err := client.Do(r)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			if response.StatusCode != tt.status || response.Header.Get("Access-Control-Allow-Origin") == "*" {
				t.Fatalf("status=%d headers=%v", response.StatusCode, response.Header)
			}
		})
	}
}

func TestServeBinaryAuthenticatedRemoteCLI(t *testing.T) {
	dir := t.TempDir()
	tokenPath := filepath.Join(dir, "api-token")
	if err := os.WriteFile(tokenPath, []byte("synthetic-cli-token-0123456789abcdef0123456789\n"), 0600); err != nil {
		t.Fatal(err)
	}
	base := startSecurityServer(t, "--api-token-file", tokenPath, "--allow-origin", "https://trusted.example")
	response, err := http.Get(base + "/tasks")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 401 {
		t.Fatalf("anonymous read=%d", response.StatusCode)
	}
	run := func(tokenFile string, args ...string) (string, error) {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, binaryPath, args...)
		cmd.Env = append(os.Environ(), "HOME="+dir, "TASC_DB_PATH="+filepath.Join(dir, "must-not-exist.db"), "TASC_REMOTE="+base, "TASC_API_TOKEN_FILE="+tokenFile)
		output, err := cmd.CombinedOutput()
		return string(output), err
	}
	// Existing list uses a printing Run handler (1455 tracks its exit status).
	// Assert actual HTTP denial, rather than mistaking that exit status for access.
	if out, _ := run("", "list"); !strings.Contains(out, "401 Unauthorized") {
		t.Fatalf("anonymous CLI did not report denial: %s", out)
	}
	if out, err := run(tokenPath, "add", "Credential integration task"); err != nil {
		t.Fatalf("authenticated create: %v %s", err, out)
	}
	if out, err := run(tokenPath, "list"); err != nil || !strings.Contains(out, "Credential integration task") {
		t.Fatalf("authenticated list: %v %s", err, out)
	}
	if out, err := run(tokenPath, "modify", "1", "Authenticated update", "--yes"); err != nil {
		t.Fatalf("authenticated update: %v %s", err, out)
	}
	if out, err := run(tokenPath, "show", "1"); err != nil || !strings.Contains(out, "Authenticated update") {
		t.Fatalf("update did not persist: %v %s", err, out)
	}
	if out, err := run(tokenPath, "delete", "1", "--yes"); err != nil {
		t.Fatalf("authenticated delete: %v %s", err, out)
	}
	if out, err := run(tokenPath, "list"); err != nil || strings.Contains(out, "Authenticated update") {
		t.Fatalf("delete did not persist: %v %s", err, out)
	}
	if _, err := os.Stat(filepath.Join(dir, "must-not-exist.db")); !os.IsNotExist(err) {
		t.Fatalf("remote command touched local database: %v", err)
	}
}

func TestServeBinaryRefusesUnprotectedRemoteBeforeListen(t *testing.T) {
	for _, args := range [][]string{{"serve", "--host", "0.0.0.0", "--port", "0"}, {"serve", "--host", "::", "--port", "0"}, {"serve", "--allow-origin", "*", "--port", "0"}} {
		t.Run(fmt.Sprint(args), func(t *testing.T) {
			dir := t.TempDir()
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, binaryPath, args...)
			cmd.Env = append(os.Environ(), "HOME="+dir, "TASC_DB_PATH="+filepath.Join(dir, "test.db"), "TASC_REMOTE=", "TASC_API_TOKEN_FILE=")
			out, err := cmd.CombinedOutput()
			if err == nil || strings.Contains(string(out), "Starting tasc server on") {
				t.Fatalf("unsafe server started: %v %s", err, out)
			}
		})
	}
}
