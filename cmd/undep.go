package cmd

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
)

var undepCmd = &cobra.Command{
	Use:               "undep [blocker_id] [blocked_id]",
	Short:             "Remove a dependency (blocker no longer blocks blocked)",
	Args:              cobra.ExactArgs(2),
	ValidArgsFunction: taskIDCompletionFunc,
	RunE: func(cmd *cobra.Command, args []string) error {
		blockerID, err1 := strconv.ParseInt(args[0], 10, 64)
		blockedID, err2 := strconv.ParseInt(args[1], 10, 64)

		if err1 != nil || err2 != nil {
			return fmt.Errorf("iDs must be integers")
		}

		exists, err := dependencyExists(blockerID, blockedID)
		if err != nil {
			return fmt.Errorf("error reading dependencies: %w", err)
		}

		if !exists {
			// The common mistake is naming the pair the wrong way round, so say
			// so rather than leaving the user to guess.
			reversed, err := dependencyExists(blockedID, blockerID)
			if err == nil && reversed {
				return fmt.Errorf("no dependency from task %d to task %d. Task %d blocks task %d, run: tasc undep %d %d", blockerID, blockedID, blockedID, blockerID, blockedID, blockerID)
			}
			return fmt.Errorf("no dependency from task %d to task %d", blockerID, blockedID)
		}

		if err := CurrentStore.RemoveDependency(blockerID, blockedID); err != nil {
			return fmt.Errorf("error removing dependency: %w", err)
		}

		fmt.Printf("Task %d no longer blocks task %d.\n", blockerID, blockedID)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(undepCmd)
}
