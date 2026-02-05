package cmd

import (
	"strings"

	"github.com/spf13/cobra"
)

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
