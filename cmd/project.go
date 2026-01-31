package cmd

import (
	"database/sql"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/aescoubas/tasc/internal/db"
	"github.com/aescoubas/tasc/internal/parse"
	"github.com/spf13/cobra"
)

// Flags
var (
	projDesc    string
	projNewName string
	projUnlink  bool
	projCascade bool
	projParent  string
	projDue     string
	projStatus  string
)

var projectCmd = &cobra.Command{
	Use:   "project",
	Short: "Manage projects",
	Long:  `Create, list, edit, and delete projects.`, 
}

var projectListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all projects",
	Run: func(cmd *cobra.Command, args []string) {
		query := `
			SELECT 
				p.name, 
				p.description, 
				p.parent, 
				p.status, 
				p.due_at, 
				COUNT(t.id) as total_tasks,
				SUM(CASE WHEN t.status = 'done' THEN 1 ELSE 0 END) as completed_tasks
			FROM projects p 
			LEFT JOIN tasks t ON p.name = t.project AND t.status != 'deleted' 
			GROUP BY p.name 
			ORDER BY p.name
		`
		rows, err := db.DB.Query(query)
		if err != nil {
			fmt.Printf("Error querying projects: %v\n", err)
			return
		}
		defer rows.Close()

		type proj struct {
			name        string
			description string
			parent      string
			status      string
			due         *time.Time
			totalTasks  int
			doneTasks   int
		}

		var projects []proj
		for rows.Next() {
			var p proj
			var desc, parent, status sql.NullString
			var due sql.NullTime
			var doneTasks sql.NullInt64 // Safety for SUM
			
			if err := rows.Scan(&p.name, &desc, &parent, &status, &due, &p.totalTasks, &doneTasks); err != nil {
				continue
			}
			p.description = desc.String
			p.parent = parent.String
			p.status = status.String
			if due.Valid {
				p.due = &due.Time
			}
			if doneTasks.Valid {
				p.doneTasks = int(doneTasks.Int64)
			}
			projects = append(projects, p)
		}

		// Build tree
		tree := make(map[string][]proj)
		var roots []proj
		
		for _, p := range projects {
			if p.parent == "" {
				roots = append(roots, p)
			} else {
				tree[p.parent] = append(tree[p.parent], p)
			}
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
		fmt.Fprintln(w, "Name\tStatus\tDue\tProgress\tTasks\tDescription")
		fmt.Fprintln(w, "----\t------\t---\t--------\t-----\t-----------")

		var printTree func(p proj, level int)
		printTree = func(p proj, level int) {
			indent := strings.Repeat("  ", level)
			prefix := ""
			if level > 0 {
				prefix = "└─ "
			}
			
			dueStr := "-"
			if p.due != nil {
				dueStr = p.due.Format("2006-01-02")
			}
			
			status := p.status
			if status == "" { status = "active" }

			pct := 0
			if p.totalTasks > 0 {
				pct = (p.doneTasks * 100) / p.totalTasks
			}
			progStr := fmt.Sprintf("%d%%", pct)

			fmt.Fprintf(w, "%s%s%s\t%s\t%s\t%s\t%d\t%s\n", indent, prefix, p.name, status, dueStr, progStr, p.totalTasks, p.description)

			children := tree[p.name]
			for _, child := range children {
				printTree(child, level+1)
			}
		}

		for _, root := range roots {
			printTree(root, 0)
		}
		w.Flush()

		if len(projects) == 0 {
			fmt.Println("No projects found.")
		}
	},
}

var projectCreateCmd = &cobra.Command{
	Use:   "create [name]",
	Short: "Create a new project",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]
		
		var dueAt *time.Time
		if projDue != "" {
			t, err := parse.Date(projDue)
			if err != nil {
				fmt.Printf("Invalid due date: %v\n", err)
				return
			}
			dueAt = t
		}

		// Validate parent
		var parent *string
		if projParent != "" {
			var exists int
			err := db.DB.QueryRow("SELECT 1 FROM projects WHERE name = ?", projParent).Scan(&exists)
			if err != nil {
				fmt.Printf("Parent project '%s' not found.\n", projParent)
				return
			}
			parent = &projParent
		}

		query := `INSERT INTO projects (name, description, parent, due_at, status) VALUES (?, ?, ?, ?, 'active')`
		_, err := db.DB.Exec(query, name, projDesc, parent, dueAt)
		if err != nil {
			if strings.Contains(err.Error(), "UNIQUE constraint failed") {
				fmt.Printf("Project '%s' already exists.\n", name)
			} else {
				fmt.Printf("Error creating project: %v\n", err)
			}
			return
		}
		fmt.Printf("Project '%s' created.\n", name)
	},
}

var projectEditCmd = &cobra.Command{
	Use:   "edit [old_name]",
	Short: "Edit a project",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		oldName := args[0]

		tx, err := db.DB.Begin()
		if err != nil {
			fmt.Printf("Error starting transaction: %v\n", err)
			return
		}
		defer tx.Rollback()

		var exists int
		err = tx.QueryRow("SELECT 1 FROM projects WHERE name = ?", oldName).Scan(&exists)
		if err != nil {
			fmt.Printf("Project '%s' not found.\n", oldName)
			return
		}

		if cmd.Flags().Changed("desc") {
			_, err = tx.Exec("UPDATE projects SET description = ? WHERE name = ?", projDesc, oldName)
			if err != nil {
				fmt.Printf("Error updating description: %v\n", err)
				return
			}
		}
		
		if cmd.Flags().Changed("parent") {
			var p *string
			if projParent != "" {
				// Prevent circular? A -> B -> A
				// Simple check: Parent cannot be self.
				if projParent == oldName {
					fmt.Println("Cannot set parent to self.")
					return
				}
				// Verify parent exists
				var pExists int
				err := tx.QueryRow("SELECT 1 FROM projects WHERE name = ?", projParent).Scan(&pExists)
				if err != nil {
					fmt.Printf("Parent project '%s' not found.\n", projParent)
					return
				}
				p = &projParent
			}
			_, err = tx.Exec("UPDATE projects SET parent = ? WHERE name = ?", p, oldName)
			if err != nil {
				fmt.Printf("Error updating parent: %v\n", err)
				return
			}
		}

		if cmd.Flags().Changed("due") {
			var d *time.Time
			if projDue != "" {
				t, err := parse.Date(projDue)
				if err != nil {
					fmt.Printf("Invalid due date: %v\n", err)
					return
				}
				d = t
			}
			_, err = tx.Exec("UPDATE projects SET due_at = ? WHERE name = ?", d, oldName)
			if err != nil {
				fmt.Printf("Error updating due date: %v\n", err)
				return
			}
		}

		if cmd.Flags().Changed("status") {
			_, err = tx.Exec("UPDATE projects SET status = ? WHERE name = ?", projStatus, oldName)
			if err != nil {
				fmt.Printf("Error updating status: %v\n", err)
				return
			}
		}

		if cmd.Flags().Changed("name") && projNewName != "" {
			_, err = tx.Exec("UPDATE projects SET name = ? WHERE name = ?", projNewName, oldName)
			if err != nil {
				fmt.Printf("Error renaming project: %v\n", err)
				return
			}
			// Update tasks
			_, err = tx.Exec("UPDATE tasks SET project = ? WHERE project = ?", projNewName, oldName)
			if err != nil {
				fmt.Printf("Error updating tasks: %v\n", err)
				return
			}
			// Update child projects
			_, err = tx.Exec("UPDATE projects SET parent = ? WHERE parent = ?", projNewName, oldName)
			if err != nil {
				fmt.Printf("Error updating child projects: %v\n", err)
				return
			}

			fmt.Printf("Project renamed to '%s'.\n", projNewName)
		}

		if err := tx.Commit(); err != nil {
			fmt.Printf("Error committing changes: %v\n", err)
			return
		}
		fmt.Println("Project updated.")
	},
}

var projectArchiveCmd = &cobra.Command{
	Use: "archive [name]",
	Short: "Archive a project",
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]
		_, err := db.DB.Exec("UPDATE projects SET status = 'archived' WHERE name = ?", name)
		if err != nil {
			fmt.Printf("Error archiving project: %v\n", err)
			return
		}
		fmt.Printf("Project '%s' archived.\n", name)
	},
}

var projectDeleteCmd = &cobra.Command{
	Use:   "delete [name]",
	Short: "Delete a project",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]

		tx, err := db.DB.Begin()
		if err != nil {
			fmt.Printf("Error starting transaction: %v\n", err)
			return
		}
		defer tx.Rollback()
		
		// Helper to find all descendants
		var findAllDescendants func(root string) ([]string, error)
		findAllDescendants = func(root string) ([]string, error) {
		
rows, err := tx.Query("SELECT name FROM projects WHERE parent = ?", root)
			if err != nil {
				return nil, err
			}
			defer rows.Close()
			
			var descendants []string
			for rows.Next() {
				var child string
				rows.Scan(&child)
				descendants = append(descendants, child)
				sub, err := findAllDescendants(child)
				if err != nil { return nil, err } 
				descendants = append(descendants, sub...)
			}
			return descendants, nil
		}

		// Check for tasks and subprojects
		var taskCount int
		err = tx.QueryRow("SELECT COUNT(*) FROM tasks WHERE project = ? AND status != 'deleted'", name).Scan(&taskCount)
		
		descendants, err := findAllDescendants(name)
		if err != nil {
			fmt.Printf("Error checking subprojects: %v\n", err)
			return
		}

		if taskCount > 0 || len(descendants) > 0 {
			if !projUnlink && !projCascade {
				fmt.Printf("Project '%s' has %d active tasks and %d subprojects.\n", name, taskCount, len(descendants))
				fmt.Println("Use --unlink to keep tasks (remove project association) or --cascade to delete EVERYTHING.")
				return
			}

			if projCascade {
				// Delete tasks in this project
				_, err = tx.Exec("UPDATE tasks SET status = 'deleted' WHERE project = ?", name)
				if err != nil {
					fmt.Printf("Error deleting associated tasks: %v\n", err)
					return
				}
				
				for _, d := range descendants {
					// Delete tasks for descendant
					_, err = tx.Exec("UPDATE tasks SET status = 'deleted' WHERE project = ?", d)
					// Delete project
					_, err = tx.Exec("DELETE FROM projects WHERE name = ?", d)
				}
				
				fmt.Printf("Deleted %d associated tasks and %d subprojects.\n", taskCount, len(descendants))
			} else if projUnlink {
				// Unlink tasks
				_, err = tx.Exec("UPDATE tasks SET project = NULL WHERE project = ?", name)
				// FK says ON DELETE SET NULL. So subprojects become root projects.
				fmt.Printf("Unlinked %d tasks. Subprojects are now top-level.\n", taskCount)
			}
		}

		_, err = tx.Exec("DELETE FROM projects WHERE name = ?", name)
		if err != nil {
			fmt.Printf("Error deleting project: %v\n", err)
			return
		}

		if err := tx.Commit(); err != nil {
			fmt.Printf("Error committing transaction: %v\n", err)
			return
		}
		fmt.Printf("Project '%s' deleted.\n", name)
	},
}

func init() {
	rootCmd.AddCommand(projectCmd)
	
	projectCmd.AddCommand(projectListCmd)
	
	projectCmd.AddCommand(projectCreateCmd)
	projectCreateCmd.Flags().StringVarP(&projDesc, "desc", "d", "", "Project description")
	projectCreateCmd.Flags().StringVarP(&projParent, "parent", "p", "", "Parent project")
	projectCreateCmd.Flags().StringVarP(&projDue, "due", "t", "", "Due date")

	projectCmd.AddCommand(projectEditCmd)
	projectEditCmd.Flags().StringVarP(&projNewName, "name", "n", "", "New project name")
	projectEditCmd.Flags().StringVarP(&projDesc, "desc", "d", "", "New description")
	projectEditCmd.Flags().StringVarP(&projParent, "parent", "p", "", "New parent project")
	projectEditCmd.Flags().StringVarP(&projDue, "due", "t", "", "New due date")
	projectEditCmd.Flags().StringVarP(&projStatus, "status", "s", "", "New status (active/archived)")

	projectCmd.AddCommand(projectArchiveCmd)

	projectCmd.AddCommand(projectDeleteCmd)
	projectDeleteCmd.Flags().BoolVar(&projUnlink, "unlink", false, "Unlink tasks")
	projectDeleteCmd.Flags().BoolVar(&projCascade, "cascade", false, "Delete associated tasks and subprojects")
}