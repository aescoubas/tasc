package cmd

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/aescoubas/tasc/internal/db"
	"github.com/spf13/cobra"
	"github.com/tj/go-naturaldate"
)

var (
	project   string
	due       string
	scheduled string
	estimate  string
)

var addCmd = &cobra.Command{
	Use:   "add [description]",
	Short: "Add a new task",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		description := strings.Join(args, " ")

		var dueAt *time.Time
		if due != "" {
			t, err := parseDate(due)
			if err != nil {
				fmt.Printf("Invalid due date format: %v\n", err)
				return
			}
			dueAt = t
		}

		var scheduledAt *time.Time
		if scheduled != "" {
			t, err := parseDate(scheduled)
			if err != nil {
				fmt.Printf("Invalid scheduled date format: %v\n", err)
				return
			}
			scheduledAt = t
		}

		query := `INSERT INTO tasks (description, project, due_at, scheduled_at, estimate, created_at) VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`
		stmt, err := db.DB.Prepare(query)
		if err != nil {
			fmt.Printf("Error preparing statement: %v\n", err)
			return
		}
		defer stmt.Close()

		res, err := stmt.Exec(description, project, dueAt, scheduledAt, estimate)
		if err != nil {
			fmt.Printf("Error executing statement: %v\n", err)
			return
		}

		id, _ := res.LastInsertId()
		fmt.Printf("Created task %d.\n", id)
	},
}

func init() {
	rootCmd.AddCommand(addCmd)
	addCmd.Flags().StringVarP(&project, "project", "p", "", "Project name")
	addCmd.Flags().StringVarP(&due, "due", "d", "", "Due date (YYYY-MM-DD)")
	addCmd.Flags().StringVarP(&scheduled, "scheduled", "s", "", "Scheduled date (YYYY-MM-DD)")
	addCmd.Flags().StringVarP(&estimate, "estimate", "e", "", "Time estimate (e.g. 2h, 30m)")
}

func parseDate(s string) (*time.Time, error) {
	formats := []string{
		"2006-01-02",
		"2006/01/02",
		time.RFC3339,
	}
	for _, f := range formats {
		t, err := time.Parse(f, s)
		if err == nil {
			return &t, nil
		}
	}

	// Try natural date parsing
	processed := preprocessDate(s)
	t, err := naturaldate.Parse(processed, time.Now())
	if err == nil {
		return &t, nil
	}

	return nil, fmt.Errorf("could not parse date %q", s)
}

func preprocessDate(s string) string {
	s = strings.TrimSpace(s)
	// 1. Split number and unit if stuck together: "10days" -> "10 days"
	re := regexp.MustCompile(`^(\d+)([a-zA-Z]+)$`)
	s = re.ReplaceAllString(s, "$1 $2")

	// 2. "week" -> "next week"
	if s == "week" {
		return "next week"
	}

	// 3. If it looks like a duration "10 days", "2 weeks", prepend "in " to force future
	reDuration := regexp.MustCompile(`^\d+\s+(day|days|week|weeks|month|months|year|years)$`)
	if reDuration.MatchString(s) {
		return "in " + s
	}

	return s
}
