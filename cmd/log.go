package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/aescoubas/tasc/internal/config"
	"github.com/aescoubas/tasc/internal/db"
	"github.com/aescoubas/tasc/internal/parse"
	"github.com/spf13/cobra"
)

var logCmd = &cobra.Command{
	Use:   "log [title]",
	Short: "Log a task that is already completed",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.LoadConfig()
		if err != nil {
			cfg = config.DefaultConfig()
		}

		title := strings.Join(args, " ")

		var dueAt *time.Time
		if due != "" {
			t, err := parse.Date(due, cfg.EndOfDay)
			if err != nil {
				fmt.Printf("Invalid due date format: %v\n", err)
				return
			}
			dueAt = t
		}

		query := `INSERT INTO tasks (title, project, due_at, estimate, status, created_at, completed_at) VALUES (?, ?, ?, ?, 'done', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`
		stmt, err := db.DB.Prepare(query)
		if err != nil {
			fmt.Printf("Error preparing statement: %v\n", err)
			return
		}
		defer stmt.Close()

		res, err := stmt.Exec(title, project, dueAt, estimate)
		if err != nil {
			fmt.Printf("Error executing statement: %v\n", err)
			return
		}

		id, _ := res.LastInsertId()
		fmt.Printf("Logged task %d.\n", id)
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
