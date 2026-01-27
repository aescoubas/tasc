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

var modifyCmd = &cobra.Command{
	Use:   "modify [id] [description]",
	Short: "Modify a task",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		id, err := strconv.Atoi(args[0])
		if err != nil {
			fmt.Println("Invalid task ID")
			return
		}

		// 1. Fetch task details to check recurrence
		var t models.Task
		var projectVal sql.NullString
		var dueAt sql.NullTime
		var scheduledAt sql.NullTime
		var estimateVal sql.NullString
		var rec sql.NullString

		queryFetch := "SELECT id, description, project, recurrence, due_at, scheduled_at, estimate FROM tasks WHERE id = ?"
		row := db.DB.QueryRow(queryFetch, id)
		if err := row.Scan(&t.ID, &t.Description, &projectVal, &rec, &dueAt, &scheduledAt, &estimateVal); err == nil {
			t.Project = projectVal.String
			if rec.Valid {
				t.Recurrence = rec.String
			}
			if dueAt.Valid {
				t.DueAt = &dueAt.Time
			}
			if scheduledAt.Valid {
				t.ScheduledAt = &scheduledAt.Time
			}
			if estimateVal.Valid {
				t.Estimate = estimateVal.String
			}
		}

		var updates []string
		var values []interface{}

		// 2. Handle Recurrence Prompt
		if t.Recurrence != "" {
			fmt.Printf("This is a recurring task ('%s').\n", t.Recurrence)
			fmt.Print("Do you want to (u)pdate the series (affects future) or (o)nly this instance (detach)? [u/o]: ")
			var resp string
			fmt.Scanln(&resp)
			resp = strings.ToLower(resp)
			
			if resp == "o" || resp == "only" {
				// Detach: Spawn next task with OLD values, clear recurrence on CURRENT
				spawnNextTask(t)
				updates = append(updates, "recurrence = NULL")
			}
			// If 'u', we just proceed. The next task generated (when this one is done) will inherit the new values.
		}

		// Check if description is provided as args
		if len(args) > 1 {
			desc := strings.Join(args[1:], " ")
			updates = append(updates, "description = ?")
			values = append(values, desc)
		}

		// Check flags
		if cmd.Flags().Changed("project") {
			updates = append(updates, "project = ?")
			values = append(values, project)
		}

		if cmd.Flags().Changed("due") {
			if due == "" {
				updates = append(updates, "due_at = NULL")
			} else {
				parsed, err := parseDate(due)
				if err != nil {
					fmt.Printf("Invalid due date: %v\n", err)
					return
				}
				updates = append(updates, "due_at = ?")
				values = append(values, parsed)
			}
		}

		if cmd.Flags().Changed("scheduled") {
			if scheduled == "" {
				updates = append(updates, "scheduled_at = NULL")
			} else {
				parsed, err := parseDate(scheduled)
				if err != nil {
					fmt.Printf("Invalid scheduled date: %v\n", err)
					return
				}
				updates = append(updates, "scheduled_at = ?")
				values = append(values, parsed)
			}
		}

		if cmd.Flags().Changed("estimate") {
			updates = append(updates, "estimate = ?")
			values = append(values, estimate)
		}
		
		if cmd.Flags().Changed("recurrence") {
			if recurrenceFlag == "" {
				updates = append(updates, "recurrence = NULL")
			} else {
				updates = append(updates, "recurrence = ?")
				values = append(values, recurrenceFlag)
			}
		}

		if len(updates) == 0 {
			fmt.Println("No changes specified.")
			return
		}

		query := fmt.Sprintf("UPDATE tasks SET %s WHERE id = ?", strings.Join(updates, ", "))
		values = append(values, id)

		_, err = db.DB.Exec(query, values...)
		if err != nil {
			fmt.Printf("Error updating task: %v\n", err)
			return
		}

		fmt.Printf("Task %d modified.\n", id)
	},
}

func init() {
	rootCmd.AddCommand(modifyCmd)
	modifyCmd.Flags().StringVarP(&project, "project", "p", "", "Project name")
	modifyCmd.Flags().StringVarP(&due, "due", "d", "", "Due date (YYYY-MM-DD)")
	modifyCmd.Flags().StringVarP(&scheduled, "scheduled", "s", "", "Scheduled date (YYYY-MM-DD)")
	modifyCmd.Flags().StringVarP(&estimate, "estimate", "e", "", "Time estimate")
	modifyCmd.Flags().StringVarP(&recurrenceFlag, "recurrence", "r", "", "Recurrence rule")
}
