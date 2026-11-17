package cmd

import (
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/aescoubas/tasc/internal/auth"
	"github.com/aescoubas/tasc/internal/db"
	"github.com/aescoubas/tasc/internal/server"
	"github.com/aescoubas/tasc/internal/store/sqlite"
	"github.com/spf13/cobra"
)

var (
	port                                      int
	listenHost, apiTokenFile, tlsCert, tlsKey string
	allowedOrigins                            []string
)

func validateListener(host string, port int, token, cert, key string) error {
	ip := net.ParseIP(host)
	if ip == nil || port < 0 || port > 65535 {
		return fmt.Errorf("host must be an IP literal and port must be between 0 and 65535")
	}
	if (cert == "") != (key == "") {
		return fmt.Errorf("TLS requires both --tls-cert and --tls-key")
	}
	if !ip.IsLoopback() && (token == "" || cert == "" || key == "") {
		return fmt.Errorf("non-loopback serving requires --api-token-file, --tls-cert and --tls-key")
	}
	return nil
}

var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "Start the tasc server (REST API)",
	Long:  `Starts a lightweight REST API server for accessing the tasc database.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// 1. Initialize Store
		// db.DB is already initialized by OnInitialize(db.InitDB) in root.go
		if db.DB == nil {
			return fmt.Errorf("database not initialized")
		}

		st := sqlite.NewSQLiteStore(db.DB)

		// 2. Initialize Server
		var token string
		if apiTokenFile != "" {
			var err error
			token, err = auth.ReadToken(apiTokenFile)
			if err != nil {
				return err
			}
		}
		if err := validateListener(listenHost, port, token, tlsCert, tlsKey); err != nil {
			return err
		}
		srv, err := server.NewServerWithOptions(st, server.Options{Token: token, AllowedOrigins: allowedOrigins})
		if err != nil {
			return err
		}

		// 3. Start Listener
		addr := net.JoinHostPort(listenHost, strconv.Itoa(port))
		httpServer := &http.Server{Addr: addr, Handler: srv, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second}
		listener, err := net.Listen("tcp", addr)
		if err != nil {
			return err
		}
		defer listener.Close()
		fmt.Printf("Starting tasc server on %s...\n", listener.Addr())
		if tlsCert != "" {
			return httpServer.ServeTLS(listener, tlsCert, tlsKey)
		}
		return httpServer.Serve(listener)
	},
}

func init() {
	serveCmd.Flags().IntVarP(&port, "port", "P", 8081, "Port to listen on")
	serveCmd.Flags().StringVar(&listenHost, "host", "127.0.0.1", "IP address to listen on; non-loopback requires authentication and TLS")
	serveCmd.Flags().StringVar(&apiTokenFile, "api-token-file", "", "Runtime owner bearer-token file (mode 0600 or 0400)")
	serveCmd.Flags().StringVar(&tlsCert, "tls-cert", "", "TLS certificate file")
	serveCmd.Flags().StringVar(&tlsKey, "tls-key", "", "TLS private key file")
	serveCmd.Flags().StringSliceVar(&allowedOrigins, "allow-origin", nil, "Exact browser origin to allow (repeatable; no cross-origin access by default)")
	rootCmd.AddCommand(serveCmd)
}
