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
	// Priority: Scheduled -> Due -> Now
	var base time.Time
	var isScheduledBased bool

	if t.ScheduledAt != nil {
		base = *t.ScheduledAt
		isScheduledBased = true
	} else if t.DueAt != nil {
		base = *t.DueAt
	} else {
		base = time.Now()
	}

	nextDate, err := recurrence.Next(base, t.Recurrence)
	if err != nil {
		fmt.Printf("Error calculating next recurrence: %v\n", err)
		return
	}

	// Calculate new fields
	var newScheduled *time.Time
	var newDue *time.Time

	if isScheduledBased {
		newScheduled = &nextDate
		// Shift due date if exists
		if t.DueAt != nil {
			diff := nextDate.Sub(base)
			nd := t.DueAt.Add(diff)
			newDue = &nd
		}
	} else {
		newScheduled = &nextDate
	}

	// Insert new task
	newTask := models.Task{
		Description: t.Description,
		Project:     t.Project,
		Recurrence:  t.Recurrence,
		DueAt:       newDue,
		ScheduledAt: newScheduled,
		Estimate:    t.Estimate,
	}

	id, err := CurrentStore.CreateTask(newTask)
	if err != nil {
		fmt.Printf("Error creating next recurring task: %v\n", err)
		return
	}
	fmt.Printf("Created next recurring task %d (Scheduled: %s).\n", id, nextDate.Format("2006-01-02"))
}
