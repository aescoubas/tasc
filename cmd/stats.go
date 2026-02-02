package cmd

import (
	"database/sql"
	"fmt"
	"math"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/aescoubas/tasc/internal/db"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

var (
	statsDisplay string

	titleStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#FAFAFA")).
		Background(lipgloss.Color("#7D56F4")).
		Padding(0, 1).
		MarginBottom(1)

	sectionStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("#04B575")).
		MarginTop(1).
		MarginBottom(0)
)

var statsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show enhanced productivity metrics",
	Run: func(cmd *cobra.Command, args []string) {
		// Fetch all components
		closure := renderClosureRates()
		status := renderStatusBreakdown()
		timeAlloc := renderTimeAllocation()
		stale := renderStaleTasks()
		friction := renderFrictionAnalysis()
		est := renderEstimationAccuracy()
		burndown := renderBurndown()

		if statsDisplay == "list" {
			fmt.Println(titleStyle.Render("Enhanced Productivity Metrics"))
			fmt.Println(closure)
			fmt.Println(timeAlloc)
			fmt.Println(status)
			fmt.Println(stale)
			fmt.Println(friction)
			fmt.Println(est)
			fmt.Println(burndown)
		} else {
			// Dashboard View
			fmt.Println(titleStyle.Render("Productivity Dashboard"))

			// Layout:
			// Row 1: Closure | Status | Estimation
			// Row 2: Time Alloc | Stale | Friction
			// Row 3: Burndown

			// Helper to box content
			box := lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("238")).
				Padding(0, 1).
				MarginRight(1)

			row1 := lipgloss.JoinHorizontal(lipgloss.Top,
				box.Render(closure),
				box.Render(status),
				box.Render(est),
			)

			row2 := lipgloss.JoinHorizontal(lipgloss.Top,
				box.Render(timeAlloc),
				box.Render(stale),
				box.Render(friction),
			)

			// Burndown full width (boxed)
			row3 := box.Copy().MarginTop(1).Render(burndown)

			fmt.Println(row1)
			fmt.Println(row2)
			fmt.Println(row3)
		}
	},
}

func renderClosureRates() string {
	var sb strings.Builder
	sb.WriteString(sectionStyle.Render("Closure Rates") + "\n")

	queries := map[string]string{
		"Last 7 Days":  "SELECT COUNT(*) FROM tasks WHERE status = 'done' AND completed_at >= datetime('now', '-7 days')",
		"Last 30 Days": "SELECT COUNT(*) FROM tasks WHERE status = 'done' AND completed_at >= datetime('now', '-30 days')",
		"All Time":     "SELECT COUNT(*) FROM tasks WHERE status = 'done'",
	}

	labels := []string{"Last 7 Days", "Last 30 Days", "All Time"}

	w := tabwriter.NewWriter(&sb, 0, 0, 2, ' ', 0)
	for _, label := range labels {
		var count int
		err := db.DB.QueryRow(queries[label]).Scan(&count)
		if err != nil {
			continue
		}
		fmt.Fprintf(w, "%s:\t%d\n", label, count)
	}
	w.Flush()
	return sb.String()
}

func renderTimeAllocation() string {
	var sb strings.Builder
	sb.WriteString(sectionStyle.Render("Time Allocation") + "\n")

	query := `SELECT project, SUM(time_spent) FROM tasks WHERE time_spent > 0 GROUP BY project ORDER BY SUM(time_spent) DESC LIMIT 10`
	rows, err := db.DB.Query(query)
	if err != nil {
		return "Error loading data"
	}
	defer rows.Close()

	w := tabwriter.NewWriter(&sb, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "Project\tDuration")
	fmt.Fprintln(w, "-------\t--------")

	var totalSeconds int64

	for rows.Next() {
		var project string
		var seconds int64
		if err := rows.Scan(&project, &seconds); err != nil {
			continue
		}
		if project == "" {
			project = "[No Project]"
		}
		dur := time.Duration(seconds) * time.Second
		// Truncate project name if too long for dashboard
		if len(project) > 15 {
			project = project[:12] + "..."
		}
		fmt.Fprintf(w, "%s\t%s\n", project, dur.String())
		totalSeconds += seconds
	}
	w.Flush()
	return sb.String()
}

func renderStatusBreakdown() string {
	var sb strings.Builder
	sb.WriteString(sectionStyle.Render("Status Breakdown") + "\n")

	query := `SELECT status, COUNT(*) FROM tasks GROUP BY status ORDER BY COUNT(*) DESC`
	rows, err := db.DB.Query(query)
	if err != nil {
		return "Error loading data"
	}
	defer rows.Close()

	w := tabwriter.NewWriter(&sb, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "Status\tCount")
	fmt.Fprintln(w, "------\t-----")

	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			continue
		}
		fmt.Fprintf(w, "%s\t%d\n", strings.Title(status), count)
	}
	w.Flush()
	return sb.String()
}

func renderStaleTasks() string {
	var sb strings.Builder
	sb.WriteString(sectionStyle.Render("Stale Tasks (>30d)") + "\n")

	query := `
		SELECT id, title, created_at 
		FROM tasks 
		WHERE status != 'done' AND status != 'deleted' AND created_at < datetime('now', '-30 days')
		ORDER BY created_at ASC 
		LIMIT 5`
	

rows, err := db.DB.Query(query)
	if err != nil {
		return "Error loading data"
	}
	defer rows.Close()

	count := 0
	w := tabwriter.NewWriter(&sb, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tAge\tTask")
	fmt.Fprintln(w, "--\t---\t----")

	for rows.Next() {
		count++
		var id int
		var title string
		var created time.Time
		if err := rows.Scan(&id, &title, &created); err != nil {
			continue
		}
		
		age := time.Since(created)
		days := int(age.Hours() / 24)
		
		titleShort := title
		if len(titleShort) > 20 {
			titleShort = titleShort[:17] + "..."
		}

		fmt.Fprintf(w, "%d\t%dd\t%s\n", id, days, titleShort)
	}
	w.Flush()

	if count == 0 {
		sb.WriteString("No stale tasks! \n")
	}
	return sb.String()
}

func renderFrictionAnalysis() string {
	var sb strings.Builder
	sb.WriteString(sectionStyle.Render("Top Rescheduled") + "\n")

	query := `
		SELECT id, title, reschedule_count 
		FROM tasks 
		WHERE reschedule_count > 0 AND status != 'deleted'
		ORDER BY reschedule_count DESC 
		LIMIT 5`


rows, err := db.DB.Query(query)
	if err != nil {
		return "Error loading data"
	}
	defer rows.Close()

	count := 0
	w := tabwriter.NewWriter(&sb, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\t#\tTask")
	fmt.Fprintln(w, "--\t-\t----")

	for rows.Next() {
		count++
		var id int
		var title string
		var rCount int
		if err := rows.Scan(&id, &title, &rCount); err != nil {
			continue
		}
		
		titleShort := title
		if len(titleShort) > 20 {
			titleShort = titleShort[:17] + "..."
		}

		fmt.Fprintf(w, "%d\t%d\t%s\n", id, rCount, titleShort)
	}
	w.Flush()

	if count == 0 {
		sb.WriteString("No rescheduled tasks.\n")
	}
	return sb.String()
}

func renderEstimationAccuracy() string {
	var sb strings.Builder
	sb.WriteString(sectionStyle.Render("Estimation Accuracy") + "\n")

	query := `
		SELECT estimate, time_spent 
		FROM tasks 
		WHERE status = 'done' AND estimate IS NOT NULL AND estimate != '' AND time_spent > 0`
	

rows, err := db.DB.Query(query)
	if err != nil {
		return "Error loading data"
	}
	defer rows.Close()

	var totalDiff float64
	var count int
	var overestimateCount int
	var underestimateCount int

	for rows.Next() {
		var estStr string
		var actualSec int64
		if err := rows.Scan(&estStr, &actualSec); err != nil {
			continue
		}

		estDur, err := parseDurationString(estStr)
		if err != nil {
			continue
		}
		
		estSec := estDur.Seconds()
		if estSec == 0 { continue }

		diff := float64(actualSec) - estSec
		totalDiff += math.Abs(diff) / estSec

		if diff < 0 {
			overestimateCount++
		} else {
			underestimateCount++
		}
		count++
	}

	if count == 0 {
		sb.WriteString("No data available.\n")
		return sb.String()
	}

	avgError := (totalDiff / float64(count)) * 100
	fmt.Fprintf(&sb, "Avg Error: %.1f%%\n", avgError)
	
	tendency := "Balanced"
	if overestimateCount > underestimateCount + (count/5) {
		tendency = "Pessimistic"
	} else if underestimateCount > overestimateCount + (count/5) {
		tendency = "Optimistic"
	}
	fmt.Fprintf(&sb, "Trend:     %s\n", tendency)
	return sb.String()
}

func renderBurndown() string {
	var sb strings.Builder
	sb.WriteString(sectionStyle.Render("Burndown (Last 14 Days)") + "\n")


rows, err := db.DB.Query("SELECT created_at, completed_at FROM tasks WHERE status != 'deleted'")
	if err != nil {
		return "Error loading data"
	}
	defer rows.Close()

	type taskDates struct {
		created   time.Time
		completed *time.Time
	}
	var allTasks []taskDates

	for rows.Next() {
		var c time.Time
		var comp sql.NullTime
		if err := rows.Scan(&c, &comp); err != nil {
			continue
		}
		td := taskDates{created: c}
		if comp.Valid {
			td.completed = &comp.Time
		}
		allTasks = append(allTasks, td)
	}

	now := time.Now()
	days := 14
	counts := make([]int, days)
	maxCount := 0

	for i := 0; i < days; i++ {
		d := now.AddDate(0, 0, -(days - 1 - i))
		endOfDay := time.Date(d.Year(), d.Month(), d.Day(), 23, 59, 59, 0, d.Location())

		c := 0
		for _, t := range allTasks {
			if t.created.After(endOfDay) {
				continue
			}
			if t.completed != nil && t.completed.Before(endOfDay) {
				continue 
			}
			c++
		}
		counts[i] = c
		if c > maxCount {
			maxCount = c
		}
	}

	// Render Horizontal Bar Chart
	for i := 0; i < days; i++ {
		d := now.AddDate(0, 0, -(days - 1 - i))
		label := d.Format("Jan 02")
		
		val := counts[i]
		barLen := 0
		if maxCount > 0 {
			barLen = int((float64(val) / float64(maxCount)) * 50) 
		}
		
		bar := strings.Repeat("█", barLen)
		if barLen == 0 && val > 0 {
			bar = "▏"
		}
		
		fmt.Fprintf(&sb, "%s: %s %d\n", label, lipgloss.NewStyle().Foreground(lipgloss.Color("4")).Render(bar), val)
	}
	return sb.String()
}

func parseDurationString(s string) (time.Duration, error) {
	if s == "" {
		return 0, fmt.Errorf("empty string")
	}
	if strings.HasSuffix(s, "d") {
		valStr := strings.TrimSuffix(s, "d")
		var days float64
		_, err := fmt.Sscanf(valStr, "%f", &days)
		if err != nil {
			return 0, err
		}
		return time.Duration(days * 24 * float64(time.Hour)), nil
	}
	return time.ParseDuration(s)
}

func init() {
	rootCmd.AddCommand(statsCmd)
	statsCmd.Flags().StringVarP(&statsDisplay, "display", "d", "dashboard", "Display mode: dashboard or list")
}
