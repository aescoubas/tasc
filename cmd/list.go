package cmd

import (
	"fmt"
	"os"
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
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
	"golang.org/x/term"
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
		filter.Projects = filterProjects

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
			t, err := parse.Date(filterDueBefore)
			if err == nil {
				filter.DueBefore = t
			}
		}
		if filterDueAfter != "" {
			t, err := parse.Date(filterDueAfter)
			if err == nil {
				filter.DueAfter = t
			}
		}
		if filterSchBefore != "" {
			t, err := parse.Date(filterSchBefore)
			if err == nil {
				filter.ScheduledBefore = t
			}
		}
		if filterSchAfter != "" {
			t, err := parse.Date(filterSchAfter)
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
		fetchedTasks = scheduling.ApplyAutoSchedule(fetchedTasks)

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
		type displayRow struct {
			id       string
			project  string
			status   string
			desc     string
			created  string
			age      string
			due      string
			sch      string
			rsch     string
			est      string
			score    string
			duration string
			tier     ui.UrgencyTier
		}

						var rowsData []displayRow
						widths := make([]int, 12)
						headers := []string{"ID", "Project", "Status", "Title", "Created", "Age", "Due", "Scheduled", "Rsch", "Est", "Score", "Duration"}
				
						for i, h := range headers {					widths[i] = len(h)
				}
		
				for _, item := range tasks {
					t := item.task
		
					finalTier := ui.GetUrgencyTier(t)
		
					dueStr := "-"
					if t.DueAt != nil {
						if cfg.RelativeDates {
							dueStr = ui.FormatRelative(*t.DueAt)
						} else {
							dueStr = t.DueAt.Format("2006-01-02")
						}
					}
		
					schStr := "-"
					if t.ScheduledAt != nil {
						if cfg.RelativeDates {
							schStr = ui.FormatRelative(*t.ScheduledAt)
						} else {
							schStr = t.ScheduledAt.Format("2006-01-02")
						}
					}
		
					rschStr := "-"
					if t.RescheduleCount > 0 {
						rschStr = fmt.Sprintf("%d", t.RescheduleCount)
					}
		
					estStr := "-"
					if t.Estimate != "" {
						estStr = t.Estimate
					}
		
					desc := strings.ReplaceAll(t.Title, "\n", " ")
					if len(desc) > 50 {
						desc = desc[:47] + "..."
					}
					duration := t.TimeSpent
					if t.ActiveStart != nil {
						desc = "ONGOING: " + desc
						duration += int64(time.Since(*t.ActiveStart).Seconds())
					}
		
					durStr := "-"
					if duration > 0 {
						d := time.Duration(duration) * time.Second
						durStr = d.String()
					}
		
					projectStr := t.Project
					if len(projectStr) > 20 {
						projectStr = projectStr[:17] + "..."
					}
					
					statusStr := string(t.Status)
		
					row := displayRow{
						id:       fmt.Sprintf("%d", t.ID),
						project:  projectStr,
						status:   statusStr,
						desc:     desc,
						created:  t.CreatedAt.Format("2006-01-02"),
					age:      t.AgeString(),
					due:      dueStr,
					sch:      schStr,
					rsch:     rschStr,
					est:      estStr,
					score:    fmt.Sprintf("%.1f", item.score),
					duration: durStr,
					tier:     finalTier,
					}
		
					rowsData = append(rowsData, row)
		
					if len(row.id) > widths[0] { widths[0] = len(row.id) }
					if len(row.project) > widths[1] { widths[1] = len(row.project) }
					if len(row.status) > widths[2] { widths[2] = len(row.status) }
					if len(row.desc) > widths[3] { widths[3] = len(row.desc) }
					if len(row.created) > widths[4] { widths[4] = len(row.created) }
					if len(row.age) > widths[5] { widths[5] = len(row.age) }
					if len(row.due) > widths[6] { widths[6] = len(row.due) }
					if len(row.sch) > widths[7] { widths[7] = len(row.sch) }
					if len(row.rsch) > widths[8] { widths[8] = len(row.rsch) }
					if len(row.est) > widths[9] { widths[9] = len(row.est) }
					if len(row.score) > widths[10] { widths[10] = len(row.score) }
					if len(row.duration) > widths[11] { widths[11] = len(row.duration) }
				}
		
				totalWidth := 0
				for _, w := range widths {
					totalWidth += w
				}
				totalWidth += (len(widths) - 1) * 3 
		
				width, height, err := term.GetSize(int(os.Stdout.Fd()))
				if err == nil && width > 0 {
					if totalWidth > width {
						fixed := totalWidth - widths[3]
						avail := width - fixed
						
						if avail < 10 {
							avail = 10 
						}
		
						if avail < widths[3] {
							widths[3] = avail
							for k, row := range rowsData {
								if len(row.desc) > avail {
									if avail > 3 {
										rowsData[k].desc = row.desc[:avail-3] + "..."
									} else {
										rowsData[k].desc = row.desc[:avail]
									}
								}
							}
							totalWidth = fixed + avail
						}
					}
				}
		
				if !showAll {
					if err == nil && height > 0 {
						linesPerTask := 1
						if width > 0 {
							linesPerTask = (totalWidth + width - 1) / width
						}
						
						headerLines := 1
						if width > 0 {
							headerLines = (totalWidth + width - 1) / width
						}
		
						availableLines := height - 4 - headerLines
						if availableLines < 0 {
							availableLines = 0
						}
		
						tasksToShow := 0
						if linesPerTask > 0 {
							tasksToShow = availableLines / linesPerTask
						}
		
						if tasksToShow < len(rowsData) {
							rowsData = rowsData[:tasksToShow]
						}
					}
				}
		
				styles := make(map[ui.UrgencyTier]lipgloss.Style)
				styles[ui.TierOverdue] = ui.GetStyleForTier(ui.TierOverdue, cfg)
				styles[ui.TierToday] = ui.GetStyleForTier(ui.TierToday, cfg)
				styles[ui.TierTomorrow] = ui.GetStyleForTier(ui.TierTomorrow, cfg)
				styles[ui.TierWeek] = ui.GetStyleForTier(ui.TierWeek, cfg)
				styles[ui.TierDefault] = ui.GetStyleForTier(ui.TierDefault, cfg)
		
				var headerStr strings.Builder
				for i, h := range headers {
					fmt.Fprintf(&headerStr, "%-*s", widths[i], h)
					if i < len(headers)-1 {
						headerStr.WriteString("   ")
					}
				}
				fmt.Println(headerStr.String())
		
				for _, row := range rowsData {			style := styles[row.tier]
			printCol := func(text string, width int, isLast bool) {
				padded := fmt.Sprintf("%*s", width, text)
				fmt.Print(style.Render(padded))
				if !isLast {
					fmt.Print(style.Render("   "))
				}
			}

			printCol(row.id, widths[0], false)
			printCol(row.project, widths[1], false)
			printCol(row.status, widths[2], false)
			printCol(row.desc, widths[3], false)
			printCol(row.created, widths[4], false)
			printCol(row.age, widths[5], false)
			printCol(row.due, widths[6], false)
			printCol(row.sch, widths[7], false)
			printCol(row.rsch, widths[8], false)
			printCol(row.est, widths[9], false)
			printCol(row.score, widths[10], false)
			printCol(row.duration, widths[11], true)
			fmt.Println()
		}
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

	listCmd.RegisterFlagCompletionFunc("project", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if CurrentStore == nil {
			return nil, cobra.ShellCompDirectiveError
		}
		projects, err := CurrentStore.ListProjects()
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}
		var names []string
		for _, p := range projects {
			names = append(names, p.Name)
		}
		return names, cobra.ShellCompDirectiveNoFileComp
	})

	rootCmd.AddCommand(listCmd)
}
