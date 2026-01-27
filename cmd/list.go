package cmd

import (
	"database/sql"
	"fmt"
	"os"
	"sort"
	"text/tabwriter"
	"time"

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
		rows, err := db.DB.Query("SELECT id, description, project, status, created_at, due_at, scheduled_at, estimate, active_start, time_spent FROM tasks WHERE status = 'pending'")
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
			var activeStart sql.NullTime
			var timeSpent sql.NullInt64

			err := rows.Scan(&t.ID, &t.Description, &project, &t.Status, &t.CreatedAt, &dueAt, &scheduledAt, &estimate, &activeStart, &timeSpent)
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
			if activeStart.Valid {
				t.ActiveStart = &activeStart.Time
			}
			if timeSpent.Valid {
				t.TimeSpent = timeSpent.Int64
			}

			score := calc.Calculate(t)
			tasks = append(tasks, taskWithScore{task: t, score: score})
		}

		// Sort by active status then score descending
		sort.Slice(tasks, func(i, j int) bool {
			activeI := tasks[i].task.ActiveStart != nil
			activeJ := tasks[j].task.ActiveStart != nil
			if activeI && !activeJ {
				return true
			}
			if !activeI && activeJ {
				return false
			}
			return tasks[i].score > tasks[j].score
		})

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
		fmt.Fprintln(w, "ID\tProject\tDescription\tCreated\tDue\tScheduled\tEst\tScore\tDuration")

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
			
			desc := t.Description
			duration := t.TimeSpent
			if t.ActiveStart != nil {
				desc = "ONGOING: " + desc
				duration += int64(time.Since(*t.ActiveStart).Seconds())
			}
			
			durStr := "-"
			if duration > 0 {
				d := time.Duration(duration) * time.Second
				durStr = d.String()
			}

			fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\t%s\t%s\t%.1f\t%s\n", t.ID, t.Project, desc, t.CreatedAt.Format("2006-01-02"), dueStr, schStr, estStr, item.score, durStr)
		}
		w.Flush()
	},
}

func init() {
	rootCmd.AddCommand(listCmd)
}
