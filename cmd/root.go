package cmd

import (
	"fmt"
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
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if db.DB == nil && requiresLocalDatabase(cmd.CommandPath()) {
			return fmt.Errorf("%s requires a local SQLite database; remote mode is not supported", cmd.CommandPath())
		}
		beginUndoTracking(cmd)
		return nil
	},
	PersistentPostRun: func(cmd *cobra.Command, args []string) {
		endUndoTracking()
	},
}

func Execute() {
	err := rootCmd.Execute()
	// Cobra does not run PersistentPostRun after a RunE failure.
	endUndoTracking()
	if err != nil {
		os.Exit(1)
	}
}

func requiresLocalDatabase(command string) bool {
	switch command {
	case "tasc ask", "tasc backup", "tasc backup list", "tasc edit",
		"tasc import-tw", "tasc log", "tasc project archive", "tasc project create",
		"tasc project delete", "tasc renumber", "tasc restore", "tasc seed",
		"tasc serve", "tasc ui", "tasc undo", "tasc vague":
		return true
	}
	return false
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
		// Log to file for troubleshooting
		f, err := os.OpenFile("/tmp/tasc_monitor.log", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
		if err == nil {
			log.SetOutput(f)
			log.Println("TASC MCP STARTED")
			// Print env vars to debug context
			log.Printf("HOME: %s", os.Getenv("HOME"))
			log.Printf("PWD: %s", os.Getenv("PWD"))
		} else {
			// If file fails, fallback to discard
			log.SetOutput(io.Discard)
		}
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
