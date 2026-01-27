package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/aescoubas/tasc/internal/config"
	"github.com/aescoubas/tasc/internal/db"
	"github.com/spf13/cobra"
)

var backupCmd = &cobra.Command{
	Use:   "backup",
	Short: "Backup the database",
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.LoadConfig()
		if err != nil {
			// Even if load fails, DefaultConfig might have been used inside LoadConfig on error,
			// but here we just want a safe config.
			cfg = config.DefaultConfig()
		}

		// Ensure backup dir exists
		if err := os.MkdirAll(cfg.BackupDir, 0755); err != nil {
			fmt.Printf("Error creating backup directory: %v\n", err)
			return
		}

		filename := fmt.Sprintf("tasc_backup_%s.db", time.Now().Format("20060102_150405"))
		fullPath := filepath.Join(cfg.BackupDir, filename)

		// Use SQLite VACUUM INTO for safe online backup
		_, err = db.DB.Exec(fmt.Sprintf("VACUUM INTO '%s'", fullPath))
		if err != nil {
			fmt.Printf("Error creating backup: %v\n", err)
			return
		}

		fmt.Printf("Backup created at %s\n", fullPath)
	},
}

var backupListCmd = &cobra.Command{
	Use:   "list",
	Short: "List available backups",
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.LoadConfig()
		if err != nil {
			cfg = config.DefaultConfig()
		}

		entries, err := os.ReadDir(cfg.BackupDir)
		if err != nil {
			if os.IsNotExist(err) {
				fmt.Println("No backups found (directory does not exist).")
				return
			}
			fmt.Printf("Error listing backups: %v\n", err)
			return
		}

		if len(entries) == 0 {
			fmt.Println("No backups found.")
			return
		}

		fmt.Println("Available backups:")
		for _, e := range entries {
			if !e.IsDir() {
				info, err := e.Info()
				if err != nil {
					continue
				}
				fmt.Printf("- %s (%s)\n", e.Name(), formatBytes(info.Size()))
			}
		}
	},
}

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

func init() {
	rootCmd.AddCommand(backupCmd)
	backupCmd.AddCommand(backupListCmd)
}
