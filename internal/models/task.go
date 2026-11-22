package models

import (
	"cmp"
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
	ID              int64      `json:"id" yaml:"id"`
	Title           string     `json:"title" yaml:"title"`
	Description     string     `json:"description,omitempty" yaml:"description,omitempty"`
	Project         string     `json:"project" yaml:"project"`
	Status          TaskStatus `json:"status" yaml:"status"`
	CreatedAt       time.Time  `json:"created_at" yaml:"created_at"`
	CompletedAt     *time.Time `json:"completed_at,omitempty" yaml:"completed_at,omitempty"`
	DueAt           *time.Time `json:"due_at,omitempty" yaml:"due_at,omitempty"`
	DueDateOnly     *bool      `json:"due_date_only,omitempty" yaml:"-"` // nil for legacy deadlines
	ScheduleBlock   string     `json:"schedule_block,omitempty" yaml:"schedule_block,omitempty"`
	Estimate        string     `json:"estimate,omitempty" yaml:"estimate,omitempty"`
	Recurrence      string     `json:"recurrence,omitempty" yaml:"recurrence,omitempty"`
	ActiveStart     *time.Time `json:"active_start,omitempty" yaml:"active_start,omitempty"`
	TimeSpent       int64      `json:"time_spent" yaml:"time_spent"` // in seconds
	RescheduleCount int        `json:"reschedule_count" yaml:"reschedule_count"`
	UserPriority    *int       `json:"user_priority,omitempty" yaml:"-"`
	IsBlocked       bool       `json:"is_blocked" yaml:"is_blocked"`

	Assignment *Assignment `json:"assignment,omitempty" yaml:"-"`
}

const (
	HighestUserPriority = 1
	LowestUserPriority  = 5
)

func CompareTaskPriority(a, b Task, descending bool) int {
	result := cmp.Compare(a.effectiveUserPriority(), b.effectiveUserPriority())
	if result == 0 {
		return cmp.Compare(a.ID, b.ID)
	}
	if descending {
		return -result
	}
	return result
}

func (t Task) effectiveUserPriority() int {
	if t.UserPriority == nil {
		return LowestUserPriority
	}
	return *t.UserPriority
}

type TaskDependency struct {
	BlockerID int64 `json:"blocker_id"`
	BlockedID int64 `json:"blocked_id"`
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
