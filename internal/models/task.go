package models

import (
	"fmt"
	"time"
)

type TaskStatus string

const (
	StatusBacklog   TaskStatus = "backlog"
	StatusOngoing   TaskStatus = "ongoing"
	StatusDone      TaskStatus = "done"
	StatusBlocked   TaskStatus = "blocked"
	StatusUndefined TaskStatus = "undefined"
	StatusDeleted   TaskStatus = "deleted"
)

type Task struct {
	ID          int64      `json:"id" yaml:"id"`
	Description string     `json:"description" yaml:"description"`
	Project     string     `json:"project" yaml:"project"`
	Status      TaskStatus `json:"status" yaml:"status"`
	CreatedAt   time.Time  `json:"created_at" yaml:"created_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty" yaml:"completed_at,omitempty"`
	DueAt       *time.Time `json:"due_at,omitempty" yaml:"due_at,omitempty"`
	ScheduledAt *time.Time `json:"scheduled_at,omitempty" yaml:"scheduled_at,omitempty"`
	Estimate    string     `json:"estimate,omitempty" yaml:"estimate,omitempty"`
	Recurrence  string     `json:"recurrence,omitempty" yaml:"recurrence,omitempty"`
	ActiveStart *time.Time `json:"active_start,omitempty" yaml:"active_start,omitempty"`
	TimeSpent   int64      `json:"time_spent" yaml:"time_spent"` // in seconds
}

func (t Task) AgeString() string {
	d := time.Since(t.CreatedAt)
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}
