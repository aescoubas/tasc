package cmd

import (
	"io"
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
	// Detect if running "mcp" command to redirect logs early
	isMCP := false
	for _, arg := range os.Args {
		if arg == "mcp" {
			isMCP = true
			break
		}
	}

	if isMCP {
		// Silence logs for MCP to prevent protocol corruption
		log.SetOutput(io.Discard)
	}

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
