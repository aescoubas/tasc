package cmd

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/aescoubas/tasc/internal/models"
)

// graphFixture builds the task map for IDs 1..n, since buildGraphView only uses
// it to decide which endpoints are live.
func graphFixture(n int64) map[int64]models.Task {
	tasks := make(map[int64]models.Task, n)
	for id := int64(1); id <= n; id++ {
		tasks[id] = models.Task{ID: id, Title: fmt.Sprintf("task %d", id)}
	}
	return tasks
}

func deps(pairs ...[2]int64) []models.TaskDependency {
	out := make([]models.TaskDependency, 0, len(pairs))
	for _, p := range pairs {
		out = append(out, models.TaskDependency{BlockerID: p[0], BlockedID: p[1]})
	}
	return out
}

func nodesOf(v graphView) map[int64]bool {
	nodes := make(map[int64]bool)
	for from, tos := range v.adj {
		nodes[from] = true
		for _, to := range tos {
			nodes[to] = true
		}
	}
	return nodes
}

func assertNodes(t *testing.T, v graphView, want []int64) {
	t.Helper()

	got := nodesOf(v)
	wantSet := make(map[int64]bool, len(want))
	for _, id := range want {
		wantSet[id] = true
		if !got[id] {
			t.Errorf("task %d missing from the view", id)
		}
	}
	for id := range got {
		if !wantSet[id] {
			t.Errorf("task %d should not be in the view", id)
		}
	}
}

// 5 blocks 1, 1 blocks 2, 2 blocks 3, and 4 also blocks 3. Task 4 is the one to
// watch: it shares a blocked task with the chain but is neither a blocker nor a
// dependent of task 2.
func chainFixture() []models.TaskDependency {
	return deps([2]int64{5, 1}, [2]int64{1, 2}, [2]int64{2, 3}, [2]int64{4, 3})
}

func TestBuildGraphViewUnfocusedKeepsEveryEdge(t *testing.T) {
	v := buildGraphView(chainFixture(), graphFixture(5), 0)

	if v.edges != 4 {
		t.Errorf("edges = %d, want 4", v.edges)
	}
	assertNodes(t, v, []int64{1, 2, 3, 4, 5})
	if v.cyclic {
		t.Error("cyclic = true on an acyclic graph")
	}
	// 4 and 5 are the only tasks nothing blocks.
	if got := fmt.Sprint(v.roots); got != "[4 5]" {
		t.Errorf("roots = %s, want [4 5]", got)
	}
}

func TestBuildGraphViewFocusWalksBothDirections(t *testing.T) {
	v := buildGraphView(chainFixture(), graphFixture(5), 2)

	// Upwards: 1 blocks 2 and 5 blocks 1. Downwards: 2 blocks 3.
	assertNodes(t, v, []int64{1, 2, 3, 5})
	if got := fmt.Sprint(v.roots); got != "[5]" {
		t.Errorf("roots = %s, want [5]", got)
	}
}

// A task that merely shares a blocked task with the focus is not part of why the
// focus is held up, so it stays out even though the graph is connected.
func TestBuildGraphViewFocusExcludesSiblingBlocker(t *testing.T) {
	v := buildGraphView(chainFixture(), graphFixture(5), 2)

	if nodesOf(v)[4] {
		t.Error("task 4 blocks only task 3, so it should not appear in task 2's view")
	}
	if v.edges != 3 {
		t.Errorf("edges = %d, want 3 — the 4 -> 3 edge should be dropped", v.edges)
	}
}

func TestBuildGraphViewFocusKeepsEveryBlockerOfTheFocus(t *testing.T) {
	// Focusing 3 does pull in 4: it is a direct blocker of the focus.
	v := buildGraphView(chainFixture(), graphFixture(5), 3)

	assertNodes(t, v, []int64{1, 2, 3, 4, 5})
	if got := fmt.Sprint(v.roots); got != "[4 5]" {
		t.Errorf("roots = %s, want [4 5]", got)
	}
}

func TestBuildGraphViewIsolatedFocusHasNoEdges(t *testing.T) {
	// Task 6 exists but no edge touches it.
	v := buildGraphView(chainFixture(), graphFixture(6), 6)

	if v.edges != 0 {
		t.Errorf("edges = %d, want 0", v.edges)
	}
	if len(v.roots) != 0 {
		t.Errorf("roots = %v, want none", v.roots)
	}
}

func TestBuildGraphViewUnrelatedComponentIsExcluded(t *testing.T) {
	// 6 blocks 7, with no path to the 1..5 chain at all.
	all := append(chainFixture(), deps([2]int64{6, 7})...)

	v := buildGraphView(all, graphFixture(7), 2)

	assertNodes(t, v, []int64{1, 2, 3, 5})
}

// An edge is only drawable when both endpoints are live. A deleted or mistyped
// endpoint is absent from the task map, and foreign_keys is off on this
// connection, so nothing upstream catches it.
func TestBuildGraphViewDropsEdgesWithDeadEndpoints(t *testing.T) {
	d := deps([2]int64{1, 2}, [2]int64{2, 999}, [2]int64{998, 1})

	v := buildGraphView(d, graphFixture(2), 0)

	if v.edges != 1 {
		t.Errorf("edges = %d, want 1", v.edges)
	}
	assertNodes(t, v, []int64{1, 2})
}

func TestBuildGraphViewReportsCycles(t *testing.T) {
	// `dep` refuses a direct A<->B pair but nothing refuses a longer loop.
	d := deps([2]int64{1, 2}, [2]int64{2, 3}, [2]int64{3, 1})

	v := buildGraphView(d, graphFixture(3), 0)

	if !v.cyclic {
		t.Error("cyclic = false, want true — every node has an incoming edge")
	}
	// With no real root, all of them are drawn from.
	if got := fmt.Sprint(v.roots); got != "[1 2 3]" {
		t.Errorf("roots = %s, want [1 2 3]", got)
	}
}

// capturePrint drives the real printer and returns what it wrote. If the path
// guard in print were removed this would not return — a hang or a stack
// overflow rather than a quiet pass, which is the point.
func capturePrint(t *testing.T, p graphPrinter, roots []int64) string {
	t.Helper()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}

	saved := os.Stdout
	os.Stdout = w
	done := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		done <- buf.String()
	}()

	for _, root := range roots {
		p.print(root, 0, map[int64]bool{})
	}

	os.Stdout = saved
	_ = w.Close()
	out := <-done
	_ = r.Close()

	return out
}

// A cycle must not be rendered by recursing into it forever. The printer cuts a
// branch that re-enters its own path, which bounds the walk.
func TestGraphPrinterTerminatesOnCycle(t *testing.T) {
	tasks := graphFixture(3)
	v := buildGraphView(deps([2]int64{1, 2}, [2]int64{2, 3}, [2]int64{3, 1}), tasks, 0)

	out := capturePrint(t, graphPrinter{adj: v.adj, tasks: tasks}, v.roots)

	if !strings.Contains(out, "(cycle)") {
		t.Errorf("output does not mark the cycle:\n%s", out)
	}
	// Three roots, each drawing the three nodes plus the line where the loop
	// closes. Any more than that means the guard is letting it go round twice.
	if got := strings.Count(out, "\n"); got != 12 {
		t.Errorf("printed %d lines, want 12:\n%s", got, out)
	}
}

func TestGraphPrinterMarksTheFocusedTask(t *testing.T) {
	tasks := graphFixture(3)
	v := buildGraphView(deps([2]int64{1, 2}, [2]int64{2, 3}), tasks, 2)

	out := capturePrint(t, graphPrinter{adj: v.adj, tasks: tasks, focus: 2}, v.roots)

	if !strings.Contains(out, "← this task") {
		t.Errorf("focused task is not marked:\n%s", out)
	}
	if got := strings.Count(out, "← this task"); got != 1 {
		t.Errorf("marked %d lines, want 1:\n%s", got, out)
	}
	// Unfocused, nothing is marked.
	plain := capturePrint(t, graphPrinter{adj: v.adj, tasks: tasks}, v.roots)
	if strings.Contains(plain, "← this task") {
		t.Errorf("unfocused output should carry no marker:\n%s", plain)
	}
}

// Map iteration order is random, so the same input drawn twice must still give
// the same roots and the same child order.
func TestBuildGraphViewIsDeterministic(t *testing.T) {
	all := append(chainFixture(), deps([2]int64{6, 7}, [2]int64{6, 3})...)
	tasks := graphFixture(7)

	first := fmt.Sprintf("%v", buildGraphView(all, tasks, 0).roots)
	firstChildren := fmt.Sprintf("%v", buildGraphView(all, tasks, 0).adj[6])

	for i := 0; i < 50; i++ {
		v := buildGraphView(all, tasks, 0)
		if got := fmt.Sprintf("%v", v.roots); got != first {
			t.Fatalf("roots = %s on run %d, want %s", got, i, first)
		}
		if got := fmt.Sprintf("%v", v.adj[6]); got != firstChildren {
			t.Fatalf("children of 6 = %s on run %d, want %s", got, i, firstChildren)
		}
	}

	if firstChildren != "[3 7]" {
		t.Errorf("children of 6 = %s, want them sorted as [3 7]", firstChildren)
	}
}
