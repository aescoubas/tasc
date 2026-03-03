package cmd

import (
	"fmt"

	"github.com/aescoubas/tasc/internal/store/sqlite"
	"github.com/spf13/cobra"
)

var renumberAutoApprove bool

var renumberCmd = &cobra.Command{
	Use:   "renumber",
	Short: "Renumber open task IDs starting at 0",
	Run: func(cmd *cobra.Command, args []string) {
		localStore, ok := CurrentStore.(*sqlite.SQLiteStore)
		if !ok {
			fmt.Println("Error: renumber is only supported with a local SQLite database.")
			return
		}

		if !renumberAutoApprove {
			res := AskConfirmation("Renumber open tasks starting at ID 0? Done/deleted tasks will be moved to negative IDs.")
			if res == ConfirmNo {
				fmt.Println("Renumber cancelled.")
				return
			}
		}

		count, err := localStore.RenumberTasks(0)
		if err != nil {
			fmt.Printf("Error renumbering tasks: %v\n", err)
			return
		}

		fmt.Printf("Renumbered %d open tasks starting at ID 0. Done/deleted tasks now use negative IDs.\n", count)
	},
}

func init() {
	rootCmd.AddCommand(renumberCmd)
	renumberCmd.Flags().BoolVarP(&renumberAutoApprove, "yes", "y", false, "Auto-approve renumbering without confirmation")
}
