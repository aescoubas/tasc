package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/aescoubas/tasc/internal/config"
	"github.com/aescoubas/tasc/internal/models"
	"github.com/aescoubas/tasc/internal/parse"
	"github.com/spf13/cobra"
)

var logCmd = &cobra.Command{
	Use:   "log [title]",
	Short: "Log a task that is already completed",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.LoadConfig()
		if err != nil {
			cfg = config.DefaultConfig()
		}

		title := strings.Join(args, " ")

		var dueAt *time.Time
		if due != "" {
			t, err := parse.Date(due, cfg.EndOfDay)
			if err != nil {
				return fmt.Errorf("invalid due date format: %w", err)
			}
			dueAt = t
		}

		id, err := CurrentStore.LogTask(models.Task{Title: title, Project: project, DueAt: dueAt, Estimate: estimate})
		if err != nil {
			return fmt.Errorf("log task: %w", err)
		}

		fmt.Printf("Logged task %d.\n", id)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(logCmd)
	logCmd.Flags().StringVarP(&project, "project", "p", "", "Project name")
	logCmd.Flags().StringVarP(&due, "due", "d", "", "Due date (YYYY-MM-DD, DD.MM.YYYY, DD/MM/YYYY, DD-MM-YYYY, or natural language)")
	logCmd.Flags().StringVarP(&estimate, "estimate", "e", "", "Time estimate (e.g. 2h, 30m)")

	logCmd.RegisterFlagCompletionFunc("project", projectCompletionFunc)
	logCmd.RegisterFlagCompletionFunc("due", dateCompletionFunc)
}
