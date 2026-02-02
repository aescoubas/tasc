package cmd

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/aescoubas/tasc/internal/config"
	"github.com/aescoubas/tasc/internal/db"
	"github.com/aescoubas/tasc/internal/models"
	"github.com/aescoubas/tasc/internal/scheduling"
	"github.com/aescoubas/tasc/internal/ui"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var (
	calNext bool
	calView string
)

var calendarCmd = &cobra.Command{
	Use:   "calendar",
	Short: "Show a calendar view of tasks",
	Run: func(cmd *cobra.Command, args []string) {
		cfg, _ := config.LoadConfig()

		now := time.Now()
		// Adjust 'now' based on --next flag (simple shift)
		// We define 'focusDate' which determines the range.
		focusDate := now

		// Calculate range based on View
		var start, end time.Time
		view := strings.ToLower(calView)

		switch view {
		case "day":
			if calNext {
				focusDate = focusDate.AddDate(0, 0, 1)
			}
			start = time.Date(focusDate.Year(), focusDate.Month(), focusDate.Day(), 0, 0, 0, 0, focusDate.Location())
			end = start.AddDate(0, 0, 1)
		case "month":
			if calNext {
				focusDate = focusDate.AddDate(0, 1, 0)
			}
			start = time.Date(focusDate.Year(), focusDate.Month(), 1, 0, 0, 0, 0, focusDate.Location())
			end = start.AddDate(0, 1, 0)
		case "quarter":
			if calNext {
				focusDate = focusDate.AddDate(0, 3, 0)
			}
			month := focusDate.Month()
			qStartMonth := time.Month(((int(month)-1)/3)*3 + 1)
			start = time.Date(focusDate.Year(), qStartMonth, 1, 0, 0, 0, 0, focusDate.Location())
			end = start.AddDate(0, 3, 0)
		case "year":
			if calNext {
				focusDate = focusDate.AddDate(1, 0, 0)
			}
			start = time.Date(focusDate.Year(), 1, 1, 0, 0, 0, 0, focusDate.Location())
			end = start.AddDate(1, 0, 0)
		case "week":
			fallthrough
		default:
			// Default to Week
			view = "week"
			if calNext {
				focusDate = focusDate.AddDate(0, 0, 7)
			}
			weekday := int(focusDate.Weekday())
			if weekday == 0 {
				weekday = 7
			}
			start = focusDate.AddDate(0, 0, -(weekday - 1))
			start = time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, start.Location())
			end = start.AddDate(0, 0, 7)
		}

		// Fetch Tasks
		tasks, err := fetchTasksInRange(start, end)
		if err != nil {
			fmt.Printf("Error fetching tasks: %v\n", err)
			return
		}

		// Apply Smart Auto-Schedule (Virtual Times)
		tasks = scheduling.ApplyAutoSchedule(tasks)

		// Render
		width, _, err := term.GetSize(int(os.Stdout.Fd()))
		if err != nil || width <= 0 {
			width = 80
		}

		fmt.Printf("Calendar View: %s (%s - %s)\n", strings.Title(view), start.Format("2006-01-02"), end.AddDate(0, 0, -1).Format("2006-01-02"))

		switch view {
		case "day":
			renderDayView(tasks, start, width, cfg)
		case "week":
			renderWeekView(tasks, start, width, cfg, now)
		case "month":
			renderMonthView(tasks, start, width, cfg, now)
		case "quarter":
			renderQuarterView(tasks, start, width, cfg, now)
		case "year":
			renderYearView(tasks, start, width, cfg, now)
		}
	},
}

func fetchTasksInRange(start, end time.Time) ([]models.Task, error) {
	query := `
		SELECT 
			id, description, project, status, due_at, scheduled_at, estimate,
			EXISTS(SELECT 1 FROM task_dependencies WHERE blocked_id = tasks.id) as is_blocked
		FROM tasks 
		WHERE status NOT IN ('done', 'deleted', 'undefined') 
		AND (
			(scheduled_at >= ? AND scheduled_at < ?)
			OR 
			(scheduled_at IS NULL AND due_at >= ? AND due_at < ?)
		)
		ORDER BY scheduled_at, due_at
	`
	sDate := start.Format("2006-01-02")
	eDate := end.Format("2006-01-02")

	rows, err := db.DB.Query(query, sDate, eDate, sDate, eDate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []models.Task
	for rows.Next() {
		var t models.Task
		var dueAt, scheduledAt *time.Time
		var est string
		var isBlocked bool
		err := rows.Scan(&t.ID, &t.Description, &t.Project, &t.Status, &dueAt, &scheduledAt, &est, &isBlocked)
		if err != nil {
			continue
		}
		t.DueAt = dueAt
		t.ScheduledAt = scheduledAt
		t.Estimate = est
		t.IsBlocked = isBlocked
		tasks = append(tasks, t)
	}
	return tasks, nil
}

func renderDayView(tasks []models.Task, date time.Time, width int, cfg config.Config) {
	if len(tasks) == 0 {
		fmt.Println("No tasks scheduled.")
		return
	}
	for _, t := range tasks {
		renderTaskLine(t, width, cfg)
	}
}

func renderWeekView(tasks []models.Task, startOfWeek time.Time, width int, cfg config.Config, now time.Time) {
	// Bucket tasks
	dayTasks := make([][]models.Task, 7)
	for _, t := range tasks {
		targetTime := getTaskDate(t)
		daysDiff := int(targetTime.Sub(startOfWeek).Hours() / 24)
		if daysDiff >= 0 && daysDiff < 7 {
			dayTasks[daysDiff] = append(dayTasks[daysDiff], t)
		}
	}

	colWidth := (width / 7) - 2
	if colWidth < 10 {
		colWidth = 10
	}

	var columns []string
	
	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(cfg.Colors.Default)).
		Align(lipgloss.Center).
		Width(colWidth).
		PaddingBottom(1).
		Border(lipgloss.NormalBorder(), false, false, true, false).
		BorderForeground(lipgloss.Color("240"))

	for i := 0; i < 7; i++ {
		dayDate := startOfWeek.AddDate(0, 0, i)
		dayName := dayDate.Format("Mon 02")
		
		isToday := isSameDay(dayDate, now)
		hStyle := headerStyle.Copy()
		if isToday {
			hStyle = hStyle.Foreground(lipgloss.Color(cfg.Colors.Today)).Bold(true)
		}

		var cells []string
		cells = append(cells, hStyle.Render(dayName))

		for _, t := range dayTasks[i] {
			cells = append(cells, renderTaskBox(t, colWidth, cfg))
		}
		
		colContent := lipgloss.JoinVertical(lipgloss.Left, cells...)
		columns = append(columns, colContent)
	}
	
	fmt.Println(lipgloss.JoinHorizontal(lipgloss.Top, columns...))
}

func renderMonthView(tasks []models.Task, startOfMonth time.Time, width int, cfg config.Config, now time.Time) {
	// Simple Month Grid
	// 7 Columns
	colWidth := (width / 7) - 2
	if colWidth < 8 {
		colWidth = 8
	}

	// Calculate offset for first day
	startWeekday := int(startOfMonth.Weekday()) // Sun=0
	if startWeekday == 0 { startWeekday = 7 }
	offset := startWeekday - 1 // Mon=0

	// Total days in month
	nextMonth := startOfMonth.AddDate(0, 1, 0)
	daysInMonth := int(nextMonth.Sub(startOfMonth).Hours() / 24)

	// Bucket tasks
	dayTasks := make(map[int][]models.Task)
	for _, t := range tasks {
		d := getTaskDate(t)
		if d.Month() == startOfMonth.Month() {
			dayTasks[d.Day()] = append(dayTasks[d.Day()], t)
		}
	}

	// Styles
	cellStyle := lipgloss.NewStyle().
		Width(colWidth).
		Height(5). // Fixed height for grid uniformity? Or variable? 
		           // Variable height rows in calendar grid is hard in terminal without advanced layout.
				   // Let's try fixed height or just min height.
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("238"))
	
	headerStyle := lipgloss.NewStyle().Width(colWidth).Align(lipgloss.Center).Bold(true)

	// Headers
	days := []string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"}
	var headers []string
	for _, d := range days {
		headers = append(headers, headerStyle.Render(d))
	}
	fmt.Println(lipgloss.JoinHorizontal(lipgloss.Top, headers...))

	// Grid
	currentDay := 1
	for row := 0; currentDay <= daysInMonth; row++ {
		var rowCells []string
		for col := 0; col < 7; col++ {
			if (row == 0 && col < offset) || currentDay > daysInMonth {
				// Empty cell
				rowCells = append(rowCells, cellStyle.Render(""))
				continue
			}

			// Content
			var content strings.Builder
			dayDate := time.Date(startOfMonth.Year(), startOfMonth.Month(), currentDay, 0,0,0,0, startOfMonth.Location())
			
			dateStr := fmt.Sprintf("%d", currentDay)
			if isSameDay(dayDate, now) {
				dateStr = lipgloss.NewStyle().Foreground(lipgloss.Color(cfg.Colors.Today)).Bold(true).Render(dateStr)
			}
			content.WriteString(dateStr + "\n")

			// Tasks (tiny representation)
			ts := dayTasks[currentDay]
			for i, t := range ts {
				if i >= 3 {
					content.WriteString(fmt.Sprintf("+%d more", len(ts)-3))
					break
				}
				// Tiny: "• Desc"
				desc := t.Description
				if len(desc) > colWidth-3 {
					desc = desc[:colWidth-3]
				}
				
				icon := "•"
				if t.IsBlocked {
					icon = "🚫"
				} else if t.Status == "ongoing" {
					icon = "▶"
				} else if t.Status == "done" {
					icon = "✓"
				}

				// Color dot by project?
				style := ui.GetTaskStyle(t, cfg)
				content.WriteString(style.Render(fmt.Sprintf("%s %s", icon, desc)) + "\n")
			}

			rowCells = append(rowCells, cellStyle.Render(content.String()))
			currentDay++
		}
		fmt.Println(lipgloss.JoinHorizontal(lipgloss.Top, rowCells...))
	}
}

func renderQuarterView(tasks []models.Task, startOfQuarter time.Time, width int, cfg config.Config, now time.Time) {
	// Grid: 4 columns
	colWidth := (width / 4) - 2
	if colWidth < 15 {
		colWidth = 15
	}
	
	// Bucket tasks by Week (relative to startOfQuarter)
	// We'll just iterate 13-14 weeks
	weekTasks := make(map[int][]models.Task)
	
	// Determine "Standard" Weeks starting Monday
	// Align startOfQuarter to the preceding Monday
	qStartMonday := startOfQuarter
	for qStartMonday.Weekday() != time.Monday {
		qStartMonday = qStartMonday.AddDate(0, 0, -1)
	}

	for _, t := range tasks {
		d := getTaskDate(t)
		// Find which week index this task belongs to
		// Days since qStartMonday
		diff := int(d.Sub(qStartMonday).Hours() / 24)
		if diff < 0 { continue }
		weekIdx := diff / 7
		weekTasks[weekIdx] = append(weekTasks[weekIdx], t)
	}

	cellStyle := lipgloss.NewStyle().
		Width(colWidth).
		Height(6). // Enough for header + a few tasks
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("238"))

	var rows []string
	
	// 4 columns per row
	// Usually 13-14 weeks in a quarter
	weeksInQuarter := 14 
	
	for i := 0; i < weeksInQuarter; i += 4 {
		var rowCells []string
		for j := 0; j < 4; j++ {
			weekIdx := i + j
			if weekIdx >= weeksInQuarter {
				rowCells = append(rowCells, cellStyle.Render(""))
				continue
			}
			
			// Week Start Date
			wStart := qStartMonday.AddDate(0, 0, weekIdx*7)
			wEnd := wStart.AddDate(0, 0, 6)
			
			// Header
			header := fmt.Sprintf("W%d: %s", weekIdx+1, wStart.Format("Jan 02"))
			if wStart.Month() != wEnd.Month() {
				header = fmt.Sprintf("W%d: %s-%s", weekIdx+1, wStart.Format("Jan 02"), wEnd.Format("02"))
			}
			
			// Highlight if "Now" is in this week
			if now.After(wStart) && now.Before(wEnd.AddDate(0,0,1)) {
				header = lipgloss.NewStyle().Foreground(lipgloss.Color(cfg.Colors.Today)).Bold(true).Render(header)
			}
			
			var content strings.Builder
			content.WriteString(header + "\n")
			content.WriteString(strings.Repeat("-", colWidth-2) + "\n")
			
			ts := weekTasks[weekIdx]
			content.WriteString(fmt.Sprintf("Total: %d\n", len(ts)))
			
			// List top 3
			for k, t := range ts {
				if k >= 3 { break }
				desc := t.Description
				if len(desc) > colWidth-5 {
					desc = desc[:colWidth-5]
				}
				
				icon := "•"
				if t.IsBlocked {
					icon = "🚫"
				} else if t.Status == "ongoing" {
					icon = "▶"
				} else if t.Status == "done" {
					icon = "✓"
				}

				style := ui.GetTaskStyle(t, cfg)
				content.WriteString(style.Render(fmt.Sprintf("%s %s", icon, desc)) + "\n")
			}
			
			rowCells = append(rowCells, cellStyle.Render(content.String()))
		}
		rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, rowCells...))
	}
	
	fmt.Println(lipgloss.JoinVertical(lipgloss.Left, rows...))
}

func renderYearView(tasks []models.Task, startOfYear time.Time, width int, cfg config.Config, now time.Time) {
	// Grid: 4 columns x 3 rows = 12 months
	colWidth := (width / 4) - 2
	if colWidth < 15 {
		colWidth = 15
	}
	
	// Bucket by Month (1-12)
	monthTasks := make(map[time.Month][]models.Task)
	for _, t := range tasks {
		d := getTaskDate(t)
		monthTasks[d.Month()] = append(monthTasks[d.Month()], t)
	}

	cellStyle := lipgloss.NewStyle().
		Width(colWidth).
		Height(5).
		Border(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("238"))

	var rows []string
	
	for i := 1; i <= 12; i += 4 {
		var rowCells []string
		for j := 0; j < 4; j++ {
			monthIdx := i + j
			if monthIdx > 12 { break }
			
			m := time.Month(monthIdx)
			ts := monthTasks[m]
			
			header := m.String()
			if now.Month() == m && now.Year() == startOfYear.Year() {
				header = lipgloss.NewStyle().Foreground(lipgloss.Color(cfg.Colors.Today)).Bold(true).Render(header)
			}
			
			var content strings.Builder
			content.WriteString(header + "\n")
			content.WriteString(strings.Repeat("-", colWidth-2) + "\n")
			content.WriteString(fmt.Sprintf("Tasks: %d\n", len(ts)))
			
			// Maybe a mini bar chart? "||||"
			bars := len(ts)
			if bars > colWidth - 2 { bars = colWidth - 2 }
			if bars > 0 {
				content.WriteString(strings.Repeat("■", bars))
			}
			
			rowCells = append(rowCells, cellStyle.Render(content.String()))
		}
		rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Top, rowCells...))
	}
	fmt.Println(lipgloss.JoinVertical(lipgloss.Left, rows...))
}

func renderTaskBox(t models.Task, width int, cfg config.Config) string {
	projStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(cfg.Colors.Default))
	baseStyle := ui.GetTaskStyle(t, cfg)
	
	taskStyle := baseStyle.Copy().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")).
		Padding(0, 1).
		Width(width - 2)

	line1 := fmt.Sprintf("%d %s", t.ID, t.Project)
	if len(line1) > width-4 {
		line1 = line1[:width-4]
	}
	
	desc := strings.ReplaceAll(t.Description, "\n", " ")
	if len(desc) > width-6 {
		desc = desc[:width-6] + ".."
	}
	
	statusChar := " "
	if t.IsBlocked {
		statusChar = "🚫"
	} else if t.Status == "done" {
		statusChar = "✓"
	} else if t.Status == "ongoing" {
		statusChar = "▶"
	}

	line2 := fmt.Sprintf("%s %s", statusChar, desc)
	return taskStyle.Render(fmt.Sprintf("%s\n%s", projStyle.Render(line1), line2))
}

func renderTaskLine(t models.Task, width int, cfg config.Config) {
	fmt.Println(ui.FormatTask(t, cfg))
}

func getTaskDate(t models.Task) time.Time {
	if t.ScheduledAt != nil {
		return *t.ScheduledAt
	}
	if t.DueAt != nil {
		return *t.DueAt
	}
	return time.Time{}
}

func isSameDay(t1, t2 time.Time) bool {
	return t1.Year() == t2.Year() && t1.Month() == t2.Month() && t1.Day() == t2.Day()
}

func init() {
	calendarCmd.Flags().BoolVarP(&calNext, "next", "n", false, "Show next period")
	calendarCmd.Flags().StringVarP(&calView, "view", "v", "week", "View mode: day, week, month, quarter, year")
	rootCmd.AddCommand(calendarCmd)
}
