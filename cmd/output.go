package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

const (
	OutputTable = "table"
	OutputJSON  = "json"
)

func validateOutputFormat(output string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(output))
	if normalized == "" {
		normalized = OutputTable
	}

	switch normalized {
	case OutputTable, OutputJSON:
		return normalized, nil
	default:
		return "", fmt.Errorf("invalid output format %q (supported: table, json)", output)
	}
}

func outputFormatCompletionFunc(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	options := []string{OutputTable, OutputJSON}

	var filtered []string
	for _, opt := range options {
		if strings.HasPrefix(opt, strings.ToLower(toComplete)) {
			filtered = append(filtered, opt)
		}
	}

	return filtered, cobra.ShellCompDirectiveNoFileComp
}
