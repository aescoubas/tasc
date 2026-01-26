package cmd

import (
	"database/sql"
	"fmt"
	"os"
	"sort"
	"text/tabwriter"

	"github.com/aescoubas/tasc/internal/db"
	"github.com/aescoubas/tasc/internal/models"
	"github.com/aescoubas/tasc/internal/priority"
	"github.com/spf13/cobra"
)

type taskWithScore struct {
	task  models.Task
	score float64
}

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

		var tasks []taskWithScore
		calc := priority.NewCalculator()

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

			if dueAt.Valid {
				t.DueAt = &dueAt.Time
			}
			if scheduledAt.Valid {
				t.ScheduledAt = &scheduledAt.Time
			}
			if estimate.Valid {
				t.Estimate = estimate.String
			}

			score := calc.Calculate(t)
			tasks = append(tasks, taskWithScore{task: t, score: score})
		}

		// Sort by score descending
		sort.Slice(tasks, func(i, j int) bool {
			return tasks[i].score > tasks[j].score
		})

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
		fmt.Fprintln(w, "ID\tProject\tDescription\tCreated\tDue\tScheduled\tEst\tScore")

		for _, item := range tasks {
			t := item.task
			
			dueStr := "-"
			if t.DueAt != nil {
				dueStr = t.DueAt.Format("2006-01-02")
			}
			
			schStr := "-"
			if t.ScheduledAt != nil {
				schStr = t.ScheduledAt.Format("2006-01-02")
			}

			estStr := "-"
			if t.Estimate != "" {
				estStr = t.Estimate
			}

			fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\t%s\t%s\t%.1f\n", t.ID, t.Project, t.Description, t.CreatedAt.Format("2006-01-02"), dueStr, schStr, estStr, item.score)
		}
		w.Flush()
	},
}

func init() {
	rootCmd.AddCommand(listCmd)
}
