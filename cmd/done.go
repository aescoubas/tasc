package cmd

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
)

var doneCmd = &cobra.Command{
	Use:               "done [id...]",
	Short:             "Mark task(s) as completed",
	Args:              cobra.MinimumNArgs(1),
	ValidArgsFunction: pendingTaskIDCompletionFunc,
	RunE: func(cmd *cobra.Command, args []string) error {
		var ids []int64
		seen := make(map[int64]bool)
		for _, arg := range args {
			id, err := strconv.ParseInt(arg, 10, 64)
			if err != nil {
				return fmt.Errorf("invalid task ID: %s", arg)
			}
			if !seen[id] {
				ids = append(ids, id)
				seen[id] = true
			}
		}

		updates := map[string]interface{}{
			"status": "done",
		}

		if err := CurrentStore.BatchUpdateTasks(ids, updates); err != nil {
			return fmt.Errorf("error completing tasks: %w", err)
		}

		if len(ids) == 1 {
			fmt.Printf("Task %d marked as done.\n", ids[0])
		} else {
			fmt.Printf("Marked %d tasks as done.\n", len(ids))
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(doneCmd)
}
