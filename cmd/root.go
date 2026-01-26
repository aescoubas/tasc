package cmd

import (
	"os"

	"github.com/aescoubas/tasc/internal/db"
	"github.com/spf13/cobra"
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
	cobra.OnInitialize(db.InitDB)
}
