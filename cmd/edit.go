package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"

	"strings"

	"github.com/aescoubas/tasc/internal/models"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var editCmd = &cobra.Command{
	Use:   "edit [id]",
	Short: "Edit a task in your default editor",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil {
			return fmt.Errorf("invalid task ID: %s", args[0])
		}
		t, err := CurrentStore.GetTask(id)
		if err != nil {
			return fmt.Errorf("error fetching task: %w", err)
		}

		// 2. Marshal to YAML
		data, err := yaml.Marshal(&t)
		if err != nil {
			return fmt.Errorf("error marshaling task: %w", err)
		}

		// Post-process YAML to force multiline description
		var node yaml.Node
		if err := yaml.Unmarshal(data, &node); err != nil {
			return fmt.Errorf("error processing YAML: %w", err)
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
			return fmt.Errorf("error re-marshaling task: %w", err)
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
			return fmt.Errorf("error creating temp file: %w", err)
		}
		defer os.Remove(tmpFile.Name())

		if _, err := tmpFile.Write(data); err != nil {
			return fmt.Errorf("error writing temp file: %w", err)
		}
		if err := tmpFile.Close(); err != nil {
			return fmt.Errorf("error closing temp file: %w", err)
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
			return fmt.Errorf("error running editor: %w", err)
		}

		// 5. Read back
		newData, err := os.ReadFile(tmpFile.Name())
		if err != nil {
			return fmt.Errorf("error reading temp file: %w", err)
		}

		// 6. Unmarshal
		var newT models.Task
		if err := yaml.Unmarshal(newData, &newT); err != nil {
			return fmt.Errorf("error parsing YAML: %w", err)
		}
		if newT.ID != t.ID {
			return fmt.Errorf("task ID cannot be changed in the editor")
		}

		// 7. Update DB
		rescheduleIncrement := 0
		if (t.DueAt == nil && newT.DueAt != nil) ||
			(t.DueAt != nil && newT.DueAt == nil) ||
			(t.DueAt != nil && newT.DueAt != nil && !t.DueAt.Equal(*newT.DueAt)) {
			rescheduleIncrement = 1
		}

		newT.RescheduleCount = t.RescheduleCount + rescheduleIncrement
		if err := CurrentStore.UpdateTask(newT); err != nil {
			return fmt.Errorf("error updating task: %w", err)
		}

		fmt.Printf("Task %d updated.\n", newT.ID)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(editCmd)
}
