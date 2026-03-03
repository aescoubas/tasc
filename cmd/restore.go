package cmd

import (
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/aescoubas/tasc/internal/config"
	"github.com/aescoubas/tasc/internal/db"
	_ "github.com/mattn/go-sqlite3"
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

		dbPath := db.GetDBPath()
		tempPath := dbPath + ".restore-tmp"
		cleanupTemp := false
		defer func() {
			if cleanupTemp {
				_ = os.Remove(tempPath)
			}
		}()

		_ = os.Remove(tempPath)

		// Copy backup into temp file first.
		src, err := os.Open(backupPath)
		if err != nil {
			fmt.Printf("Error opening backup file: %v\n", err)
			return
		}
		defer src.Close()

		dst, err := os.Create(tempPath)
		if err != nil {
			fmt.Printf("Error creating restore temp file: %v\n", err)
			return
		}
		cleanupTemp = true

		_, err = io.Copy(dst, src)
		closeErr := dst.Close()
		if err == nil && closeErr != nil {
			err = closeErr
		}
		if err != nil {
			fmt.Printf("Error restoring database: %v\n", err)
			return
		}

		// Validate copied DB before replacing the live one.
		validateDB, err := sql.Open("sqlite3", tempPath)
		if err != nil {
			fmt.Printf("Error validating restored database: %v\n", err)
			return
		}
		var integrity string
		err = validateDB.QueryRow("PRAGMA integrity_check").Scan(&integrity)
		closeErr = validateDB.Close()
		if err == nil && closeErr != nil {
			err = closeErr
		}
		if err != nil {
			fmt.Printf("Error validating restored database: %v\n", err)
			return
		}
		if integrity != "ok" {
			fmt.Printf("Error validating restored database: integrity_check returned %q\n", integrity)
			return
		}

		// Close existing DB connection before atomic replacement.
		if db.DB != nil {
			if err := db.DB.Close(); err != nil {
				fmt.Printf("Error closing current database: %v\n", err)
				return
			}
			db.DB = nil
		}

		if err := os.Rename(tempPath, dbPath); err != nil {
			fmt.Printf("Error replacing database file: %v\n", err)
			return
		}
		cleanupTemp = false

		fmt.Printf("Successfully restored database from %s.\n", backupName)
	},
}

func init() {
	rootCmd.AddCommand(restoreCmd)
}
