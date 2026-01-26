package cmd

import (
	"database/sql"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"

	"github.com/aescoubas/tasc/internal/db"
	"github.com/aescoubas/tasc/internal/models"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var editCmd = &cobra.Command{
	Use:   "edit [id]",
	Short: "Edit a task in your default editor",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		idStr := args[0]
		
		// 1. Fetch Task
		var t models.Task
		var project sql.NullString
		query := "SELECT id, description, project, status, created_at, due_at FROM tasks WHERE id = ?"
		row := db.DB.QueryRow(query, idStr)
		
		err := row.Scan(&t.ID, &t.Description, &project, &t.Status, &t.CreatedAt, &t.DueAt)
		if err != nil {
			fmt.Printf("Error fetching task: %v\n", err)
			return
		}
		t.Project = project.String

		// 2. Marshal to YAML
		data, err := yaml.Marshal(&t)
		if err != nil {
			fmt.Printf("Error marshaling task: %v\n", err)
			return
		}

		// 3. Write to temp file
		tmpFile, err := ioutil.TempFile("", "tasc-*.yaml")
		if err != nil {
			fmt.Printf("Error creating temp file: %v\n", err)
			return
		}
		defer os.Remove(tmpFile.Name())

		if _, err := tmpFile.Write(data); err != nil {
			fmt.Printf("Error writing temp file: %v\n", err)
			return
		}
		if err := tmpFile.Close(); err != nil {
			fmt.Printf("Error closing temp file: %v\n", err)
			return
		}

		// 4. Open Editor
		editor := os.Getenv("EDITOR")
		if editor == "" {
			editor = "vim" // Default fallback
		}

		c := exec.Command(editor, tmpFile.Name())
		c.Stdin = os.Stdin
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr

		if err := c.Run(); err != nil {
			fmt.Printf("Error running editor: %v\n", err)
			return
		}

		// 5. Read back
		newData, err := ioutil.ReadFile(tmpFile.Name())
		if err != nil {
			fmt.Printf("Error reading temp file: %v\n", err)
			return
		}

		// 6. Unmarshal
		var newT models.Task
		if err := yaml.Unmarshal(newData, &newT); err != nil {
			fmt.Printf("Error parsing YAML: %v\n", err)
			return
		}

		// 7. Update DB
		updateQuery := `UPDATE tasks SET description = ?, project = ?, status = ?, due_at = ? WHERE id = ?`
		_, err = db.DB.Exec(updateQuery, newT.Description, newT.Project, newT.Status, newT.DueAt, newT.ID)
		if err != nil {
			fmt.Printf("Error updating task: %v\n", err)
			return
		}

		fmt.Printf("Task %d updated.\n", newT.ID)
	},
}

func init() {
	rootCmd.AddCommand(editCmd)
}
