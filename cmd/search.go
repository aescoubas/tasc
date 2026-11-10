package cmd

import (
	"fmt"
	"strings"

	"github.com/aescoubas/tasc/internal/config"
	"github.com/aescoubas/tasc/internal/models"
	"github.com/aescoubas/tasc/internal/store"
	"github.com/aescoubas/tasc/internal/ui"
	"github.com/spf13/cobra"
)

var searchCmd = &cobra.Command{
	Use:   "search [query]",
	Short: "Fuzzy search tasks",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.LoadConfig()
		if err != nil {
			cfg = config.DefaultConfig()
		}

		queryStr := strings.Join(args, " ")

		matches, err := CurrentStore.SearchTasks(queryStr)
		if err != nil {
			return fmt.Errorf("error searching tasks: %w", err)
		}

		var ids []int64
		for _, task := range matches {
			ids = append(ids, task.ID)
		}

		if len(ids) == 0 {
			fmt.Println("No matches found.")
			return nil
		}

		// Fetch full tasks
		filter := store.TaskFilter{
			IDs: ids,
		}
		tasks, err := CurrentStore.ListTasks(filter)
		if err != nil {
			return fmt.Errorf("error fetching tasks: %w", err)
		}

		// Re-order tasks to match search rank (ids order)
		taskMap := make(map[int64]models.Task)
		for _, t := range tasks {
			taskMap[t.ID] = t
		}

		var sortedTasks []models.Task
		for _, id := range ids {
			if t, ok := taskMap[id]; ok {
				sortedTasks = append(sortedTasks, t)
			}
		}

		// Fetch projects for rendering
		var projects []models.Project
		if projs, err := CurrentStore.ListProjects(); err == nil {
			projects = projs
		}

		opts := ui.TableOptions{
			ShowAll: true, // Search results usually short, show all
		}

		// Pass nil for scores (all 0.0)
		ui.RenderTaskTable(sortedTasks, nil, cfg, projects, opts)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(searchCmd)
}
