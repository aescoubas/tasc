package cmd

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/aescoubas/tasc/internal/config"
	"github.com/aescoubas/tasc/internal/models"
	"github.com/aescoubas/tasc/internal/parse"
	"github.com/aescoubas/tasc/internal/priority"
	"github.com/aescoubas/tasc/internal/scheduling"
	"github.com/aescoubas/tasc/internal/store"
	"github.com/aescoubas/tasc/internal/ui"
	"github.com/spf13/cobra"
)

type taskWithScore struct {
	task  models.Task
	score float64
}

var (
	sortBy   string
	sortDesc bool
	showAll  bool
	forceRelative bool
	showAbsolute bool

	// Filters
	filterProjects  []string
	filterIDs       []string
	filterStatus    []string
	filterDueBefore string
	filterDueAfter  string
	filterSchBefore string
	filterSchAfter  string
	filterScoreMin  float64
	filterScoreMax  float64
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List pending tasks",
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.LoadConfig()
		if err != nil {
			cfg = config.DefaultConfig()
		}

		if cmd.Flags().Changed("relative") {
			cfg.RelativeDates = forceRelative
		}
		if cmd.Flags().Changed("absolute") {
			if showAbsolute {
				cfg.RelativeDates = false
			}
		}

		// Parse Filters
		filter := store.TaskFilter{}
		
		// Fetch all projects to handle hierarchy
		projectParent := make(map[string]string)
		projectShortName := make(map[string]string)
		projectChildren := make(map[string][]string)
		var allProjects []models.Project
		if projects, err := CurrentStore.ListProjects(); err == nil {
			allProjects = projects
			for _, p := range allProjects {
				if p.ShortName != "" {
					projectShortName[p.Name] = p.ShortName
				}
				if p.Parent != "" {
					projectParent[p.Name] = p.Parent
					projectChildren[p.Parent] = append(projectChildren[p.Parent], p.Name)
				}
			}
		}

		// Expand filterProjects to include children
		if len(filterProjects) > 0 {
			expanded := make(map[string]bool)
			var queue []string
			queue = append(queue, filterProjects...)
			
			for len(queue) > 0 {
				curr := queue[0]
				queue = queue[1:]
				if expanded[curr] { continue }
				expanded[curr] = true
				
				if children, ok := projectChildren[curr]; ok {
					queue = append(queue, children...)
				}
			}
			
			filter.Projects = nil
			for p := range expanded {
				filter.Projects = append(filter.Projects, p)
			}
		} else {
			filter.Projects = filterProjects
		}

		for _, s := range filterStatus {
			filter.Status = append(filter.Status, models.TaskStatus(s))
		}

		for _, s := range filterIDs {
			id, err := strconv.ParseInt(s, 10, 64)
			if err == nil {
				filter.IDs = append(filter.IDs, id)
			}
		}

		if filterDueBefore != "" {
			t, err := parse.Date(filterDueBefore, "")
			if err == nil {
				filter.DueBefore = t
			}
		}
		if filterDueAfter != "" {
			t, err := parse.Date(filterDueAfter, "")
			if err == nil {
				filter.DueAfter = t
			}
		}
		if filterSchBefore != "" {
			t, err := parse.Date(filterSchBefore, "")
			if err == nil {
				filter.ScheduledBefore = t
			}
		}
		if filterSchAfter != "" {
			t, err := parse.Date(filterSchAfter, "")
			if err == nil {
				filter.ScheduledAfter = t
			}
		}

		// 1. Fetch tasks via Store
		fetchedTasks, err := CurrentStore.ListTasks(filter)
		if err != nil {
			fmt.Printf("Error querying tasks: %v\n", err)
			return
		}

		// Apply Smart Auto-Schedule (Virtual Times)
		fetchedTasks = scheduling.ApplyAutoSchedule(fetchedTasks, cfg.TimeBlocks)

		var tasks []taskWithScore
		calc := priority.NewCalculator()

		for _, t := range fetchedTasks {
			score := calc.Calculate(t)
			tasks = append(tasks, taskWithScore{task: t, score: score})
		}

		// Filter by Score (Post-calculation)
		if cmd.Flags().Changed("score-min") || cmd.Flags().Changed("score-max") {
			var filtered []taskWithScore
			for _, t := range tasks {
				if cmd.Flags().Changed("score-min") && t.score < filterScoreMin {
					continue
				}
				if cmd.Flags().Changed("score-max") && t.score > filterScoreMax {
					continue
				}
				filtered = append(filtered, t)
			}
			tasks = filtered
		}

		// --- Dependency-Aware Priority Propagation ---
		// Fetch dependencies
		deps, err := CurrentStore.GetDependencies()
		if err == nil {
			// Map for quick lookup
			baseScores := make(map[int64]float64)
			for _, t := range tasks {
				baseScores[t.task.ID] = t.score
			}

			boosts := make(map[int64]float64)

			// Relaxation loop to propagate scores (Blocker gets boost from Blocked)
			// Max 10 iterations to handle chains
			for k := 0; k < 10; k++ {
				newBoosts := make(map[int64]float64)
				changed := false

				for _, dep := range deps {
					blockedBase, ok := baseScores[dep.BlockedID]
					if !ok { continue } 
					
					blockedTotal := blockedBase + boosts[dep.BlockedID]
					inherited := blockedTotal * 0.5
					
					if inherited > newBoosts[dep.BlockerID] {
						newBoosts[dep.BlockerID] = inherited
					}
				}

				for id, val := range newBoosts {
					if val != boosts[id] {
						changed = true
						break
					}
				}
				boosts = newBoosts
				if !changed {
					break
				}
			}

			// Apply boosts
			for i := range tasks {
				if b, ok := boosts[tasks[i].task.ID]; ok {
					tasks[i].score += b
				}
			}
		}

		// Sort tasks
		sort.Slice(tasks, func(i, j int) bool {
			if sortBy == "" {
				// Default: Sort by active status then score descending
				activeI := tasks[i].task.ActiveStart != nil
				activeJ := tasks[j].task.ActiveStart != nil
				if activeI && !activeJ {
					return true
				}
				if !activeI && activeJ {
					return false
				}
				return tasks[i].score > tasks[j].score
			}

			t1 := tasks[i].task
			t2 := tasks[j].task
			var less bool

			switch strings.ToLower(sortBy) {
			case "id":
				less = t1.ID < t2.ID
			case "project":
				less = strings.ToLower(t1.Project) < strings.ToLower(t2.Project)
			case "title", "description", "desc":
				less = strings.ToLower(t1.Title) < strings.ToLower(t2.Title)
			case "created", "created_at":
				less = t1.CreatedAt.Before(t2.CreatedAt)
			case "age":
				less = t2.CreatedAt.Before(t1.CreatedAt)
			case "due", "due_at":
				if t1.DueAt == nil && t2.DueAt == nil {
					less = false
				} else if t1.DueAt == nil {
					less = false // nil > non-nil
				} else if t2.DueAt == nil {
					less = true
				} else {
					less = t1.DueAt.Before(*t2.DueAt)
				}
			case "scheduled", "scheduled_at", "sch":
				if t1.ScheduledAt == nil && t2.ScheduledAt == nil {
					less = false
				} else if t1.ScheduledAt == nil {
					less = false
				} else if t2.ScheduledAt == nil {
					less = true
				} else {
					less = t1.ScheduledAt.Before(*t2.ScheduledAt)
				}
			case "estimate", "est":
				less = t1.Estimate < t2.Estimate
			case "score":
				less = tasks[i].score < tasks[j].score
			case "duration", "time_spent":
				d1 := t1.TimeSpent
				if t1.ActiveStart != nil {
					d1 += int64(time.Since(*t1.ActiveStart).Seconds())
				}
				d2 := t2.TimeSpent
				if t2.ActiveStart != nil {
					d2 += int64(time.Since(*t2.ActiveStart).Seconds())
				}
				less = d1 < d2
			default:
				less = t1.ID < t2.ID
			}

			if sortDesc {
				return !less
			}
			return less
		})

		// Prepare data for display
		var displayTasks []models.Task
		displayScores := make(map[int64]float64)
		for _, item := range tasks {
			displayTasks = append(displayTasks, item.task)
			displayScores[item.task.ID] = item.score
		}

		opts := ui.TableOptions{
			ShowAll: showAll,
		}

		ui.RenderTaskTable(displayTasks, displayScores, cfg, allProjects, opts)
	},
}


func init() {
	listCmd.Flags().StringVarP(&sortBy, "sort", "s", "", "Sort by field (id, project, description, created, age, due, scheduled, estimate, score, duration)")
	listCmd.Flags().BoolVarP(&sortDesc, "desc", "d", false, "Sort in descending order")
	listCmd.Flags().BoolVarP(&showAll, "all", "a", false, "Show all tasks (do not truncate to screen height)")
	listCmd.Flags().BoolVarP(&forceRelative, "relative", "R", false, "Force relative dates")
	listCmd.Flags().BoolVarP(&showAbsolute, "absolute", "A", false, "Force absolute dates")

	listCmd.Flags().StringSliceVarP(&filterProjects, "project", "p", nil, "Filter by project (comma separated)")
	listCmd.Flags().StringSliceVar(&filterStatus, "status", nil, "Filter by status (comma separated: backlog, ongoing, done, blocked)")
	listCmd.Flags().StringSliceVarP(&filterIDs, "id", "i", nil, "Filter by ID (comma separated)")

	listCmd.Flags().StringVar(&filterDueBefore, "due-before", "", "Filter tasks due before date")
	listCmd.Flags().StringVar(&filterDueAfter, "due-after", "", "Filter tasks due after date")
	listCmd.Flags().StringVar(&filterSchBefore, "scheduled-before", "", "Filter tasks scheduled before date")
	listCmd.Flags().StringVar(&filterSchAfter, "scheduled-after", "", "Filter tasks scheduled after date")

	listCmd.Flags().Float64Var(&filterScoreMin, "score-min", 0, "Minimum priority score")
	listCmd.Flags().Float64Var(&filterScoreMax, "score-max", 0, "Maximum priority score")

	listCmd.RegisterFlagCompletionFunc("project", projectCompletionFunc)

	listCmd.RegisterFlagCompletionFunc("due-before", dateCompletionFunc)
	listCmd.RegisterFlagCompletionFunc("due-after", dateCompletionFunc)
	listCmd.RegisterFlagCompletionFunc("scheduled-before", dateCompletionFunc)
	listCmd.RegisterFlagCompletionFunc("scheduled-after", dateCompletionFunc)

	rootCmd.AddCommand(listCmd)
}
