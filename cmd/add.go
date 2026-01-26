package cmd

import (
	"fmt"
	"strings"

	"github.com/aescoubas/tasc/internal/db"
	"github.com/spf13/cobra"
)

var project string

var addCmd = &cobra.Command{
	Use:   "add [description]",
	Short: "Add a new task",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		description := strings.Join(args, " ")
		
		query := `INSERT INTO tasks (description, project, created_at) VALUES (?, ?, CURRENT_TIMESTAMP)`
		stmt, err := db.DB.Prepare(query)
		if err != nil {
			fmt.Printf("Error preparing statement: %v\n", err)
			return
		}
		defer stmt.Close()

		res, err := stmt.Exec(description, project)
		if err != nil {
			fmt.Printf("Error executing statement: %v\n", err)
			return
		}

		id, _ := res.LastInsertId()
		fmt.Printf("Created task %d.\n", id)
	},
}

func init() {
	rootCmd.AddCommand(addCmd)
	addCmd.Flags().StringVarP(&project, "project", "p", "", "Project name")
}

