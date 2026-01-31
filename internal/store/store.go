package store

import "github.com/aescoubas/tasc/internal/models"

type TaskFilter struct {
	Status         []models.TaskStatus
	Project        string
	IncludeDeleted bool
}

type Store interface {
	// Tasks
	CreateTask(t models.Task) (int64, error)
	GetTask(id int64) (models.Task, error)
	UpdateTask(t models.Task) error
	DeleteTask(id int64) error // Soft delete usually
	ListTasks(filter TaskFilter) ([]models.Task, error)
	
	// Task Actions (Specific updates)
	MarkDone(id int64) error
	StartTask(id int64) error
	StopTask(id int64) error
    GetActiveTask() (*models.Task, error)

	// Dependencies
	AddDependency(blockerID, blockedID int64) error
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
