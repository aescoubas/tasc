package cmd

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/aescoubas/tasc/internal/db"
	"github.com/aescoubas/tasc/internal/store"
	"github.com/aescoubas/tasc/internal/store/sqlite"
	"github.com/spf13/cobra"
)

func ensureStore() {
	if CurrentStore == nil {
		db.InitDB()
		if db.DB != nil {
			CurrentStore = sqlite.NewSQLiteStore(db.DB)
		}
	}
}

func dateCompletionFunc(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	var options []string

	// Basic relative dates
	basics := []string{"today", "tomorrow", "next week"}
	
	// Weekdays
	weekdays := []string{
		"monday", "tuesday", "wednesday", "thursday", "friday", "saturday", "sunday",
	}

	// "Next" weekdays
	nextWeekdays := []string{}
	for _, w := range weekdays {
		nextWeekdays = append(nextWeekdays, "next "+w)
	}

	options = append(options, basics...)
	options = append(options, weekdays...)
	options = append(options, nextWeekdays...)

	// Filter based on toComplete to reduce noise if the shell doesn't do it well
	var filtered []string
	for _, opt := range options {
		if strings.HasPrefix(opt, strings.ToLower(toComplete)) {
			filtered = append(filtered, opt)
		}
	}

	return filtered, cobra.ShellCompDirectiveNoFileComp
}

func projectCompletionFunc(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	ensureStore()
	if CurrentStore == nil {
		return nil, cobra.ShellCompDirectiveError
	}
	projects, err := CurrentStore.ListProjects()
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	var names []string
	for _, p := range projects {
		if strings.HasPrefix(p.Name, toComplete) {
			names = append(names, p.Name)
		}
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}

func recurrenceCompletionFunc(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	options := []string{
		"daily",
		"weekly",
		"monthly",
		"yearly",
		"weekdays",
		"every 2 weeks",
		"every 3 days",
	}
	var filtered []string
	for _, opt := range options {
		if strings.HasPrefix(opt, toComplete) {
			filtered = append(filtered, opt)
		}
	}
	return filtered, cobra.ShellCompDirectiveNoFileComp
}

func taskIDCompletionFunc(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	ensureStore()
	if CurrentStore == nil {
		return nil, cobra.ShellCompDirectiveError
	}
	
	// We want all tasks that are not deleted? Or just all tasks?
	// Usually for 'modify' or 'delete', we might want to see titles.
	// Cobra expects "value\tdescription" for zsh/fish, or just "value" for bash.
	// We can provide "ID\tTitle".
	
	tasks, err := CurrentStore.ListTasks(store.TaskFilter{}) // List all tasks
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}

	var suggestions []string
	for _, t := range tasks {
		if t.Status == "deleted" {
			continue
		}
		sID := strconv.FormatInt(t.ID, 10)
		if strings.HasPrefix(sID, toComplete) {
			// Format: "ID\tTitle"
			suggestions = append(suggestions, fmt.Sprintf("%s\t%s", sID, t.Title))
		}
	}
	return suggestions, cobra.ShellCompDirectiveNoFileComp
}

func activeTaskIDCompletionFunc(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	ensureStore()
	if CurrentStore == nil {
		return nil, cobra.ShellCompDirectiveError
	}

	// Filter for active tasks (ActiveStart != nil)
	tasks, err := CurrentStore.ListTasks(store.TaskFilter{})
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}

	var suggestions []string
	for _, t := range tasks {
		if t.ActiveStart != nil {
			sID := strconv.FormatInt(t.ID, 10)
			if strings.HasPrefix(sID, toComplete) {
				suggestions = append(suggestions, fmt.Sprintf("%s\t%s (Active)", sID, t.Title))
			}
		}
	}
	return suggestions, cobra.ShellCompDirectiveNoFileComp
}

func pendingTaskIDCompletionFunc(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	ensureStore()
	if CurrentStore == nil {
		return nil, cobra.ShellCompDirectiveError
	}

	tasks, err := CurrentStore.ListTasks(store.TaskFilter{})
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}

	var suggestions []string
	for _, t := range tasks {
		// Suggest tasks that are not done and not deleted
		if t.Status != "done" && t.Status != "deleted" {
			sID := strconv.FormatInt(t.ID, 10)
			if strings.HasPrefix(sID, toComplete) {
				suggestions = append(suggestions, fmt.Sprintf("%s\t%s", sID, t.Title))
			}
		}
	}
	return suggestions, cobra.ShellCompDirectiveNoFileComp
}
