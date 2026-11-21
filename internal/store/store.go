package store

import (
	"errors"
	"math"
	"time"

	"github.com/aescoubas/tasc/internal/models"
)

// ErrInvalidDependency identifies a link that would violate the task graph rules.
var ErrInvalidDependency = errors.New("invalid dependency")

// ErrInvalidCompletion identifies a deleted task or a recurrence that cannot advance.
var ErrInvalidCompletion = errors.New("invalid completion")

// ErrTaskDescriptionConflict identifies a stale description edit.
var ErrTaskDescriptionConflict = errors.New("task description changed or is unavailable")

var ErrInvalidUserPriority = errors.New("user_priority must be an integer from 1 to 5")

// ValidateUserPriority accepts nil to clear the value and a JSON number or int to set it.
func ValidateUserPriority(value any) error {
	switch value := value.(type) {
	case nil:
		return nil
	case int:
		if value >= models.HighestUserPriority && value <= models.LowestUserPriority {
			return nil
		}
	case float64:
		if value >= models.HighestUserPriority && value <= models.LowestUserPriority && value == math.Trunc(value) {
			return nil
		}
	}
	return ErrInvalidUserPriority
}

type ProjectCreateOptions struct {
	KeepShortNameEmpty bool `json:"keep_short_name_empty,omitempty"`
	RequireParent      bool `json:"require_parent,omitempty"`
}

type ProjectDeleteMode string

const (
	ProjectDeleteEmpty   ProjectDeleteMode = "empty"
	ProjectDeleteUnlink  ProjectDeleteMode = "unlink"
	ProjectDeleteCascade ProjectDeleteMode = "cascade"
)

type ProjectDeleteResult struct {
	Tasks       int `json:"tasks"`
	Subprojects int `json:"subprojects"`
}

var ErrProjectNotEmpty = errors.New("project has tasks or subprojects")

type TaskFilter struct {
	// Status matches the displayed status: open dependencies override it with blocked.
	Status         []models.TaskStatus
	Projects       []string
	IDs            []int64
	IncludeDeleted bool
	DueBefore      *time.Time
	DueAfter       *time.Time
	// ExcludeArchivedProjects matches the task's assigned project. Unassigned
	// tasks and tasks without a matching project record are kept.
	ExcludeArchivedProjects bool
}

type Store interface {
	// Tasks
	CreateTask(t models.Task) (int64, error)
	LogTask(t models.Task) (int64, error)
	GetTask(id int64) (models.Task, error)
	// UpdateTask preserves user priority. Use BatchUpdateTasks for explicit priority edits.
	UpdateTask(t models.Task) error
	UpdateTaskDescription(id int64, expected, description string) error
	DeleteTask(id int64) error // Soft delete usually
	ListTasks(filter TaskFilter) ([]models.Task, error)

	// Task Actions (Specific updates)
	// MarkDone atomically finalizes the timer and recurrence. Already-done tasks
	// are a no-op; missing tasks return sql.ErrNoRows.
	MarkDone(id int64) error
	MarkVague(id int64) error
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
	CreateProject(p models.Project, options ProjectCreateOptions) error
	UpdateProject(oldName string, p models.Project) error
	ArchiveProject(name string) error
	DeleteProject(name string) error
	DeleteProjectWithMode(name string, mode ProjectDeleteMode) (ProjectDeleteResult, error)

	// Search
	SearchTasks(query string) ([]models.Task, error)
}
