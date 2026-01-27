package cmd

import (
	"fmt"
	"strconv"
	"time"

	"github.com/aescoubas/tasc/internal/db"
	"github.com/spf13/cobra"
)

var stopCmd = &cobra.Command{
	Use:   "stop [id]",
	Short: "Stop tracking time for a task",
	Args:  func(cmd *cobra.Command, args []string) error {
		if len(args) > 1 {
			return fmt.Errorf("accepts at most 1 arg(s), received %d", len(args))
		}
		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		var targetID int64 = -1
		if len(args) > 0 {
			id, err := strconv.Atoi(args[0])
			if err != nil {
				fmt.Println("Invalid task ID")
				return
			}
			targetID = int64(id)
		}

		tx, err := db.DB.Begin()
		if err != nil {
			fmt.Printf("Error starting transaction: %v\n", err)
			return
		}
		defer tx.Rollback()

		query := "SELECT id, active_start FROM tasks WHERE active_start IS NOT NULL"
	
rows, err := tx.Query(query)
		if err != nil {
			fmt.Printf("Error querying active tasks: %v\n", err)
			return
		}

		var activeID int64
		var activeStart time.Time
		found := false

		for rows.Next() {
			if err := rows.Scan(&activeID, &activeStart); err != nil {
				rows.Close()
				fmt.Printf("Error scanning task: %v\n", err)
				return
			}
			
			// If targetID is specified, only stop if it matches
			if targetID != -1 && activeID != targetID {
				continue
			}
			
			// We found a task to stop
			found = true
			break // Only stop one
		}
		rows.Close()

		if !found {
			if targetID != -1 {
				fmt.Printf("Task %d is not active.\n", targetID)
			} else {
				fmt.Println("No active task to stop.")
			}
			return
		}

		now := time.Now()
		elapsed := int64(now.Sub(activeStart).Seconds())

		_, err = tx.Exec("UPDATE tasks SET active_start = NULL, time_spent = time_spent + ? WHERE id = ?", elapsed, activeID)
		if err != nil {
			fmt.Printf("Error stopping task: %v\n", err)
			return
		}

		if err := tx.Commit(); err != nil {
			fmt.Printf("Error committing transaction: %v\n", err)
			return
		}

		fmt.Printf("Stopped task %d (added %ds, total duration recorded).\n", activeID, elapsed)
	},
}

func init() {
	rootCmd.AddCommand(stopCmd)
}
