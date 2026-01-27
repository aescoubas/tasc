package cmd

import (
	"fmt"
	"strconv"

	"github.com/aescoubas/tasc/internal/db"
	"github.com/spf13/cobra"
)

var depCmd = &cobra.Command{
	Use:   "dep [blocker_id] [blocked_id]",
	Short: "Add a dependency (blocker blocks blocked)",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		blockerID, err1 := strconv.Atoi(args[0])
		blockedID, err2 := strconv.Atoi(args[1])

		if err1 != nil || err2 != nil {
			fmt.Println("IDs must be integers")
			return
		}

		if blockerID == blockedID {
			fmt.Println("A task cannot block itself")
			return
		}

		// Check circular dependency (simple 1-level check for now, can be improved)
		// Or let SQLite recursion handle it later?
		// For now, just insert.

		query := `INSERT INTO task_dependencies (blocker_id, blocked_id) VALUES (?, ?)`
		_, err := db.DB.Exec(query, blockerID, blockedID)
		if err != nil {
			fmt.Printf("Error adding dependency: %v\n", err)
			return
		}

		fmt.Printf("Task %d now blocks task %d.\n", blockerID, blockedID)
	},
}

func init() {
	rootCmd.AddCommand(depCmd)
}
