package cmd

import (
	"database/sql"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/aescoubas/tasc/internal/db"
	"github.com/aescoubas/tasc/internal/models"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List pending tasks",
	Run: func(cmd *cobra.Command, args []string) {
		rows, err := db.DB.Query("SELECT id, description, project, status, created_at, due_at, scheduled_at, estimate FROM tasks WHERE status = 'pending'")
		if err != nil {
			fmt.Printf("Error querying tasks: %v\n", err)
			return
		}
		defer rows.Close()

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
		fmt.Fprintln(w, "ID\tProject\tDescription\tCreated\tDue\tScheduled\tEst")

		for rows.Next() {
			var t models.Task
			var project sql.NullString
			var dueAt sql.NullTime
			var scheduledAt sql.NullTime
			var estimate sql.NullString

			err := rows.Scan(&t.ID, &t.Description, &project, &t.Status, &t.CreatedAt, &dueAt, &scheduledAt, &estimate)
			if err != nil {
				fmt.Printf("Error scanning row: %v\n", err)
				continue
			}
			t.Project = project.String

			dueStr := "-"
			if dueAt.Valid {
				dueStr = dueAt.Time.Format("2006-01-02")
			}
			
			schStr := "-"
			if scheduledAt.Valid {
				schStr = scheduledAt.Time.Format("2006-01-02")
			}

			estStr := "-"
			if estimate.Valid {
				estStr = estimate.String
			}

			fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\t%s\t%s\n", t.ID, t.Project, t.Description, t.CreatedAt.Format("2006-01-02"), dueStr, schStr, estStr)
		}
		w.Flush()
	},
}

func init() {
	rootCmd.AddCommand(listCmd)
}
