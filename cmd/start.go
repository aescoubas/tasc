package cmd

import (
	"fmt"
	"strconv"
	"time"

	"github.com/aescoubas/tasc/internal/db"
	"github.com/spf13/cobra"
)

var startCmd = &cobra.Command{
	Use:   "start [id]",
	Short: "Start tracking time for a task",
	Args:  cobra.ExactArgs(1),
	ValidArgsFunction: pendingTaskIDCompletionFunc,
	Run: func(cmd *cobra.Command, args []string) {
		id, err := strconv.Atoi(args[0])
		if err != nil {
			fmt.Println("Invalid task ID")
			return
		}

		tx, err := db.DB.Begin()
		if err != nil {
			fmt.Printf("Error starting transaction: %v\n", err)
			return
		}
		defer tx.Rollback()

		// 1. Stop any currently active task

		rows, err := tx.Query("SELECT id, active_start FROM tasks WHERE active_start IS NOT NULL")
		if err != nil {
			fmt.Printf("Error checking active tasks: %v\n", err)
			return
		}

		var activeID int64
		var activeStart time.Time
		taskFound := false

		// There should be at most one, but let's handle list
		for rows.Next() {
			err := rows.Scan(&activeID, &activeStart)
			if err != nil {
				rows.Close()
				fmt.Printf("Error scanning active task: %v\n", err)
				return
			}
			taskFound = true

			// If the task we are starting is already active, we can either:
			// A) Do nothing (it's already started)
			// B) Restart it (reset start time?) - bad for tracking
			// Let's assume A.
			if activeID == int64(id) {
				rows.Close()
				fmt.Printf("Task %d is already active.\n", id)
				return
			}
		}
		rows.Close()

		now := time.Now()

		if taskFound {
			elapsed := int64(now.Sub(activeStart).Seconds())
			_, err = tx.Exec("UPDATE tasks SET active_start = NULL, time_spent = time_spent + ? WHERE id = ?", elapsed, activeID)
			if err != nil {
				fmt.Printf("Error stopping active task %d: %v\n", activeID, err)
				return
			}
			fmt.Printf("Stopped task %d (added %ds).\n", activeID, elapsed)
		}

		// 2. Start the new task
		// Verify task exists and is pending/completed? Assuming we can track any task.
		res, err := tx.Exec("UPDATE tasks SET active_start = ?, status = 'ongoing' WHERE id = ?", now, id)
		if err != nil {
			fmt.Printf("Error starting task %d: %v\n", id, err)
			return
		}

		rowsAffected, _ := res.RowsAffected()
		if rowsAffected == 0 {
			fmt.Printf("Task %d not found.\n", id)
			return
		}

		if err := tx.Commit(); err != nil {
			fmt.Printf("Error committing transaction: %v\n", err)
			return
		}

		fmt.Printf("Started task %d.\n", id)
	},
}

func init() {
	rootCmd.AddCommand(startCmd)
}
