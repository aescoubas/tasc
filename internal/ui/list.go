package ui

import (
	"fmt"
	"github.com/aescoubas/tasc/internal/models"
)

type item struct {
	task models.Task
}

func (i item) Title() string       { return i.task.Description }
func (i item) Description() string { return fmt.Sprintf("[%s] Created: %s", i.task.Project, i.task.CreatedAt.Format("2006-01-02")) }
func (i item) FilterValue() string { return i.task.Description + " " + i.task.Project }
