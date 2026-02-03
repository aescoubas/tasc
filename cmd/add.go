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

var (
	project        string
	description    string
	due            string
	scheduled      string
	estimate       string
	recurrenceFlag string
	block          string
)

var addCmd = &cobra.Command{
	Use:   "add [title]",
	Short: "Add a new task",
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

		var scheduledAt *time.Time
		if scheduled != "" {
			t, err := parse.Date(scheduled, cfg.EndOfDay)
			if err != nil {
				fmt.Printf("Invalid scheduled date format: %v\n", err)
				return
			}
			scheduledAt = t
		}

		// If scheduled is set but due is not, default due to scheduled
		if dueAt == nil && scheduledAt != nil {
			dueAt = scheduledAt
		}

		// Handle Block Default logic
		if block == "" && scheduledAt != nil {
			// If scheduled is date-only (floating), default to morning
			if scheduledAt.Hour() == 0 && scheduledAt.Minute() == 0 {
				block = "morning"
			}
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

		if estimate == "" {
			estimate = "20m"
		}

		task := models.Task{
			Title:         title,
			Description:   description,
			Project:       project,
			DueAt:         dueAt,
			ScheduledAt:   scheduledAt,
			ScheduleBlock: block,
			Estimate:      estimate,
			Recurrence:    recurrenceFlag,
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
	addCmd.Flags().StringVarP(&description, "desc", "m", "", "Detailed description")
	addCmd.Flags().StringVarP(&due, "due", "d", "", "Due date (YYYY-MM-DD)")
	addCmd.Flags().StringVarP(&scheduled, "scheduled", "s", "", "Scheduled date (YYYY-MM-DD)")
	addCmd.Flags().StringVarP(&estimate, "estimate", "e", "", "Time estimate (e.g. 2h, 30m)")
	addCmd.Flags().StringVarP(&recurrenceFlag, "recurrence", "r", "", "Recurrence rule (e.g. 'daily', 'every 2 weeks')")
	addCmd.Flags().StringVarP(&block, "block", "b", "", "Time block (e.g. morning, afternoon)")

	addCmd.RegisterFlagCompletionFunc("project", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if CurrentStore == nil {
			return nil, cobra.ShellCompDirectiveError
		}
		projects, err := CurrentStore.ListProjects()
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}
		var names []string
		for _, p := range projects {
			names = append(names, p.Name)
		}
		return names, cobra.ShellCompDirectiveNoFileComp
	})

	addCmd.RegisterFlagCompletionFunc("block", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		cfg, err := config.LoadConfig()
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}
		var names []string
		for name := range cfg.TimeBlocks {
			names = append(names, name)
		}
		return names, cobra.ShellCompDirectiveNoFileComp
	})
}

