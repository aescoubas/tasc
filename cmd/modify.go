package cmd

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/aescoubas/tasc/internal/config"
	"github.com/aescoubas/tasc/internal/parse"
	"github.com/spf13/cobra"
)

var modifyCmd = &cobra.Command{
	Use:               "modify [id...] [title]",
	Short:             "Modify task(s)",
	Long:              "Modify one or more tasks. If multiple IDs are provided, only flags (e.g. --due, --project) are applied. If a single ID is provided, you can also update the title.",
	Args:              cobra.MinimumNArgs(1),
	ValidArgsFunction: taskIDCompletionFunc,
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.LoadConfig()
		if err != nil {
			cfg = config.DefaultConfig()
		}

		var ids []int64
		var titleParts []string

		// Parse Args: Try to consume as many IDs as possible from the start
		parsingIDs := true
		for _, arg := range args {
			if parsingIDs {
				id, err := strconv.Atoi(arg)
				if err == nil {
					ids = append(ids, int64(id))
				} else {
					parsingIDs = false
					titleParts = append(titleParts, arg)
				}
			} else {
				titleParts = append(titleParts, arg)
			}
		}

		if len(ids) == 0 {
			fmt.Println("No valid task IDs provided.")
			return
		}

		updates := make(map[string]interface{})

		// Title handling
		if len(titleParts) > 0 {
			if len(ids) > 1 {
				fmt.Println("Cannot update title for multiple tasks at once. Please provide a single ID to update title, or use flags only for batch updates.")
				return
			}
			updates["title"] = strings.Join(titleParts, " ")
		}

		if cmd.Flags().Changed("description") {
			updates["description"] = description
		}
		if cmd.Flags().Changed("project") {
			updates["project"] = project
		}
		if cmd.Flags().Changed("due") {
			if due == "" {
				updates["due_at"] = nil
			} else {
				parsed, err := parse.Date(due, cfg.EndOfDay)
				if err != nil {
					fmt.Printf("Invalid due date: %v\n", err)
					return
				}
				updates["due_at"] = parsed
			}
		}
		if cmd.Flags().Changed("estimate") {
			updates["estimate"] = estimate
		}
		if cmd.Flags().Changed("recurrence") {
			if recurrenceFlag == "" {
				updates["recurrence"] = ""
			} else {
				updates["recurrence"] = recurrenceFlag
			}
		}
		if cmd.Flags().Changed("block") {
			updates["schedule_block"] = block
		}

		if len(updates) == 0 && len(titleParts) == 0 {
			// No changes requested?
			// Maybe just Recurrence prompt logic was triggered in original?
			// Original logic: Fetch task -> Check Recurrence prompt -> Save.
			// Batch logic: If no flags, nothing happens?
			// Unless single task...
		}

		// Validation Loop
		var approvedIDs []int64
		confirmAll := false

		for _, id := range ids {
			if confirmAll {
				approvedIDs = append(approvedIDs, id)
				continue
			}

			t, err := CurrentStore.GetTask(id)
			if err != nil {
				fmt.Printf("Task %d not found or error fetching: %v. Skipping.\n", id, err)
				continue
			}

			res := AskConfirmation(fmt.Sprintf("Modify task %d '%s'?", t.ID, t.Title))
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
			fmt.Println("No tasks selected for modification.")
			return
		}

		// Recurrence Prompt Logic (Single Task Only)
		if len(ids) == 1 {
			t, err := CurrentStore.GetTask(ids[0])
			if err == nil && t.Recurrence != "" {
				// Only if we are modifying something that impacts the series?
				// Simplification: Ask if user wants to update series or detach.
				fmt.Printf("Task %d is recurring ('%s').\n", t.ID, t.Recurrence)
				fmt.Print("Do you want to (u)pdate the series (affects future) or (o)nly this instance (detach)? [u/o]: ")
				var resp string
				fmt.Scanln(&resp)
				resp = strings.ToLower(resp)

				if resp == "o" || resp == "only" {
					// Detach: Spawn next task with OLD values
					spawnNextTask(t)
					// Clear recurrence on CURRENT (which becomes the 'instance')
					// We add this to updates
					updates["recurrence"] = ""
				}
			}
		}

		err = CurrentStore.BatchUpdateTasks(ids, updates)
		if err != nil {
			fmt.Printf("Error updating tasks: %v\n", err)
			return
		}

		if len(ids) == 1 {
			fmt.Printf("Task %d modified.\n", ids[0])
		} else {
			fmt.Printf("Modified %d tasks.\n", len(ids))
		}
	},
}

func init() {
	rootCmd.AddCommand(modifyCmd)
	modifyCmd.Flags().StringVarP(&project, "project", "p", "", "Project name")
	modifyCmd.Flags().StringVarP(&description, "description", "m", "", "Detailed description")
	modifyCmd.Flags().StringVarP(&due, "due", "d", "", "Due date (YYYY-MM-DD)")
	modifyCmd.Flags().StringVarP(&estimate, "estimate", "e", "", "Time estimate")
	modifyCmd.Flags().StringVarP(&recurrenceFlag, "recurrence", "r", "", "Recurrence rule")
	modifyCmd.Flags().StringVarP(&block, "block", "b", "", "Time block")

	modifyCmd.RegisterFlagCompletionFunc("project", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if CurrentStore == nil {
			return nil, cobra.ShellCompDirectiveError
		}
		projects, err := CurrentStore.ListProjects()
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}
		var names []string
		for _, p := range projects {
			names = append(names, p.Name)
		}
		return names, cobra.ShellCompDirectiveNoFileComp
	})

	modifyCmd.RegisterFlagCompletionFunc("block", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		cfg, err := config.LoadConfig()
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}
		var names []string
		for name := range cfg.TimeBlocks {
			names = append(names, name)
		}
		return names, cobra.ShellCompDirectiveNoFileComp
	})

	modifyCmd.RegisterFlagCompletionFunc("due", dateCompletionFunc)
	modifyCmd.RegisterFlagCompletionFunc("recurrence", recurrenceCompletionFunc)
}
