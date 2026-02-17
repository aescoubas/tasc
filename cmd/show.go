package cmd

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/aescoubas/tasc/internal/config"
	"github.com/aescoubas/tasc/internal/db"
	"github.com/aescoubas/tasc/internal/models"
	"github.com/aescoubas/tasc/internal/ui"
	"github.com/spf13/cobra"
)

var showOutput string

var showCmd = &cobra.Command{
	Use:   "show [id]",
	Short: "Show detailed information for a task",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		outputFormat, err := validateOutputFormat(showOutput)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			return
		}

		cfg, _ := config.LoadConfig()

		id, err := strconv.Atoi(args[0])
		if err != nil {
			fmt.Println("Invalid task ID")
			return
		}

		// 1. Fetch Task
		var t models.Task
		var project sql.NullString
		var description sql.NullString
		var dueAt sql.NullTime
		var estimate sql.NullString
		var recurrence sql.NullString
		var activeStart sql.NullTime
		var timeSpent sql.NullInt64
		var completedAt sql.NullTime

		query := `SELECT 
			id, title, description, project, status, created_at, 
			completed_at, due_at, estimate, 
			recurrence, active_start, time_spent, reschedule_count 
		FROM tasks WHERE id = ?`

		row := db.DB.QueryRow(query, id)
		err = row.Scan(
			&t.ID, &t.Title, &description, &project, &t.Status, &t.CreatedAt,
			&completedAt, &dueAt, &estimate,
			&recurrence, &activeStart, &timeSpent, &t.RescheduleCount,
		)

		if err == sql.ErrNoRows {
			fmt.Printf("Task %d not found.\n", id)
			return
		} else if err != nil {
			fmt.Printf("Error fetching task: %v\n", err)
			return
		}

		t.Description = description.String
		t.Project = project.String
		t.Estimate = estimate.String
		t.Recurrence = recurrence.String
		if dueAt.Valid {
			t.DueAt = &dueAt.Time
		}
		if activeStart.Valid {
			t.ActiveStart = &activeStart.Time
		}
		if timeSpent.Valid {
			t.TimeSpent = timeSpent.Int64
		}
		if completedAt.Valid {
			t.CompletedAt = &completedAt.Time
		}

		type taskRef struct {
			ID    int64  `json:"id"`
			Title string `json:"title"`
		}

		// 2. Fetch Dependencies
		// Blocked By
		blockedByRows, err := db.DB.Query(`
			SELECT t.id, t.title 
			FROM task_dependencies td 
			JOIN tasks t ON td.blocker_id = t.id 
			WHERE td.blocked_id = ?`, id)
		var blockedBy []taskRef
		if err == nil {
			defer blockedByRows.Close()
			for blockedByRows.Next() {
				var bID int64
				var bTitle string
				if err := blockedByRows.Scan(&bID, &bTitle); err == nil {
					blockedBy = append(blockedBy, taskRef{ID: bID, Title: bTitle})
				}
			}
		}

		// Blocking
		blockingRows, err := db.DB.Query(`
			SELECT t.id, t.title 
			FROM task_dependencies td 
			JOIN tasks t ON td.blocked_id = t.id 
			WHERE td.blocker_id = ?`, id)
		var blocking []taskRef
		if err == nil {
			defer blockingRows.Close()
			for blockingRows.Next() {
				var bID int64
				var bTitle string
				if err := blockingRows.Scan(&bID, &bTitle); err == nil {
					blocking = append(blocking, taskRef{ID: bID, Title: bTitle})
				}
			}
		}

		totalDuration := t.TimeSpent
		if t.ActiveStart != nil {
			totalDuration += int64(time.Since(*t.ActiveStart).Seconds())
		}

		if outputFormat == OutputJSON {
			payload := struct {
				Task             models.Task `json:"task"`
				BlockedBy        []taskRef   `json:"blocked_by"`
				Blocking         []taskRef   `json:"blocking"`
				TimeSpentSeconds int64       `json:"time_spent_seconds"`
			}{
				Task:             t,
				BlockedBy:        blockedBy,
				Blocking:         blocking,
				TimeSpentSeconds: totalDuration,
			}

			data, err := json.MarshalIndent(payload, "", "  ")
			if err != nil {
				fmt.Printf("Error encoding JSON output: %v\n", err)
				return
			}
			fmt.Println(string(data))
			return
		}

		// 3. Display
		t.IsBlocked = len(blockedBy) > 0
		style := ui.GetTaskStyle(t, cfg)

		fmt.Println(style.Render(fmt.Sprintf("Task %d: %s", t.ID, t.Title)))
		if t.Description != "" {
			fmt.Println("------------------------------------------------")
			fmt.Println(t.Description)
		}
		fmt.Println("------------------------------------------------")
		fmt.Printf("Project:       %s\n", t.Project)
		fmt.Printf("Status:        %s\n", t.Status)
		fmt.Printf("Created:       %s\n", t.CreatedAt.Format("2006-01-02 15:04"))

		if t.CompletedAt != nil {
			fmt.Printf("Completed:     %s\n", t.CompletedAt.Format("2006-01-02 15:04"))
		}

		dueStr := "-"
		if t.DueAt != nil {
			dueStr = ui.FormatDate(*t.DueAt, cfg)
		}
		fmt.Printf("Due:           %s\n", dueStr)

		if t.RescheduleCount > 0 {
			fmt.Printf("Rescheduled:   %d times\n", t.RescheduleCount)
		}

		if t.Estimate != "" {
			fmt.Printf("Estimate:      %s\n", t.Estimate)
		}

		if t.Recurrence != "" {
			fmt.Printf("Recurrence:    %s\n", t.Recurrence)
		}

		// Duration calculation
		if t.ActiveStart != nil {
			fmt.Printf("Active since:  %s\n", t.ActiveStart.Format("15:04:05"))
		}
		if totalDuration > 0 {
			dur := time.Duration(totalDuration) * time.Second
			fmt.Printf("Time Spent:    %s\n", dur.String())
		}

		// Dependencies display
		if len(blockedBy) > 0 {
			fmt.Println()
			fmt.Println("Blocked By:")
			for _, b := range blockedBy {
				fmt.Printf("  - %d (%s)\n", b.ID, b.Title)
			}
		}

		if len(blocking) > 0 {
			fmt.Println()
			fmt.Println("Blocking:")
			for _, b := range blocking {
				fmt.Printf("  - %d (%s)\n", b.ID, b.Title)
			}
		}
		fmt.Println()
	},
}

func init() {
	showCmd.Flags().StringVar(&showOutput, "output", OutputTable, "Output format (table, json)")
	showCmd.RegisterFlagCompletionFunc("output", outputFormatCompletionFunc)
	rootCmd.AddCommand(showCmd)
}
