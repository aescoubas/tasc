package config

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

type Colors struct {
	Overdue    string `yaml:"overdue"`
	OverdueBg  string `yaml:"overdue_bg"`
	Today      string `yaml:"today"`
	TodayBg    string `yaml:"today_bg"`
	Tomorrow   string `yaml:"tomorrow"`
	TomorrowBg string `yaml:"tomorrow_bg"`
	Week       string `yaml:"week"`
	WeekBg     string `yaml:"week_bg"`
	Blocked    string `yaml:"blocked"`
	BlockedBg  string `yaml:"blocked_bg"`
	Default    string `yaml:"default"`
	DefaultBg  string `yaml:"default_bg"`
}

type TimeBlock struct {
	Start string `yaml:"start"` // HH:MM
	End   string `yaml:"end"`   // HH:MM
}

type Config struct {
	Colors        Colors               `yaml:"colors"`
	BackupDir     string               `yaml:"backup_dir"`
	RelativeDates bool                 `yaml:"relative_dates"`
	DateFormat    string               `yaml:"date_format"`
	EndOfDay      string               `yaml:"end_of_day"`
	TimeBlocks    map[string]TimeBlock `yaml:"time_blocks"`
}

func DefaultConfig() Config {
	home, _ := os.UserHomeDir()
	return Config{
		Colors: Colors{
			Overdue:    "1",   // ANSI Red
			OverdueBg:  "233", // Darker Grey
			Today:      "1",   // ANSI Red
			TodayBg:    "",
			Tomorrow:   "11", // ANSI Bright Yellow
			TomorrowBg: "",
			Week:       "11", // ANSI Bright Yellow
			WeekBg:     "",
			Blocked:    "240", // ANSI Dark Grey
			BlockedBg:  "",
			Default:    "7", // ANSI White/Light Grey
			DefaultBg:  "",
		},
		BackupDir:     filepath.Join(home, ".config", "tasc", "backups"),
		RelativeDates: true,
		DateFormat:    "02-01-2006", // Default to DD-MM-YYYY
		EndOfDay:      "18:00",
		TimeBlocks: map[string]TimeBlock{
			"morning":   {Start: "06:00", End: "12:00"},
			"afternoon": {Start: "13:00", End: "18:00"},
		},
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
