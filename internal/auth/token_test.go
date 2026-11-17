package auth

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRuntimeTokenFiles(t *testing.T) {
	for _, tt := range []struct {
		name, content string
		mode          os.FileMode
		valid         bool
	}{
		{"valid", strings.Repeat("x", 32) + "\n", 0600, true},
		{"readonly", strings.Repeat("x", 32), 0400, true},
		{"short", "secret", 0600, false},
		{"multiline", strings.Repeat("x", 32) + "\nextra", 0600, false},
		{"readable", strings.Repeat("x", 32), 0644, false},
		{"oversized", strings.Repeat("x", 4098), 0600, false},
		{"spaces", strings.Repeat("x", 32) + " ", 0600, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "token")
			if err := os.WriteFile(path, []byte(tt.content), tt.mode); err != nil {
				t.Fatal(err)
			}
			token, err := ReadToken(path)
			if (err == nil) != tt.valid {
				t.Fatalf("valid=%t err=%v", tt.valid, err)
			}
			if err != nil && (token != "" || strings.Contains(err.Error(), tt.content)) {
				t.Fatal("credential leaked")
			}
		})
	}
	if _, err := ReadToken(t.TempDir()); err == nil {
		t.Fatal("directory accepted")
	}
}
