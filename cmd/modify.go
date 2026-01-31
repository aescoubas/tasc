package cmd

import (
	"fmt"
	"strconv"
	"strings"

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

		// 1. Fetch task details
		t, err := CurrentStore.GetTask(int64(id))
		if err != nil {
			fmt.Printf("Task %d not found or error: %v\n", id, err)
			return
		}

		// 2. Handle Recurrence Prompt
		// We preserve original for spawnNextTask if needed
		originalTask := t

		if t.Recurrence != "" {
			fmt.Printf("This is a recurring task ('%s').\n", t.Recurrence)
			fmt.Print("Do you want to (u)pdate the series (affects future) or (o)nly this instance (detach)? [u/o]: ")
			var resp string
			fmt.Scanln(&resp)
			resp = strings.ToLower(resp)
			
			if resp == "o" || resp == "only" {
				// Detach: Spawn next task with OLD values, clear recurrence on CURRENT
				spawnNextTask(originalTask)
				t.Recurrence = "" // Clear on current
			}
			// If 'u', we just proceed.
		}

		// Check if description is provided as args
		if len(args) > 1 {
			t.Description = strings.Join(args[1:], " ")
		}

		// Check flags
		if cmd.Flags().Changed("project") {
			t.Project = project
		}

		if cmd.Flags().Changed("due") {
			if due == "" {
				t.DueAt = nil
			} else {
				parsed, err := parseDate(due)
				if err != nil {
					fmt.Printf("Invalid due date: %v\n", err)
					return
				}
				t.DueAt = parsed
			}
		}

		if cmd.Flags().Changed("scheduled") {
			if scheduled == "" {
				if t.ScheduledAt != nil {
					t.RescheduleCount++
				}
				t.ScheduledAt = nil
			} else {
				parsed, err := parseDate(scheduled)
				if err != nil {
					fmt.Printf("Invalid scheduled date: %v\n", err)
					return
				}
				
				if t.ScheduledAt == nil || !t.ScheduledAt.Equal(*parsed) {
					t.RescheduleCount++
				}
				t.ScheduledAt = parsed
			}
		}

		if cmd.Flags().Changed("estimate") {
			t.Estimate = estimate
		}
		
		if cmd.Flags().Changed("recurrence") {
			if recurrenceFlag == "" {
				t.Recurrence = ""
			} else {
				t.Recurrence = recurrenceFlag
			}
		}

		// Save updates
		err = CurrentStore.UpdateTask(t)
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
	modifyCmd.Flags().StringVarP(&recurrenceFlag, "recurrence", "r", "", "Recurrence rule")
}
