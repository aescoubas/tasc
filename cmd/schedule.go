package cmd

import (
	"fmt"
	"time"

	"github.com/aescoubas/tasc/internal/config"
	"github.com/aescoubas/tasc/internal/models"
	"github.com/aescoubas/tasc/internal/scheduling"
	"github.com/aescoubas/tasc/internal/store"
)

// scheduleVisibleTasks plans with all open tasks on the visible days, so
// filtering the display cannot free capacity. Earlier fixed tasks can overlap
// a visible day when their estimate spans midnight.
func scheduleVisibleTasks(visible []models.Task, blocks map[string]config.TimeBlock) (map[int64]string, error) {
	days := make(map[string]bool)
	for _, task := range visible {
		if scheduling.IsFloating(task) {
			days[task.DueAt.Format(time.DateOnly)] = true
		}
	}
	if len(days) == 0 {
		return nil, nil
	}
	open, err := CurrentStore.ListTasks(store.TaskFilter{})
	if err != nil {
		return nil, fmt.Errorf("query tasks for scheduling: %w", err)
	}
	var plan []models.Task
	for _, task := range open {
		if task.DueAt != nil && (!scheduling.IsFloating(task) || days[task.DueAt.Format(time.DateOnly)]) {
			plan = append(plan, task)
		}
	}
	unscheduled := scheduling.ApplyAutoSchedule(plan, blocks)
	plannedDates := make(map[int64]*time.Time, len(plan))
	for _, task := range plan {
		plannedDates[task.ID] = task.DueAt
	}
	visibleUnscheduled := make(map[int64]string)
	for i, task := range visible {
		if date, ok := plannedDates[task.ID]; ok && scheduling.IsFloating(task) {
			visible[i].DueAt = date
		}
		if reason := unscheduled[task.ID]; reason != "" {
			visibleUnscheduled[task.ID] = reason
		}
	}
	return visibleUnscheduled, nil
}
