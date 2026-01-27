package config

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Colors struct {
	Overdue  string `yaml:"overdue"`
	Today    string `yaml:"today"`
	Tomorrow string `yaml:"tomorrow"`
	Week     string `yaml:"week"`
	Default  string `yaml:"default"`
}

type Config struct {
	Colors    Colors `yaml:"colors"`
	BackupDir string `yaml:"backup_dir"`
}

func DefaultConfig() Config {
	home, _ := os.UserHomeDir()
	return Config{
		Colors: Colors{
			Overdue:  "#FF0000", // Red
			Today:    "#FFA500", // Orange
			Tomorrow: "#FFFF00", // Yellow
			Week:     "#00FF00", // Green
			Default:  "#FFFFFF", // White
		},
		BackupDir: filepath.Join(home, ".config", "tasc", "backups"),
	}
}

func LoadConfig() (Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return DefaultConfig(), err
	}

	configPath := filepath.Join(home, ".tasc.yaml")
	data, err := os.ReadFile(configPath)
	if os.IsNotExist(err) {
		return DefaultConfig(), nil
	}
	if err != nil {
		return DefaultConfig(), err
	}

	var cfg Config
	// Initialize with defaults in case file is partial
	cfg = DefaultConfig()

	err = yaml.Unmarshal(data, &cfg)
	if err != nil {
		return DefaultConfig(), err
	}

	return cfg, nil
}
