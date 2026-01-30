package cmd

import (
	"database/sql"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/aescoubas/tasc/internal/db"
	"github.com/spf13/cobra"
)

// Flags
var (
	projDesc    string
	projNewName string
	projUnlink  bool
	projCascade bool
)

var projectCmd = &cobra.Command{
	Use:   "project",
	Short: "Manage projects",
	Long:  `Create, list, edit, and delete projects.`, // Corrected: Removed unnecessary backticks around the string literal
}

var projectListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all projects",
	Run: func(cmd *cobra.Command, args []string) {
		query := `
			SELECT p.name, p.description, COUNT(t.id) as task_count 
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

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
		fmt.Fprintln(w, "Name\tTasks\tDescription")
		fmt.Fprintln(w, "----\t-----\t-----------")

		count := 0
		for rows.Next() {
			var name string
			var desc sql.NullString
			var taskCount int
			if err := rows.Scan(&name, &desc, &taskCount); err != nil {
				continue
			}
			fmt.Fprintf(w, "%s\t%d\t%s\n", name, taskCount, desc.String)
			count++
		}
		w.Flush()

		if count == 0 {
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
		
		query := `INSERT INTO projects (name, description) VALUES (?, ?)`
		_, err := db.DB.Exec(query, name, projDesc)
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
	Short: "Edit a project (rename or update description)",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		oldName := args[0]

		tx, err := db.DB.Begin()
		if err != nil {
			fmt.Printf("Error starting transaction: %v\n", err)
			return
		}
		defer tx.Rollback()

		// check existence
		var exists int
		err = tx.QueryRow("SELECT 1 FROM projects WHERE name = ?", oldName).Scan(&exists)
		if err != nil {
			fmt.Printf("Project '%s' not found.\n", oldName)
			return
		}

		// Update description
		if cmd.Flags().Changed("desc") {
			_, err = tx.Exec("UPDATE projects SET description = ? WHERE name = ?", projDesc, oldName)
			if err != nil {
				fmt.Printf("Error updating description: %v\n", err)
				return
			}
		}

		// Rename
		if cmd.Flags().Changed("name") && projNewName != "" {
			// Update projects table
			_, err = tx.Exec("UPDATE projects SET name = ? WHERE name = ?", projNewName, oldName)
			if err != nil {
				fmt.Printf("Error renaming project: %v\n", err)
				return
			}
			// Update tasks table
			_, err = tx.Exec("UPDATE tasks SET project = ? WHERE project = ?", projNewName, oldName)
			if err != nil {
				fmt.Printf("Error updating associated tasks: %v\n", err)
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

		// Check for tasks
		var taskCount int
		err = tx.QueryRow("SELECT COUNT(*) FROM tasks WHERE project = ? AND status != 'deleted'", name).Scan(&taskCount)
		if err != nil {
			fmt.Printf("Error checking tasks: %v\n", err)
			return
		}

		if taskCount > 0 {
			if !projUnlink && !projCascade {
				fmt.Printf("Project '%s' has %d active tasks.\n", name, taskCount)
				fmt.Println("Use --unlink to keep tasks (remove project association) or --cascade to delete tasks.")
				return
			}

			if projCascade {
				_, err = tx.Exec("UPDATE tasks SET status = 'deleted' WHERE project = ?", name)
				if err != nil {
					fmt.Printf("Error deleting associated tasks: %v\n", err)
					return
				}
				fmt.Printf("Deleted %d associated tasks.\n", taskCount)
			} else if projUnlink {
				_, err = tx.Exec("UPDATE tasks SET project = NULL WHERE project = ?", name)
				if err != nil {
					fmt.Printf("Error unlinking associated tasks: %v\n", err)
					return
				}
				fmt.Printf("Unlinked %d tasks.\n", taskCount)
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

	projectCmd.AddCommand(projectEditCmd)
	projectEditCmd.Flags().StringVarP(&projNewName, "name", "n", "", "New project name")
	projectEditCmd.Flags().StringVarP(&projDesc, "desc", "d", "", "New description")

	projectCmd.AddCommand(projectDeleteCmd)
	projectDeleteCmd.Flags().BoolVar(&projUnlink, "unlink", false, "Unlink tasks (keep them but remove project)")
	projectDeleteCmd.Flags().BoolVar(&projCascade, "cascade", false, "Delete associated tasks")
}
