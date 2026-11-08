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
	Run: func(cmd *cobra.Command, args []string) {
		blockerID, err1 := strconv.ParseInt(args[0], 10, 64)
		blockedID, err2 := strconv.ParseInt(args[1], 10, 64)

		if err1 != nil || err2 != nil {
			fmt.Println("IDs must be integers")
			return
		}

		if blockerID == blockedID {
			fmt.Println("A task cannot block itself")
			return
		}

		// foreign_keys is off on this connection, so the schema's REFERENCES
		// clauses do not catch a mistyped ID — an edge would be stored pointing
		// at nothing.
		if err := ensureTasksExist([]int64{blockerID, blockedID}); err != nil {
			fmt.Printf("Error: %v\n", err)
			return
		}

		exists, err := dependencyExists(blockerID, blockedID)
		if err != nil {
			fmt.Printf("Error reading dependencies: %v\n", err)
			return
		}
		if exists {
			fmt.Printf("Task %d already blocks task %d.\n", blockerID, blockedID)
			return
		}

		// Two tasks blocking each other can never be satisfied, so refuse rather
		// than storing the deadlock.
		reversed, err := dependencyExists(blockedID, blockerID)
		if err != nil {
			fmt.Printf("Error reading dependencies: %v\n", err)
			return
		}
		if reversed {
			fmt.Printf("Task %d already blocks task %d — adding the reverse would deadlock both.\n", blockedID, blockerID)
			fmt.Printf("Remove the existing edge first: tasc undep %d %d\n", blockedID, blockerID)
			return
		}

		if err := CurrentStore.AddDependency(blockerID, blockedID); err != nil {
			fmt.Printf("Error adding dependency: %v\n", err)
			return
		}

		fmt.Printf("Task %d now blocks task %d.\n", blockerID, blockedID)
	},
}

func init() {
	rootCmd.AddCommand(depCmd)
}
