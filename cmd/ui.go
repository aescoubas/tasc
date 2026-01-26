package cmd

import (
	"fmt"
	"os"

	"github.com/aescoubas/tasc/internal/ui"
	"github.com/spf13/cobra"
	tea "github.com/charmbracelet/bubbletea"
)

var uiCmd = &cobra.Command{
	Use:   "ui",
	Short: "Open the interactive terminal UI",
	Run: func(cmd *cobra.Command, args []string) {
		// DB is already inited by rootCmd.OnInitialize
		// But if we run this standalone, we need to ensure it's ready.
		// However, cobra runs OnInitialize before Run, so we are good.

		p := tea.NewProgram(ui.InitialModel(), tea.WithAltScreen())
		if _, err := p.Run(); err != nil {
			fmt.Printf("Error running TUI: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(uiCmd)
}

