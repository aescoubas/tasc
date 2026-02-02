package scheduling

import (
	"sort"
	"time"

	"github.com/aescoubas/tasc/internal/models"
	"github.com/aescoubas/tasc/internal/priority"
)

// ApplyAutoSchedule processes a list of tasks and assigns virtual ScheduledAt times
// for "Floating" tasks (ScheduledAt is 00:00:00).
// It respects "Fixed" tasks (ScheduledAt has time) and stacks floating tasks
// starting at 07:00 based on priority.
func ApplyAutoSchedule(tasks []models.Task) []models.Task {
	// 1. Group by Day
	tasksByDay := make(map[string][]int) // "YYYY-MM-DD" -> []indices
	for i, t := range tasks {
		if t.ScheduledAt != nil {
			key := t.ScheduledAt.Format("2006-01-02")
			tasksByDay[key] = append(tasksByDay[key], i)
		}
	}

	calc := priority.NewCalculator()
	
	// Process each day
	for _, indices := range tasksByDay {
		// Separate Fixed vs Floating
		var fixed []int
		var floating []int

		for _, idx := range indices {
			t := tasks[idx]
			if isFloating(*t.ScheduledAt) {
				floating = append(floating, idx)
			} else {
				fixed = append(fixed, idx)
			}
		}

		if len(floating) == 0 {
			continue
		}

		// Sort Floating by Priority
		sort.Slice(floating, func(i, j int) bool {
			return calc.Calculate(tasks[floating[i]]) > calc.Calculate(tasks[floating[j]])
		})

		// Sort Fixed by Time (for collision detection optimization)
		sort.Slice(fixed, func(i, j int) bool {
			return tasks[fixed[i]].ScheduledAt.Before(*tasks[fixed[j]].ScheduledAt)
		})

		// Build Busy Intervals from Fixed tasks
		type Interval struct {
			Start time.Time
			End   time.Time
		}
		var busy []Interval
		for _, idx := range fixed {
			t := tasks[idx]
			start := *t.ScheduledAt
			dur := getDuration(t)
			end := start.Add(dur)
			busy = append(busy, Interval{start, end})
		}

		// Assign times to Floating
		// Start at 07:00 on that day
		day := *tasks[floating[0]].ScheduledAt // They all share the day
		cursor := time.Date(day.Year(), day.Month(), day.Day(), 7, 0, 0, 0, day.Location())

		for _, idx := range floating {
			dur := getDuration(tasks[idx])
			
			// Find a slot
			for {
				candidateStart := cursor
				candidateEnd := cursor.Add(dur)
				
				// Check collision
				collision := false
				var nextAvailable time.Time
				
				for _, interval := range busy {
					// Overlap logic: Start < IntervalEnd AND End > IntervalStart
					if candidateStart.Before(interval.End) && candidateEnd.After(interval.Start) {
						collision = true
						nextAvailable = interval.End
						break // Optimization: Pick first collision to jump over? Or closest? 
						// Because 'busy' is sorted, we encounter them in order.
					}
				}

				if !collision {
					// Found a slot!
					// Assign virtual time
					// We must create a new Time object to avoid referencing the shared 00:00 object
					newTime := candidateStart
					tasks[idx].ScheduledAt = &newTime
					
					// Add to busy (so next floating doesn't overlap this one)
					busy = append(busy, Interval{candidateStart, candidateEnd})
					// Resort busy? Or just append. Append is fine if we check all.
					
					// Move cursor
					cursor = candidateEnd
					break
				} else {
					// Jump cursor
					cursor = nextAvailable
				}
			}
		}
	}

	return tasks
}

func isFloating(t time.Time) bool {
	return t.Hour() == 0 && t.Minute() == 0 && t.Second() == 0
}

func getDuration(t models.Task) time.Duration {
	if t.Estimate == "" {
		return 20 * time.Minute
	}
	d, err := time.ParseDuration(t.Estimate)
	if err != nil {
		return 20 * time.Minute
	}
	return d
}
