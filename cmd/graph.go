package cmd

import (
	"database/sql"
	"fmt"

	"github.com/aescoubas/tasc/internal/config"
	"github.com/aescoubas/tasc/internal/db"
	"github.com/aescoubas/tasc/internal/models"
	"github.com/aescoubas/tasc/internal/ui"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

var graphCmd = &cobra.Command{
	Use:   "graph",
	Short: "Visualize task dependencies",
	Run: func(cmd *cobra.Command, args []string) {
		cfg, _ := config.LoadConfig()

		// Fetch all dependencies with details for coloring
		rows, err := db.DB.Query(`
			SELECT 
				t1.id, t1.description, t1.project, t1.status, t1.due_at, t1.scheduled_at,
				t2.id, t2.description, t2.project, t2.status, t2.due_at, t2.scheduled_at
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
		adj := make(map[int64][]models.Task)
		tasks := make(map[int64]models.Task)

		hasIncoming := make(map[int64]bool)

		for rows.Next() {
			var t1, t2 models.Task
			var due1, sch1, due2, sch2 sql.NullTime

			if err := rows.Scan(
				&t1.ID, &t1.Description, &t1.Project, &t1.Status, &due1, &sch1,
				&t2.ID, &t2.Description, &t2.Project, &t2.Status, &due2, &sch2,
			); err != nil {
				continue
			}

			if due1.Valid { t1.DueAt = &due1.Time }
			if sch1.Valid { t1.ScheduledAt = &sch1.Time }
			if due2.Valid { t2.DueAt = &due2.Time }
			if sch2.Valid { t2.ScheduledAt = &sch2.Time }

			adj[t1.ID] = append(adj[t1.ID], t2)
			tasks[t1.ID] = t1
			tasks[t2.ID] = t2
			hasIncoming[t2.ID] = true
		}

		// Mark blocked tasks
		for id, t := range tasks {
			if hasIncoming[id] {
				t.IsBlocked = true
				tasks[id] = t
			}
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
				printTree(id, adj, tasks, 0, cfg)
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
			printTree(root, adj, tasks, 0, cfg)
		}
	},
}

func printTree(id int64, adj map[int64][]models.Task, tasks map[int64]models.Task, level int, cfg config.Config) {
	indent := ""
	for i := 0; i < level; i++ {
		indent += "  "
	}

	arrow := "└─"
	if level == 0 {
		arrow = "●"
	}

	t := tasks[id]
	// Use FormatTask for consistent rendering (handles color, icons, etc.)
	// But we might want to trim it slightly if it's too long? FormatTask doesn't truncate.
	line := ui.FormatTask(t, cfg)
	fmt.Printf("%s%s %s\n", indent, arrow, line)

	for _, child := range adj[id] {
		printTree(child.ID, adj, tasks, level+1, cfg)
	}
}

func init() {
	rootCmd.AddCommand(graphCmd)
}
