package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallTascTaskManagerCopiesBundledFiles(t *testing.T) {
	root := t.TempDir()

	installedDir, err := InstallTascTaskManager(root, false)
	if err != nil {
		t.Fatalf("InstallTascTaskManager() error = %v", err)
	}

	if installedDir != filepath.Join(root, SkillName) {
		t.Fatalf("installed dir = %q, want %q", installedDir, filepath.Join(root, SkillName))
	}

	files := []string{
		"SKILL.md",
		filepath.Join("references", "tools.md"),
		filepath.Join("references", "workflows.md"),
		filepath.Join("agents", "openai.yaml"),
	}

	for _, rel := range files {
		path := filepath.Join(installedDir, rel)
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("expected %s to exist: %v", rel, err)
		}
		if info.IsDir() {
			t.Fatalf("expected %s to be a file", rel)
		}
	}

	skillBody, err := os.ReadFile(filepath.Join(installedDir, "SKILL.md"))
	if err != nil {
		t.Fatalf("reading SKILL.md: %v", err)
	}
	if !strings.Contains(string(skillBody), "tasc_list_tasks") {
		t.Fatalf("SKILL.md should include MCP interaction guidance")
	}
}

func TestInstallTascTaskManagerRequiresForceToReplaceExistingSkill(t *testing.T) {
	root := t.TempDir()

	if _, err := InstallTascTaskManager(root, false); err != nil {
		t.Fatalf("initial install failed: %v", err)
	}

	if _, err := InstallTascTaskManager(root, false); err == nil {
		t.Fatal("expected reinstall without force to fail")
	}
}

func TestInstallTascTaskManagerForceReplacesDestination(t *testing.T) {
	root := t.TempDir()

	installedDir, err := InstallTascTaskManager(root, false)
	if err != nil {
		t.Fatalf("initial install failed: %v", err)
	}

	extraPath := filepath.Join(installedDir, "stale.txt")
	if err := os.WriteFile(extraPath, []byte("stale"), 0o644); err != nil {
		t.Fatalf("writing stale file: %v", err)
	}

	if _, err := InstallTascTaskManager(root, true); err != nil {
		t.Fatalf("force install failed: %v", err)
	}

	if _, err := os.Stat(extraPath); !os.IsNotExist(err) {
		t.Fatalf("expected stale file to be removed, got err=%v", err)
	}
}

func TestDefaultCodexRoot(t *testing.T) {
	t.Run("uses CODEX_HOME when set", func(t *testing.T) {
		codexHome := filepath.Join(t.TempDir(), "codex-home")
		t.Setenv("CODEX_HOME", codexHome)
		t.Setenv("HOME", filepath.Join(t.TempDir(), "ignored-home"))

		got, err := DefaultCodexRoot()
		if err != nil {
			t.Fatalf("DefaultCodexRoot() error = %v", err)
		}

		want := filepath.Join(codexHome, "skills")
		if got != want {
			t.Fatalf("DefaultCodexRoot() = %q, want %q", got, want)
		}
	})

	t.Run("falls back to ~/.codex/skills", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("CODEX_HOME", "")
		t.Setenv("HOME", home)

		got, err := DefaultCodexRoot()
		if err != nil {
			t.Fatalf("DefaultCodexRoot() error = %v", err)
		}

		want := filepath.Join(home, ".agents", "skills")
		if got != want {
			t.Fatalf("DefaultCodexRoot() = %q, want %q", got, want)
		}
	})
}

func TestDefaultClaudeRoot(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	got, err := DefaultClaudeRoot()
	if err != nil {
		t.Fatalf("DefaultClaudeRoot() error = %v", err)
	}

	want := filepath.Join(home, ".claude", "skills")
	if got != want {
		t.Fatalf("DefaultClaudeRoot() = %q, want %q", got, want)
	}
}

func TestDefaultGeminiRoot(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	got, err := DefaultGeminiRoot()
	if err != nil {
		t.Fatalf("DefaultGeminiRoot() error = %v", err)
	}

	want := filepath.Join(home, ".gemini", "skills")
	if got != want {
		t.Fatalf("DefaultGeminiRoot() = %q, want %q", got, want)
	}
}
