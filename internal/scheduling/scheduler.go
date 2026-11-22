package scheduling

import (
	"cmp"
	"slices"
	"time"

	"github.com/aescoubas/tasc/internal/config"
	"github.com/aescoubas/tasc/internal/models"
	"github.com/aescoubas/tasc/internal/priority"
)

type interval struct {
	start time.Time
	end   time.Time
}

// ApplyAutoSchedule assigns virtual due times to floating tasks in the slice.
// Tasks that cannot fit keep their due date and are returned with a reason.
func ApplyAutoSchedule(tasks []models.Task, blocks map[string]config.TimeBlock) map[int64]string {
	floatingByDay := make(map[time.Time][]int)
	var busy []interval
	for i, task := range tasks {
		if task.DueAt == nil || task.Status == models.StatusDone || task.Status == models.StatusDeleted {
			continue
		}
		if IsFloating(task) {
			year, month, day := task.DueAt.Date()
			date := time.Date(year, month, day, 0, 0, 0, 0, time.Local)
			floatingByDay[date] = append(floatingByDay[date], i)
		} else {
			busy = append(busy, interval{*task.DueAt, task.DueAt.Add(duration(task))})
		}
	}

	days := make([]time.Time, 0, len(floatingByDay))
	for day := range floatingByDay {
		days = append(days, day)
	}
	slices.SortFunc(days, time.Time.Compare)

	slices.SortFunc(busy, compareStart)
	calculator := priority.NewCalculator()
	unscheduled := make(map[int64]string)

	for _, day := range days {
		type placement struct {
			index  int
			window interval
			score  float64
		}
		var placements []placement
		for _, index := range floatingByDay[day] {
			task := tasks[index]
			window, ok := blockWindow(day, task.ScheduleBlock, blocks)
			if !ok {
				unscheduled[task.ID] = "invalid time block"
				continue
			}
			placements = append(placements, placement{index, window, calculator.Calculate(task)})
		}
		slices.SortFunc(placements, func(a, b placement) int {
			taskA, taskB := tasks[a.index], tasks[b.index]
			return cmp.Or(compareStart(a.window, b.window),
				cmp.Compare(taskA.ScheduleBlock, taskB.ScheduleBlock),
				cmp.Compare(b.score, a.score),
				cmp.Compare(taskA.ID, taskB.ID))
		})

		for _, placement := range placements {
			task := &tasks[placement.index]
			slot, ok := firstSlot(placement.window, duration(*task), busy)
			if !ok {
				unscheduled[task.ID] = "no capacity"
				continue
			}
			task.DueAt = &slot.start
			index, _ := slices.BinarySearchFunc(busy, slot, compareStart)
			busy = slices.Insert(busy, index, slot)
		}
	}
	return unscheduled
}

func IsFloating(task models.Task) bool {
	t := task.DueAt
	if t == nil || task.Status == models.StatusDone || task.Status == models.StatusDeleted {
		return false
	}
	if task.DueDateOnly != nil {
		return *task.DueDateOnly && task.ScheduleBlock != ""
	}
	return t.Hour() == 0 && t.Minute() == 0 && t.Second() == 0 && t.Nanosecond() == 0
}

func blockWindow(day time.Time, name string, blocks map[string]config.TimeBlock) (interval, bool) {
	block := config.TimeBlock{Start: "07:00", End: "24:00"}
	if name != "" {
		var ok bool
		block, ok = blocks[name]
		if !ok {
			return interval{}, false
		}
	}
	start, startOK := blockBoundary(day, block.Start)
	end, endOK := blockBoundary(day, block.End)
	return interval{start, end}, startOK && endOK && start.Before(end)
}

func blockBoundary(day time.Time, value string) (time.Time, bool) {
	if value == "24:00" {
		return day.AddDate(0, 0, 1), true
	}
	clock, err := time.Parse("15:04", value)
	if err != nil || len(value) != len("15:04") {
		return time.Time{}, false
	}
	boundary := time.Date(day.Year(), day.Month(), day.Day(), clock.Hour(), clock.Minute(), 0, 0, day.Location())
	// A DST jump can make a configured wall-clock boundary nonexistent.
	return boundary, boundary.Format("15:04") == value
}

func firstSlot(window interval, length time.Duration, busy []interval) (interval, bool) {
	start := window.start
	for _, occupied := range busy {
		if !occupied.end.After(start) {
			continue
		}
		if !start.Add(length).After(occupied.start) {
			break
		}
		start = occupied.end.In(window.start.Location())
		if !start.Before(window.end) {
			return interval{}, false
		}
	}
	end := start.Add(length)
	return interval{start, end}, !end.After(window.end)
}

func compareStart(a, b interval) int {
	return a.start.Compare(b.start)
}

func duration(task models.Task) time.Duration {
	value, err := time.ParseDuration(task.Estimate)
	if err != nil || value <= 0 {
		return 20 * time.Minute
	}
	return value
}
