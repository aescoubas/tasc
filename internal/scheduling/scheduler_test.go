package scheduling

import (
	"testing"
	"time"

	"github.com/aescoubas/tasc/internal/config"
	"github.com/aescoubas/tasc/internal/models"
)

func TestApplyAutoSchedule(t *testing.T) {
	// Helpers
	fixedTime := func(h, m int) *time.Time {
		tm := time.Date(2026, 2, 1, h, m, 0, 0, time.Local)
		return &tm
	}
	floatingTime := func() *time.Time {
		tm := time.Date(2026, 2, 1, 0, 0, 0, 0, time.Local)
		return &tm
	}

	// Tasks
	// Fixed: 07:10 - 07:30 (assuming 20m estimate)
	tFixed := models.Task{
		ID:        1,
		Title:     "Fixed Task",
		DueAt:     fixedTime(7, 10),
		Estimate:  "20m",
		CreatedAt: time.Now(),
	}

	// Floating 1 (High Priority)
	tFloatHigh := models.Task{
		ID:              2,
		Title:           "High Priority",
		DueAt:           floatingTime(),
		Estimate:        "20m",
		RescheduleCount: 5,
		CreatedAt:       time.Now(),
	}

	// Floating 2 (Low Priority)
	tFloatLow := models.Task{
		ID:              3,
		Title:           "Low Priority",
		DueAt:           floatingTime(),
		Estimate:        "20m",
		RescheduleCount: 0,
		CreatedAt:       time.Now(),
	}

	tasks := []models.Task{tFloatLow, tFixed, tFloatHigh} // Mixed order

	blocks := map[string]config.TimeBlock{
		"morning":   {Start: "06:00", End: "12:00"},
		"afternoon": {Start: "13:00", End: "18:00"},
	}

	if unscheduled := ApplyAutoSchedule(tasks, blocks); len(unscheduled) != 0 {
		t.Fatalf("unexpected unscheduled tasks: %v", unscheduled)
	}

	// Map by ID for checking
	res := make(map[int64]models.Task)
	for _, t := range tasks {
		res[t.ID] = t
	}

	// Check Fixed (Should be unchanged)
	if !res[1].DueAt.Equal(*fixedTime(7, 10)) {
		t.Errorf("Fixed task moved: %v", res[1].DueAt)
	}

	// Check Floating High
	// Logic:
	// Start 07:00 (default for generic floating if block is empty/not found).
	// But wait, my new logic defaults generic floating to 07:00.
	// Fixed occupies 07:10 - 07:30.
	// Floating High starts at 07:00. Ends 07:20.
	// Collision with Fixed (07:10-07:30).
	// Next available: 07:30.
	// So it should start at 07:30.
	if !res[2].DueAt.Equal(*fixedTime(7, 30)) {
		t.Errorf("High priority task assigned %v, want 07:30", res[2].DueAt.Format("15:04"))
	}

	// Check Floating Low
	// Start 07:50 (after High). Ends 08:10.
	if !res[3].DueAt.Equal(*fixedTime(7, 50)) {
		t.Errorf("Low priority task assigned %v, want 07:50", res[3].DueAt.Format("15:04"))
	}
}

func TestScheduleWindows(t *testing.T) {
	midnight := time.Date(2030, 1, 2, 0, 0, 0, 0, time.Local)
	previous := midnight.Add(-90 * time.Minute)
	tasks := []models.Task{
		{ID: 1, DueAt: &previous, Estimate: "3h"},
		{ID: 2, DueAt: &midnight, ScheduleBlock: "early", Estimate: "30m"},
		{ID: 3, DueAt: &midnight, ScheduleBlock: "early", Estimate: "1h"},
		{ID: 4, DueAt: &midnight, ScheduleBlock: "invalid"},
		{ID: 5, DueAt: &midnight, ScheduleBlock: "overnight"},
		{ID: 6, DueAt: &midnight, ScheduleBlock: "late", Estimate: "60m"},
		{ID: 7, DueAt: &midnight, ScheduleBlock: "equal"},
	}
	unscheduled := ApplyAutoSchedule(tasks, map[string]config.TimeBlock{
		"early":     {Start: "01:00", End: "02:00"},
		"invalid":   {Start: "06:00", End: "51:00"},
		"overnight": {Start: "22:00", End: "02:00"},
		"late":      {Start: "23:00", End: "24:00"},
		"equal":     {Start: "12:00", End: "12:00"},
	})
	for id, reason := range map[int64]string{3: "no capacity", 4: "invalid time block", 5: "invalid time block", 7: "invalid time block"} {
		if unscheduled[id] != reason || !tasks[id-1].DueAt.Equal(midnight) {
			t.Errorf("task %d = %v (%q), want midnight (%q)", id, tasks[id-1].DueAt, unscheduled[id], reason)
		}
	}
	if got := tasks[1].DueAt.Format("15:04"); got != "01:30" {
		t.Errorf("earliest available slot = %s, want 01:30", got)
	}
	if got := tasks[5].DueAt.Format("15:04"); got != "23:00" {
		t.Errorf("late slot = %s, want 23:00", got)
	}
}
