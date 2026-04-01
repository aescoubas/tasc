package cmd

import (
	"path/filepath"
	"testing"
)

func TestSkillInstallCommandPresent(t *testing.T) {
	cmd, _, err := rootCmd.Find([]string{"skill", "install"})
	if err != nil {
		t.Fatalf("root command should resolve skill install: %v", err)
	}
	if cmd == nil {
		t.Fatal("skill install command should exist")
	}

	for _, flagName := range []string{"client", "dest", "force"} {
		if cmd.Flags().Lookup(flagName) == nil {
			t.Fatalf("skill install command must expose --%s", flagName)
		}
	}
}

func TestResolveSkillInstallRoot(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CODEX_HOME", "")

	tests := []struct {
		name   string
		client string
		dest   string
		want   string
	}{
		{
			name:   "explicit dest wins",
			client: "codex",
			dest:   "/tmp/custom-skills",
			want:   "/tmp/custom-skills",
		},
		{
			name:   "codex default",
			client: "codex",
			want:   filepath.Join(home, ".agents", "skills"),
		},
		{
			name:   "claude default",
			client: "claude",
			want:   filepath.Join(home, ".claude", "skills"),
		},
		{
			name:   "gemini default",
			client: "gemini",
			want:   filepath.Join(home, ".gemini", "skills"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveSkillInstallRoot(tt.client, tt.dest)
			if err != nil {
				t.Fatalf("resolveSkillInstallRoot() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("resolveSkillInstallRoot() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveSkillInstallRootRejectsUnsupportedClient(t *testing.T) {
	if _, err := resolveSkillInstallRoot("unknown", ""); err == nil {
		t.Fatal("expected unsupported client to fail")
	}
}
