package cmd

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/aescoubas/tasc/internal/db"
	"github.com/spf13/cobra"
)

var modifyCmd = &cobra.Command{
	Use:   "modify [id] [description]",
	Short: "Modify a task",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		id, err := strconv.Atoi(args[0])
		if err != nil {
			fmt.Println("Invalid task ID")
			return
		}

		var updates []string
		var values []interface{}

		// Check if description is provided as args
		if len(args) > 1 {
			desc := strings.Join(args[1:], " ")
			updates = append(updates, "description = ?")
			values = append(values, desc)
		}

		// Check flags
		if cmd.Flags().Changed("project") {
			updates = append(updates, "project = ?")
			values = append(values, project)
		}

		if cmd.Flags().Changed("due") {
			if due == "" {
				updates = append(updates, "due_at = NULL")
			} else {
				t, err := parseDate(due)
				if err != nil {
					fmt.Printf("Invalid due date: %v\n", err)
					return
				}
				updates = append(updates, "due_at = ?")
				values = append(values, t)
			}
		}

		if cmd.Flags().Changed("scheduled") {
			if scheduled == "" {
				updates = append(updates, "scheduled_at = NULL")
			} else {
				t, err := parseDate(scheduled)
				if err != nil {
					fmt.Printf("Invalid scheduled date: %v\n", err)
					return
				}
				updates = append(updates, "scheduled_at = ?")
				values = append(values, t)
			}
		}

		if cmd.Flags().Changed("estimate") {
			updates = append(updates, "estimate = ?")
			values = append(values, estimate)
		}

		if len(updates) == 0 {
			fmt.Println("No changes specified.")
			return
		}

		query := fmt.Sprintf("UPDATE tasks SET %s WHERE id = ?", strings.Join(updates, ", "))
		values = append(values, id)

		_, err = db.DB.Exec(query, values...)
		if err != nil {
			fmt.Printf("Error updating task: %v\n", err)
			return
		}

		fmt.Printf("Task %d modified.\n", id)
	},
}

func init() {
	rootCmd.AddCommand(modifyCmd)
	modifyCmd.Flags().StringVarP(&project, "project", "p", "", "Project name")
	modifyCmd.Flags().StringVarP(&due, "due", "d", "", "Due date (YYYY-MM-DD)")
	modifyCmd.Flags().StringVarP(&scheduled, "scheduled", "s", "", "Scheduled date (YYYY-MM-DD)")
	modifyCmd.Flags().StringVarP(&estimate, "estimate", "e", "", "Time estimate")
}
