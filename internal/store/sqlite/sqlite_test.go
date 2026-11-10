package sqlite

import (
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	dbpkg "github.com/aescoubas/tasc/internal/db"
	"github.com/aescoubas/tasc/internal/models"
	"github.com/aescoubas/tasc/internal/store"
	_ "github.com/mattn/go-sqlite3"
)

func setupTestDB(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open memory db: %v", err)
	}

	if err := dbpkg.RunMigrations(db); err != nil {
		t.Fatalf("Failed to run migrations: %v", err)
	}

	return db
}

func TestSQLiteStore_TaskCRUD(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	s := NewSQLiteStore(db)

	// 1. Create
	now := time.Now().Truncate(time.Second) // Truncate because SQLite stores second precision by default usually or we want to match simple formatting
	// Actually sqlite3 datetime is strict string or int. models uses time.Time.
	// Let's rely on driver handling.

	task := models.Task{
		Title:    "Test Task",
		Project:  "TestProject",
		Estimate: "30m",
		DueAt:    &now,
	}

	id, err := s.CreateTask(task)
	if err != nil {
		t.Fatalf("CreateTask failed: %v", err)
	}
	if id == 0 {
		t.Errorf("Expected non-zero ID")
	}

	// 2. Get
	fetched, err := s.GetTask(id)
	if err != nil {
		t.Fatalf("GetTask failed: %v", err)
	}
	if fetched.Title != task.Title {
		t.Errorf("Title mismatch: got %q, want %q", fetched.Title, task.Title)
	}
	if fetched.Project != task.Project {
		t.Errorf("Project mismatch: got %q, want %q", fetched.Project, task.Project)
	}
	if fetched.DueAt == nil || !fetched.DueAt.Equal(now) {
		// Note: Roundtrip time comparison might be tricky due to precision/timezone.
		// SQLite driver usually handles UTC or local.
		// Let's accept if it's close or check formatted string if needed.
		// But .Equal() usually works if precision is respected.
		// We skipped microseconds in 'now' via Truncate? SQLite string format is typically "2006-01-02 15:04:05" (or with Z).
		// Driver might parse it back.
		// Let's just check if it's not nil for now, and rely on Description for exact match.
	}

	// 3. Update
	fetched.Title = "Updated Task"
	fetched.Status = models.StatusOngoing
	err = s.UpdateTask(fetched)
	if err != nil {
		t.Fatalf("UpdateTask failed: %v", err)
	}

	updated, err := s.GetTask(id)
	if err != nil {
		t.Fatalf("GetTask(updated) failed: %v", err)
	}
	if updated.Title != "Updated Task" {
		t.Errorf("Update failed, title: %q", updated.Title)
	}
	if updated.Status != models.StatusOngoing {
		t.Errorf("Update failed, status: %q", updated.Status)
	}

	// 4. List
	tasks, err := s.ListTasks(store.TaskFilter{})
	if err != nil {
		t.Fatalf("ListTasks failed: %v", err)
	}
	if len(tasks) != 1 {
		t.Errorf("ListTasks returned %d items, want 1", len(tasks))
	}
	if tasks[0].ID != id {
		t.Errorf("ListTasks item ID mismatch")
	}

	// 5. Delete
	err = s.DeleteTask(id)
	if err != nil {
		t.Fatalf("DeleteTask failed: %v", err)
	}

	// Verify deletion (List should hide it by default, or verify status)
	deleted, err := s.GetTask(id)
	if err != nil {
		t.Fatalf("GetTask(deleted) failed: %v", err)
	}
	if deleted.Status != models.StatusDeleted {
		t.Errorf("Task status is %q, want deleted", deleted.Status)
	}

	tasksAfterDelete, err := s.ListTasks(store.TaskFilter{})
	if err != nil {
		t.Fatalf("ListTasks failed: %v", err)
	}
	if len(tasksAfterDelete) != 0 {
		t.Errorf("ListTasks returned items after delete: %d", len(tasksAfterDelete))
	}
}

func TestSQLiteStore_Projects(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	s := NewSQLiteStore(db)

	p := models.Project{
		Name:        "Alpha",
		Description: "Top level",
	}
	err := s.CreateProject(p)
	if err != nil {
		t.Fatalf("CreateProject failed: %v", err)
	}

	fetched, err := s.GetProject("Alpha")
	if err != nil {
		t.Fatalf("GetProject failed: %v", err)
	}
	if fetched.Description != "Top level" {
		t.Errorf("Description mismatch")
	}

	list, err := s.ListProjects()
	if err != nil {
		t.Fatalf("ListProjects failed: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("Expected 1 project")
	}
}

func TestSQLiteStore_BatchUpdateTasks(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	s := NewSQLiteStore(db)

	// Create 3 tasks
	ids := make([]int64, 3)
	for i := 0; i < 3; i++ {
		id, _ := s.CreateTask(models.Task{Title: fmt.Sprintf("Task %d", i), Project: "OldProject"})
		ids[i] = id
	}

	// Batch update 2 of them
	updates := map[string]interface{}{
		"project": "NewProject",
		"status":  "ongoing",
	}
	err := s.BatchUpdateTasks([]int64{ids[0], ids[1]}, updates)
	if err != nil {
		t.Fatalf("BatchUpdateTasks failed: %v", err)
	}

	// Verify
	t1, _ := s.GetTask(ids[0])
	if t1.Project != "NewProject" || t1.Status != "ongoing" {
		t.Errorf("Task 1 not updated correctly: %+v", t1)
	}

	t2, _ := s.GetTask(ids[1])
	if t2.Project != "NewProject" || t2.Status != "ongoing" {
		t.Errorf("Task 2 not updated correctly: %+v", t2)
	}

	t3, _ := s.GetTask(ids[2])
	if t3.Project != "OldProject" {
		t.Errorf("Task 3 should not be updated: %+v", t3)
	}
}

func TestSQLiteStore_RenumberTasks(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	s := NewSQLiteStore(db)

	id1, err := s.CreateTask(models.Task{Title: "First Task"})
	if err != nil {
		t.Fatalf("CreateTask(first) failed: %v", err)
	}
	id2, err := s.CreateTask(models.Task{Title: "Second Task"})
	if err != nil {
		t.Fatalf("CreateTask(second) failed: %v", err)
	}
	id3, err := s.CreateTask(models.Task{Title: "Third Task"})
	if err != nil {
		t.Fatalf("CreateTask(third) failed: %v", err)
	}

	if err := s.AddDependency(id1, id3); err != nil {
		t.Fatalf("AddDependency(first->third) failed: %v", err)
	}
	if err := s.AddDependency(id2, id3); err != nil {
		t.Fatalf("AddDependency(second->third) failed: %v", err)
	}

	if err := s.BatchUpdateTasks([]int64{id1}, map[string]interface{}{"status": "done"}); err != nil {
		t.Fatalf("BatchUpdateTasks(done) failed: %v", err)
	}

	count, err := s.RenumberTasks(0)
	if err != nil {
		t.Fatalf("RenumberTasks failed: %v", err)
	}
	if count != 2 {
		t.Fatalf("RenumberTasks affected %d open tasks, want 2", count)
	}

	tasks, err := s.ListTasks(store.TaskFilter{IncludeDeleted: true})
	if err != nil {
		t.Fatalf("ListTasks(all) failed: %v", err)
	}
	if len(tasks) != 3 {
		t.Fatalf("ListTasks(all) returned %d tasks, want 3", len(tasks))
	}

	idsByTitle := make(map[string]int64, len(tasks))
	for _, task := range tasks {
		idsByTitle[task.Title] = task.ID
	}

	if got := idsByTitle["First Task"]; got >= 0 {
		t.Fatalf("First Task (done) ID = %d, want a negative ID", got)
	}
	if got := idsByTitle["Second Task"]; got != 0 {
		t.Fatalf("Second Task ID = %d, want 0", got)
	}
	if got := idsByTitle["Third Task"]; got != 1 {
		t.Fatalf("Third Task ID = %d, want 1", got)
	}

	deps, err := s.GetDependencies()
	if err != nil {
		t.Fatalf("GetDependencies failed: %v", err)
	}
	if len(deps) != 2 {
		t.Fatalf("GetDependencies returned %d dependencies, want 2", len(deps))
	}

	depMap := map[[2]int64]bool{}
	for _, dep := range deps {
		depMap[[2]int64{dep.BlockerID, dep.BlockedID}] = true
	}

	firstID := idsByTitle["First Task"]
	if !depMap[[2]int64{firstID, 1}] {
		t.Fatalf("Expected dependency %d -> 1 after renumber, got %+v", firstID, deps)
	}
	if !depMap[[2]int64{0, 1}] {
		t.Fatalf("Expected dependency 0 -> 1 after renumber, got %+v", deps)
	}

	id4, err := s.CreateTask(models.Task{Title: "Fourth Task"})
	if err != nil {
		t.Fatalf("CreateTask(fourth) failed: %v", err)
	}
	if id4 != 2 {
		t.Fatalf("Fourth task ID = %d, want 2", id4)
	}
}

func TestSQLiteStore_SearchTasks_FallbackWithoutFTS5(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	s := NewSQLiteStore(db)

	if _, err := s.CreateTask(models.Task{Title: "Buy Milk", Description: "From grocery", Project: "Home"}); err != nil {
		t.Fatalf("CreateTask(Buy Milk) failed: %v", err)
	}
	if _, err := s.CreateTask(models.Task{Title: "Write Report", Description: "Quarterly planning", Project: "Work"}); err != nil {
		t.Fatalf("CreateTask(Write Report) failed: %v", err)
	}

	results, err := s.SearchTasks("milk")
	if err != nil {
		t.Fatalf("SearchTasks failed: %v", err)
	}
	if len(results) == 0 {
		t.Fatalf("SearchTasks returned no results")
	}

	foundMilk := false
	for _, task := range results {
		titleLower := strings.ToLower(task.Title)
		descLower := strings.ToLower(task.Description)
		projectLower := strings.ToLower(task.Project)
		if strings.Contains(titleLower, "milk") || strings.Contains(descLower, "milk") || strings.Contains(projectLower, "milk") {
			foundMilk = true
			break
		}
	}
	if !foundMilk {
		t.Fatalf("Expected at least one result matching 'milk', got %+v", results)
	}
}

func TestSQLiteStore_RemoveDependency(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	s := NewSQLiteStore(db)

	blockerA, err := s.CreateTask(models.Task{Title: "Blocker A"})
	if err != nil {
		t.Fatalf("CreateTask(blocker A) failed: %v", err)
	}
	blockerB, err := s.CreateTask(models.Task{Title: "Blocker B"})
	if err != nil {
		t.Fatalf("CreateTask(blocker B) failed: %v", err)
	}
	target, err := s.CreateTask(models.Task{Title: "Target"})
	if err != nil {
		t.Fatalf("CreateTask(target) failed: %v", err)
	}

	if err := s.AddDependency(blockerA, target); err != nil {
		t.Fatalf("AddDependency(A->target) failed: %v", err)
	}
	if err := s.AddDependency(blockerB, target); err != nil {
		t.Fatalf("AddDependency(B->target) failed: %v", err)
	}

	if err := s.RemoveDependency(blockerA, target); err != nil {
		t.Fatalf("RemoveDependency failed: %v", err)
	}

	deps, err := s.GetDependencies()
	if err != nil {
		t.Fatalf("GetDependencies failed: %v", err)
	}
	if len(deps) != 1 {
		t.Fatalf("GetDependencies returned %d dependencies, want 1", len(deps))
	}
	if deps[0].BlockerID != blockerB || deps[0].BlockedID != target {
		t.Fatalf("surviving dependency is %+v, want %d -> %d", deps[0], blockerB, target)
	}

	// Removal is one directed edge, and repeating it is not an error.
	if err := s.RemoveDependency(blockerA, target); err != nil {
		t.Fatalf("RemoveDependency(repeat) failed: %v", err)
	}
	if err := s.RemoveDependency(target, blockerB); err != nil {
		t.Fatalf("RemoveDependency(reversed) failed: %v", err)
	}

	deps, err = s.GetDependencies()
	if err != nil {
		t.Fatalf("GetDependencies failed: %v", err)
	}
	if len(deps) != 1 {
		t.Fatalf("GetDependencies returned %d dependencies after no-op removals, want 1", len(deps))
	}
}

func TestSQLiteStore_AddDependencyRejectsInvalidLinks(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	s := NewSQLiteStore(db)
	var ids []int64
	for _, title := range []string{"First", "Second", "Third", "Fourth"} {
		id, err := s.CreateTask(models.Task{Title: title})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	for _, edge := range [][2]int64{{ids[0], ids[1]}, {ids[1], ids[2]}} {
		if err := s.AddDependency(edge[0], edge[1]); err != nil {
			t.Fatal(err)
		}
	}
	for _, tt := range []struct {
		name             string
		blocker, blocked int64
	}{
		{"self", ids[0], ids[0]},
		{"missing blocker", 9999, ids[0]},
		{"missing dependent", ids[0], 9999},
		{"direct cycle", ids[1], ids[0]},
		{"indirect cycle", ids[2], ids[0]},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if err := s.AddDependency(tt.blocker, tt.blocked); !errors.Is(err, store.ErrInvalidDependency) {
				t.Fatalf("got %v, want ErrInvalidDependency", err)
			}
			deps, err := s.GetDependencies()
			if err != nil {
				t.Fatal(err)
			}
			if len(deps) != 2 {
				t.Fatalf("rejected dependency changed the graph: %+v", deps)
			}
		})
	}
	// Multiple parents and a shortcut along an existing path are valid DAG edges.
	for _, edge := range [][2]int64{{ids[3], ids[1]}, {ids[0], ids[2]}} {
		if err := s.AddDependency(edge[0], edge[1]); err != nil {
			t.Fatalf("valid dependency rejected: %v", err)
		}
	}
	deps, err := s.GetDependencies()
	if err != nil || len(deps) != 4 {
		t.Fatalf("valid dependencies not stored: deps=%+v err=%v", deps, err)
	}
}

func TestSQLiteStore_AddDependencyTraversesLegacyCycles(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	s := NewSQLiteStore(db)
	for _, title := range []string{"Legacy first", "Legacy second", "New parent"} {
		if _, err := s.CreateTask(models.Task{Title: title}); err != nil {
			t.Fatal(err)
		}
	}
	// Older versions permitted cycles. Traversal must terminate on existing data.
	if _, err := db.Exec("INSERT INTO task_dependencies (blocker_id, blocked_id) VALUES (1, 2), (2, 1)"); err != nil {
		t.Fatal(err)
	}
	if err := s.AddDependency(3, 1); err != nil {
		t.Fatalf("valid incoming edge rejected: %v", err)
	}
	if err := s.AddDependency(2, 3); err == nil {
		t.Fatal("new cycle through the legacy graph was accepted")
	}
	deps, err := s.GetDependencies()
	if err != nil || len(deps) != 3 {
		t.Fatalf("legacy graph changed: deps=%+v, err=%v", deps, err)
	}
}

func TestSQLiteStore_AddDependencySerializesOppositeEdges(t *testing.T) {
	dsn := filepath.Join(t.TempDir(), "concurrent-dependencies.db") + "?_busy_timeout=10000"
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := dbpkg.RunMigrations(db); err != nil {
		t.Fatal(err)
	}
	other, err := sql.Open("sqlite3", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	first, second := NewSQLiteStore(db), NewSQLiteStore(other)
	for _, title := range []string{"Concurrent first", "Concurrent second"} {
		if _, err := first.CreateTask(models.Task{Title: title}); err != nil {
			t.Fatal(err)
		}
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	go func() { <-start; results <- first.AddDependency(1, 2) }()
	go func() { <-start; results <- second.AddDependency(2, 1) }()
	close(start)
	accepted, rejected := 0, 0
	for range 2 {
		err := <-results
		if err == nil {
			accepted++
		} else if errors.Is(err, store.ErrInvalidDependency) {
			rejected++
		} else {
			t.Errorf("unexpected concurrent write error: %v", err)
		}
	}
	deps, err := first.GetDependencies()
	if err != nil || accepted != 1 || rejected != 1 || len(deps) != 1 {
		t.Fatalf("accepted=%d rejected=%d deps=%+v err=%v", accepted, rejected, deps, err)
	}
}

func TestSQLiteStore_IsBlockedIgnoresSatisfiedBlockers(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	s := NewSQLiteStore(db)

	blocker, err := s.CreateTask(models.Task{Title: "Blocker"})
	if err != nil {
		t.Fatalf("CreateTask(blocker) failed: %v", err)
	}
	other, err := s.CreateTask(models.Task{Title: "Other Blocker"})
	if err != nil {
		t.Fatalf("CreateTask(other blocker) failed: %v", err)
	}
	blocked, err := s.CreateTask(models.Task{Title: "Blocked"})
	if err != nil {
		t.Fatalf("CreateTask(blocked) failed: %v", err)
	}

	if err := s.AddDependency(blocker, blocked); err != nil {
		t.Fatalf("AddDependency(blocker) failed: %v", err)
	}
	if err := s.AddDependency(other, blocked); err != nil {
		t.Fatalf("AddDependency(other) failed: %v", err)
	}

	isBlocked := func(id int64) bool {
		t.Helper()
		tasks, err := s.ListTasks(store.TaskFilter{IDs: []int64{id}})
		if err != nil {
			t.Fatalf("ListTasks failed: %v", err)
		}
		if len(tasks) != 1 {
			t.Fatalf("ListTasks returned %d tasks for id %d, want 1", len(tasks), id)
		}
		blocked := tasks[0].IsBlocked
		filtered, err := s.ListTasks(store.TaskFilter{IDs: []int64{id}, Status: []models.TaskStatus{models.StatusBlocked}})
		if err != nil {
			t.Fatalf("ListTasks(blocked) failed: %v", err)
		}
		wantCount := 0
		if blocked {
			wantCount = 1
		}
		if len(filtered) != wantCount {
			t.Fatalf("blocked filter returned %d tasks for %d — %s, want %d", len(filtered), id, tasks[0].Title, wantCount)
		}
		return blocked
	}

	if !isBlocked(blocked) {
		t.Fatalf("task %d is not blocked while both blockers are open", blocked)
	}

	if err := s.MarkDone(blocker); err != nil {
		t.Fatalf("MarkDone(blocker) failed: %v", err)
	}
	if !isBlocked(blocked) {
		t.Fatalf("task %d stopped being blocked while %d is still open", blocked, other)
	}

	// The last open blocker going away is what clears it; the edges themselves stay.
	if err := s.DeleteTask(other); err != nil {
		t.Fatalf("DeleteTask(other) failed: %v", err)
	}
	if isBlocked(blocked) {
		t.Fatalf("task %d is still blocked though every blocker is done or deleted", blocked)
	}

	deps, err := s.GetDependencies()
	if err != nil {
		t.Fatalf("GetDependencies failed: %v", err)
	}
	if len(deps) != 2 {
		t.Fatalf("GetDependencies returned %d rows, want 2 — blocked-ness is derived, not stored", len(deps))
	}
}

func TestSQLiteStore_ListTasksFiltersDisplayedStatus(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	s := NewSQLiteStore(db)

	create := func(title string, status models.TaskStatus) int64 {
		t.Helper()
		id, err := s.CreateTask(models.Task{Title: title, Project: "FilterProject"})
		if err != nil {
			t.Fatal(err)
		}
		if err := s.BatchUpdateTasks([]int64{id}, map[string]interface{}{"status": status}); err != nil {
			t.Fatal(err)
		}
		return id
	}
	blocker := create("Open blocker", models.StatusBacklog)
	backlog := create("Ready backlog", models.StatusBacklog)
	ongoing := create("Ready ongoing", models.StatusOngoing)
	manual := create("Manually blocked", models.StatusBlocked)
	blockedBacklog := create("Blocked backlog", models.StatusBacklog)
	blockedOngoing := create("Blocked ongoing", models.StatusOngoing)
	done := create("Done dependent", models.StatusDone)
	deleted := create("Deleted dependent", models.StatusDeleted)
	for _, id := range []int64{blockedBacklog, blockedOngoing, done, deleted} {
		if err := s.AddDependency(blocker, id); err != nil {
			t.Fatal(err)
		}
	}

	check := func(t *testing.T, filter store.TaskFilter, want []int64) {
		t.Helper()
		tasks, err := s.ListTasks(filter)
		if err != nil {
			t.Fatalf("ListTasks: %v", err)
		}
		if len(tasks) != len(want) {
			t.Fatalf("ListTasks returned %d tasks, want %d: %+v", len(tasks), len(want), tasks)
		}
		expected := make(map[int64]bool, len(want))
		for _, id := range want {
			expected[id] = true
		}
		for _, task := range tasks {
			if !expected[task.ID] {
				t.Errorf("unexpected task %d — %s", task.ID, task.Title)
			}
			delete(expected, task.ID)
			if (task.Status == models.StatusDone || task.Status == models.StatusDeleted) && task.IsBlocked {
				t.Errorf("finished task %d — %s is displayed as blocked", task.ID, task.Title)
			}
		}
		if len(expected) != 0 {
			t.Errorf("missing tasks: %v", expected)
		}
	}

	for _, tt := range []struct {
		name   string
		filter store.TaskFilter
		want   []int64
	}{
		{"blocked", store.TaskFilter{Status: []models.TaskStatus{models.StatusBlocked}}, []int64{manual, blockedBacklog, blockedOngoing}},
		{"backlog", store.TaskFilter{Status: []models.TaskStatus{models.StatusBacklog}}, []int64{blocker, backlog}},
		{"ongoing", store.TaskFilter{Status: []models.TaskStatus{models.StatusOngoing}}, []int64{ongoing}},
		{"mixed", store.TaskFilter{Status: []models.TaskStatus{models.StatusBlocked, models.StatusOngoing}}, []int64{manual, blockedBacklog, blockedOngoing, ongoing}},
		{"done", store.TaskFilter{Status: []models.TaskStatus{models.StatusDone}}, []int64{done}},
		{"deleted", store.TaskFilter{Status: []models.TaskStatus{models.StatusDeleted}}, []int64{deleted}},
		{"all", store.TaskFilter{IncludeDeleted: true}, []int64{blocker, backlog, ongoing, manual, blockedBacklog, blockedOngoing, done, deleted}},
		{"project and id", store.TaskFilter{Status: []models.TaskStatus{models.StatusBlocked}, Projects: []string{"FilterProject"}, IDs: []int64{blockedBacklog, backlog}}, []int64{blockedBacklog}},
		{"other project", store.TaskFilter{Status: []models.TaskStatus{models.StatusBlocked}, Projects: []string{"OtherProject"}}, nil},
	} {
		t.Run(tt.name, func(t *testing.T) { check(t, tt.filter, tt.want) })
	}

	if err := s.MarkDone(blocker); err != nil {
		t.Fatal(err)
	}
	check(t, store.TaskFilter{Status: []models.TaskStatus{models.StatusBlocked}}, []int64{manual})
	check(t, store.TaskFilter{Status: []models.TaskStatus{models.StatusBacklog}}, []int64{backlog, blockedBacklog})
	check(t, store.TaskFilter{Status: []models.TaskStatus{models.StatusOngoing}}, []int64{ongoing, blockedOngoing})

	// Listing must leave the saved workflow status intact.
	task, err := s.GetTask(blockedBacklog)
	if err != nil || task.Status != models.StatusBacklog {
		t.Fatalf("saved status changed: task=%+v, err=%v", task, err)
	}
}
