package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/aescoubas/tasc/internal/agent"
	"github.com/aescoubas/tasc/internal/store"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
)

// The CLI worker is kept in the agent's background-command facility. Watching
// the agent, rather than its short-lived tool shell, bounds orphaned workers.
func init() {
	var pid int
	var name, location string
	claim := &cobra.Command{Use: "claim ID [ID…]", Short: "Claim tasks and maintain the lease while a local agent PID lives", Args: cobra.MinimumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		ids, err := store.ParseTaskIDs(args)
		if err != nil {
			return err
		}
		if strings.TrimSpace(name) == "" || pid <= 1 {
			return fmt.Errorf("--name and the long-lived agent --pid are required")
		}
		ctx, cancel, err := agent.RunContext(cmd.Context(), pid)
		if err != nil {
			return err
		}
		defer cancel()
		id := uuid.NewString()
		st := CurrentStore.ForActor(id)
		if location == "" {
			location = fmt.Sprintf("PID %d", pid)
			if pane := os.Getenv("TMUX_PANE"); pane != "" {
				location += " tmux " + pane
			}
		}
		rows, err := st.ChangeAssignments(store.AssignmentChange{Action: "claim", IDs: ids, Name: name, Location: location})
		if err != nil {
			return err
		}
		if err := json.NewEncoder(cmd.OutOrStdout()).Encode(store.ClaimResult{RunID: id, Tasks: rows}); err != nil {
			return err
		}
		store.MaintainRun(ctx, st)
		return nil
	}}
	claim.Flags().IntVar(&pid, "pid", 0, "Long-lived coding agent PID to watch (Linux)")
	claim.Flags().StringVar(&name, "name", "", "Readable agent and goal name")
	claim.Flags().StringVar(&location, "location", "", "Resumable conversation or tmux location")
	rootCmd.AddCommand(claim)
	rootCmd.AddCommand(&cobra.Command{Use: "end-run --run RUN", Short: "Release a goal's remaining assignments and stop renewing", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, args []string) error {
		if runID == "" {
			return fmt.Errorf("--run is required")
		}
		_, err := CurrentStore.ChangeAssignments(store.AssignmentChange{Action: "end"})
		return err
	}})
}
