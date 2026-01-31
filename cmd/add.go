package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/aescoubas/tasc/internal/models"
	"github.com/aescoubas/tasc/internal/parse"
	"github.com/spf13/cobra"
)

var (
	project    string
	due        string
	scheduled  string
	estimate   string
	recurrenceFlag string
)

var addCmd = &cobra.Command{
	Use:   "add [description]",
	Short: "Add a new task",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		description := strings.Join(args, " ")

		var dueAt *time.Time
		if due != "" {
			t, err := parse.Date(due)
			if err != nil {
				fmt.Printf("Invalid due date format: %v\n", err)
				return
			}
			dueAt = t
		}

		// Check project deadline
		if project != "" {
			p, err := CurrentStore.GetProject(project)
			// Ignore error (project might not exist)
			if err == nil && p.DueAt != nil {
				if dueAt == nil {
					// Default to project due date
					dueAt = p.DueAt
					fmt.Printf("Defaulting due date to project deadline: %s\n", p.DueAt.Format("2006-01-02"))
				} else {
					// Validate
					if dueAt.After(*p.DueAt) {
						fmt.Printf("Warning: Task due date (%s) is after project deadline (%s).\n", 
							dueAt.Format("2006-01-02"), p.DueAt.Format("2006-01-02"))
					}
				}
			}
		}

		var scheduledAt *time.Time
		if scheduled != "" {
			t, err := parse.Date(scheduled)
			if err != nil {
				fmt.Printf("Invalid scheduled date format: %v\n", err)
				return
			}
			scheduledAt = t
		}

		task := models.Task{
			Description: description,
			Project:     project,
			DueAt:       dueAt,
			ScheduledAt: scheduledAt,
			Estimate:    estimate,
			Recurrence:  recurrenceFlag,
		}

		id, err := CurrentStore.CreateTask(task)
		if err != nil {
			fmt.Printf("Error creating task: %v\n", err)
			return
		}

		fmt.Printf("Created task %d.\n", id)
	},
}

func init() {
	rootCmd.AddCommand(addCmd)
	addCmd.Flags().StringVarP(&project, "project", "p", "", "Project name")
	addCmd.Flags().StringVarP(&due, "due", "d", "", "Due date (YYYY-MM-DD)")
	addCmd.Flags().StringVarP(&scheduled, "scheduled", "s", "", "Scheduled date (YYYY-MM-DD)")
	addCmd.Flags().StringVarP(&estimate, "estimate", "e", "", "Time estimate (e.g. 2h, 30m)")
	addCmd.Flags().StringVarP(&recurrenceFlag, "recurrence", "r", "", "Recurrence rule (e.g. 'daily', 'every 2 weeks')")
}

