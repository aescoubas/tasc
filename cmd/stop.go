package cmd

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
)

var stopCmd = &cobra.Command{
	Use:   "stop [id]",
	Short: "Stop tracking time for a task",
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) > 1 {
			return fmt.Errorf("accepts at most 1 arg(s), received %d", len(args))
		}
		return nil
	},
	ValidArgsFunction: activeTaskIDCompletionFunc,
	RunE: func(cmd *cobra.Command, args []string) error {
		var targetID *int64
		if len(args) > 0 {
			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid task ID: %s", args[0])
			}
			targetID = &id
		}

		if targetID == nil {
			active, err := CurrentStore.GetActiveTask()
			if err != nil {
				return fmt.Errorf("error checking active task: %w", err)
			}
			if active != nil {
				targetID = &active.ID
			}
		}
		if err := CurrentStore.StopTask(targetID); err != nil {
			return fmt.Errorf("error stopping task: %w", err)
		}

		if targetID == nil {
			fmt.Println("Stopped active task.")
		} else {
			fmt.Printf("Stopped task %d.\n", *targetID)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(stopCmd)
}
