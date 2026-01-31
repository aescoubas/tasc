package models

import (
	"testing"
	"time"
)

func TestTask_AgeString(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name      string
		createdAt time.Time
		want      string
	}{
		{
			name:      "Just now",
			createdAt: now,
			want:      "0s",
		},
		{
			name:      "30 seconds ago",
			createdAt: now.Add(-30 * time.Second),
			want:      "30s",
		},
		{
			name:      "59 seconds ago",
			createdAt: now.Add(-59 * time.Second),
			want:      "59s",
		},
		{
			name:      "1 minute ago",
			createdAt: now.Add(-1 * time.Minute),
			want:      "1m",
		},
		{
			name:      "59 minutes ago",
			createdAt: now.Add(-59 * time.Minute),
			want:      "59m",
		},
		{
			name:      "1 hour ago",
			createdAt: now.Add(-1 * time.Hour),
			want:      "1h",
		},
		{
			name:      "23 hours ago",
			createdAt: now.Add(-23 * time.Hour),
			want:      "23h",
		},
		{
			name:      "1 day ago",
			createdAt: now.Add(-24 * time.Hour),
			want:      "1d",
		},
		{
			name:      "2 days ago",
			createdAt: now.Add(-48 * time.Hour),
			want:      "2d",
		},
		{
			name:      "100 days ago",
			createdAt: now.Add(-100 * 24 * time.Hour),
			want:      "100d",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := Task{CreatedAt: tt.createdAt}
			if got := task.AgeString(); got != tt.want {
				t.Errorf("Task.AgeString() = %v, want %v", got, tt.want)
			}
		})
	}
}
