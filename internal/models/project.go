package models

import "time"

type ProjectStatus string

const (
	ProjectStatusActive   ProjectStatus = "active"
	ProjectStatusArchived ProjectStatus = "archived"
)

type Project struct {
	Name        string        `json:"name" yaml:"name"`
	ShortName   string        `json:"short_name" yaml:"short_name"`
	Description string        `json:"description" yaml:"description"`
	Parent      string        `json:"parent,omitempty" yaml:"parent,omitempty"`
	Status      ProjectStatus `json:"status" yaml:"status"`
	DueAt       *time.Time    `json:"due_at,omitempty" yaml:"due_at,omitempty"`
	CreatedAt   time.Time     `json:"created_at" yaml:"created_at"`
}
