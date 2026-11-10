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
		var targetID int64 = -1
		if len(args) > 0 {
			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid task ID: %s", args[0])
			}
			targetID = id
		}

		active, err := CurrentStore.GetActiveTask()
		if err != nil {
			return fmt.Errorf("error checking active task: %w", err)
		}
		if targetID != -1 && (active == nil || active.ID != targetID) {
			return fmt.Errorf("task %d is not active", targetID)
		}
		if active == nil {
			return fmt.Errorf("no active task to stop")
		}

		if err := CurrentStore.StopTask(active.ID); err != nil {
			return fmt.Errorf("error stopping task %d: %w", active.ID, err)
		}

		fmt.Printf("Stopped task %d.\n", active.ID)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(stopCmd)
}
