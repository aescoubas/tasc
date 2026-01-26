package ui

import (
	"fmt"
	"github.com/aescoubas/tasc/internal/models"
)

type item struct {
	task models.Task
}

func (i item) Title() string       { return i.task.Description }
func (i item) Description() string {
	desc := fmt.Sprintf("[%s]", i.task.Project)
	if i.task.DueAt != nil {
		desc += fmt.Sprintf(" Due: %s", i.task.DueAt.Format("2006-01-02"))
	}
	if i.task.Estimate != "" {
		desc += fmt.Sprintf(" Est: %s", i.task.Estimate)
	}
	desc += fmt.Sprintf(" Created: %s", i.task.CreatedAt.Format("2006-01-02"))
	return desc
}
func (i item) FilterValue() string { return i.task.Description + " " + i.task.Project }
