package skills

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

const SkillName = "tasc-task-manager"

//go:embed tasc-task-manager/**
var bundled embed.FS

func DefaultCodexRoot() (string, error) {
	if codexHome := os.Getenv("CODEX_HOME"); codexHome != "" {
		return filepath.Join(codexHome, "skills"), nil
	}

	return homeScopedSkillsRoot(".agents")
}

func DefaultClaudeRoot() (string, error) {
	return homeScopedSkillsRoot(".claude")
}

func DefaultGeminiRoot() (string, error) {
	return homeScopedSkillsRoot(".gemini")
}

func homeScopedSkillsRoot(configDir string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}

	return filepath.Join(home, configDir, "skills"), nil
}

func InstallTascTaskManager(rootDir string, force bool) (string, error) {
	if rootDir == "" {
		return "", errors.New("skill root directory is required")
	}

	if err := os.MkdirAll(rootDir, 0o755); err != nil {
		return "", fmt.Errorf("create skill root directory: %w", err)
	}

	destDir := filepath.Join(rootDir, SkillName)
	if _, err := os.Stat(destDir); err == nil {
		if !force {
			return "", fmt.Errorf("skill already exists at %s", destDir)
		}
		if err := os.RemoveAll(destDir); err != nil {
			return "", fmt.Errorf("remove existing skill: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("inspect destination: %w", err)
	}

	if err := fs.WalkDir(bundled, SkillName, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		relPath, err := filepath.Rel(SkillName, path)
		if err != nil {
			return err
		}

		targetPath := filepath.Join(destDir, relPath)
		if relPath == "." {
			return os.MkdirAll(destDir, 0o755)
		}

		if d.IsDir() {
			return os.MkdirAll(targetPath, 0o755)
		}

		contents, err := bundled.ReadFile(path)
		if err != nil {
			return err
		}

		return os.WriteFile(targetPath, contents, 0o644)
	}); err != nil {
		return "", fmt.Errorf("install bundled skill: %w", err)
	}

	return destDir, nil
}
