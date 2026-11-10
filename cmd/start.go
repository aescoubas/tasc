package cmd

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
)

var startCmd = &cobra.Command{
	Use:               "start [id]",
	Short:             "Start tracking time for a task",
	Args:              cobra.ExactArgs(1),
	ValidArgsFunction: pendingTaskIDCompletionFunc,
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid task ID: %s", args[0])
		}

		// Check the target before stopping an existing timer.
		_, err = CurrentStore.GetTask(id)
		if err != nil {
			return fmt.Errorf("cannot start task %d: %w", id, err)
		}
		active, err := CurrentStore.GetActiveTask()
		if err != nil {
			return fmt.Errorf("error checking active task: %w", err)
		}
		if active != nil && active.ID == id {
			fmt.Printf("Task %d is already active.\n", id)
			return nil
		}

		if err := CurrentStore.StartTask(id); err != nil {
			return fmt.Errorf("error starting task %d: %w", id, err)
		}

		fmt.Printf("Started task %d.\n", id)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(startCmd)
}
