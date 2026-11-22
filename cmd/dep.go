package cmd

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
)

var depCmd = &cobra.Command{
	Use:               "dep [blocker_id] [blocked_id]",
	Short:             "Add a dependency (blocker blocks blocked)",
	Args:              cobra.ExactArgs(2),
	ValidArgsFunction: taskIDCompletionFunc,
	RunE: func(cmd *cobra.Command, args []string) error {
		blockerID, err1 := strconv.ParseInt(args[0], 10, 64)
		blockedID, err2 := strconv.ParseInt(args[1], 10, 64)

		if err1 != nil || err2 != nil {
			return fmt.Errorf("iDs must be integers")
		}

		if blockerID == blockedID {
			return fmt.Errorf("a task cannot block itself")
		}

		// foreign_keys is off on this connection, so the schema's REFERENCES
		// clauses do not catch a mistyped ID — an edge would be stored pointing
		// at nothing.
		if err := ensureTasksExist([]int64{blockerID, blockedID}); err != nil {
			return err
		}

		exists, err := dependencyExists(blockerID, blockedID)
		if err != nil {
			return fmt.Errorf("error reading dependencies: %w", err)
		}
		if exists {
			fmt.Printf("Task %d already blocks task %d.\n", blockerID, blockedID)
			return nil
		}

		// Two tasks blocking each other can never be satisfied, so refuse rather
		// than storing the deadlock.
		reversed, err := dependencyExists(blockedID, blockerID)
		if err != nil {
			return fmt.Errorf("error reading dependencies: %w", err)
		}
		if reversed {
			return fmt.Errorf("task %d already blocks task %d. The reverse edge would deadlock both. Remove the existing edge first: tasc undep %d %d", blockedID, blockerID, blockedID, blockerID)
		}

		if err := CurrentStore.AddDependency(blockerID, blockedID); err != nil {
			return fmt.Errorf("error adding dependency: %w", err)
		}

		fmt.Printf("Task %d now blocks task %d.\n", blockerID, blockedID)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(depCmd)
}
