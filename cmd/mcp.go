package cmd

import (
	"fmt"
	"io"
	"log"
	"os"

	"github.com/aescoubas/tasc/internal/mcp"
	"github.com/spf13/cobra"
)

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Start the MCP server (stdio)",
	Long:  `Starts the Model Context Protocol (MCP) server over standard input/output. This is designed to be used by LLM interfaces like Gemini CLI.`,
	Run: func(cmd *cobra.Command, args []string) {
		// Silence logs to avoid polluting stderr/stdout which might confuse some MCP clients
		// although stderr is technically allowed, safer to be clean.
		log.SetOutput(io.Discard)

		// CurrentStore is initialized by root.go
		srv := mcp.NewServer(CurrentStore)

		if err := srv.Serve(); err != nil {
			// specific fatal error can still go to os.Stderr if needed, or just exit.
			os.Stderr.WriteString(fmt.Sprintf("MCP Server error: %v\n", err))
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(mcpCmd)
}
