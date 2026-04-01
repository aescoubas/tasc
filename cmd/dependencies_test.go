package cmd

import (
	"database/sql"
	"strconv"
	"testing"

	dbpkg "github.com/aescoubas/tasc/internal/db"
	"github.com/aescoubas/tasc/internal/models"
	"github.com/aescoubas/tasc/internal/store"
	"github.com/aescoubas/tasc/internal/store/sqlite"
	_ "github.com/mattn/go-sqlite3"
)

func setupDependencyTestStore(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite memory db: %v", err)
	}
	if err := dbpkg.RunMigrations(db); err != nil {
		t.Fatalf("run migrations: %v", err)
	}

	CurrentStore = sqlite.NewSQLiteStore(db)
	t.Cleanup(func() {
		CurrentStore = nil
		db.Close()
	})

	return db
}

func resetDependencyCommandState() {
	project = ""
	description = ""
	due = ""
	estimate = ""
	recurrenceFlag = ""
	block = ""
	dependsOn = nil
	blocks = nil
	modifyAutoApprove = false
}

func TestParseDependencyIDs(t *testing.T) {
	got, err := parseDependencyIDs([]string{"1, 2", "2", "3", " 4 "})
	if err != nil {
		t.Fatalf("parseDependencyIDs returned error: %v", err)
	}

	want := []int64{1, 2, 3, 4}
	if len(got) != len(want) {
		t.Fatalf("parseDependencyIDs returned %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("parseDependencyIDs returned %v, want %v", got, want)
		}
	}
}

func TestParseDependencyIDsRejectsInvalidInput(t *testing.T) {
	if _, err := parseDependencyIDs([]string{"1", "nope"}); err == nil {
		t.Fatal("parseDependencyIDs should reject non-numeric values")
	}
}

func TestDependencySelectionValidateRejectsSelfAndTwoWayLinks(t *testing.T) {
	selection := dependencySelection{
		dependsOn: []int64{2, 3},
		blocks:    []int64{3, 4},
	}

	if err := selection.validate([]int64{1}); err == nil {
		t.Fatal("validate should reject selecting the same task as both blocker and blocked")
	}

	selection = dependencySelection{dependsOn: []int64{1}}
	if err := selection.validate([]int64{1}); err == nil {
		t.Fatal("validate should reject self-dependencies")
	}
}

func TestAddDependencyLinksAddsMissingAndSkipsExisting(t *testing.T) {
	setupDependencyTestStore(t)

	blockerA, err := CurrentStore.CreateTask(models.Task{Title: "Blocker A"})
	if err != nil {
		t.Fatalf("create blocker A: %v", err)
	}
	blockerB, err := CurrentStore.CreateTask(models.Task{Title: "Blocker B"})
	if err != nil {
		t.Fatalf("create blocker B: %v", err)
	}
	target, err := CurrentStore.CreateTask(models.Task{Title: "Target"})
	if err != nil {
		t.Fatalf("create target: %v", err)
	}
	blocked, err := CurrentStore.CreateTask(models.Task{Title: "Blocked"})
	if err != nil {
		t.Fatalf("create blocked: %v", err)
	}

	if err := CurrentStore.AddDependency(blockerA, target); err != nil {
		t.Fatalf("seed dependency: %v", err)
	}

	selection := dependencySelection{
		dependsOn: []int64{blockerA, blockerB},
		blocks:    []int64{blocked},
	}

	added, err := addDependencyLinks([]int64{target}, selection)
	if err != nil {
		t.Fatalf("addDependencyLinks returned error: %v", err)
	}
	if added != 2 {
		t.Fatalf("addDependencyLinks added %d links, want 2", added)
	}

	deps, err := CurrentStore.GetDependencies()
	if err != nil {
		t.Fatalf("GetDependencies failed: %v", err)
	}

	want := map[[2]int64]bool{
		{blockerA, target}: true,
		{blockerB, target}: true,
		{target, blocked}:  true,
	}
	if len(deps) != len(want) {
		t.Fatalf("GetDependencies returned %d rows, want %d", len(deps), len(want))
	}
	for _, dep := range deps {
		if !want[[2]int64{dep.BlockerID, dep.BlockedID}] {
			t.Fatalf("unexpected dependency %+v", dep)
		}
	}
}

func TestAddCommandCreatesDependencies(t *testing.T) {
	setupDependencyTestStore(t)
	resetDependencyCommandState()
	t.Cleanup(resetDependencyCommandState)

	blocker, err := CurrentStore.CreateTask(models.Task{Title: "Blocker"})
	if err != nil {
		t.Fatalf("create blocker: %v", err)
	}
	blocked, err := CurrentStore.CreateTask(models.Task{Title: "Blocked"})
	if err != nil {
		t.Fatalf("create blocked: %v", err)
	}

	dependsOn = []string{strconv.FormatInt(blocker, 10)}
	blocks = []string{strconv.FormatInt(blocked, 10)}

	addCmd.Run(addCmd, []string{"New task"})

	tasks, err := CurrentStore.ListTasks(store.TaskFilter{})
	if err != nil {
		t.Fatalf("ListTasks failed: %v", err)
	}

	var newTaskID int64
	for _, task := range tasks {
		if task.Title == "New task" {
			newTaskID = task.ID
			break
		}
	}
	if newTaskID == 0 {
		t.Fatal("new task was not created")
	}

	deps, err := CurrentStore.GetDependencies()
	if err != nil {
		t.Fatalf("GetDependencies failed: %v", err)
	}

	want := map[[2]int64]bool{
		{blocker, newTaskID}: true,
		{newTaskID, blocked}: true,
	}
	if len(deps) != len(want) {
		t.Fatalf("GetDependencies returned %d rows, want %d", len(deps), len(want))
	}
	for _, dep := range deps {
		if !want[[2]int64{dep.BlockerID, dep.BlockedID}] {
			t.Fatalf("unexpected dependency %+v", dep)
		}
	}
}

func TestModifyCommandAllowsDependencyOnlyChanges(t *testing.T) {
	setupDependencyTestStore(t)
	resetDependencyCommandState()
	t.Cleanup(resetDependencyCommandState)

	blocker, err := CurrentStore.CreateTask(models.Task{Title: "Blocker"})
	if err != nil {
		t.Fatalf("create blocker: %v", err)
	}
	target, err := CurrentStore.CreateTask(models.Task{Title: "Target"})
	if err != nil {
		t.Fatalf("create target: %v", err)
	}

	dependsOn = []string{strconv.FormatInt(blocker, 10)}
	modifyAutoApprove = true

	modifyCmd.Run(modifyCmd, []string{strconv.FormatInt(target, 10)})

	deps, err := CurrentStore.GetDependencies()
	if err != nil {
		t.Fatalf("GetDependencies failed: %v", err)
	}

	if len(deps) != 1 {
		t.Fatalf("GetDependencies returned %d rows, want 1", len(deps))
	}
	if deps[0].BlockerID != blocker || deps[0].BlockedID != target {
		t.Fatalf("unexpected dependency %+v", deps[0])
	}
}
