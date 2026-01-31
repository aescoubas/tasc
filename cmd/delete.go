package cmd

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

var deleteCmd = &cobra.Command{
	Use:   "delete [id]",
	Short: "Delete a task",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		id, err := strconv.Atoi(args[0])
		if err != nil {
			fmt.Println("Invalid task ID")
			return
		}

		// 1. Fetch task details
		t, err := CurrentStore.GetTask(int64(id))
		if err != nil {
			// Ignore if not found? Or report?
			// Delete will fail if ID invalid anyway usually.
			// But for CLI UX, maybe report.
			// Legacy code ignored error on scan if not found?
			// "Only process if found, ignore error if not found"
			// But here we need t for recurrence.
		}

		// 2. Handle Recurrence Prompt
		if t.Recurrence != "" {
			fmt.Printf("This is a recurring task ('%s').\n", t.Recurrence)
			fmt.Print("Do you want to (s)kip this instance (spawn next) or (S)top recurrence (delete all future)? [s/S]: ")
			var resp string
			fmt.Scanln(&resp)
			resp = strings.ToLower(resp)
			if resp == "s" || resp == "skip" {
				spawnNextTask(t)
			}
		}

		// 3. Soft delete
		err = CurrentStore.DeleteTask(int64(id))
		if err != nil {
			fmt.Printf("Error deleting task: %v\n", err)
			return
		}

		fmt.Printf("Task %d deleted.\n", id)
	},
}

func init() {
	rootCmd.AddCommand(deleteCmd)
}
