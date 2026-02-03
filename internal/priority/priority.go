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
			RescheduleRule,
			AgeRule,
			DueRule,
			EstimateRule,
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
// - If scheduled date is present: +20 (Base)
// - If scheduled date is in the past (or today): +15 (Cumulative +35)
// This ensures scheduled tasks are always prioritized over backlog tasks.
func ScheduledRule(t models.Task) float64 {
	if t.ScheduledAt == nil {
		return 0
	}

	score := 20.0
	now := time.Now()

	// If scheduled time is before now (meaning we passed the start time), boost it.
	if t.ScheduledAt.Before(now) {
		score += 15.0
	}

	return score
}

// RescheduleRule boosts priority for tasks that are constantly rescheduled.
// Each reschedule adds +2 to the score.
func RescheduleRule(t models.Task) float64 {
	return float64(t.RescheduleCount) * 2.0
}

// AgeRule slightly boosts older tasks to prevent them from being forgotten.
// Adds +0.1 per day of age.
func AgeRule(t models.Task) float64 {
	age := time.Since(t.CreatedAt)
	days := age.Hours() / 24.0
	return days * 0.1
}

// DueRule calculates priority based on the DueAt field.
func DueRule(t models.Task) float64 {
	if t.DueAt == nil {
		return 0
	}

	score := 0.0
	now := time.Now()
	diff := t.DueAt.Sub(now)

	if diff < 0 {
		// Overdue: High priority + scale with lateness
		daysOverdue := int(-diff.Hours() / 24)
		score = 40.0 + float64(daysOverdue)*2.0
	} else if diff < 24*time.Hour {
		// Due today/tomorrow (within 24h)
		score = 35.0
	} else if diff < 3*24*time.Hour {
		// Due soon (3 days)
		score = 30.0
	} else if diff < 7*24*time.Hour {
		// Due this week
		score = 25.0
	} else {
		// Due later
		score = 20.0
	}

	return score
}

// EstimateRule boosts short tasks (< 30m) for quick wins.
func EstimateRule(t models.Task) float64 {
	if t.Estimate == "" {
		return 0
	}
	d, err := time.ParseDuration(t.Estimate)
	if err != nil {
		return 0
	}
	if d < 30*time.Minute {
		return 3.0
	}
	return 0
}
