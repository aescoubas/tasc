package cmd

import (
	"fmt"

	"github.com/aescoubas/tasc/internal/ui"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

var uiCmd = &cobra.Command{
	Use:   "ui",
	Short: "Open the interactive terminal UI",
	RunE: func(cmd *cobra.Command, args []string) error {
		model, err := ui.InitialModel(CurrentStore)
		if err != nil {
			return fmt.Errorf("load tasks: %w", err)
		}
		p := tea.NewProgram(model, tea.WithAltScreen())
		if _, err := p.Run(); err != nil {
			return fmt.Errorf("run TUI: %w", err)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(uiCmd)
}
