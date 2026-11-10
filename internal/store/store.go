package store

import (
	"errors"
	"time"

	"github.com/aescoubas/tasc/internal/models"
)

// ErrInvalidDependency identifies a link that would violate the task graph rules.
var ErrInvalidDependency = errors.New("invalid dependency")

// ErrInvalidCompletion identifies a deleted task or a recurrence that cannot advance.
var ErrInvalidCompletion = errors.New("invalid completion")

type TaskFilter struct {
	// Status matches the displayed status: open dependencies override it with blocked.
	Status         []models.TaskStatus
	Projects       []string
	IDs            []int64
	IncludeDeleted bool
	DueBefore      *time.Time
	DueAfter       *time.Time
}

type Store interface {
	// Tasks
	CreateTask(t models.Task) (int64, error)
	GetTask(id int64) (models.Task, error)
	UpdateTask(t models.Task) error
	DeleteTask(id int64) error // Soft delete usually
	ListTasks(filter TaskFilter) ([]models.Task, error)

	// Task Actions (Specific updates)
	// MarkDone atomically finalizes the timer and recurrence. Already-done tasks
	// are a no-op; missing tasks return sql.ErrNoRows.
	MarkDone(id int64) error
	StartTask(id int64) error
	StopTask(id int64) error
	// Setting status to done uses the same completion rules, atomically for all IDs.
	BatchUpdateTasks(ids []int64, updates map[string]interface{}) error
	GetActiveTask() (*models.Task, error)

	// Dependencies
	// AddDependency requires existing, distinct tasks and rejects cycles.
	AddDependency(blockerID, blockedID int64) error
	RemoveDependency(blockerID, blockedID int64) error
	GetDependencies() ([]models.TaskDependency, error)

	// Projects
	ListProjects() ([]models.Project, error)
	GetProject(name string) (models.Project, error)
	CreateProject(p models.Project) error
	UpdateProject(oldName string, p models.Project) error
	DeleteProject(name string) error

	// Search
	SearchTasks(query string) ([]models.Task, error)
}
