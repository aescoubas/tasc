package cmd

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/aescoubas/tasc/internal/config"
	"github.com/aescoubas/tasc/internal/models"
	"github.com/aescoubas/tasc/internal/parse"
	"github.com/aescoubas/tasc/internal/store"
	"github.com/spf13/cobra"
)

var (
	modifyAutoApprove   bool
	modifyUserPriority  int
	modifyClearPriority bool
	modifyStatus        string
)

var modifyCmd = &cobra.Command{
	Use:               "modify [id...] [title]",
	Short:             "Modify task(s)",
	Long:              "Modify one or more tasks. If multiple IDs are provided, only flags (e.g. --due, --project) are applied. If a single ID is provided, you can also update the title.",
	Args:              cobra.MinimumNArgs(1),
	ValidArgsFunction: taskIDCompletionFunc,
	PreRunE: func(cmd *cobra.Command, args []string) error {
		if cmd.Flags().Changed("status") {
			switch models.TaskStatus(modifyStatus) {
			case models.StatusBacklog, models.StatusOngoing, models.StatusBlocked:
			default:
				return fmt.Errorf("invalid status %q: expected backlog, ongoing, or blocked", modifyStatus)
			}
		}
		if cmd.Flags().Changed("priority") {
			return store.ValidateUserPriority(modifyUserPriority)
		}
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.LoadConfig()
		if err != nil {
			cfg = config.DefaultConfig()
		}

		dependencySelection, err := buildDependencySelection(dependsOn, blocks)
		if err != nil {
			return fmt.Errorf("invalid dependency list: %w", err)
		}

		var ids []int64
		var titleParts []string

		// Parse Args: Try to consume as many IDs as possible from the start
		parsingIDs := true
		for _, arg := range args {
			if parsingIDs {
				id, err := strconv.ParseInt(arg, 10, 64)
				if err == nil {
					ids = append(ids, id)
				} else {
					parsingIDs = false
					titleParts = append(titleParts, arg)
				}
			} else {
				titleParts = append(titleParts, arg)
			}
		}

		if len(ids) == 0 {
			return fmt.Errorf("no valid task IDs provided.")
		}

		ids = uniqueInt64s(ids)

		updates := make(map[string]interface{})
		if cmd.Flags().Changed("status") {
			updates["status"] = modifyStatus
		}
		if modifyClearPriority {
			updates["user_priority"] = nil
		} else if cmd.Flags().Changed("priority") {
			updates["user_priority"] = modifyUserPriority
		}

		// Title handling
		if len(titleParts) > 0 {
			if len(ids) > 1 {
				return fmt.Errorf("cannot update title for multiple tasks at once. Please provide a single ID to update title, or use flags only for batch updates.")
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
					return fmt.Errorf("invalid due date: %w", err)
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

		if len(updates) == 0 && !dependencySelection.hasLinks() {
			return fmt.Errorf("no changes specified. Use flags like --due, --project, --estimate, or provide a new title.")
		}

		// Validation Loop
		var approvedIDs []int64
		confirmAll := false

		for _, id := range ids {
			if confirmAll || modifyAutoApprove {
				approvedIDs = append(approvedIDs, id)
				continue
			}

			t, err := CurrentStore.GetTask(id)
			if err != nil {
				return fmt.Errorf("fetch task %d: %w", id, err)
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
			return nil
		}

		if err := dependencySelection.validate(ids); err != nil {
			return fmt.Errorf("invalid dependencies: %w", err)
		}

		if len(ids) == 1 && !modifyAutoApprove && keepSeries(ids[0], "Do you want to (u)pdate the series or edit (o)nly this instance? [u/o]: ", "o", "only") {
			updates["keep_series"] = true
			updates["recurrence"] = ""
		}

		linksAdded, err := CurrentStore.BatchUpdateTasksWithLinks(ids, updates, dependencySelection.links())
		if err != nil {
			return fmt.Errorf("error updating tasks: %w", err)
		}

		if len(updates) == 0 && linksAdded == 0 {
			fmt.Println("No new dependency links were added.")
			return nil
		}

		if len(ids) == 1 {
			fmt.Printf("Task %d modified.\n", ids[0])
		} else {
			fmt.Printf("Modified %d tasks.\n", len(ids))
		}
		if linksAdded > 0 {
			fmt.Printf("Added %d dependency link(s).\n", linksAdded)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(modifyCmd)
	modifyCmd.Flags().IntVar(&modifyUserPriority, "priority", 0, "Set user priority (1 highest, 5 lowest)")
	modifyCmd.Flags().BoolVar(&modifyClearPriority, "clear-priority", false, "Clear user priority (sorts like 5)")
	modifyCmd.MarkFlagsMutuallyExclusive("priority", "clear-priority")
	modifyCmd.Flags().StringVarP(&project, "project", "p", "", "Project name")
	modifyCmd.Flags().StringVarP(&description, "description", "m", "", "Detailed description")
	modifyCmd.Flags().StringVar(&modifyStatus, "status", "", "Set task status (backlog, ongoing, blocked)")
	modifyCmd.Flags().StringVarP(&due, "due", "d", "", "Due date (YYYY-MM-DD, DD.MM.YYYY, DD/MM/YYYY, DD-MM-YYYY, or natural language)")
	modifyCmd.Flags().StringVarP(&estimate, "estimate", "e", "", "Time estimate")
	modifyCmd.Flags().StringVarP(&recurrenceFlag, "recurrence", "r", "", "Recurrence rule")
	modifyCmd.Flags().StringVarP(&block, "block", "b", "", "Time block")
	modifyCmd.Flags().StringSliceVar(&dependsOn, "depends-on", nil, "Add blocker task IDs (repeat or comma-separate)")
	modifyCmd.Flags().StringSliceVar(&blocks, "blocks", nil, "Add blocked task IDs (repeat or comma-separate)")
	modifyCmd.Flags().BoolVarP(&modifyAutoApprove, "yes", "y", false, "Auto-approve modifications without confirmation prompts")

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

	modifyCmd.RegisterFlagCompletionFunc("depends-on", taskIDCompletionFunc)
	modifyCmd.RegisterFlagCompletionFunc("blocks", taskIDCompletionFunc)

	modifyCmd.RegisterFlagCompletionFunc("due", dateCompletionFunc)
	modifyCmd.RegisterFlagCompletionFunc("recurrence", recurrenceCompletionFunc)
}
