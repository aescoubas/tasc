package cmd

import (
	"database/sql"
	"fmt"
	"strconv"

	"github.com/aescoubas/tasc/internal/db"
	"github.com/aescoubas/tasc/internal/models"
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
		var t models.Task
		var project sql.NullString
		var dueAt sql.NullTime
		var scheduledAt sql.NullTime
		var estimate sql.NullString
		var rec sql.NullString

		queryFetch := "SELECT id, description, project, recurrence, due_at, scheduled_at, estimate FROM tasks WHERE id = ?"
		row := db.DB.QueryRow(queryFetch, id)
		if err := row.Scan(&t.ID, &t.Description, &project, &rec, &dueAt, &scheduledAt, &estimate); err != nil {
			fmt.Printf("Error fetching task: %v\n", err)
			return
		}
		t.Project = project.String
		if rec.Valid {
			t.Recurrence = rec.String
		}
		if dueAt.Valid {
			t.DueAt = &dueAt.Time
		}
		if scheduledAt.Valid {
			t.ScheduledAt = &scheduledAt.Time
		}
		if estimate.Valid {
			t.Estimate = estimate.String
		}

		// 2. Mark as done
		queryUpdate := `UPDATE tasks SET status = 'done', completed_at = CURRENT_TIMESTAMP WHERE id = ?`
		_, err = db.DB.Exec(queryUpdate, id)
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
