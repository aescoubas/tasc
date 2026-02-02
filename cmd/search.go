package cmd

import (
	"database/sql"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/aescoubas/tasc/internal/db"
	"github.com/aescoubas/tasc/internal/models"
	"github.com/spf13/cobra"
)

var searchCmd = &cobra.Command{
	Use:   "search [query]",
	Short: "Fuzzy search tasks",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		queryStr := strings.Join(args, " ")

		// Use FTS5 MATCH operator
		query := `
			SELECT id, title, project, status, created_at 
			FROM tasks 
			WHERE id IN (
				SELECT rowid FROM tasks_fts WHERE tasks_fts MATCH ? ORDER BY rank
			)
		`

		rows, err := db.DB.Query(query, queryStr)
		if err != nil {
			fmt.Printf("Error searching tasks: %v\n", err)
			return
		}
		defer rows.Close()

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
		fmt.Fprintln(w, "ID\tProject\tStatus\tTitle\tCreated")

		count := 0
		for rows.Next() {
			count++
			var t models.Task
			var project sql.NullString
			err := rows.Scan(&t.ID, &t.Title, &project, &t.Status, &t.CreatedAt)
			if err != nil {
				fmt.Printf("Error scanning row: %v\n", err)
				continue
			}
			t.Project = project.String
			fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\n", t.ID, t.Project, t.Status, t.Title, t.CreatedAt.Format("2006-01-02"))
		}
		w.Flush()

		if count == 0 {
			fmt.Println("No matches found.")
		}
	},
}

func init() {
	rootCmd.AddCommand(searchCmd)
}
