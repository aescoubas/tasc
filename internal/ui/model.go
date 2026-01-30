package ui

import (
	"database/sql"
	"log"
	"sort"

	"github.com/aescoubas/tasc/internal/db"
	"github.com/aescoubas/tasc/internal/models"
	"github.com/aescoubas/tasc/internal/priority"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var docStyle = lipgloss.NewStyle().Margin(1, 2)

type Model struct {
	list list.Model
}

func InitialModel() Model {
	// 1. Fetch tasks
	tasks := loadTasks()

	// 2. Convert to list.Item
	items := make([]list.Item, len(tasks))
	for i, t := range tasks {
		items[i] = item{task: t}
	}

	// 3. Setup list
	l := list.New(items, list.NewDefaultDelegate(), 0, 0)
	l.Title = "My Tasks"
	l.AdditionalShortHelpKeys = func() []key.Binding {
		return []key.Binding{
			key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "done")),
		}
	}

	return Model{list: l}
}

func loadTasks() []models.Task {
	query := "SELECT id, description, project, status, created_at, due_at, scheduled_at, estimate, active_start, time_spent, reschedule_count FROM tasks WHERE status != 'done' AND status != 'deleted' ORDER BY created_at DESC"
	rows, err := db.DB.Query(query)
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()

	var tasks []models.Task
	for rows.Next() {
		var t models.Task
		var project sql.NullString
		var dueAt sql.NullTime
		var scheduledAt sql.NullTime
		var estimate sql.NullString
		var activeStart sql.NullTime
		var timeSpent sql.NullInt64

		if err := rows.Scan(&t.ID, &t.Description, &project, &t.Status, &t.CreatedAt, &dueAt, &scheduledAt, &estimate, &activeStart, &timeSpent, &t.RescheduleCount); err != nil {
			continue
		}
		t.Project = project.String
		if dueAt.Valid {
			t.DueAt = &dueAt.Time
		}
		if scheduledAt.Valid {
			t.ScheduledAt = &scheduledAt.Time
		}
		if estimate.Valid {
			t.Estimate = estimate.String
		}
		if activeStart.Valid {
			t.ActiveStart = &activeStart.Time
		}
		if timeSpent.Valid {
			t.TimeSpent = timeSpent.Int64
		}
		tasks = append(tasks, t)
	}

	// Sort by priority score
	calc := priority.NewCalculator()
	sort.Slice(tasks, func(i, j int) bool {
		activeI := tasks[i].ActiveStart != nil
		activeJ := tasks[j].ActiveStart != nil
		if activeI && !activeJ {
			return true
		}
		if !activeI && activeJ {
			return false
		}
		return calc.Calculate(tasks[i]) > calc.Calculate(tasks[j])
	})

	return tasks
}

func markTaskDone(id int64) {
	query := `UPDATE tasks SET status = 'done', completed_at = CURRENT_TIMESTAMP WHERE id = ?`
	_, err := db.DB.Exec(query, id)
	if err != nil {
		log.Printf("Error marking task done: %v", err)
	}
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.list.FilterState() == list.Filtering {
			break
		}
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "d":
			if i, ok := m.list.SelectedItem().(item); ok {
				markTaskDone(i.task.ID)
				index := m.list.Index()
				m.list.RemoveItem(index)
				cmd := m.list.NewStatusMessage(docStyle.Foreground(lipgloss.Color("#04B575")).Render("Task completed!"))
				return m, cmd
			}
		}
	case tea.WindowSizeMsg:
		h, v := docStyle.GetFrameSize()
		m.list.SetSize(msg.Width-h, msg.Height-v)
	}

	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m Model) View() string {
	return docStyle.Render(m.list.View())
}
