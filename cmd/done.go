package cmd

import (
	"fmt"
	"strconv"
	"time"

	"github.com/spf13/cobra"
)

var doneCmd = &cobra.Command{
	Use:   "done [id...]",
	Short: "Mark task(s) as completed",
	Args:  cobra.MinimumNArgs(1),
	ValidArgsFunction: pendingTaskIDCompletionFunc,
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

		// handle recurrence for each task individually before batch update
		// because recurrence logic (spawning new tasks) is complex and per-task
		for _, id := range ids {
			t, err := CurrentStore.GetTask(id)
			if err == nil && t.Recurrence != "" {
				spawnNextTask(t)
			}
		}

		updates := map[string]interface{}{
			"status":       "done",
			"completed_at": time.Now(),
		}

		err := CurrentStore.BatchUpdateTasks(ids, updates)
		if err != nil {
			fmt.Printf("Error updating tasks: %v\n", err)
			return
		}

		if len(ids) == 1 {
			fmt.Printf("Task %d marked as done.\n", ids[0])
		} else {
			fmt.Printf("Marked %d tasks as done.\n", len(ids))
		}
	},
}

func init() {
	rootCmd.AddCommand(doneCmd)
}
