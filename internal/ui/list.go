package ui

import (
	"fmt"
	"time"

	"github.com/aescoubas/tasc/internal/models"
)

type item struct {
	task models.Task
}

func (i item) Title() string {
	if i.task.ActiveStart != nil {
		return "▶ " + i.task.Description
	}
	return i.task.Description
}
func (i item) Description() string {
	desc := fmt.Sprintf("[%s]", i.task.Project)
	if i.task.ActiveStart != nil {
		dur := time.Since(*i.task.ActiveStart) + time.Duration(i.task.TimeSpent)*time.Second
		desc += fmt.Sprintf(" Active: %s", dur.Round(time.Second))
	} else if i.task.TimeSpent > 0 {
		d := time.Duration(i.task.TimeSpent) * time.Second
		desc += fmt.Sprintf(" Time: %s", d)
	}

	if i.task.DueAt != nil {
		desc += fmt.Sprintf(" Due: %s", i.task.DueAt.Format("2006-01-02"))
	}
	if i.task.Estimate != "" {
		desc += fmt.Sprintf(" Est: %s", i.task.Estimate)
	}
	desc += fmt.Sprintf(" Created: %s", i.task.CreatedAt.Format("2006-01-02"))
	desc += fmt.Sprintf(" Age: %s", i.task.AgeString())
	return desc
}
func (i item) FilterValue() string { return i.task.Description + " " + i.task.Project }
