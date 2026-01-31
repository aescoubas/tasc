package cmd

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
)

var doneCmd = &cobra.Command{
	Use:   "done [id]",
	Short: "Mark a task as completed",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		id, err := strconv.Atoi(args[0])
		if err != nil {
			fmt.Println("Invalid task ID")
			return
		}

		// 1. Fetch task details for recurrence check
		t, err := CurrentStore.GetTask(int64(id))
		if err != nil {
			fmt.Printf("Error fetching task: %v\n", err)
			return
		}

		// 2. Mark as done
		err = CurrentStore.MarkDone(int64(id))
		if err != nil {
			fmt.Printf("Error updating task: %v\n", err)
			return
		}
		fmt.Printf("Task %d marked as done.\n", id)

		// 3. Handle Recurrence
		if t.Recurrence != "" {
			spawnNextTask(t)
		}
	},
}

func init() {
	rootCmd.AddCommand(doneCmd)
}
