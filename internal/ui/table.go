package ui

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/aescoubas/tasc/internal/config"
	"github.com/aescoubas/tasc/internal/models"
	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"
)

// TableOptions controls how the task table is rendered.
type TableOptions struct {
	ShowAll       bool // If true, do not truncate to screen height
	MarkRecurring bool
}

// RenderTaskTable prints a formatted table of tasks to stdout.
func RenderTaskTable(tasks []models.Task, scores map[int64]float64, cfg config.Config, projects []models.Project, opts TableOptions) {
	// Pre-process project hierarchy
	projectParent := make(map[string]string)
	projectShortName := make(map[string]string)
	for _, p := range projects {
		if p.ShortName != "" {
			projectShortName[p.Name] = p.ShortName
		}
		if p.Parent != "" {
			projectParent[p.Name] = p.Parent
		}
	}

	type displayRow struct {
		id       string
		project  string
		status   string
		desc     string
		created  string
		age      string
		due      string
		rsch     string
		est      string
		score    string
		duration string
		tier     UrgencyTier
	}

	var rowsData []displayRow
	widths := make([]int, 11)
	headers := []string{"ID", "Project", "Status", "Title", "Created", "Age", "Due", "Rsch", "Est", "Score", "Duration"}

	for i, h := range headers {
		widths[i] = len(h)
	}

	for _, t := range tasks {
		finalTier := GetUrgencyTier(t)

		dueStr := "-"
		if t.DueAt != nil {
			if cfg.RelativeDates {
				dueStr = FormatRelative(*t.DueAt)
			} else {
				dueStr = FormatDate(*t.DueAt, cfg)
			}
		}

		rschStr := "-"
		if t.RescheduleCount > 0 {
			rschStr = fmt.Sprintf("%d", t.RescheduleCount)
		}

		estStr := "-"
		if t.Estimate != "" {
			estStr = t.Estimate
		}

		desc := strings.ReplaceAll(t.Title, "\n", " ")
		if lipgloss.Width(desc) > 50 {
			desc = lipgloss.NewStyle().MaxWidth(47).Render(desc) + "..."
		}
		duration := t.TimeSpent
		if t.ActiveStart != nil {
			desc = "ONGOING: " + desc
			duration += int64(time.Since(*t.ActiveStart).Seconds())
		}
		if opts.MarkRecurring && t.Recurrence != "" {
			desc = "↻ " + desc
		}

		durStr := "-"
		if duration > 0 {
			d := time.Duration(duration) * time.Second
			durStr = d.String()
		}

		projectStr := t.Project
		// Resolve full project path
		pathParts := []string{t.Project}
		currP := t.Project
		for {
			parent, ok := projectParent[currP]
			if !ok || parent == "" {
				break
			}

			displayName := parent
			if sn, ok := projectShortName[parent]; ok && sn != "" {
				displayName = sn
			}

			pathParts = append([]string{displayName}, pathParts...)
			currP = parent
		}
		if len(pathParts) > 1 {
			prefix := strings.Join(pathParts[:len(pathParts)-1], ".")
			leaf := pathParts[len(pathParts)-1]
			projectStr = fmt.Sprintf("[%s] %s", prefix, leaf)
		}

		if len(projectStr) > 20 {
			projectStr = projectStr[:17] + "..."
		}

		statusStr := string(t.Status)

		scoreVal := 0.0
		if scores != nil {
			scoreVal = scores[t.ID]
		}

		row := displayRow{
			id:       fmt.Sprintf("%d", t.ID),
			project:  projectStr,
			status:   statusStr,
			desc:     desc,
			created:  FormatDate(t.CreatedAt, cfg),
			age:      t.AgeString(),
			due:      dueStr,
			rsch:     rschStr,
			est:      estStr,
			score:    fmt.Sprintf("%.1f", scoreVal),
			duration: durStr,
			tier:     finalTier,
		}

		rowsData = append(rowsData, row)

		if len(row.id) > widths[0] {
			widths[0] = len(row.id)
		}
		if len(row.project) > widths[1] {
			widths[1] = len(row.project)
		}
		if len(row.status) > widths[2] {
			widths[2] = len(row.status)
		}
		widths[3] = max(widths[3], lipgloss.Width(row.desc))
		if len(row.created) > widths[4] {
			widths[4] = len(row.created)
		}
		if len(row.age) > widths[5] {
			widths[5] = len(row.age)
		}
		if len(row.due) > widths[6] {
			widths[6] = len(row.due)
		}
		if len(row.rsch) > widths[7] {
			widths[7] = len(row.rsch)
		}
		if len(row.est) > widths[8] {
			widths[8] = len(row.est)
		}
		if len(row.score) > widths[9] {
			widths[9] = len(row.score)
		}
		if len(row.duration) > widths[10] {
			widths[10] = len(row.duration)
		}
	}

	totalWidth := 0
	for _, w := range widths {
		totalWidth += w
	}
	totalWidth += (len(widths) - 1) * 3

	width, height, err := term.GetSize(int(os.Stdout.Fd()))
	if err == nil && width > 0 {
		if totalWidth > width {
			fixed := totalWidth - widths[3]
			avail := width - fixed

			if avail < 10 {
				avail = 10
			}

			if avail < widths[3] {
				widths[3] = avail
				totalWidth = fixed + avail
			}
		}
	}

	if !opts.ShowAll {
		if err == nil && height > 0 {
			linesPerTask := 1
			if width > 0 {
				linesPerTask = (totalWidth + width - 1) / width
			}

			headerLines := 1
			if width > 0 {
				headerLines = (totalWidth + width - 1) / width
			}

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

	styles := make(map[UrgencyTier]lipgloss.Style)
	styles[TierOverdue] = GetStyleForTier(TierOverdue, cfg)
	styles[TierToday] = GetStyleForTier(TierToday, cfg)
	styles[TierTomorrow] = GetStyleForTier(TierTomorrow, cfg)
	styles[TierWeek] = GetStyleForTier(TierWeek, cfg)
	styles[TierDefault] = GetStyleForTier(TierDefault, cfg)
	styles[TierBlocked] = GetStyleForTier(TierBlocked, cfg) // Added Blocked style

	var headerStr strings.Builder
	for i, h := range headers {
		fmt.Fprintf(&headerStr, "%-*s", widths[i], h)
		if i < len(headers)-1 {
			headerStr.WriteString("   ")
		}
	}
	fmt.Println(headerStr.String())

	for _, row := range rowsData {
		if lipgloss.Width(row.desc) > widths[3] {
			row.desc = lipgloss.NewStyle().MaxWidth(widths[3]-3).Render(row.desc) + "..."
		}
		style := styles[row.tier]
		printCol := func(text string, width int, isLast bool) {
			padded := strings.Repeat(" ", max(0, width-lipgloss.Width(text))) + text
			fmt.Print(style.Render(padded))
			if !isLast {
				fmt.Print(style.Render("   "))
			}
		}

		printCol(row.id, widths[0], false)
		printCol(row.project, widths[1], false)
		printCol(row.status, widths[2], false)
		printCol(row.desc, widths[3], false)
		printCol(row.created, widths[4], false)
		printCol(row.age, widths[5], false)
		printCol(row.due, widths[6], false)
		printCol(row.rsch, widths[7], false)
		printCol(row.est, widths[8], false)
		printCol(row.score, widths[9], false)
		printCol(row.duration, widths[10], true)
		fmt.Println()
	}
}
