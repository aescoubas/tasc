package cmd

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
)

var deleteAutoApprove bool

var deleteCmd = &cobra.Command{
	Use:               "delete [id...]",
	Short:             "Delete task(s)",
	Args:              cobra.MinimumNArgs(1),
	ValidArgsFunction: taskIDCompletionFunc,
	RunE: func(cmd *cobra.Command, args []string) error {
		var ids []int64
		for _, arg := range args {
			id, err := strconv.ParseInt(arg, 10, 64)
			if err != nil {
				return fmt.Errorf("invalid task ID: %q", arg)
			}
			ids = append(ids, id)
		}
		ids = uniqueInt64s(ids)

		// Validation Loop
		var approvedIDs []int64
		confirmAll := false

		for _, id := range ids {
			// If we already confirmed all, just add and continue
			if confirmAll || deleteAutoApprove {
				approvedIDs = append(approvedIDs, id)
				continue
			}

			// Fetch task to show title
			t, err := CurrentStore.GetTask(id)
			if err != nil {
				return fmt.Errorf("fetch task %d: %w", id, err)
			}

			res := AskConfirmation(fmt.Sprintf("Delete task %d '%s'?", t.ID, t.Title))
			switch res {
			case ConfirmYes:
				approvedIDs = append(approvedIDs, id)
			case ConfirmAll:
				confirmAll = true
				approvedIDs = append(approvedIDs, id)
			case ConfirmNo:
				// skip
			}
		}

		ids = approvedIDs

		if len(ids) == 0 {
			fmt.Println("No tasks selected for deletion.")
			return nil
		}

		// Handle recurrence prompt ONLY if single task is being deleted.
		// For batch delete, we assume user knows what they are doing or we skip recurrence logic
		// (or implicitly stop it). Prompting for 10 tasks is bad UX.
		// Let's adopt a policy: Batch delete = "Stop recurrence" (Delete all future).
		updates := map[string]interface{}{"status": "deleted"}
		if len(ids) == 1 && !deleteAutoApprove && keepSeries(ids[0], "Do you want to (s)kip this instance (spawn next) or stop recurrence? [skip/stop]: ", "s", "skip") {
			updates["keep_series"] = true
		}

		err := CurrentStore.BatchUpdateTasks(ids, updates)
		if err != nil {
			return fmt.Errorf("error deleting tasks: %w", err)
		}

		if len(ids) == 1 {
			fmt.Printf("Task %d deleted.\n", ids[0])
		} else {
			fmt.Printf("Deleted %d tasks.\n", len(ids))
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(deleteCmd)
	deleteCmd.Flags().BoolVarP(&deleteAutoApprove, "yes", "y", false, "Auto-approve deletion without confirmation prompts")
}
