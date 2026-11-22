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
	ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) == 0 {
			// TODO: Add task ID completion if desired
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		// Completion for the date part
		return dateCompletionFunc(cmd, args, toComplete)
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.LoadConfig()
		if err != nil {
			cfg = config.DefaultConfig()
		}

		id, err := strconv.Atoi(args[0])
		if err != nil {
			return fmt.Errorf("invalid task ID")
		}

		dateStr := strings.Join(args[1:], " ")
		newDate, dateOnly, err := parse.DateWithPrecision(dateStr, cfg.EndOfDay)
		if err != nil {
			return fmt.Errorf("invalid date format: %w", err)
		}

		// Fetch task
		t, err := CurrentStore.GetTask(int64(id))
		if err != nil {
			return fmt.Errorf("fetch task %d: %w", id, err)
		}

		// Update DueAt
		// Logic: If moving to a DIFFERENT date, increment reschedule count
		isDifferent := true
		if t.DueAt != nil {
			if t.DueAt.Equal(*newDate) {
				isDifferent = false
			}
		}

		if isDifferent {
			t.RescheduleCount++
		}

		oldDate := "no due date"
		if t.DueAt != nil {
			oldDate = t.DueAt.Format("2006-01-02 15:04")
		}

		t.DueAt = newDate
		t.DueDateOnly = &dateOnly

		err = CurrentStore.UpdateTask(t)
		if err != nil {
			return fmt.Errorf("error updating task: %w", err)
		}

		fmt.Printf("Rescheduled task %d from %s to %s\n", id, oldDate, newDate.Format("2006-01-02 15:04"))
		return nil
	},
}

func init() {
	rootCmd.AddCommand(rescheduleCmd)
}
