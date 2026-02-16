package cmd

import (
	"fmt"
	"time"

	"github.com/aescoubas/tasc/internal/models"
	"github.com/aescoubas/tasc/internal/recurrence"
)

// spawnNextTask calculates the next occurrence and inserts a new task.
func spawnNextTask(t models.Task) {
	if t.Recurrence == "" {
		return
	}

	// Determine base date
	// Priority: Due -> Now
	var base time.Time

	if t.DueAt != nil {
		base = *t.DueAt
	} else {
		base = time.Now()
	}

	nextDate, err := recurrence.Next(base, t.Recurrence)
	if err != nil {
		fmt.Printf("Error calculating next recurrence: %v\n", err)
		return
	}

	newDue := &nextDate

	// Insert new task
	newTask := models.Task{
		Title:       t.Title,
		Description: t.Description,
		Project:     t.Project,
		Recurrence:  t.Recurrence,
		DueAt:       newDue,
		Estimate:    t.Estimate,
	}

	id, err := CurrentStore.CreateTask(newTask)
	if err != nil {
		fmt.Printf("Error creating next recurring task: %v\n", err)
		return
	}
	fmt.Printf("Created next recurring task %d (Due: %s).\n", id, nextDate.Format("2006-01-02"))
}
