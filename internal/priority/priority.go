package priority

import (
	"time"

	"github.com/aescoubas/tasc/internal/models"
)

// Rule is a function that calculates a score component for a task.
type Rule func(t models.Task) float64

// Calculator holds the logic for calculating task priority.
type Calculator struct {
	Rules []Rule
}

// NewCalculator creates a new Calculator with default rules.
func NewCalculator() *Calculator {
	return &Calculator{
		Rules: []Rule{
			ScheduledRule,
		},
	}
}

// Calculate returns the total priority score for a task.
func (c *Calculator) Calculate(t models.Task) float64 {
	var score float64
	for _, rule := range c.Rules {
		score += rule(t)
	}
	return score
}

// ScheduledRule calculates priority based on the ScheduledAt field.
// - If scheduled date is present: +5
// - If scheduled date is in the past (or today): +10 (Cumulative +15)
// - If scheduled date is in the future: -5 (Cumulative 0)
// This effectively hides or lowers future scheduled tasks while boosting current ones.
func ScheduledRule(t models.Task) float64 {
	if t.ScheduledAt == nil {
		return 0
	}

	score := 5.0
	now := time.Now()

	// If scheduled time is before now (meaning we passed the start time), boost it.
	if t.ScheduledAt.Before(now) {
		score += 10.0
	} else {
		// If it's in the future, we might want to lower it so it doesn't clutter the view
		// of actionable things.
		score -= 10.0
	}

	return score
}
