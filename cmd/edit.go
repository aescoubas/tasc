package cmd

import (
	"database/sql"
	"fmt"
	"os"
	"os/exec"

	"strings"

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
		var description sql.NullString
		var estimate sql.NullString
		var recurrence sql.NullString

		query := "SELECT id, title, description, project, status, created_at, due_at, scheduled_at, estimate, recurrence FROM tasks WHERE id = ?"
		row := db.DB.QueryRow(query, idStr)

		err := row.Scan(&t.ID, &t.Title, &description, &project, &t.Status, &t.CreatedAt, &t.DueAt, &t.ScheduledAt, &estimate, &recurrence)
		if err != nil {
			fmt.Printf("Error fetching task: %v\n", err)
			return
		}
		t.Description = description.String
		t.Project = project.String
		t.Estimate = estimate.String
		t.Recurrence = recurrence.String

		// 2. Marshal to YAML
		data, err := yaml.Marshal(&t)
		if err != nil {
			fmt.Printf("Error marshaling task: %v\n", err)
			return
		}

		// Post-process YAML to force multiline description
		var node yaml.Node
		if err := yaml.Unmarshal(data, &node); err != nil {
			fmt.Printf("Error processing YAML: %v\n", err)
			return
		}

		if len(node.Content) > 0 && node.Content[0].Kind == yaml.MappingNode {
			mapping := node.Content[0]
			found := false
			for i := 0; i < len(mapping.Content); i += 2 {
				if mapping.Content[i].Value == "description" {
					mapping.Content[i+1].Style = yaml.LiteralStyle
					found = true
					break
				}
			}

			if !found {
				keyNode := &yaml.Node{Kind: yaml.ScalarNode, Value: "description"}
				valNode := &yaml.Node{Kind: yaml.ScalarNode, Value: "", Style: yaml.LiteralStyle}

				// Insert after title
				inserted := false
				for i := 0; i < len(mapping.Content); i += 2 {
					if mapping.Content[i].Value == "title" {
						// Insert at i+2
						mapping.Content = append(mapping.Content[:i+2], append([]*yaml.Node{keyNode, valNode}, mapping.Content[i+2:]...)...)
						inserted = true
						break
					}
				}
				if !inserted {
					mapping.Content = append(mapping.Content, keyNode, valNode)
				}
			}
		}

		data, err = yaml.Marshal(&node)
		if err != nil {
			fmt.Printf("Error re-marshaling task: %v\n", err)
			return
		}

		// Force |- for empty strings if yaml.v3 output ""
		sData := string(data)
		if strings.Contains(sData, "description: \"\"") {
			sData = strings.Replace(sData, "description: \"\"", "description: |-", 1)
			data = []byte(sData)
		}

		// 3. Write to temp file
		tmpFile, err := os.CreateTemp("", "tasc-*.yaml")
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
		newData, err := os.ReadFile(tmpFile.Name())
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
		rescheduleIncrement := 0
		if (t.ScheduledAt == nil && newT.ScheduledAt != nil) ||
			(t.ScheduledAt != nil && newT.ScheduledAt == nil) ||
			(t.ScheduledAt != nil && newT.ScheduledAt != nil && !t.ScheduledAt.Equal(*newT.ScheduledAt)) {
			rescheduleIncrement = 1
		}

		updateQuery := `UPDATE tasks SET title = ?, description = ?, project = ?, status = ?, due_at = ?, scheduled_at = ?, estimate = ?, recurrence = ?, reschedule_count = reschedule_count + ? WHERE id = ?`
		_, err = db.DB.Exec(updateQuery, newT.Title, newT.Description, newT.Project, newT.Status, newT.DueAt, newT.ScheduledAt, newT.Estimate, newT.Recurrence, rescheduleIncrement, newT.ID)
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
