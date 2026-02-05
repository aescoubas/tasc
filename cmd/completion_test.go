package cmd

import (
	"testing"

	"github.com/aescoubas/tasc/internal/models"
	"github.com/aescoubas/tasc/internal/store"
	"github.com/spf13/cobra"
)

type mockStore struct{}

func (m *mockStore) CreateTask(t models.Task) (int64, error) { return 0, nil }
func (m *mockStore) GetTask(id int64) (models.Task, error) { return models.Task{}, nil }
func (m *mockStore) UpdateTask(t models.Task) error { return nil }
func (m *mockStore) DeleteTask(id int64) error { return nil }
func (m *mockStore) ListTasks(filter store.TaskFilter) ([]models.Task, error) { return nil, nil }
func (m *mockStore) MarkDone(id int64) error { return nil }
func (m *mockStore) StartTask(id int64) error { return nil }
func (m *mockStore) StopTask(id int64) error { return nil }
func (m *mockStore) BatchUpdateTasks(ids []int64, updates map[string]interface{}) error { return nil }
func (m *mockStore) GetActiveTask() (*models.Task, error) { return nil, nil }
func (m *mockStore) AddDependency(blockerID, blockedID int64) error { return nil }
func (m *mockStore) GetDependencies() ([]models.TaskDependency, error) { return nil, nil }
func (m *mockStore) ListProjects() ([]models.Project, error) {
	return []models.Project{
		{Name: "work"},
		{Name: "personal"},
		{Name: "work.project1"},
	}, nil
}
func (m *mockStore) GetProject(name string) (models.Project, error) { return models.Project{}, nil }
func (m *mockStore) CreateProject(p models.Project) error { return nil }
func (m *mockStore) UpdateProject(oldName string, p models.Project) error { return nil }
func (m *mockStore) DeleteProject(name string) error { return nil }
func (m *mockStore) SearchTasks(query string) ([]models.Task, error) { return nil, nil }

func TestProjectCompletionFunc(t *testing.T) {
	// Setup mock store
	originalStore := CurrentStore
	CurrentStore = &mockStore{}
	defer func() { CurrentStore = originalStore }()

	tests := []struct {
		toComplete string
		want       []string
	}{
		{
			toComplete: "w",
			want:       []string{"work", "work.project1"},
		},
		{
			toComplete: "per",
			want:       []string{"personal"},
		},
		{
			toComplete: "nonexistent",
			want:       []string(nil),
		},
		{
			toComplete: "",
			want:       []string{"work", "personal", "work.project1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.toComplete, func(t *testing.T) {
			got, _ := projectCompletionFunc(&cobra.Command{}, nil, tt.toComplete)
			
			if len(got) != len(tt.want) {
				t.Errorf("got %d items, want %d", len(got), len(tt.want))
			}

			for _, w := range tt.want {
				found := false
				for _, g := range got {
					if g == w {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected %q in results, got %v", w, got)
				}
			}
		})
	}
}

func TestDateCompletionFunc(t *testing.T) {
	tests := []struct {
		toComplete string
		want       []string
		contains   []string
	}{
		{
			toComplete: "sun",
			contains:   []string{"sunday"},
		},
		{
			toComplete: "ne",
			contains:   []string{"next week", "next monday", "next friday"},
		},
		{
			toComplete: "tom",
			contains:   []string{"tomorrow"},
		},
		{
			toComplete: "nonexistent",
			want:       []string(nil), // Empty slice or nil
		},
	}

	for _, tt := range tests {
		t.Run(tt.toComplete, func(t *testing.T) {
			got, _ := dateCompletionFunc(&cobra.Command{}, nil, tt.toComplete)
			
			if tt.want != nil {
				if len(got) != len(tt.want) {
					t.Errorf("got %d items, want %d", len(got), len(tt.want))
				}
			}

			if len(tt.contains) > 0 {
				for _, c := range tt.contains {
					found := false
					for _, g := range got {
						if g == c {
							found = true
							break
						}
					}
					if !found {
						t.Errorf("expected %q in results, got %v", c, got)
					}
				}
			}
		})
	}
}
