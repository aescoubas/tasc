package models

import "time"

type Project struct {
	Name        string    `json:"name" yaml:"name"`
	Description string    `json:"description" yaml:"description"`
	CreatedAt   time.Time `json:"created_at" yaml:"created_at"`
}
