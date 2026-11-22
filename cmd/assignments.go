package cmd

import (
	"fmt"
	"strings"

	"github.com/aescoubas/tasc/internal/store"
	"github.com/spf13/cobra"
)

func init() {
	assignments := &cobra.Command{Use: "assignments", Short: "List assigned tasks grouped by owner and run", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		rows, err := CurrentStore.Assignments()
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			fmt.Fprintln(cmd.OutOrStdout(), "No assigned tasks.")
			return nil
		}
		last := ""
		for _, row := range rows {
			label := row.Assignment.String()
			if label != last {
				fmt.Fprint(cmd.OutOrStdout(), label)
				if row.Assignment.Location != "" {
					fmt.Fprintf(cmd.OutOrStdout(), " · %s", row.Assignment.Location)
				}
				fmt.Fprintln(cmd.OutOrStdout())
				last = label
			}
			fmt.Fprintf(cmd.OutOrStdout(), "  %d — %s (%s)\n", row.ID, row.Title, row.Status)
		}
		return nil
	}}
	rootCmd.AddCommand(assignments)
	for _, action := range []string{"assign", "release"} {
		var target string
		c := &cobra.Command{Use: action + " ID [ID…]", Args: cobra.MinimumNArgs(1), Short: "Release task assignments", RunE: func(cmd *cobra.Command, args []string) error {
			ids, err := store.ParseTaskIDs(args)
			if err != nil {
				return err
			}
			if strings.EqualFold(target, store.HumanOwner) {
				target = ""
			}
			rows, err := CurrentStore.ChangeAssignments(store.AssignmentChange{Action: action, IDs: ids, TargetRun: target})
			if err != nil {
				return err
			}
			failed := false
			for _, row := range rows {
				fmt.Fprintf(cmd.OutOrStdout(), "%d — %s: %s", row.ID, row.Title, row.Result)
				if row.Assignment != nil {
					fmt.Fprintf(cmd.OutOrStdout(), " · %s", row.Assignment)
				}
				fmt.Fprintln(cmd.OutOrStdout())
				if row.Result != "assigned" && row.Result != "released" {
					failed = true
				}
			}
			if failed {
				return fmt.Errorf("could not %s every task", action)
			}
			return nil
		}}
		if action == "assign" {
			c.Short = "Assign tasks to Alex or an active agent run"
			c.Flags().StringVar(&target, "to", store.HumanOwner, "Alex or the run ID shown by assignments")
		}
		rootCmd.AddCommand(c)
	}
}
