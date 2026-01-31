package cmd

import (
	"fmt"
	"log"
	"net/http"

	"github.com/aescoubas/tasc/internal/db"
	"github.com/aescoubas/tasc/internal/server"
	"github.com/aescoubas/tasc/internal/store/sqlite"
	"github.com/spf13/cobra"
)

var port int

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the tasc server (REST API)",
	Long:  `Starts a lightweight REST API server for accessing the tasc database.`,
	Run: func(cmd *cobra.Command, args []string) {
		// 1. Initialize Store
		// db.DB is already initialized by OnInitialize(db.InitDB) in root.go
		if db.DB == nil {
			log.Fatal("Database not initialized")
		}

		st := sqlite.NewSQLiteStore(db.DB)
		
		// 2. Initialize Server
		srv := server.NewServer(st)

		// 3. Start Listener
		addr := fmt.Sprintf(":%d", port)
		fmt.Printf("Starting tasc server on %s...\n", addr)
		
		if err := http.ListenAndServe(addr, srv); err != nil {
			log.Fatalf("Server failed: %v", err)
		}
	},
}

func init() {
	serveCmd.Flags().IntVarP(&port, "port", "P", 8081, "Port to listen on")
	rootCmd.AddCommand(serveCmd)
}

