package cmd

import (
	"fmt"
	"sort"
	"strconv"

	"github.com/aescoubas/tasc/internal/config"
	"github.com/aescoubas/tasc/internal/models"
	"github.com/aescoubas/tasc/internal/store"
	"github.com/aescoubas/tasc/internal/ui"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/cobra"
)

// graphStatuses is every status except deleted. A done blocker still belongs in
// the picture — it is what explains why its subtree is no longer held up — so
// the default ListTasks filter, which drops done and undefined, is too narrow
// here.
var graphStatuses = []models.TaskStatus{
	models.StatusBacklog,
	models.StatusOngoing,
	models.StatusDone,
	models.StatusBlocked,
	models.StatusUndefined,
}

var graphCmd = &cobra.Command{
	Use:   "graph [id]",
	Short: "Visualize task dependencies",
	Long: `Visualize task dependencies as a tree.

With no argument, draws every dependency in the database. Given a task ID,
draws only that task's own graph: what blocks it, transitively, and what it
blocks, transitively.`,
	Args:              cobra.MaximumNArgs(1),
	ValidArgsFunction: taskIDCompletionFunc,
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, _ := config.LoadConfig()

		deps, err := CurrentStore.GetDependencies()
		if err != nil {
			return fmt.Errorf("get dependencies: %w", err)
		}

		tasks, err := graphTasks(deps)
		if err != nil {
			return err
		}

		var focus int64
		if len(args) == 1 {
			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid task ID %q", args[0])
			}
			if _, ok := tasks[id]; !ok {
				return fmt.Errorf("task %d not found", id)
			}
			focus = id
		}

		view := buildGraphView(deps, tasks, focus)

		if view.edges == 0 {
			if focus != 0 {
				fmt.Printf("Task %d %s has no dependencies.\n", focus, tasks[focus].Title)
			} else {
				fmt.Println("No dependencies found.")
			}
			return nil
		}

		style := lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Bold(true)
		if focus != 0 {
			fmt.Println(style.Render(fmt.Sprintf("Dependency Graph: %d %s", focus, tasks[focus].Title)))
		} else {
			fmt.Println(style.Render("Dependency Graph"))
		}
		if view.cyclic {
			fmt.Println("Circular dependency detected or no clear roots.")
		}

		p := graphPrinter{adj: view.adj, tasks: tasks, cfg: cfg, focus: focus}
		for _, root := range view.roots {
			p.print(root, 0, map[int64]bool{})
		}
		return nil
	},
}

// graphView is the drawable graph: an adjacency list of blocker -> blocked, and
// the roots to start drawing from.
type graphView struct {
	adj   map[int64][]int64
	roots []int64
	edges int
	// cyclic means every node had an incoming edge, so there was no real root
	// and roots holds all of them instead.
	cyclic bool
}

// buildGraphView turns the stored edges into the tree to draw. A focus of 0
// means the whole graph; otherwise the view is narrowed to that task's own
// graph — its transitive blockers and the tasks it transitively blocks.
//
// Note that this is deliberately not the whole connected component: a sibling
// task that merely shares a blocker with something in view is not part of why
// the focused task is held up, and pulling it in makes a focused graph drift
// back towards the global one.
func buildGraphView(deps []models.TaskDependency, tasks map[int64]models.Task, focus int64) graphView {
	// blocks maps blocker -> blocked; blockedBy is the same edges reversed,
	// which is what lets a focused view walk upwards to a task's blockers. An
	// edge is only drawable if both endpoints are live: foreign_keys is off on
	// this connection, so a mistyped ID can leave an edge pointing at nothing,
	// and a deleted task is deliberately not shown.
	blocks := make(map[int64][]int64)
	blockedBy := make(map[int64][]int64)
	for _, d := range deps {
		if _, ok := tasks[d.BlockerID]; !ok {
			continue
		}
		if _, ok := tasks[d.BlockedID]; !ok {
			continue
		}
		blocks[d.BlockerID] = append(blocks[d.BlockerID], d.BlockedID)
		blockedBy[d.BlockedID] = append(blockedBy[d.BlockedID], d.BlockerID)
	}

	// GetDependencies has no ORDER BY, so sort every child list: without this
	// the same database draws its tree in a different order each run.
	for id := range blocks {
		sortIDs(blocks[id])
	}

	inView := func(int64) bool { return true }
	if focus != 0 {
		seen := map[int64]bool{focus: true}
		collectReachable(focus, blockedBy, seen)
		collectReachable(focus, blocks, seen)
		inView = func(id int64) bool { return seen[id] }
	}

	v := graphView{adj: make(map[int64][]int64, len(blocks))}
	nodes := make(map[int64]bool)
	hasIncoming := make(map[int64]bool)
	for from, tos := range blocks {
		if !inView(from) {
			continue
		}
		for _, to := range tos {
			if !inView(to) {
				continue
			}
			v.adj[from] = append(v.adj[from], to)
			nodes[from] = true
			nodes[to] = true
			hasIncoming[to] = true
			v.edges++
		}
	}

	for id := range nodes {
		if !hasIncoming[id] {
			v.roots = append(v.roots, id)
		}
	}

	// Every node having an incoming edge means they all sit on cycles, so there
	// is no root to start from. Draw from all of them; the printer stops where a
	// branch re-enters its own path.
	if len(v.roots) == 0 && len(nodes) > 0 {
		v.cyclic = true
		for id := range nodes {
			v.roots = append(v.roots, id)
		}
	}
	sortIDs(v.roots)

	return v
}

// graphTasks loads every task referenced by deps, keyed by ID and with deleted
// tasks left out.
//
// The bulk ListTasks is one call and carries IsBlocked, which is what colours
// the tree. The remote backend drops filter.Status on the floor, though, so
// against a server the done tasks come back missing and have to be fetched one
// by one — GetTask applies no status filter on either backend. Those fall back
// to IsBlocked false, which only understates the 🚫 marker on a done task.
func graphTasks(deps []models.TaskDependency) (map[int64]models.Task, error) {
	listed, err := CurrentStore.ListTasks(store.TaskFilter{Status: graphStatuses})
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}

	tasks := make(map[int64]models.Task, len(listed))
	for _, t := range listed {
		if t.Status == models.StatusDeleted {
			continue
		}
		tasks[t.ID] = t
	}

	for _, id := range referencedIDs(deps) {
		if _, ok := tasks[id]; ok {
			continue
		}
		t, err := CurrentStore.GetTask(id)
		if err != nil || t.Status == models.StatusDeleted {
			continue
		}
		tasks[id] = t
	}

	return tasks, nil
}

func referencedIDs(deps []models.TaskDependency) []int64 {
	ids := make([]int64, 0, len(deps)*2)
	for _, d := range deps {
		ids = append(ids, d.BlockerID, d.BlockedID)
	}
	return uniqueInt64s(ids)
}

func collectReachable(from int64, edges map[int64][]int64, seen map[int64]bool) {
	for _, next := range edges[from] {
		if seen[next] {
			continue
		}
		seen[next] = true
		collectReachable(next, edges, seen)
	}
}

func sortIDs(ids []int64) {
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
}

type graphPrinter struct {
	adj   map[int64][]int64
	tasks map[int64]models.Task
	cfg   config.Config
	focus int64
}

// print draws the subtree under id. path holds the ancestors of the current
// node, so a branch that re-enters it is reported and cut rather than recursed
// into forever — `dep` only refuses a direct A↔B pair, so a longer cycle can be
// stored.
func (p graphPrinter) print(id int64, level int, path map[int64]bool) {
	indent := ""
	for i := 0; i < level; i++ {
		indent += "  "
	}

	arrow := "└─"
	if level == 0 {
		arrow = "●"
	}

	t := p.tasks[id]

	if path[id] {
		cycle := lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
		fmt.Printf("%s%s %s\n", indent, arrow, cycle.Render(fmt.Sprintf("↻ %d %s (cycle)", id, t.Title)))
		return
	}

	// FormatTask gives the same rendering as every other list — colour, icons,
	// the blocked marker.
	line := ui.FormatTask(t, p.cfg)
	if p.focus != 0 && id == p.focus {
		line += lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Render(" ← this task")
	}
	fmt.Printf("%s%s %s\n", indent, arrow, line)

	path[id] = true
	for _, child := range p.adj[id] {
		p.print(child, level+1, path)
	}
	delete(path, id)
}

func init() {
	rootCmd.AddCommand(graphCmd)
}
