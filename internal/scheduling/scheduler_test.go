package scheduling

import (
	"testing"
	"time"

	"github.com/aescoubas/tasc/internal/models"
)

func TestApplyAutoSchedule(t *testing.T) {
	// Helpers
	fixedTime := func(h, m int) *time.Time {
		tm := time.Date(2026, 2, 1, h, m, 0, 0, time.UTC)
		return &tm
	}
	floatingTime := func() *time.Time {
		tm := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
		return &tm
	}

	// Tasks
	// Fixed: 07:10 - 07:30 (assuming 20m estimate)
	tFixed := models.Task{
		ID:          1,
		Title:       "Fixed Task",
		ScheduledAt: fixedTime(7, 10),
		Estimate:    "20m",
		CreatedAt:   time.Now(),
	}

	// Floating 1 (High Priority - due soon)
	due := time.Now().Add(1 * time.Hour)
	tFloatHigh := models.Task{
		ID:          2,
		Title:       "High Priority",
		ScheduledAt: floatingTime(),
		Estimate:    "20m",
		DueAt:       &due,
		CreatedAt:   time.Now(),
	}

	// Floating 2 (Low Priority - due later)
	dueLater := time.Now().Add(100 * time.Hour)
	tFloatLow := models.Task{
		ID:          3,
		Title:       "Low Priority",
		ScheduledAt: floatingTime(),
		Estimate:    "20m",
		DueAt:       &dueLater,
		CreatedAt:   time.Now(),
	}

	tasks := []models.Task{tFloatLow, tFixed, tFloatHigh} // Mixed order

	scheduled := ApplyAutoSchedule(tasks)

	// Map by ID for checking
	res := make(map[int64]models.Task)
	for _, t := range scheduled {
		res[t.ID] = t
	}

	// Check Fixed (Should be unchanged)
	if !res[1].ScheduledAt.Equal(*fixedTime(7, 10)) {
		t.Errorf("Fixed task moved: %v", res[1].ScheduledAt)
	}

	// Check Floating High
	// Logic:
	// Start 07:00. Duration 20m. Ends 07:20.
	// Fixed occupies 07:10 - 07:30.
	// Overlap? Yes.
	// Move cursor to Fixed End (07:30).
	// Try 07:30. Ends 07:50.
	// Any overlap? No.
	// Assign 07:30.
	if !res[2].ScheduledAt.Equal(*fixedTime(7, 30)) {
		t.Errorf("High priority task assigned %v, want 07:30", res[2].ScheduledAt.Format("15:04"))
	}

	// Check Floating Low
	// Start 07:50 (after High). Ends 08:10.
	if !res[3].ScheduledAt.Equal(*fixedTime(7, 50)) {
		t.Errorf("Low priority task assigned %v, want 07:50", res[3].ScheduledAt.Format("15:04"))
	}
}
