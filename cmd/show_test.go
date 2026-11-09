package cmd

import (
	"fmt"
	"testing"

	"github.com/aescoubas/tasc/internal/models"
)

// seedShowTasks creates n backlog tasks in a fresh store, asserting they land on
// IDs 1..n so the edges below can name them directly.
func seedShowTasks(t *testing.T, n int64) {
	t.Helper()

	for i := int64(1); i <= n; i++ {
		id, err := CurrentStore.CreateTask(models.Task{
			Title:  fmt.Sprintf("task %d", i),
			Status: models.StatusBacklog,
		})
		if err != nil {
			t.Fatalf("create task %d: %v", i, err)
		}
		if id != i {
			t.Fatalf("task %d landed on id %d; the fixture assumes 1..n", i, id)
		}
	}
}

func addShowEdges(t *testing.T, edges ...[2]int64) {
	t.Helper()

	for _, e := range edges {
		if err := CurrentStore.AddDependency(e[0], e[1]); err != nil {
			t.Fatalf("add dependency %d -> %d: %v", e[0], e[1], err)
		}
	}
}

func refIDs(refs []taskRef) string {
	ids := make([]int64, 0, len(refs))
	for _, r := range refs {
		ids = append(ids, r.ID)
	}
	return fmt.Sprint(ids)
}

// showNeighbours is what replaced the two JOINs, so the behaviour it has to keep
// is: only open blockers in "Blocked By", everything blocked in "Blocking".
func TestShowNeighboursSplitsBothDirections(t *testing.T) {
	setupDependencyTestStore(t)
	seedShowTasks(t, 4)

	// 1 and 3 block 2; 2 blocks 4.
	addShowEdges(t, [2]int64{1, 2}, [2]int64{3, 2}, [2]int64{2, 4})

	blockedBy, blocking, err := showNeighbours(2)
	if err != nil {
		t.Fatalf("showNeighbours: %v", err)
	}

	if got := refIDs(blockedBy); got != "[1 3]" {
		t.Errorf("blockedBy = %s, want [1 3]", got)
	}
	if got := refIDs(blocking); got != "[4]" {
		t.Errorf("blocking = %s, want [4]", got)
	}
	if len(blockedBy) > 0 && blockedBy[0].Title != "task 1" {
		t.Errorf("blockedBy[0].Title = %q, want \"task 1\"", blockedBy[0].Title)
	}
}

// A done blocker no longer blocks anything, so it drops out of "Blocked By" —
// otherwise show would contradict is_blocked everywhere else. It stays in the
// blocker's own "Blocking" list, since the edge still exists.
func TestShowNeighboursHidesSatisfiedBlockers(t *testing.T) {
	setupDependencyTestStore(t)
	seedShowTasks(t, 3)
	addShowEdges(t, [2]int64{1, 3}, [2]int64{2, 3})

	if err := CurrentStore.MarkDone(1); err != nil {
		t.Fatalf("mark task 1 done: %v", err)
	}

	blockedBy, _, err := showNeighbours(3)
	if err != nil {
		t.Fatalf("showNeighbours: %v", err)
	}
	if got := refIDs(blockedBy); got != "[2]" {
		t.Errorf("blockedBy = %s, want [2] — the done blocker should drop out", got)
	}

	// The done task still reports what it blocks.
	_, blocking, err := showNeighbours(1)
	if err != nil {
		t.Fatalf("showNeighbours: %v", err)
	}
	if got := refIDs(blocking); got != "[3]" {
		t.Errorf("blocking = %s, want [3]", got)
	}
}

// foreign_keys is off on this connection, so an edge can point at a row that is
// not there. The old JOIN silently skipped it; so must this.
func TestShowNeighboursSkipsEdgesWithNoTask(t *testing.T) {
	db := setupDependencyTestStore(t)
	seedShowTasks(t, 2)
	addShowEdges(t, [2]int64{1, 2})
	// Insert edges straight into the table so they bypass the ensureTasksExist
	// check `tasc dep` applies.
	if _, err := db.Exec(
		"INSERT INTO task_dependencies (blocker_id, blocked_id) VALUES (?, ?), (?, ?)",
		999, 2, 2, 998,
	); err != nil {
		t.Fatalf("insert dangling edges: %v", err)
	}

	blockedBy, blocking, err := showNeighbours(2)
	if err != nil {
		t.Fatalf("showNeighbours: %v", err)
	}
	if got := refIDs(blockedBy); got != "[1]" {
		t.Errorf("blockedBy = %s, want [1] — the edge from 999 has no task", got)
	}
	if got := refIDs(blocking); got != "[]" {
		t.Errorf("blocking = %s, want [] — the edge to 998 has no task", got)
	}
}

// Neither JOIN had an ORDER BY, so the lists came out in row order. They are
// sorted now; the same input must give the same answer every time.
func TestShowNeighboursIsDeterministic(t *testing.T) {
	setupDependencyTestStore(t)
	seedShowTasks(t, 6)
	// Added out of order on purpose.
	addShowEdges(t, [2]int64{5, 1}, [2]int64{2, 1}, [2]int64{4, 1}, [2]int64{1, 6}, [2]int64{1, 3})

	for i := 0; i < 20; i++ {
		blockedBy, blocking, err := showNeighbours(1)
		if err != nil {
			t.Fatalf("showNeighbours: %v", err)
		}
		if got := refIDs(blockedBy); got != "[2 4 5]" {
			t.Fatalf("blockedBy = %s on run %d, want [2 4 5]", got, i)
		}
		if got := refIDs(blocking); got != "[3 6]" {
			t.Fatalf("blocking = %s on run %d, want [3 6]", got, i)
		}
	}
}

func TestShowNeighboursOnATaskWithNoEdges(t *testing.T) {
	setupDependencyTestStore(t)

	if _, err := CurrentStore.CreateTask(models.Task{Title: "lonely", Status: models.StatusBacklog}); err != nil {
		t.Fatalf("create task: %v", err)
	}

	blockedBy, blocking, err := showNeighbours(1)
	if err != nil {
		t.Fatalf("showNeighbours: %v", err)
	}
	if len(blockedBy) != 0 || len(blocking) != 0 {
		t.Errorf("blockedBy = %v, blocking = %v, want both empty", blockedBy, blocking)
	}
}
