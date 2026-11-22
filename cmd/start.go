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
