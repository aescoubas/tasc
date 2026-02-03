package cmd

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

var deleteCmd = &cobra.Command{
	Use:   "delete [id...]",
	Short: "Delete task(s)",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		var ids []int64
		for _, arg := range args {
			id, err := strconv.Atoi(arg)
			if err != nil {
				fmt.Printf("Invalid task ID: %s\n", arg)
				continue
			}
			ids = append(ids, int64(id))
		}

		if len(ids) == 0 {
			return
		}

		// Handle recurrence prompt ONLY if single task is being deleted.
		// For batch delete, we assume user knows what they are doing or we skip recurrence logic 
		// (or implicitly stop it). Prompting for 10 tasks is bad UX.
		// Let's adopt a policy: Batch delete = "Stop recurrence" (Delete all future).
		if len(ids) == 1 {
			t, err := CurrentStore.GetTask(ids[0])
			if err == nil && t.Recurrence != "" {
				fmt.Printf("This is a recurring task ('%s').\n", t.Recurrence)
				fmt.Print("Do you want to (s)kip this instance (spawn next) or (S)top recurrence (delete all future)? [s/S]: ")
				var resp string
				fmt.Scanln(&resp)
				resp = strings.ToLower(resp)
				if resp == "s" || resp == "skip" {
					spawnNextTask(t)
				}
			}
		}

		updates := map[string]interface{}{
			"status": "deleted",
		}

		err := CurrentStore.BatchUpdateTasks(ids, updates)
		if err != nil {
			fmt.Printf("Error deleting tasks: %v\n", err)
			return
		}

		if len(ids) == 1 {
			fmt.Printf("Task %d deleted.\n", ids[0])
		} else {
			fmt.Printf("Deleted %d tasks.\n", len(ids))
		}
	},
}

func init() {
	rootCmd.AddCommand(deleteCmd)
}
