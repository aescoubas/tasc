package cmd

import (
	"fmt"
	"strconv"

	"github.com/spf13/cobra"
)

var vagueCmd = &cobra.Command{
	Use:   "vague [id]",
	Short: "Mark a task as poorly defined",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid task ID: %s", args[0])
		}
		if err := CurrentStore.MarkVague(id); err != nil {
			return fmt.Errorf("mark task %d as undefined: %w", id, err)
		}

		fmt.Printf("Task %d marked as undefined.\n", id)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(vagueCmd)
}
