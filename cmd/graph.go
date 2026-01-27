package cmd

import (
	"fmt"

	"github.com/aescoubas/tasc/internal/db"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

var graphCmd = &cobra.Command{
	Use:   "graph",
	Short: "Visualize task dependencies",
	Run: func(cmd *cobra.Command, args []string) {
		// Fetch all dependencies
		rows, err := db.DB.Query(`
			SELECT 
				t1.id, t1.description, 
				t2.id, t2.description 
			FROM task_dependencies td
			JOIN tasks t1 ON td.blocker_id = t1.id
			JOIN tasks t2 ON td.blocked_id = t2.id
			WHERE t1.status != 'deleted' AND t2.status != 'deleted'
		`)
		if err != nil {
			fmt.Printf("Error querying dependencies: %v\n", err)
			return
		}
		defer rows.Close()

		// Build adjacency list
		adj := make(map[int64][]struct {
			ID   int64
			Desc string
		})
		tasks := make(map[int64]string)

		hasIncoming := make(map[int64]bool)

		for rows.Next() {
			var bID, bdID int64
			var bDesc, bdDesc string
			if err := rows.Scan(&bID, &bDesc, &bdID, &bdDesc); err != nil {
				continue
			}
			adj[bID] = append(adj[bID], struct {
				ID   int64
				Desc string
			}{bdID, bdDesc})
			tasks[bID] = bDesc
			tasks[bdID] = bdDesc
			hasIncoming[bdID] = true
		}

		// Find roots (tasks that are not blocked by anything in this graph subset)
		var roots []int64
		for id := range adj {
			if !hasIncoming[id] {
				roots = append(roots, id)
			}
		}

		if len(roots) == 0 && len(adj) > 0 {
			fmt.Println("Circular dependency detected or no clear roots.")
			// Just print all
			for id := range adj {
				printTree(id, adj, tasks, 0)
			}
			return
		}

		if len(adj) == 0 {
			fmt.Println("No dependencies found.")
			return
		}

		style := lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Bold(true)

		fmt.Println(style.Render("Dependency Graph"))
		for _, root := range roots {
			printTree(root, adj, tasks, 0)
		}
	},
}

func printTree(id int64, adj map[int64][]struct {
	ID   int64
	Desc string
}, tasks map[int64]string, level int) {
	indent := ""
	for i := 0; i < level; i++ {
		indent += "  "
	}

	arrow := "└─"
	if level == 0 {
		arrow = "●"
	}

	fmt.Printf("%s%s %d: %s\n", indent, arrow, id, tasks[id])

	for _, child := range adj[id] {
		printTree(child.ID, adj, tasks, level+1)
	}
}

func init() {
	rootCmd.AddCommand(graphCmd)
}
