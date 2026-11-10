package cmd

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/aescoubas/tasc/internal/models"
	"github.com/aescoubas/tasc/internal/store"
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
	RunE: func(cmd *cobra.Command, args []string) error {
		tasks, err := CurrentStore.ListTasks(store.TaskFilter{IncludeDeleted: true})
		if err != nil {
			return fmt.Errorf("error loading statistics: %w", err)
		}
		now := time.Now()
		// Fetch all components
		closure := renderClosureRates(tasks, now)
		status := renderStatusBreakdown(tasks)
		timeAlloc := renderTimeAllocation(tasks)
		stale := renderStaleTasks(tasks, now)
		friction := renderFrictionAnalysis(tasks)
		est := renderEstimationAccuracy(tasks)
		burndown := renderBurndown(tasks, now)

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
		return nil
	},
}

func renderClosureRates(tasks []models.Task, now time.Time) string {
	var sb strings.Builder
	sb.WriteString(sectionStyle.Render("Closure Rates") + "\n")

	labels := []string{"Last 7 Days", "Last 30 Days", "All Time"}
	counts := make([]int, len(labels))
	for _, task := range tasks {
		if task.Status != models.StatusDone {
			continue
		}
		counts[2]++
		if task.CompletedAt != nil {
			if !task.CompletedAt.Before(now.Add(-7 * 24 * time.Hour)) {
				counts[0]++
			}
			if !task.CompletedAt.Before(now.Add(-30 * 24 * time.Hour)) {
				counts[1]++
			}
		}
	}

	w := tabwriter.NewWriter(&sb, 0, 0, 2, ' ', 0)
	for i, label := range labels {
		fmt.Fprintf(w, "%s:\t%d\n", label, counts[i])
	}
	w.Flush()
	return sb.String()
}

func renderTimeAllocation(tasks []models.Task) string {
	var sb strings.Builder
	sb.WriteString(sectionStyle.Render("Time Allocation") + "\n")

	secondsByProject := make(map[string]int64)
	for _, task := range tasks {
		if task.TimeSpent > 0 {
			secondsByProject[task.Project] += task.TimeSpent
		}
	}
	var projects []string
	for project := range secondsByProject {
		projects = append(projects, project)
	}
	sort.Slice(projects, func(i, j int) bool {
		if secondsByProject[projects[i]] == secondsByProject[projects[j]] {
			return projects[i] < projects[j]
		}
		return secondsByProject[projects[i]] > secondsByProject[projects[j]]
	})
	if len(projects) > 10 {
		projects = projects[:10]
	}

	w := tabwriter.NewWriter(&sb, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "Project\tDuration")
	fmt.Fprintln(w, "-------\t--------")

	for _, project := range projects {
		seconds := secondsByProject[project]
		if project == "" {
			project = "[No Project]"
		}
		dur := time.Duration(seconds) * time.Second
		// Truncate project name if too long for dashboard
		if len(project) > 15 {
			project = project[:12] + "..."
		}
		fmt.Fprintf(w, "%s\t%s\n", project, dur.String())
	}
	w.Flush()
	return sb.String()
}

func renderStatusBreakdown(tasks []models.Task) string {
	var sb strings.Builder
	sb.WriteString(sectionStyle.Render("Status Breakdown") + "\n")

	counts := make(map[models.TaskStatus]int)
	for _, task := range tasks {
		counts[task.Status]++
	}
	var statuses []models.TaskStatus
	for status := range counts {
		statuses = append(statuses, status)
	}
	sort.Slice(statuses, func(i, j int) bool {
		if counts[statuses[i]] == counts[statuses[j]] {
			return statuses[i] < statuses[j]
		}
		return counts[statuses[i]] > counts[statuses[j]]
	})

	w := tabwriter.NewWriter(&sb, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "Status\tCount")
	fmt.Fprintln(w, "------\t-----")

	for _, status := range statuses {
		fmt.Fprintf(w, "%s\t%d\n", strings.Title(string(status)), counts[status])
	}
	w.Flush()
	return sb.String()
}

func renderStaleTasks(tasks []models.Task, now time.Time) string {
	var sb strings.Builder
	sb.WriteString(sectionStyle.Render("Stale Tasks (>30d)") + "\n")

	var stale []models.Task
	for _, task := range tasks {
		if task.Status != models.StatusDone && task.Status != models.StatusDeleted && task.CreatedAt.Before(now.Add(-30*24*time.Hour)) {
			stale = append(stale, task)
		}
	}
	sort.Slice(stale, func(i, j int) bool {
		if stale[i].CreatedAt.Equal(stale[j].CreatedAt) {
			return stale[i].ID < stale[j].ID
		}
		return stale[i].CreatedAt.Before(stale[j].CreatedAt)
	})
	if len(stale) > 5 {
		stale = stale[:5]
	}

	w := tabwriter.NewWriter(&sb, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tAge\tTask")
	fmt.Fprintln(w, "--\t---\t----")

	for _, task := range stale {
		age := now.Sub(task.CreatedAt)
		days := int(age.Hours() / 24)

		titleShort := task.Title
		if len(titleShort) > 20 {
			titleShort = titleShort[:17] + "..."
		}

		fmt.Fprintf(w, "%d\t%dd\t%s\n", task.ID, days, titleShort)
	}
	w.Flush()

	if len(stale) == 0 {
		sb.WriteString("No stale tasks! \n")
	}
	return sb.String()
}

func renderFrictionAnalysis(tasks []models.Task) string {
	var sb strings.Builder
	sb.WriteString(sectionStyle.Render("Top Rescheduled") + "\n")

	var rescheduled []models.Task
	for _, task := range tasks {
		if task.RescheduleCount > 0 && task.Status != models.StatusDeleted {
			rescheduled = append(rescheduled, task)
		}
	}
	sort.Slice(rescheduled, func(i, j int) bool {
		if rescheduled[i].RescheduleCount == rescheduled[j].RescheduleCount {
			return rescheduled[i].ID < rescheduled[j].ID
		}
		return rescheduled[i].RescheduleCount > rescheduled[j].RescheduleCount
	})
	if len(rescheduled) > 5 {
		rescheduled = rescheduled[:5]
	}

	w := tabwriter.NewWriter(&sb, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\t#\tTask")
	fmt.Fprintln(w, "--\t-\t----")

	for _, task := range rescheduled {
		titleShort := task.Title
		if len(titleShort) > 20 {
			titleShort = titleShort[:17] + "..."
		}

		fmt.Fprintf(w, "%d\t%d\t%s\n", task.ID, task.RescheduleCount, titleShort)
	}
	w.Flush()

	if len(rescheduled) == 0 {
		sb.WriteString("No rescheduled tasks.\n")
	}
	return sb.String()
}

func renderEstimationAccuracy(tasks []models.Task) string {
	var sb strings.Builder
	sb.WriteString(sectionStyle.Render("Estimation Accuracy") + "\n")

	var totalDiff float64
	var count int
	var overestimateCount int
	var underestimateCount int

	for _, task := range tasks {
		if task.Status != models.StatusDone || task.Estimate == "" || task.TimeSpent <= 0 {
			continue
		}

		estDur, err := parseDurationString(task.Estimate)
		if err != nil {
			continue
		}

		estSec := estDur.Seconds()
		if estSec == 0 {
			continue
		}

		diff := float64(task.TimeSpent) - estSec
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
	if overestimateCount > underestimateCount+(count/5) {
		tendency = "Pessimistic"
	} else if underestimateCount > overestimateCount+(count/5) {
		tendency = "Optimistic"
	}
	fmt.Fprintf(&sb, "Trend:     %s\n", tendency)
	return sb.String()
}

func renderBurndown(tasks []models.Task, now time.Time) string {
	var sb strings.Builder
	sb.WriteString(sectionStyle.Render("Burndown (Last 14 Days)") + "\n")

	days := 14
	counts := make([]int, days)
	maxCount := 0

	for i := 0; i < days; i++ {
		d := now.AddDate(0, 0, -(days - 1 - i))
		endOfDay := time.Date(d.Year(), d.Month(), d.Day(), 23, 59, 59, 0, d.Location())

		c := 0
		for _, task := range tasks {
			if task.Status == models.StatusDeleted || task.CreatedAt.After(endOfDay) {
				continue
			}
			if task.CompletedAt != nil && task.CompletedAt.Before(endOfDay) {
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
