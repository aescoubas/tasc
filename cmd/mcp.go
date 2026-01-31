package cmd

import (
	"log"

	"github.com/aescoubas/tasc/internal/mcp"
	"github.com/spf13/cobra"
)

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Start the MCP server (stdio)",
	Long:  `Starts the Model Context Protocol (MCP) server over standard input/output. This is designed to be used by LLM interfaces like Gemini CLI.`,
	Run: func(cmd *cobra.Command, args []string) {
		// CurrentStore is initialized by root.go
		srv := mcp.NewServer(CurrentStore)

		if err := srv.Serve(); err != nil {
			log.Fatalf("MCP Server error: %v", err)
		}
	},
}

func init() {
	rootCmd.AddCommand(mcpCmd)
}
