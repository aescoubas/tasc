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
	estimate       string
	recurrenceFlag string
	block          string
)

var addCmd = &cobra.Command{
	Use:   "add [title]",
	Short: "Add a new task",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.LoadConfig()
		if err != nil {
			cfg = config.DefaultConfig()
		}

		dependencySelection, err := buildDependencySelection(dependsOn, blocks)
		if err != nil {
			return fmt.Errorf("invalid dependency list: %w", err)
		}
		if err := dependencySelection.validate(nil); err != nil {
			return fmt.Errorf("invalid dependencies: %w", err)
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

		// Defaulting Logic:
		// 1. If due is not set, default to 1 week from today.
		if dueAt == nil {
			t, err := parse.Date("in 1 week", cfg.EndOfDay)
			if err == nil {
				dueAt = t
			}
		}

		// Handle Block Default logic
		if block == "" && dueAt != nil {
			// If due is date-only (floating), default to morning.
			if dueAt.Hour() == 0 && dueAt.Minute() == 0 {
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
			ScheduleBlock: block,
			Estimate:      estimate,
			Recurrence:    recurrenceFlag,
		}

		id, linksAdded, err := CurrentStore.CreateTaskWithLinks(task, dependencySelection.links())
		if err != nil {
			return fmt.Errorf("error creating task: %w", err)
		}

		fmt.Printf("Created task %d.\n", id)
		if linksAdded > 0 {
			fmt.Printf("Added %d dependency link(s).\n", linksAdded)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(addCmd)
	addCmd.Flags().StringVarP(&project, "project", "p", "", "Project name")
	addCmd.Flags().StringVarP(&description, "description", "m", "", "Detailed description")
	addCmd.Flags().StringVarP(&due, "due", "d", "", "Due date (YYYY-MM-DD, DD.MM.YYYY, DD/MM/YYYY, DD-MM-YYYY, or natural language)")
	addCmd.Flags().StringVarP(&estimate, "estimate", "e", "", "Time estimate (e.g. 2h, 30m)")
	addCmd.Flags().StringVarP(&recurrenceFlag, "recurrence", "r", "", "Recurrence rule (e.g. 'daily', 'every 2 weeks')")
	addCmd.Flags().StringVarP(&block, "block", "b", "", "Time block (e.g. morning, afternoon)")
	addCmd.Flags().StringSliceVar(&dependsOn, "depends-on", nil, "Blocker task IDs for the new task (repeat or comma-separate)")
	addCmd.Flags().StringSliceVar(&blocks, "blocks", nil, "Task IDs that the new task should block (repeat or comma-separate)")

	addCmd.RegisterFlagCompletionFunc("project", projectCompletionFunc)
	addCmd.RegisterFlagCompletionFunc("depends-on", taskIDCompletionFunc)
	addCmd.RegisterFlagCompletionFunc("blocks", taskIDCompletionFunc)

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

	addCmd.RegisterFlagCompletionFunc("due", dateCompletionFunc)
	addCmd.RegisterFlagCompletionFunc("recurrence", recurrenceCompletionFunc)
}
