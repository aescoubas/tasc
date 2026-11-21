package ui

import (
	"sort"

	"github.com/aescoubas/tasc/internal/models"
	"github.com/aescoubas/tasc/internal/priority"
	"github.com/aescoubas/tasc/internal/store"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var docStyle = lipgloss.NewStyle().Margin(1, 2)

type Model struct {
	list  list.Model
	store store.Store
}

func InitialModel(st store.Store) (Model, error) {
	items, err := loadItems(st)
	if err != nil {
		return Model{}, err
	}

	l := list.New(items, list.NewDefaultDelegate(), 0, 0)
	l.Title = "My Tasks"
	l.AdditionalShortHelpKeys = func() []key.Binding {
		return []key.Binding{
			key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "done")),
		}
	}

	return Model{list: l, store: st}, nil
}

func loadItems(st store.Store) ([]list.Item, error) {
	tasks, err := st.ListTasks(store.TaskFilter{Status: []models.TaskStatus{
		models.StatusBacklog, models.StatusOngoing, models.StatusBlocked, models.StatusUndefined,
	}})
	if err != nil {
		return nil, err
	}

	calc := priority.NewCalculator()
	sort.SliceStable(tasks, func(i, j int) bool {
		activeI := tasks[i].ActiveStart != nil
		activeJ := tasks[j].ActiveStart != nil
		if activeI != activeJ {
			return activeI
		}
		scoreI, scoreJ := calc.Calculate(tasks[i]), calc.Calculate(tasks[j])
		if scoreI != scoreJ {
			return scoreI > scoreJ
		}
		return tasks[i].CreatedAt.After(tasks[j].CreatedAt)
	})

	items := make([]list.Item, len(tasks))
	for i, task := range tasks {
		items[i] = item{task: task}
	}
	return items, nil
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
				if err := m.store.MarkDone(i.task.ID); err != nil {
					return m, m.list.NewStatusMessage("Error completing task: " + err.Error())
				}
				items, err := loadItems(m.store)
				if err != nil {
					return m, m.list.NewStatusMessage("Task completed but the list could not refresh: " + err.Error())
				}
				refresh := m.list.SetItems(items)
				cmd := m.list.NewStatusMessage(docStyle.Foreground(lipgloss.Color("#04B575")).Render("Task completed!"))
				return m, tea.Batch(refresh, cmd)
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
