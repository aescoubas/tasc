package cmd

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/aescoubas/tasc/internal/config"
	"github.com/aescoubas/tasc/internal/parse"
	"github.com/spf13/cobra"
)

var rescheduleCmd = &cobra.Command{
	Use:     "reschedule [id] [date]",
	Aliases: []string{"rsch", "mv"},
	Short:   "Reschedule a task to a new date/time",
	Args:    cobra.MinimumNArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.LoadConfig()
		if err != nil {
			cfg = config.DefaultConfig()
		}

		id, err := strconv.Atoi(args[0])
		if err != nil {
			fmt.Println("Invalid task ID")
			return
		}

		dateStr := strings.Join(args[1:], " ")
		newDate, err := parse.Date(dateStr, cfg.EndOfDay)
		if err != nil {
			fmt.Printf("Invalid date format: %v\n", err)
			return
		}

		// Fetch task
		t, err := CurrentStore.GetTask(int64(id))
		if err != nil {
			fmt.Printf("Task %d not found.\n", id)
			return
		}

		// Update ScheduledAt
		// Logic: If moving to a DIFFERENT date, increment reschedule count
		isDifferent := true
		if t.ScheduledAt != nil {
			if t.ScheduledAt.Equal(*newDate) {
				isDifferent = false
			}
		}

		if isDifferent {
			t.RescheduleCount++
		}
		
		oldDate := "unscheduled"
		if t.ScheduledAt != nil {
			oldDate = t.ScheduledAt.Format("2006-01-02 15:04")
		}

		t.ScheduledAt = newDate

		err = CurrentStore.UpdateTask(t)
		if err != nil {
			fmt.Printf("Error updating task: %v\n", err)
			return
		}

		fmt.Printf("Rescheduled task %d from %s to %s\n", id, oldDate, newDate.Format("2006-01-02 15:04"))
	},
}

func init() {
	rootCmd.AddCommand(rescheduleCmd)
}
