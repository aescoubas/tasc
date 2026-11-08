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
	Run: func(cmd *cobra.Command, args []string) {
		blockerID, err1 := strconv.ParseInt(args[0], 10, 64)
		blockedID, err2 := strconv.ParseInt(args[1], 10, 64)

		if err1 != nil || err2 != nil {
			fmt.Println("IDs must be integers")
			return
		}

		exists, err := dependencyExists(blockerID, blockedID)
		if err != nil {
			fmt.Printf("Error reading dependencies: %v\n", err)
			return
		}

		if !exists {
			fmt.Printf("No dependency from task %d to task %d.\n", blockerID, blockedID)

			// The common mistake is naming the pair the wrong way round, so say
			// so rather than leaving the user to guess.
			reversed, err := dependencyExists(blockedID, blockerID)
			if err == nil && reversed {
				fmt.Printf("Task %d blocks task %d — run: tasc undep %d %d\n", blockedID, blockerID, blockedID, blockerID)
			}
			return
		}

		if err := CurrentStore.RemoveDependency(blockerID, blockedID); err != nil {
			fmt.Printf("Error removing dependency: %v\n", err)
			return
		}

		fmt.Printf("Task %d no longer blocks task %d.\n", blockerID, blockedID)
	},
}

func init() {
	rootCmd.AddCommand(undepCmd)
}
