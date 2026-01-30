package priority

import (
	"testing"
	"time"

	"github.com/aescoubas/tasc/internal/models"
)

func TestDueRule(t *testing.T) {
	now := time.Now()
	
	tests := []struct {
		name     string
		dueDelta time.Duration // relative to now
		isOverdue bool
		minScore float64
		maxScore float64
	}{
		{"Overdue by 1 hour", -1 * time.Hour, true, 20.0, 22.0},
		{"Overdue by 5 days", -5 * 24 * time.Hour, true, 30.0, 30.0}, // 20 + 5*2
		{"Due in 1 hour (Today)", 1 * time.Hour, false, 15.0, 15.0},
		{"Due in 2 days (Soon)", 48 * time.Hour, false, 10.0, 10.0},
		{"Due in 5 days (Week)", 5 * 24 * time.Hour, false, 5.0, 5.0},
		{"Due in 10 days (Later)", 10 * 24 * time.Hour, false, -5.0, -5.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			due := now.Add(tt.dueDelta)
			task := models.Task{DueAt: &due}
			
			score := DueRule(task)
			
			if score < tt.minScore || score > tt.maxScore {
				t.Errorf("DueRule() score = %v, want between %v and %v", score, tt.minScore, tt.maxScore)
			}
		})
	}

	t.Run("No Due Date", func(t *testing.T) {
		task := models.Task{DueAt: nil}
		if score := DueRule(task); score != 0 {
			t.Errorf("DueRule() = %v, want 0", score)
		}
	})
}

func TestEstimateRule(t *testing.T) {
	tests := []struct {
		estimate string
		want     float64
	}{
		{"10m", 3.0},
		{"25m", 3.0},
		{"30m", 0.0}, // Boundary: strictly less than 30m? or <=? Let's say < 30m is "Quick win"
		{"1h", 0.0},
		{"invalid", 0.0},
		{"", 0.0},
	}

	for _, tt := range tests {
		t.Run(tt.estimate, func(t *testing.T) {
			task := models.Task{Estimate: tt.estimate}
			if got := EstimateRule(task); got != tt.want {
				t.Errorf("EstimateRule(%q) = %v, want %v", tt.estimate, got, tt.want)
			}
		})
	}
}
