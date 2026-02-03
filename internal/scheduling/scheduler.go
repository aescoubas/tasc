package scheduling

import (
	"sort"
	"time"

	"github.com/aescoubas/tasc/internal/config"
	"github.com/aescoubas/tasc/internal/models"
	"github.com/aescoubas/tasc/internal/priority"
)

// ApplyAutoSchedule processes a list of tasks and assigns virtual ScheduledAt times
// for "Floating" tasks.
// A task is Floating if ScheduledAt has time 00:00:00 (or if ScheduleBlock is set).
// It respects "Fixed" tasks (ScheduledAt has time) and stacks floating tasks
// starting at 07:00 (or block start time) based on priority.
func ApplyAutoSchedule(tasks []models.Task, blocks map[string]config.TimeBlock) []models.Task {
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
			if isFloating(t) {
				floating = append(floating, idx)
			} else {
				fixed = append(fixed, idx)
			}
		}

		if len(floating) == 0 {
			continue
		}

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

		// Group Floating by Block
		floatingByBlock := make(map[string][]int)
		for _, idx := range floating {
			blockName := tasks[idx].ScheduleBlock
			floatingByBlock[blockName] = append(floatingByBlock[blockName], idx)
		}

		// Sort block keys to process in order (e.g. Morning before Afternoon)
		// We use the start time of the block to sort.
		blockNames := make([]string, 0, len(floatingByBlock))
		for name := range floatingByBlock {
			blockNames = append(blockNames, name)
		}
		
		sort.Slice(blockNames, func(i, j int) bool {
			nameI := blockNames[i]
			nameJ := blockNames[j]
			
			// Generic/Empty block treatment:
			// If empty, treat as 07:00 (legacy default)
			startI := getBlockStart(nameI, blocks)
			startJ := getBlockStart(nameJ, blocks)
			
			return startI < startJ
		})

		// Process each block group
		day := *tasks[floating[0]].ScheduledAt // Shared day
		
		for _, blockName := range blockNames {
			blockIndices := floatingByBlock[blockName]

			// Sort by Priority within block
			sort.Slice(blockIndices, func(i, j int) bool {
				return calc.Calculate(tasks[blockIndices[i]]) > calc.Calculate(tasks[blockIndices[j]])
			})

			// Determine Start Time
			startStr := getBlockStart(blockName, blocks)
			// Parse HH:MM
			startTime, _ := time.Parse("15:04", startStr)
			
			cursor := time.Date(day.Year(), day.Month(), day.Day(), startTime.Hour(), startTime.Minute(), 0, 0, day.Location())

			for _, idx := range blockIndices {
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
							break 
						}
					}

					if !collision {
						// Found a slot!
						newTime := candidateStart
						tasks[idx].ScheduledAt = &newTime
						
						busy = append(busy, Interval{candidateStart, candidateEnd})
						cursor = candidateEnd
						break
					} else {
						cursor = nextAvailable
					}
				}
			}
		}
	}

	return tasks
}

func isFloating(t models.Task) bool {
	if t.ScheduledAt == nil {
		return false
	}
	// If ScheduleBlock is explicit, it's floating (even if time is set, we might override? No, if time is set it's fixed usually)
	// But requirement says "schedules it for tomorrow morning block".
	// If the user adds --scheduled tomorrow --block morning, scheduledAt is 00:00.
	// So checking for 00:00 is correct for date-only scheduling.
	// What if user sets 06:00 AND block morning? Then it's Fixed at 06:00, but tagged as Morning.
	// In that case, we should treat it as Fixed.
	return t.ScheduledAt.Hour() == 0 && t.ScheduledAt.Minute() == 0 && t.ScheduledAt.Second() == 0
}

func getBlockStart(name string, blocks map[string]config.TimeBlock) string {
	if name == "" {
		return "07:00" // Default start for generic floating tasks
	}
	if b, ok := blocks[name]; ok {
		return b.Start
	}
	return "07:00" // Fallback
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
