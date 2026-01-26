package cmd

import (
	"fmt"
	"strconv"

	"github.com/aescoubas/tasc/internal/db"
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

		// Soft delete
		query := `UPDATE tasks SET status = 'deleted' WHERE id = ?`
		_, err = db.DB.Exec(query, id)
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
