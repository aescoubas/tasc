package cmd

import (
	"fmt"
	"time"

	"github.com/aescoubas/tasc/internal/db"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

var calendarCmd = &cobra.Command{
	Use:   "calendar",
	Short: "Show a calendar of due tasks",
	Run: func(cmd *cobra.Command, args []string) {
		now := time.Now()
		year, month, _ := now.Date()

		fmt.Printf("      %s %d\n", month, year)
		fmt.Println("Su Mo Tu We Th Fr Sa")

		// Get first day of month
		firstDay := time.Date(year, month, 1, 0, 0, 0, 0, time.Local)
		// Get last day of month
		lastDay := firstDay.AddDate(0, 1, -1)

		// Fetch due dates
		query := `SELECT strftime('%d', due_at) FROM tasks 
		t	  WHERE strftime('%Y-%m', due_at) = ? AND status = 'pending'`

		rows, err := db.DB.Query(query, fmt.Sprintf("%04d-%02d", year, month))
		if err != nil {
			fmt.Printf("Error querying due dates: %v\n", err)
			return
		}
		defer rows.Close()

		dueDays := make(map[int]bool)
		for rows.Next() {
			var day int
			rows.Scan(&day)
			dueDays[day] = true
		}

		// Rendering
		styleDefault := lipgloss.NewStyle().Width(3).Align(lipgloss.Right)
		styleToday := styleDefault.Copy().Foreground(lipgloss.Color("86")).Bold(true) // Aqua
		styleDue := styleDefault.Copy().Foreground(lipgloss.Color("205")).Bold(true)  // Pink
		styleDueAndToday := styleDue.Copy().Background(lipgloss.Color("236"))

		// Padding for start of week
		weekday := int(firstDay.Weekday())
		for i := 0; i < weekday; i++ {
			fmt.Print("   ")
		}

		for day := 1; day <= lastDay.Day(); day++ {
			isToday := (day == now.Day())
			isDue := dueDays[day]

			var s lipgloss.Style
			if isToday && isDue {
				s = styleDueAndToday
			} else if isToday {
				s = styleToday
			} else if isDue {
				s = styleDue
			} else {
				s = styleDefault
			}

			fmt.Print(s.Render(fmt.Sprintf("%d", day)))

			if (weekday+day)%7 == 0 {
				fmt.Println()
			}
		}
		fmt.Println()
	},
}

func init() {
	rootCmd.AddCommand(calendarCmd)
}
