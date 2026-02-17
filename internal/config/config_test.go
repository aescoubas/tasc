package config

import (
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Colors.Overdue != "1" {
		t.Errorf("Default overdue color mismatch")
	}
	if cfg.BackupDir == "" {
		t.Errorf("BackupDir should not be empty")
	}
	if cfg.EndOfDay != "20:00" {
		t.Errorf("EndOfDay = %q, want %q", cfg.EndOfDay, "20:00")
	}
}
