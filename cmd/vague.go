package cmd

import (
	"fmt"
	"strconv"

	"github.com/aescoubas/tasc/internal/db"
	"github.com/aescoubas/tasc/internal/models"
	"github.com/spf13/cobra"
)

var vagueCmd = &cobra.Command{
	Use:   "vague [id]",
	Short: "Mark a task as poorly defined",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		id, err := strconv.Atoi(args[0])
		if err != nil {
			fmt.Println("Invalid task ID")
			return
		}

		// Update status to undefined
		query := `UPDATE tasks SET status = ? WHERE id = ?`
		result, err := db.DB.Exec(query, models.StatusUndefined, id)
		if err != nil {
			fmt.Printf("Error marking task as undefined: %v\n", err)
			return
		}

		rowsAffected, err := result.RowsAffected()
		if err != nil {
			fmt.Printf("Error checking rows affected: %v\n", err)
			return
		}

		if rowsAffected == 0 {
			fmt.Printf("Task %d not found.\n", id)
			return
		}

		fmt.Printf("Task %d marked as undefined.\n", id)
	},
}

func init() {
	rootCmd.AddCommand(vagueCmd)
}
