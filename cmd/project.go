package cmd

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"slices"
	"strings"
	"time"

	"github.com/aescoubas/tasc/internal/config"
	"github.com/aescoubas/tasc/internal/db"
	"github.com/aescoubas/tasc/internal/models"
	"github.com/aescoubas/tasc/internal/parse"
	"github.com/aescoubas/tasc/internal/store"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"golang.org/x/term"
	"gopkg.in/yaml.v3"
)

// Flags
var (
	projDesc     string
	projNewName  string
	projUnlink   bool
	projCascade  bool
	projArchived bool
	projParent   string
	projDue      string
	projStatus   string
)

var projectCmd = &cobra.Command{
	Use:   "project",
	Short: "Manage projects",
	Long:  `Create, list, edit, and delete projects.`,
}

var projectListCmd = &cobra.Command{
	Use:   "list",
	Short: "List projects",
	RunE: func(cmd *cobra.Command, args []string) error {
		allProjects, err := CurrentStore.ListProjects()
		if err != nil {
			return fmt.Errorf("error loading projects: %w", err)
		}
		tasks, err := CurrentStore.ListTasks(store.TaskFilter{IncludeDeleted: true})
		if err != nil {
			return fmt.Errorf("error loading project tasks: %w", err)
		}

		totals := make(map[string]int)
		completed := make(map[string]int)
		for _, task := range tasks {
			if task.Status != models.StatusDeleted {
				totals[task.Project]++
				if task.Status == models.StatusDone {
					completed[task.Project]++
				}
			}
		}
		children := make(map[string][]models.Project)
		var roots []models.Project
		for _, project := range allProjects {
			if project.Parent == "" {
				roots = append(roots, project)
				continue
			}
			children[project.Parent] = append(children[project.Parent], project)
		}

		width, _, err := term.GetSize(int(os.Stdout.Fd()))
		if err != nil || width <= 0 {
			width = 80
		}
		headers := []string{"Name", "Status", "Created", "Due", "Progress", "Tasks", "Description"}
		rows := [][]string{headers}
		singleLine := strings.NewReplacer("\r\n", " ", "\r", " ", "\n", " ", "\t", "    ")
		var addRows func(projects []models.Project, level int)
		addRows = func(projects []models.Project, level int) {
			for _, p := range projects {
				if p.Status == models.ProjectStatusArchived && !projArchived {
					addRows(children[p.Name], level)
					continue
				}

				name := singleLine.Replace(p.Name)
				if level > 0 {
					name = strings.Repeat("  ", level) + "└─ " + name
				}
				status := p.Status
				if status == "" {
					status = models.ProjectStatusActive
				}
				due := "-"
				if p.DueAt != nil {
					due = p.DueAt.Format(time.DateOnly)
				}
				progress := 0
				if totals[p.Name] > 0 {
					progress = completed[p.Name] * 100 / totals[p.Name]
				}
				description := p.Description
				if end := strings.IndexAny(description, "\r\n"); end >= 0 {
					description = strings.TrimRight(description[:end], " \t") + "..."
				}
				rows = append(rows, []string{name, singleLine.Replace(string(status)), p.CreatedAt.Format(time.DateOnly), due,
					fmt.Sprintf("%d%%", progress), fmt.Sprint(totals[p.Name]), singleLine.Replace(description)})
				addRows(children[p.Name], level+1)
			}
		}
		addRows(roots, 0)
		if len(rows) == 1 {
			fmt.Println("No projects found.")
			return nil
		}
		printProjectTable(rows, width)
		return nil
	},
}

// printProjectTable measures terminal cells so wide characters align with the headers.
func printProjectTable(rows [][]string, width int) {
	headers := rows[0]
	widths := make([]int, len(headers))
	for _, row := range rows {
		for i, value := range row {
			widths[i] = max(widths[i], min(lipgloss.Width(value), width))
		}
	}
	const gap = "  "
	total := len(gap) * (len(widths) - 1)
	for _, w := range widths {
		total += w
	}
	for total > width {
		widest := slices.Index(widths, slices.Max(widths))
		if widths[widest] <= 1 {
			break
		}
		widths[widest]--
		total--
	}

	printRow := func(row []string) {
		cells := make([]string, len(row))
		for i, value := range row {
			columnWidth := widths[i]
			cells[i] = lipgloss.NewStyle().Width(columnWidth).Render(truncateProjectCell(value, columnWidth))
		}
		fmt.Println(strings.TrimRight(strings.Join(cells, gap), " "))
	}
	printRow(headers)
	rule := make([]string, len(headers))
	for i, name := range headers {
		rule[i] = strings.Repeat("-", min(len(name), widths[i]))
	}
	printRow(rule)
	for _, row := range rows[1:] {
		printRow(row)
	}
}

func truncateProjectCell(value string, width int) string {
	if lipgloss.Width(value) <= width {
		return value
	}
	const ellipsis = "..."
	if width <= len(ellipsis) {
		return ellipsis[:width]
	}
	cropped := lipgloss.NewStyle().MaxWidth(width - len(ellipsis)).Render(value)
	return strings.TrimRight(cropped, " ") + ellipsis
}

var projectCreateCmd = &cobra.Command{
	Use:   "create [name]",
	Short: "Create a new project",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.LoadConfig()
		if err != nil {
			cfg = config.DefaultConfig()
		}

		name := args[0]

		var dueAt *time.Time
		if projDue != "" {
			t, err := parse.Date(projDue, cfg.EndOfDay)
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
		_, err = db.DB.Exec(query, name, projDesc, parent, dueAt)
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
	Use:     "edit [old_name]",
	Aliases: []string{"modify"},
	Short:   "Edit a project",
	Args:    cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.LoadConfig()
		if err != nil {
			cfg = config.DefaultConfig()
		}

		oldName := args[0]

		// 1. Fetch current project state
		p, err := CurrentStore.GetProject(oldName)
		if err != nil {
			fmt.Printf("Error fetching project '%s': %v\n", oldName, err)
			return
		}

		// 2. Check if flags are used
		flagsChanged := false
		cmd.Flags().Visit(func(f *pflag.Flag) {
			flagsChanged = true
		})

		if flagsChanged {
			// Apply flag overrides
			if cmd.Flags().Changed("desc") {
				p.Description = projDesc
			}
			if cmd.Flags().Changed("parent") {
				if projParent == oldName {
					fmt.Println("Cannot set parent to self.")
					return
				}
				// Verify parent exists if set
				if projParent != "" {
					_, err := CurrentStore.GetProject(projParent)
					if err != nil {
						fmt.Printf("Parent project '%s' not found.\n", projParent)
						return
					}
				}
				p.Parent = projParent
			}
			if cmd.Flags().Changed("due") {
				if projDue == "" {
					p.DueAt = nil
				} else {
					t, err := parse.Date(projDue, cfg.EndOfDay)
					if err != nil {
						fmt.Printf("Invalid due date: %v\n", err)
						return
					}
					p.DueAt = t
				}
			}
			if cmd.Flags().Changed("status") {
				p.Status = models.ProjectStatus(projStatus)
			}

			// New Name handling
			if cmd.Flags().Changed("name") && projNewName != "" {
				p.Name = projNewName
			}

		} else {
			// 3. Interactive Mode
			type projectEditYAML struct {
				Name        string `yaml:"name"`
				ShortName   string `yaml:"short_name"`
				Description string `yaml:"description"`
				Parent      string `yaml:"parent,omitempty"`
				Status      string `yaml:"status"`
				DueAt       string `yaml:"due_at,omitempty"`
			}

			// Map to YAML struct
			y := projectEditYAML{
				Name:        p.Name,
				ShortName:   p.ShortName,
				Description: p.Description,
				Parent:      p.Parent,
				Status:      string(p.Status),
			}
			if p.DueAt != nil {
				y.DueAt = p.DueAt.Format("2006-01-02")
			}

			// Marshal
			data, err := yaml.Marshal(y)
			if err != nil {
				fmt.Printf("Error preparing editor: %v\n", err)
				return
			}

			// Create temp file
			tmpfile, err := ioutil.TempFile("", "tasc-project-*.yaml")
			if err != nil {
				fmt.Printf("Error creating temp file: %v\n", err)
				return
			}
			defer os.Remove(tmpfile.Name())

			if _, err := tmpfile.Write(data); err != nil {
				fmt.Printf("Error writing temp file: %v\n", err)
				return
			}
			if err := tmpfile.Close(); err != nil {
				fmt.Printf("Error closing temp file: %v\n", err)
				return
			}

			// Open Editor
			editor := os.Getenv("EDITOR")
			if editor == "" {
				editor = "vim"
			}

			editCmd := exec.Command(editor, tmpfile.Name())
			editCmd.Stdin = os.Stdin
			editCmd.Stdout = os.Stdout
			editCmd.Stderr = os.Stderr

			if err := editCmd.Run(); err != nil {
				fmt.Printf("Error running editor: %v\n", err)
				return
			}

			// Read back
			newData, err := ioutil.ReadFile(tmpfile.Name())
			if err != nil {
				fmt.Printf("Error reading updated file: %v\n", err)
				return
			}

			// Unmarshal
			var newY projectEditYAML
			if err := yaml.Unmarshal(newData, &newY); err != nil {
				fmt.Printf("Error parsing YAML: %v\n", err)
				return
			}

			// Apply back to p
			p.Name = newY.Name
			p.ShortName = newY.ShortName
			p.Description = newY.Description
			p.Parent = newY.Parent
			p.Status = models.ProjectStatus(newY.Status)

			if newY.DueAt != "" {
				t, err := parse.Date(newY.DueAt, cfg.EndOfDay)
				if err != nil {
					fmt.Printf("Invalid due date in YAML: %v\n", err)
					return
				}
				p.DueAt = t
			} else {
				p.DueAt = nil
			}
		}

		// 4. Update
		if err := CurrentStore.UpdateProject(oldName, p); err != nil {
			fmt.Printf("Error updating project: %v\n", err)
			return
		}

		fmt.Printf("Project '%s' updated.\n", p.Name)
	},
}

var projectArchiveCmd = &cobra.Command{
	Use:   "archive [name]",
	Short: "Archive a project",
	Args:  cobra.ExactArgs(1),
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
				if err != nil {
					return nil, err
				}
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
	projectListCmd.Flags().BoolVar(&projArchived, "archived", false, "Include archived projects")

	projectCmd.AddCommand(projectCreateCmd)
	projectCreateCmd.Flags().StringVarP(&projDesc, "desc", "d", "", "Project description")
	projectCreateCmd.Flags().StringVarP(&projParent, "parent", "p", "", "Parent project")
	projectCreateCmd.Flags().StringVarP(&projDue, "due", "t", "", "Due date")
	projectCreateCmd.RegisterFlagCompletionFunc("due", dateCompletionFunc)

	projectCmd.AddCommand(projectEditCmd)
	projectEditCmd.Flags().StringVarP(&projNewName, "name", "n", "", "New project name")
	projectEditCmd.Flags().StringVarP(&projDesc, "desc", "d", "", "New description")
	projectEditCmd.Flags().StringVarP(&projParent, "parent", "p", "", "New parent project")
	projectEditCmd.Flags().StringVarP(&projDue, "due", "t", "", "New due date")
	projectEditCmd.Flags().StringVarP(&projStatus, "status", "s", "", "New status (active/archived)")
	projectEditCmd.RegisterFlagCompletionFunc("due", dateCompletionFunc)

	projectCmd.AddCommand(projectArchiveCmd)

	projectCmd.AddCommand(projectDeleteCmd)
	projectDeleteCmd.Flags().BoolVar(&projUnlink, "unlink", false, "Unlink tasks")
	projectDeleteCmd.Flags().BoolVar(&projCascade, "cascade", false, "Delete associated tasks and subprojects")
}
