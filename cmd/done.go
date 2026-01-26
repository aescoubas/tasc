package cmd

import (
	"fmt"
	"strconv"

	"github.com/aescoubas/tasc/internal/db"
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

		query := `UPDATE tasks SET status = 'completed', completed_at = CURRENT_TIMESTAMP WHERE id = ?`
		_, err = db.DB.Exec(query, id)
		if err != nil {
			fmt.Printf("Error updating task: %v\n", err)
			return
		}

		fmt.Printf("Task %d marked as completed.\n", id)
	},
}

func init() {
	rootCmd.AddCommand(doneCmd)
}
