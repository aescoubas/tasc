package cmd

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/aescoubas/tasc/internal/db"
	"github.com/spf13/cobra"
)

var statsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show task statistics",
	Run: func(cmd *cobra.Command, args []string) {
		query := `SELECT status, COUNT(*) FROM tasks GROUP BY status`
	
rows, err := db.DB.Query(query)
		if err != nil {
			fmt.Printf("Error querying stats: %v\n", err)
			return
		}
		defer rows.Close()

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
		fmt.Fprintln(w, "Status\tCount")

		for rows.Next() {
			var status string
			var count int
			if err := rows.Scan(&status, &count); err != nil {
				fmt.Printf("Error scanning row: %v\n", err)
				continue
			}
			fmt.Fprintf(w, "%s\t%d\n", status, count)
		}
		w.Flush()
	},
}

func init() {
	rootCmd.AddCommand(statsCmd)
}

