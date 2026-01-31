package cmd

import (
	"log"
	"os"

	"github.com/aescoubas/tasc/internal/db"
	"github.com/aescoubas/tasc/internal/store"
	"github.com/aescoubas/tasc/internal/store/remote"
	"github.com/aescoubas/tasc/internal/store/sqlite"
	"github.com/spf13/cobra"
)

var (
	remoteURL    string
	CurrentStore store.Store
)

var rootCmd = &cobra.Command{
	Use:   "tasc",
	Short: "A modern, snappy task manager",
	Long:  `Tasc is a fast and efficient CLI task manager for your terminal.`,
}

func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig)
	rootCmd.PersistentFlags().StringVar(&remoteURL, "remote", "", "Remote server URL (e.g. http://localhost:8080)")
}

func initConfig() {
	if remoteURL == "" {
		remoteURL = os.Getenv("TASC_REMOTE")
	}

	if remoteURL != "" {
		CurrentStore = remote.NewClient(remoteURL)
	} else {
		db.InitDB()
		if db.DB == nil {
			log.Fatal("Failed to initialize database")
		}
		CurrentStore = sqlite.NewSQLiteStore(db.DB)
	}
}
