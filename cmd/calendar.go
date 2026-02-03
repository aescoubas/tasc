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
		tasks = scheduling.ApplyAutoSchedule(tasks, cfg.TimeBlocks)

		// Render
		width, _, err := term.GetSize(int(os.Stdout.Fd()))
		if err != nil || width <= 0 {
			width = 80
		}

		fmt.Printf("Calendar View: %s (%s - %s)\n", strings.Title(view), start.Format("2006-01-02"), end.AddDate(0, 0, -1).Format("2006-01-02"))

		switch view {
		case "day":
			renderTimeGrid([]time.Time{start}, tasks, width, cfg, now)
		case "week":
			var days []time.Time
			for i := 0; i < 7; i++ {
				days = append(days, start.AddDate(0, 0, i))
			}
			renderTimeGrid(days, tasks, width, cfg, now)
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
			id, title, project, status, due_at, scheduled_at, estimate,
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
		err := rows.Scan(&t.ID, &t.Title, &t.Project, &t.Status, &dueAt, &scheduledAt, &est, &isBlocked)
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

func renderTimeGrid(days []time.Time, tasks []models.Task, width int, cfg config.Config, now time.Time) {
	// 1. Setup Grid Dimensions
	// Left column: Time (e.g. "06:00") -> Fixed width 6 chars
	timeColWidth := 6
	dayColWidth := (width - timeColWidth) / len(days)
	if dayColWidth < 10 {
		dayColWidth = 10
	}

	// 2. Define Time Range (Dynamic or Fixed? Fixed for now 06:00 - 23:00)
	startHour := 6
	endHour := 23

	// 3. Bucket tasks by Day and Hour
	// Map[dayIndex][hour] -> []Task
	buckets := make(map[int]map[int][]models.Task)
	for dIdx := range days {
		buckets[dIdx] = make(map[int][]models.Task)
	}

	for _, t := range tasks {
		d := getTaskDate(t)
		
		// Find Day Index
		dayIdx := -1
		for i, day := range days {
			if isSameDay(d, day) {
				dayIdx = i
				break
			}
		}
		if dayIdx == -1 { continue }

		// Bucket by Hour
		h := d.Hour()
		if h < startHour { h = startHour } // Clamp to view? or ignore? Clamp for visibility
		if h > endHour { h = endHour }
		
		buckets[dayIdx][h] = append(buckets[dayIdx][h], t)
	}

	// 4. Render Header
	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(cfg.Colors.Default)).
		Align(lipgloss.Center).
		Width(dayColWidth).
		PaddingBottom(1).
		Border(lipgloss.NormalBorder(), false, false, true, false).
		BorderForeground(lipgloss.Color("240"))

	var headerCells []string
	// Empty cell for time column
	headerCells = append(headerCells, lipgloss.NewStyle().Width(timeColWidth).Render(""))

	for _, day := range days {
		dayName := day.Format("Mon 02")
		hStyle := headerStyle.Copy()
		if isSameDay(day, now) {
			hStyle = hStyle.Foreground(lipgloss.Color(cfg.Colors.Today)).Bold(true)
		}
		headerCells = append(headerCells, hStyle.Render(dayName))
	}
	fmt.Println(lipgloss.JoinHorizontal(lipgloss.Top, headerCells...))

	// 5. Render Time Rows
	timeStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		Width(timeColWidth).
		Align(lipgloss.Right).
		PaddingRight(1)

	for h := startHour; h <= endHour; h++ {
		var rowCells []string
		
		// Time Label
		rowCells = append(rowCells, timeStyle.Render(fmt.Sprintf("%02d:00", h)))

		// Day Columns
		for dIdx := range days {
			ts := buckets[dIdx][h]
			
			var cellContent []string
			if len(ts) > 0 {
				for _, t := range ts {
					cellContent = append(cellContent, renderCompactTaskBox(t, dayColWidth, cfg))
				}
			} else {
				// Empty placeholder to maintain column width? 
				// Lipgloss JoinVertical might collapse empty strings.
				// We can add a spacer or rely on fixed width cells if using a grid layout, 
				// but here we are joining boxes.
				// Let's add an empty box of width? No, just empty string is fine if we use JoinHorizontal correctly.
				// Actually, to ensure alignment, we might need to pad.
				// But since we print line by line, alignment depends on content width.
				// Lipgloss styles usually enforce width.
				cellContent = append(cellContent, lipgloss.NewStyle().Width(dayColWidth).Render(""))
			}
			
			// Join tasks vertically within the hour cell
			col := lipgloss.JoinVertical(lipgloss.Left, cellContent...)
			// Ensure cell has full width
			col = lipgloss.NewStyle().Width(dayColWidth).Render(col)
			
			rowCells = append(rowCells, col)
		}

		// Join Row
		fmt.Println(lipgloss.JoinHorizontal(lipgloss.Top, rowCells...))
		
		// Add a separator line? Optional, maybe too noisy.
		// fmt.Println(lipgloss.NewStyle().Foreground(lipgloss.Color("236")).Render(strings.Repeat("-", width)))
	}
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
				desc := t.Title
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
				desc := t.Title
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

func renderCompactTaskBox(t models.Task, width int, cfg config.Config) string {
	baseStyle := ui.GetTaskStyle(t, cfg)
	
	taskStyle := baseStyle.Copy().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("240")).
		Padding(0, 0).
		Width(width - 2)

	desc := strings.ReplaceAll(t.Title, "\n", " ")
	
	statusChar := ""
	if t.IsBlocked {
		statusChar = "🚫"
	} else if t.Status == "done" {
		statusChar = "✓"
	} else if t.Status == "ongoing" {
		statusChar = "▶"
	}

	// Compact format: "ID Proj Status Desc"
	content := fmt.Sprintf("%d %s %s %s", t.ID, t.Project, statusChar, desc)
	if len(content) > width-4 {
		content = content[:width-4] + ".."
	}
	
	return taskStyle.Render(content)
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
