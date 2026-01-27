package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/aescoubas/tasc/internal/config"
	"github.com/aescoubas/tasc/internal/db"
	"github.com/spf13/cobra"
)

var restoreCmd = &cobra.Command{
	Use:   "restore [backup_name]",
	Short: "Restore the database from a backup",
	Long: `Restore the database from a specific backup file.
Provide the filename (e.g., tasc_backup_20260127_120000.db) or the full path.
WARNING: This will overwrite your current database.`, 
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.LoadConfig()
		if err != nil {
			cfg = config.DefaultConfig()
		}

		if len(args) == 0 {
			fmt.Println("Please specify a backup to restore.")
			fmt.Println("\nAvailable backups:")
			entries, err := os.ReadDir(cfg.BackupDir)
			if err == nil {
				for _, e := range entries {
					if !e.IsDir() {
						fmt.Printf("- %s\n", e.Name())
					}
				}
			} else if os.IsNotExist(err) {
				fmt.Println("No backups found.")
			}
			return
		}

		backupName := args[0]
		backupPath := filepath.Join(cfg.BackupDir, backupName)

		// Check if it exists in backup dir
		if _, err := os.Stat(backupPath); os.IsNotExist(err) {
			// Check if provided as full path
			if _, err := os.Stat(backupName); err == nil {
				backupPath = backupName
			} else {
				fmt.Printf("Backup file not found: %s\n", backupName)
				return
			}
		}

		// Close existing DB connection
		if db.DB != nil {
			db.DB.Close()
		}

		dbPath := db.GetDBPath()

		// Copy file
		src, err := os.Open(backupPath)
		if err != nil {
			fmt.Printf("Error opening backup file: %v\n", err)
			return
		}
		defer src.Close()

		dst, err := os.Create(dbPath)
		if err != nil {
			fmt.Printf("Error writing to database file: %v\n", err)
			return
		}
		defer dst.Close()

		_, err = io.Copy(dst, src)
		if err != nil {
			fmt.Printf("Error restoring database: %v\n", err)
			return
		}

		fmt.Printf("Successfully restored database from %s.\n", backupName)
	},
}

func init() {
	rootCmd.AddCommand(restoreCmd)
}
