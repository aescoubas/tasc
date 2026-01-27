package models

import (
	"time"
)

type TaskStatus string

const (
	StatusPending       TaskStatus = "pending"
	StatusCompleted     TaskStatus = "completed"
	StatusDeleted       TaskStatus = "deleted"
	StatusPoorlyDefined TaskStatus = "poorly_defined"
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
	ActiveStart *time.Time `json:"active_start,omitempty" yaml:"active_start,omitempty"`
	TimeSpent   int64      `json:"time_spent" yaml:"time_spent"` // in seconds
}
