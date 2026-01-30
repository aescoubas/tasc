package cmd

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/aescoubas/tasc/internal/db"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

var (
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
		fmt.Println(titleStyle.Render("Enhanced Productivity Metrics"))

		// 1. Closure Rates
		printClosureRates()

		// 2. Time Allocation
		printTimeAllocation()

		// 3. Task Status Breakdown (Existing functionality refined)
		printStatusBreakdown()
	},
}

func printClosureRates() {
	fmt.Println(sectionStyle.Render("Closure Rates"))

	queries := map[string]string{
		"Last 7 Days":  "SELECT COUNT(*) FROM tasks WHERE status = 'done' AND completed_at >= datetime('now', '-7 days')",
		"Last 30 Days": "SELECT COUNT(*) FROM tasks WHERE status = 'done' AND completed_at >= datetime('now', '-30 days')",
		"All Time":     "SELECT COUNT(*) FROM tasks WHERE status = 'done'",
	}

	// Order for display
	labels := []string{"Last 7 Days", "Last 30 Days", "All Time"}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	for _, label := range labels {
		var count int
		err := db.DB.QueryRow(queries[label]).Scan(&count)
		if err != nil {
			fmt.Printf("Error querying %s: %v\n", label, err)
			continue
		}
		fmt.Fprintf(w, "%s:\t%d\n", label, count)
	}
	w.Flush()
}

func printTimeAllocation() {
	fmt.Println(sectionStyle.Render("Time Allocation (by Project)"))

	query := `SELECT project, SUM(time_spent) FROM tasks WHERE time_spent > 0 GROUP BY project ORDER BY SUM(time_spent) DESC`
	rows, err := db.DB.Query(query)
	if err != nil {
		fmt.Printf("Error querying time allocation: %v\n", err)
		return
	}
	defer rows.Close()

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
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
		fmt.Fprintf(w, "%s\t%s\n", project, dur.String())
		totalSeconds += seconds
	}

	if totalSeconds > 0 {
		fmt.Fprintln(w, "-------	--------")
		fmt.Fprintf(w, "Total\t%s\n", (time.Duration(totalSeconds) * time.Second).String())
	}
	w.Flush()
}

func printStatusBreakdown() {
	fmt.Println(sectionStyle.Render("Current Status Breakdown"))
	query := `SELECT status, COUNT(*) FROM tasks GROUP BY status ORDER BY COUNT(*) DESC`
	rows, err := db.DB.Query(query)
	if err != nil {
		fmt.Printf("Error querying stats: %v\n", err)
		return
	}
	defer rows.Close()

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
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
}

func init() {
	rootCmd.AddCommand(statsCmd)
}