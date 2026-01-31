package cmd

import (
	"database/sql"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/aescoubas/tasc/internal/config"
	"github.com/aescoubas/tasc/internal/db"
	"github.com/aescoubas/tasc/internal/models"
	"github.com/aescoubas/tasc/internal/priority"
	"github.com/aescoubas/tasc/internal/ui"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

type taskWithScore struct {
	task  models.Task
	score float64
}

var (
	sortBy   string
	sortDesc bool
	showAll  bool
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

		rows, err := db.DB.Query(`
			SELECT 
				id, description, project, status, created_at, due_at, scheduled_at, estimate, active_start, time_spent, reschedule_count,
				EXISTS(SELECT 1 FROM task_dependencies WHERE blocked_id = tasks.id) as is_blocked
			FROM tasks 
			WHERE status NOT IN ('done', 'deleted', 'undefined')
		`)
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
			var isBlocked bool

			err := rows.Scan(&t.ID, &t.Description, &project, &t.Status, &t.CreatedAt, &dueAt, &scheduledAt, &estimate, &activeStart, &timeSpent, &t.RescheduleCount, &isBlocked)
			if err != nil {
				fmt.Printf("Error scanning row: %v\n", err)
				continue
			}
			t.Project = project.String
			t.IsBlocked = isBlocked

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
			case "age":
				// Sort by age: older tasks (smaller CreatedAt) are "greater" in age?
				// Usually "sort by age" means "oldest first" or "newest first".
				// If we stick to "less" meaning "comes first":
				// If we want shortest age first (newest): CreatedAt descending (t1 > t2)
				// If we want longest age first (oldest): CreatedAt ascending (t1 < t2)
				// Let's assume standard ascending sort = smallest age (newest) first?
				// Or does "sort by age" mean "oldest first"?
				// Let's alias "age" to "created_at" which is Oldest First (Ascending date).
				// Wait, CreatedAt ascending means Oldest Date (e.g. 2020) < Newest Date (2025).
				// So sort by CreatedAt ASC puts Oldest at top.
				// Age: Oldest has BIG age. Newest has SMALL age.
				// If I sort by Age ASC (Smallest Age first), I want Newest First.
				// So Age ASC == CreatedAt DESC.
				less = t2.CreatedAt.Before(t1.CreatedAt)
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
			status   string
			desc     string
			created  string
			age      string
			due      string
			sch      string
			rsch     string
			est      string
			score    string
			duration string
			tier     ui.UrgencyTier
		}

		var rowsData []displayRow
		widths := make([]int, 12) // 12 columns
		headers := []string{"ID", "Project", "Status", "Description", "Created", "Age", "Due", "Scheduled", "Rsch", "Est", "Score", "Duration"}

		// Initialize widths with headers
		for i, h := range headers {
			widths[i] = len(h)
		}

		for _, item := range tasks {
			t := item.task

			// Determine Urgency
			finalTier := ui.GetUrgencyTier(t)

			// Format Fields
			dueStr := "-"
			if t.DueAt != nil {
				dueStr = t.DueAt.Format("2006-01-02")
			}

			schStr := "-"
			if t.ScheduledAt != nil {
				schStr = t.ScheduledAt.Format("2006-01-02")
			}

			rschStr := "-"
			if t.RescheduleCount > 0 {
				rschStr = fmt.Sprintf("%d", t.RescheduleCount)
			}

			estStr := "-"
			if t.Estimate != "" {
				estStr = t.Estimate
			}

			desc := strings.ReplaceAll(t.Description, "\n", " ")
			if len(desc) > 50 {
				desc = desc[:47] + "..."
			}
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

			projectStr := t.Project
			if len(projectStr) > 20 {
				projectStr = projectStr[:17] + "..."
			}
			
			statusStr := string(t.Status)

			row := displayRow{
				id:       fmt.Sprintf("%d", t.ID),
				project:  projectStr,
				status:   statusStr,
				desc:     desc,
				created:  t.CreatedAt.Format("2006-01-02"),
				age:      t.AgeString(),
				due:      dueStr,
				sch:      schStr,
				rsch:     rschStr,
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
			if len(row.status) > widths[2] {
				widths[2] = len(row.status)
			}
			if len(row.desc) > widths[3] {
				widths[3] = len(row.desc)
			}
			if len(row.created) > widths[4] {
				widths[4] = len(row.created)
			}
			if len(row.age) > widths[5] {
				widths[5] = len(row.age)
			}
			if len(row.due) > widths[6] {
				widths[6] = len(row.due)
			}
			if len(row.sch) > widths[7] {
				widths[7] = len(row.sch)
			}
			if len(row.rsch) > widths[8] {
				widths[8] = len(row.rsch)
			}
			if len(row.est) > widths[9] {
				widths[9] = len(row.est)
			}
			if len(row.score) > widths[10] {
				widths[10] = len(row.score)
			}
			if len(row.duration) > widths[11] {
				widths[11] = len(row.duration)
			}
		}

		// Calculate total table width
		totalWidth := 0
		for _, w := range widths {
			totalWidth += w
		}
		totalWidth += (len(widths) - 1) * 3 // Gaps

		// Check terminal constraints for Width
		width, height, err := term.GetSize(int(os.Stdout.Fd()))
		if err == nil && width > 0 {
			if totalWidth > width {
				// Reduce Description width
				// widths[3] is Desc
				fixed := totalWidth - widths[3]
				avail := width - fixed
				
				if avail < 10 {
					avail = 10 // Minimum sensible width
				}

				if avail < widths[3] {
					widths[3] = avail
					// Re-truncate descriptions
					for k, row := range rowsData {
						if len(row.desc) > avail {
							if avail > 3 {
								rowsData[k].desc = row.desc[:avail-3] + "..."
							} else {
								rowsData[k].desc = row.desc[:avail]
							}
						}
					}
					// Recalculate totalWidth for height logic
					totalWidth = fixed + avail
				}
			}
		}

		// Limit output to screen height (accounting for wrapping)
		if !showAll {
			if err == nil && height > 0 {
				linesPerTask := 1
				if width > 0 {
					// Ceiling division
					linesPerTask = (totalWidth + width - 1) / width
				}
				
				// Calculate header lines
				headerLines := 1
				if width > 0 {
					headerLines = (totalWidth + width - 1) / width
				}

				// Available lines for tasks (Reserve 4 lines for prompt/safety)
				availableLines := height - 4 - headerLines
				if availableLines < 0 {
					availableLines = 0
				}

				tasksToShow := 0
				if linesPerTask > 0 {
					tasksToShow = availableLines / linesPerTask
				}

				if tasksToShow < len(rowsData) {
					rowsData = rowsData[:tasksToShow]
				}
			}
		}

		// Render
		// Styles
		styles := make(map[ui.UrgencyTier]lipgloss.Style)
		
		styles[ui.TierOverdue] = ui.GetStyleForTier(ui.TierOverdue, cfg)
		styles[ui.TierToday] = ui.GetStyleForTier(ui.TierToday, cfg)
		styles[ui.TierTomorrow] = ui.GetStyleForTier(ui.TierTomorrow, cfg)
		styles[ui.TierWeek] = ui.GetStyleForTier(ui.TierWeek, cfg)
		styles[ui.TierDefault] = ui.GetStyleForTier(ui.TierDefault, cfg)

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

			// Helper to print a column
			printCol := func(text string, width int, isLast bool) {
				padded := fmt.Sprintf("%-*s", width, text)
				fmt.Print(style.Render(padded))
				if !isLast {
					fmt.Print(style.Render("   ")) // 3 spaces gap with style
				}
			}

			printCol(row.id, widths[0], false)
			printCol(row.project, widths[1], false)
			printCol(row.status, widths[2], false)
			printCol(row.desc, widths[3], false)
			printCol(row.created, widths[4], false)
			printCol(row.age, widths[5], false)
			printCol(row.due, widths[6], false)
			printCol(row.sch, widths[7], false)
			printCol(row.rsch, widths[8], false)
			printCol(row.est, widths[9], false)
			printCol(row.score, widths[10], false)
			printCol(row.duration, widths[11], true)
			fmt.Println()
		}
	},
}

func init() {
	listCmd.Flags().StringVarP(&sortBy, "sort", "s", "", "Sort by field (id, project, description, created, age, due, scheduled, estimate, score, duration)")
	listCmd.Flags().BoolVarP(&sortDesc, "desc", "d", false, "Sort in descending order")
	listCmd.Flags().BoolVarP(&showAll, "all", "a", false, "Show all tasks (do not truncate to screen height)")
	rootCmd.AddCommand(listCmd)
}
