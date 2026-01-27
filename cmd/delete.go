package cmd

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"

	"github.com/aescoubas/tasc/internal/db"
	"github.com/aescoubas/tasc/internal/models"
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
		var t models.Task
		var project sql.NullString
		var dueAt sql.NullTime
		var scheduledAt sql.NullTime
		var estimate sql.NullString
		var rec sql.NullString

		queryFetch := "SELECT id, description, project, recurrence, due_at, scheduled_at, estimate FROM tasks WHERE id = ?"
		row := db.DB.QueryRow(queryFetch, id)
		if err := row.Scan(&t.ID, &t.Description, &project, &rec, &dueAt, &scheduledAt, &estimate); err == nil {
			// Only process if found, ignore error if not found (delete will fail gracefully or succeed with 0 rows)
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
