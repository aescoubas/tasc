package cmd

import (
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/aescoubas/tasc/internal/config"
	"github.com/aescoubas/tasc/internal/db"
	"github.com/aescoubas/tasc/internal/models"
	"github.com/aescoubas/tasc/internal/priority"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

type taskWithScore struct {
	task  models.Task
	score float64
}

type urgencyTier int

const (
	tierOverdue urgencyTier = iota
	tierToday
	tierTomorrow
	tierWeek
	tierDefault
)

var (
	sortBy   string
	sortDesc bool
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List pending tasks",
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.LoadConfig()
		if err != nil {
			// Fallback to default if loading fails, maybe warn user?
			// For CLI, silence is usually golden unless verbose.
			cfg = config.DefaultConfig()
		}

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

		// Sort tasks
		sort.Slice(tasks, func(i, j int) bool {
			if sortBy == "" {
				// Default: Sort by active status then score descending
				activeI := tasks[i].task.ActiveStart != nil
				activeJ := tasks[j].task.ActiveStart != nil
				if activeI && !activeJ {
					return true
				}
				if !activeI && activeJ {
					return false
				}
				return tasks[i].score > tasks[j].score
			}

			t1 := tasks[i].task
			t2 := tasks[j].task
			var less bool

			switch strings.ToLower(sortBy) {
			case "id":
				less = t1.ID < t2.ID
			case "project":
				less = strings.ToLower(t1.Project) < strings.ToLower(t2.Project)
			case "description", "desc":
				less = strings.ToLower(t1.Description) < strings.ToLower(t2.Description)
			case "created", "created_at":
				less = t1.CreatedAt.Before(t2.CreatedAt)
			case "due", "due_at":
				if t1.DueAt == nil && t2.DueAt == nil {
					less = false
				} else if t1.DueAt == nil {
					less = false // nil > non-nil
				} else if t2.DueAt == nil {
					less = true
				} else {
					less = t1.DueAt.Before(*t2.DueAt)
				}
			case "scheduled", "scheduled_at", "sch":
				if t1.ScheduledAt == nil && t2.ScheduledAt == nil {
					less = false
				} else if t1.ScheduledAt == nil {
					less = false
				} else if t2.ScheduledAt == nil {
					less = true
				} else {
					less = t1.ScheduledAt.Before(*t2.ScheduledAt)
				}
			case "estimate", "est":
				less = t1.Estimate < t2.Estimate
			case "score":
				less = tasks[i].score < tasks[j].score
			case "duration", "time_spent":
				d1 := t1.TimeSpent
				if t1.ActiveStart != nil {
					d1 += int64(time.Since(*t1.ActiveStart).Seconds())
				}
				d2 := t2.TimeSpent
				if t2.ActiveStart != nil {
					d2 += int64(time.Since(*t2.ActiveStart).Seconds())
				}
				less = d1 < d2
			default:
				// Fallback to default sort if unknown field (Active then Score Desc)
				// Or just return false to keep stability?
				// Let's fallback to ID for stability if unknown
				less = t1.ID < t2.ID
			}

			if sortDesc {
				return !less
			}
			return less
		})

		// Prepare data for display
		type displayRow struct {
			id       string
			project  string
			desc     string
			created  string
			due      string
			sch      string
			est      string
			score    string
			duration string
			tier     urgencyTier
		}

		var rowsData []displayRow
		widths := make([]int, 9) // 9 columns
		headers := []string{"ID", "Project", "Description", "Created", "Due", "Scheduled", "Est", "Score", "Duration"}

		// Initialize widths with headers
		for i, h := range headers {
			widths[i] = len(h)
		}

		now := time.Now()
		startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		tomorrowStart := startOfDay.AddDate(0, 0, 1)
		weekStart := startOfDay.AddDate(0, 0, 2) // Start of "Next 7 days excluding tomorrow"
		weekEnd := startOfDay.AddDate(0, 0, 9)   // End of that week (today + 1 + 1 + 7)

		getTier := func(t *time.Time) urgencyTier {
			if t == nil {
				return tierDefault
			}
			if t.Before(now) {
				return tierOverdue
			}
			// Check if it's today (since we already checked < now, this covers "rest of today")
			if t.Before(tomorrowStart) {
				return tierToday
			}
			if t.Before(weekStart) { // Before (Today + 2) means Tomorrow
				return tierTomorrow
			}
			if t.Before(weekEnd) {
				return tierWeek
			}
			return tierDefault
		}

		for _, item := range tasks {
			t := item.task

			// Determine Urgency
			tierDue := getTier(t.DueAt)
			tierSch := getTier(t.ScheduledAt)

			// Use the most urgent tier (lower enum value is higher urgency)
			finalTier := tierDefault
			if tierDue < finalTier {
				finalTier = tierDue
			}
			if tierSch < finalTier {
				finalTier = tierSch
			}

			// Format Fields
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

			row := displayRow{
				id:       fmt.Sprintf("%d", t.ID),
				project:  t.Project,
				desc:     desc,
				created:  t.CreatedAt.Format("2006-01-02"),
				due:      dueStr,
				sch:      schStr,
				est:      estStr,
				score:    fmt.Sprintf("%.1f", item.score),
				duration: durStr,
				tier:     finalTier,
			}

			rowsData = append(rowsData, row)

			// Update widths
			if len(row.id) > widths[0] {
				widths[0] = len(row.id)
			}
			if len(row.project) > widths[1] {
				widths[1] = len(row.project)
			}
			if len(row.desc) > widths[2] {
				widths[2] = len(row.desc)
			}
			if len(row.created) > widths[3] {
				widths[3] = len(row.created)
			}
			if len(row.due) > widths[4] {
				widths[4] = len(row.due)
			}
			if len(row.sch) > widths[5] {
				widths[5] = len(row.sch)
			}
			if len(row.est) > widths[6] {
				widths[6] = len(row.est)
			}
			if len(row.score) > widths[7] {
				widths[7] = len(row.score)
			}
			if len(row.duration) > widths[8] {
				widths[8] = len(row.duration)
			}
		}

		// Render
		// Styles
		styles := make(map[urgencyTier]lipgloss.Style)
		styles[tierOverdue] = lipgloss.NewStyle().Foreground(lipgloss.Color(cfg.Colors.Overdue))
		styles[tierToday] = lipgloss.NewStyle().Foreground(lipgloss.Color(cfg.Colors.Today))
		styles[tierTomorrow] = lipgloss.NewStyle().Foreground(lipgloss.Color(cfg.Colors.Tomorrow))
		styles[tierWeek] = lipgloss.NewStyle().Foreground(lipgloss.Color(cfg.Colors.Week))
		styles[tierDefault] = lipgloss.NewStyle().Foreground(lipgloss.Color(cfg.Colors.Default))

		// Header
		// We pad headers manually too
		var headerStr strings.Builder
		for i, h := range headers {
			// Pad right
			fmt.Fprintf(&headerStr, "%-*s", widths[i], h)
			if i < len(headers)-1 {
				headerStr.WriteString("   ") // 3 spaces gap
			}
		}
		fmt.Println(headerStr.String())

		// Rows
		for _, row := range rowsData {
			style := styles[row.tier]

			// We print field by field to ensure padding is OUTSIDE the style (though if we style the text, padding naturally stays outside if we printf normally)
			// Actually, easiest is to pad the string first, THEN style it?
			// If we pad first: "text   " -> style("text   ") -> background color might show if we had one. Foreground is safe.
			// But if we use lipgloss.Style.Render(text), it just wraps.
			// Let's pad manually in the Printf.

			// Helper to print a column
			printCol := func(text string, width int, isLast bool) {
				padded := fmt.Sprintf("%-*s", width, text)
				fmt.Print(style.Render(padded))
				if !isLast {
					fmt.Print("   ") // 3 spaces gap (uncolored? or colored?)
					// Usually gap is uncolored.
				}
			}

			printCol(row.id, widths[0], false)
			printCol(row.project, widths[1], false)
			printCol(row.desc, widths[2], false)
			printCol(row.created, widths[3], false)
			printCol(row.due, widths[4], false)
			printCol(row.sch, widths[5], false)
			printCol(row.est, widths[6], false)
			printCol(row.score, widths[7], false)
			printCol(row.duration, widths[8], true)
			fmt.Println()
		}
	},
}

func init() {
	listCmd.Flags().StringVarP(&sortBy, "sort", "s", "", "Sort by field (id, project, description, created, due, scheduled, estimate, score, duration)")
	listCmd.Flags().BoolVarP(&sortDesc, "desc", "d", false, "Sort in descending order")
	rootCmd.AddCommand(listCmd)
}
