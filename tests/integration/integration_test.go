package integration

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

var binaryPath string

func TestMain(m *testing.M) {
	// 1. Build the binary
	tmpDir, err := os.MkdirTemp("", "tasc-integration-build")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create temp dir: %v\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(tmpDir)

	binaryPath = filepath.Join(tmpDir, "tasc")
	if os.Getenv("GOOS") == "windows" {
		binaryPath += ".exe"
	}

	// Assuming we are in tests/integration, root is ../..
	cmd := exec.Command("go", "build", "-tags", "fts5", "-o", binaryPath, "../../main.go")
	if out, err := cmd.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to build tasc: %v\n%s\n", err, out)
		os.Exit(1)
	}

	// 2. Run tests
	os.Exit(m.Run())
}

func runTasc(t *testing.T, dbPath string, args ...string) (string, error) {
	cmd := exec.Command(binaryPath, args...)
	cmd.Env = append(os.Environ(), "TASC_DB_PATH="+dbPath)
	// Isolate config by setting HOME to the temp dir containing the DB
	// This prevents reading the user's ~/.tasc.yaml
	cmd.Env = append(cmd.Env, "HOME="+filepath.Dir(dbPath))

	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	err := cmd.Run()
	return out.String(), err
}

func TestTaskLifecycle(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	// 1. Add Task
	out, err := runTasc(t, dbPath, "add", "Buy Integration Milk", "--estimate", "30m")
	if err != nil {
		t.Fatalf("Add failed: %v\nOutput: %s", err, out)
	}
	if !strings.Contains(out, "Created task") {
		t.Errorf("Unexpected output from add: %s", out)
	}

	// 2. List Tasks
	out, err = runTasc(t, dbPath, "list")
	if err != nil {
		t.Fatalf("List failed: %v\nOutput: %s", err, out)
	}
	if !strings.Contains(out, "Buy Integration Milk") {
		t.Errorf("List missing task. Output:\n%s", out)
	}
	if !strings.Contains(out, "30m") {
		t.Errorf("List missing estimate. Output:\n%s", out)
	}
	if strings.Contains(out, "Scheduled") {
		t.Errorf("List output still contains Scheduled column. Output:\n%s", out)
	}

	// 3. Mark Done
	// Extract ID? Usually 1 for first task.
	out, err = runTasc(t, dbPath, "done", "1")
	if err != nil {
		t.Fatalf("Done failed: %v\nOutput: %s", err, out)
	}
	if !strings.Contains(out, "marked as done") {
		t.Errorf("Unexpected output from done: %s", out)
	}

	// 4. List Again (Should be empty)
	out, err = runTasc(t, dbPath, "list")
	if err != nil {
		t.Fatalf("List 2 failed: %v\nOutput: %s", err, out)
	}
	if strings.Contains(out, "Buy Integration Milk") {
		t.Errorf("Task still visible in list after done. Output:\n%s", out)
	}
}

func TestProjectWorkflow(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "proj.db")

	// 1. Create Project
	out, err := runTasc(t, dbPath, "project", "create", "Work", "--desc", "My Job")
	if err != nil {
		t.Fatalf("Project create failed: %v\nOutput: %s", err, out)
	}
	if !strings.Contains(out, "Project 'Work' created") {
		t.Errorf("Unexpected output: %s", out)
	}

	// 2. Add Task to Project
	out, err = runTasc(t, dbPath, "add", "Deploy Prod", "-p", "Work")
	if err != nil {
		t.Fatalf("Add task failed: %v\nOutput: %s", err, out)
	}

	// 3. List Project Tasks
	out, err = runTasc(t, dbPath, "list")
	if err != nil {
		t.Fatalf("List failed: %v\nOutput: %s", err, out)
	}
	if !strings.Contains(out, "Work") || !strings.Contains(out, "Deploy Prod") {
		t.Errorf("List missing project context. Output:\n%s", out)
	}

	// 4. Check Project List stats
	out, err = runTasc(t, dbPath, "project", "list")
	if err != nil {
		t.Fatalf("Project list failed: %v\nOutput: %s", err, out)
	}
	// Should see "Work" and "1" task total, "0%" progress
	if !strings.Contains(out, "Work") {
		t.Errorf("Project list missing Work. Output:\n%s", out)
	}
}

func TestListOverdueFilter(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "overdue.db")

	// Create one overdue and one future task.
	out, err := runTasc(t, dbPath, "add", "Past Due Task", "--due", "2000-01-01")
	if err != nil {
		t.Fatalf("Add overdue task failed: %v\nOutput: %s", err, out)
	}
	out, err = runTasc(t, dbPath, "add", "Future Task", "--due", "2999-01-01")
	if err != nil {
		t.Fatalf("Add future task failed: %v\nOutput: %s", err, out)
	}

	out, err = runTasc(t, dbPath, "list", "--overdue")
	if err != nil {
		t.Fatalf("List --overdue failed: %v\nOutput: %s", err, out)
	}
	if !strings.Contains(out, "Past Due Task") {
		t.Errorf("Expected overdue task in output. Output:\n%s", out)
	}
	if strings.Contains(out, "Future Task") {
		t.Errorf("Did not expect future task in overdue output. Output:\n%s", out)
	}
}

func TestModifyAndDeleteAutoApprove(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "auto-approve.db")

	out, err := runTasc(t, dbPath, "add", "Auto Approve Task")
	if err != nil {
		t.Fatalf("Add failed: %v\nOutput: %s", err, out)
	}

	out, err = runTasc(t, dbPath, "modify", "1", "Renamed Task", "--yes")
	if err != nil {
		t.Fatalf("Modify --yes failed: %v\nOutput: %s", err, out)
	}
	if !strings.Contains(out, "Task 1 modified.") {
		t.Errorf("Unexpected modify output: %s", out)
	}

	out, err = runTasc(t, dbPath, "list")
	if err != nil {
		t.Fatalf("List after modify failed: %v\nOutput: %s", err, out)
	}
	if !strings.Contains(out, "Renamed Task") {
		t.Errorf("Task title was not modified. Output:\n%s", out)
	}

	out, err = runTasc(t, dbPath, "delete", "1", "--yes")
	if err != nil {
		t.Fatalf("Delete --yes failed: %v\nOutput: %s", err, out)
	}
	if !strings.Contains(out, "Task 1 deleted.") {
		t.Errorf("Unexpected delete output: %s", out)
	}

	out, err = runTasc(t, dbPath, "list")
	if err != nil {
		t.Fatalf("List after delete failed: %v\nOutput: %s", err, out)
	}
	if strings.Contains(out, "Renamed Task") {
		t.Errorf("Deleted task still appears in list. Output:\n%s", out)
	}
}

func TestRestoreFromBackupRestoresPreviousState(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "restore.db")
	backupPath := filepath.Join(tmpDir, "restore-backup.db")

	out, err := runTasc(t, dbPath, "add", "Task Before Backup")
	if err != nil {
		t.Fatalf("Add before backup failed: %v\nOutput: %s", err, out)
	}

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("Open db for backup failed: %v", err)
	}
	if _, err := db.Exec("VACUUM INTO ?", backupPath); err != nil {
		db.Close()
		t.Fatalf("VACUUM INTO backup failed: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close db after backup failed: %v", err)
	}

	out, err = runTasc(t, dbPath, "add", "Task After Backup")
	if err != nil {
		t.Fatalf("Add after backup failed: %v\nOutput: %s", err, out)
	}

	out, err = runTasc(t, dbPath, "restore", backupPath)
	if err != nil {
		t.Fatalf("Restore failed: %v\nOutput: %s", err, out)
	}
	if !strings.Contains(out, "Successfully restored database") {
		t.Fatalf("Unexpected restore output: %s", out)
	}

	reopened, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("Reopen db failed: %v", err)
	}
	defer reopened.Close()

	rows, err := reopened.Query("SELECT title FROM tasks ORDER BY id")
	if err != nil {
		t.Fatalf("Query restored tasks failed: %v", err)
	}
	defer rows.Close()

	var titles []string
	for rows.Next() {
		var title string
		if err := rows.Scan(&title); err != nil {
			t.Fatalf("Scan restored title failed: %v", err)
		}
		titles = append(titles, title)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("Iterating restored titles failed: %v", err)
	}

	if len(titles) != 1 || titles[0] != "Task Before Backup" {
		t.Fatalf("Unexpected restored tasks: %#v", titles)
	}
}

func TestDateOnlyDueUsesEightPmDefault(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "due-default.db")

	out, err := runTasc(t, dbPath, "add", "Date-only due task", "--due", "2030-01-15")
	if err != nil {
		t.Fatalf("Add with date-only due failed: %v\nOutput: %s", err, out)
	}

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("Open db failed: %v", err)
	}
	defer db.Close()

	var dueAt time.Time
	if err := db.QueryRow(`SELECT due_at FROM tasks WHERE title = ?`, "Date-only due task").Scan(&dueAt); err != nil {
		t.Fatalf("Query due_at failed: %v", err)
	}

	if dueAt.Hour() != 20 || dueAt.Minute() != 0 {
		t.Fatalf("date-only due stored as %02d:%02d, want 20:00", dueAt.Hour(), dueAt.Minute())
	}
}

func TestListOutputJSON(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "list-json.db")

	out, err := runTasc(t, dbPath, "add", "List JSON Task")
	if err != nil {
		t.Fatalf("Add failed: %v\nOutput: %s", err, out)
	}

	out, err = runTasc(t, dbPath, "list", "--output", "json")
	if err != nil {
		t.Fatalf("List --output json failed: %v\nOutput: %s", err, out)
	}

	var payload []struct {
		Task struct {
			ID    int64  `json:"id"`
			Title string `json:"title"`
		} `json:"task"`
		Score float64 `json:"score"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("List --output json returned invalid JSON: %v\nOutput: %s", err, out)
	}
	if len(payload) != 1 {
		t.Fatalf("Expected 1 task in list JSON output, got %d", len(payload))
	}
	if payload[0].Task.Title != "List JSON Task" {
		t.Fatalf("Unexpected title in JSON output: %q", payload[0].Task.Title)
	}
}

func TestShowOutputJSON(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "show-json.db")

	out, err := runTasc(t, dbPath, "add", "Show JSON Task")
	if err != nil {
		t.Fatalf("Add failed: %v\nOutput: %s", err, out)
	}

	out, err = runTasc(t, dbPath, "show", "1", "--output", "json")
	if err != nil {
		t.Fatalf("Show --output json failed: %v\nOutput: %s", err, out)
	}

	var payload struct {
		Task struct {
			ID    int64  `json:"id"`
			Title string `json:"title"`
		} `json:"task"`
		BlockedBy []struct {
			ID    int64  `json:"id"`
			Title string `json:"title"`
		} `json:"blocked_by"`
		Blocking []struct {
			ID    int64  `json:"id"`
			Title string `json:"title"`
		} `json:"blocking"`
	}
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("Show --output json returned invalid JSON: %v\nOutput: %s", err, out)
	}
	if payload.Task.ID != 1 {
		t.Fatalf("Expected task ID 1, got %d", payload.Task.ID)
	}
	if payload.Task.Title != "Show JSON Task" {
		t.Fatalf("Unexpected title in JSON output: %q", payload.Task.Title)
	}
}

func TestRenumberCommand(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "renumber.db")

	out, err := runTasc(t, dbPath, "add", "First")
	if err != nil {
		t.Fatalf("Add first failed: %v\nOutput: %s", err, out)
	}
	out, err = runTasc(t, dbPath, "add", "Second")
	if err != nil {
		t.Fatalf("Add second failed: %v\nOutput: %s", err, out)
	}
	out, err = runTasc(t, dbPath, "add", "Third")
	if err != nil {
		t.Fatalf("Add third failed: %v\nOutput: %s", err, out)
	}

	out, err = runTasc(t, dbPath, "dep", "1", "3")
	if err != nil {
		t.Fatalf("dep 1 3 failed: %v\nOutput: %s", err, out)
	}

	out, err = runTasc(t, dbPath, "renumber", "--yes")
	if err != nil {
		t.Fatalf("renumber --yes failed: %v\nOutput: %s", err, out)
	}
	if !strings.Contains(out, "Renumbered 3 open tasks") {
		t.Fatalf("Unexpected renumber output: %s", out)
	}

	out, err = runTasc(t, dbPath, "show", "0", "--output", "json")
	if err != nil {
		t.Fatalf("show 0 failed after renumber: %v\nOutput: %s", err, out)
	}
	var firstPayload struct {
		Task struct {
			ID    int64  `json:"id"`
			Title string `json:"title"`
		} `json:"task"`
	}
	if err := json.Unmarshal([]byte(out), &firstPayload); err != nil {
		t.Fatalf("show 0 returned invalid JSON: %v\nOutput: %s", err, out)
	}
	if firstPayload.Task.ID != 0 || firstPayload.Task.Title != "First" {
		t.Fatalf("Unexpected task 0 payload: %+v", firstPayload.Task)
	}

	out, err = runTasc(t, dbPath, "show", "2", "--output", "json")
	if err != nil {
		t.Fatalf("show 2 failed after renumber: %v\nOutput: %s", err, out)
	}
	var thirdPayload struct {
		Task struct {
			ID    int64  `json:"id"`
			Title string `json:"title"`
		} `json:"task"`
		BlockedBy []struct {
			ID int64 `json:"id"`
		} `json:"blocked_by"`
	}
	if err := json.Unmarshal([]byte(out), &thirdPayload); err != nil {
		t.Fatalf("show 2 returned invalid JSON: %v\nOutput: %s", err, out)
	}
	if thirdPayload.Task.ID != 2 || thirdPayload.Task.Title != "Third" {
		t.Fatalf("Unexpected task 2 payload: %+v", thirdPayload.Task)
	}
	if len(thirdPayload.BlockedBy) != 1 || thirdPayload.BlockedBy[0].ID != 0 {
		t.Fatalf("Dependency was not remapped to 0 -> 2: %+v", thirdPayload.BlockedBy)
	}

	out, err = runTasc(t, dbPath, "add", "After Renumber")
	if err != nil {
		t.Fatalf("Add after renumber failed: %v\nOutput: %s", err, out)
	}
	if !strings.Contains(out, "Created task 3.") {
		t.Fatalf("Expected next task ID to be 3, output: %s", out)
	}
}

func TestRenumberCommandSkipsDoneAndDeleted(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "renumber-open-only.db")

	out, err := runTasc(t, dbPath, "add", "Done Task")
	if err != nil {
		t.Fatalf("Add done task seed failed: %v\nOutput: %s", err, out)
	}
	out, err = runTasc(t, dbPath, "add", "Open One")
	if err != nil {
		t.Fatalf("Add open one failed: %v\nOutput: %s", err, out)
	}
	out, err = runTasc(t, dbPath, "add", "Open Two")
	if err != nil {
		t.Fatalf("Add open two failed: %v\nOutput: %s", err, out)
	}

	out, err = runTasc(t, dbPath, "done", "1")
	if err != nil {
		t.Fatalf("Done 1 failed: %v\nOutput: %s", err, out)
	}

	out, err = runTasc(t, dbPath, "renumber", "--yes")
	if err != nil {
		t.Fatalf("renumber --yes failed: %v\nOutput: %s", err, out)
	}
	if !strings.Contains(out, "Renumbered 2 open tasks") {
		t.Fatalf("Unexpected renumber output: %s", out)
	}

	out, err = runTasc(t, dbPath, "show", "0", "--output", "json")
	if err != nil {
		t.Fatalf("show 0 failed after renumber: %v\nOutput: %s", err, out)
	}
	var openZeroPayload struct {
		Task struct {
			ID    int64  `json:"id"`
			Title string `json:"title"`
		} `json:"task"`
	}
	if err := json.Unmarshal([]byte(out), &openZeroPayload); err != nil {
		t.Fatalf("show 0 returned invalid JSON: %v\nOutput: %s", err, out)
	}
	if openZeroPayload.Task.Title != "Open One" {
		t.Fatalf("Task 0 should be Open One, got %q", openZeroPayload.Task.Title)
	}

	out, err = runTasc(t, dbPath, "show", "1", "--output", "json")
	if err != nil {
		t.Fatalf("show 1 failed after renumber: %v\nOutput: %s", err, out)
	}
	var openOnePayload struct {
		Task struct {
			ID    int64  `json:"id"`
			Title string `json:"title"`
		} `json:"task"`
	}
	if err := json.Unmarshal([]byte(out), &openOnePayload); err != nil {
		t.Fatalf("show 1 returned invalid JSON: %v\nOutput: %s", err, out)
	}
	if openOnePayload.Task.Title != "Open Two" {
		t.Fatalf("Task 1 should be Open Two, got %q", openOnePayload.Task.Title)
	}

	out, err = runTasc(t, dbPath, "list", "--status", "done", "--output", "json")
	if err != nil {
		t.Fatalf("list done json failed: %v\nOutput: %s", err, out)
	}
	var donePayload []struct {
		Task struct {
			ID    int64  `json:"id"`
			Title string `json:"title"`
		} `json:"task"`
	}
	if err := json.Unmarshal([]byte(out), &donePayload); err != nil {
		t.Fatalf("done list JSON invalid: %v\nOutput: %s", err, out)
	}
	if len(donePayload) != 1 {
		t.Fatalf("Expected 1 done task, got %d", len(donePayload))
	}
	if donePayload[0].Task.Title != "Done Task" {
		t.Fatalf("Expected done task title Done Task, got %q", donePayload[0].Task.Title)
	}
	if donePayload[0].Task.ID >= 0 {
		t.Fatalf("Expected done task to have negative ID, got %d", donePayload[0].Task.ID)
	}

	out, err = runTasc(t, dbPath, "add", "Open Three")
	if err != nil {
		t.Fatalf("Add after renumber failed: %v\nOutput: %s", err, out)
	}
	if !strings.Contains(out, "Created task 2.") {
		t.Fatalf("Expected next open task ID to be 2, output: %s", out)
	}
}

func TestUndoNoActions(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "undo-empty.db")

	out, err := runTasc(t, dbPath, "undo", "--yes")
	if err != nil {
		t.Fatalf("undo --yes on empty history failed: %v\nOutput: %s", err, out)
	}
	if !strings.Contains(out, "Nothing to undo.") {
		t.Fatalf("Unexpected output for empty undo history: %s", out)
	}
}

func TestUndoAddRemovesTask(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "undo-add.db")

	out, err := runTasc(t, dbPath, "add", "Undo Add Task")
	if err != nil {
		t.Fatalf("add failed: %v\nOutput: %s", err, out)
	}

	out, err = runTasc(t, dbPath, "undo", "--yes")
	if err != nil {
		t.Fatalf("undo --yes failed: %v\nOutput: %s", err, out)
	}
	if !strings.Contains(out, "Undid action") {
		t.Fatalf("Unexpected undo output: %s", out)
	}

	out, err = runTasc(t, dbPath, "list")
	if err != nil {
		t.Fatalf("list failed: %v\nOutput: %s", err, out)
	}
	if strings.Contains(out, "Undo Add Task") {
		t.Fatalf("Task still present after undo add. Output:\n%s", out)
	}
}

func TestUndoDoneRestoresTask(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "undo-done.db")

	out, err := runTasc(t, dbPath, "add", "Undo Done Task")
	if err != nil {
		t.Fatalf("add failed: %v\nOutput: %s", err, out)
	}

	out, err = runTasc(t, dbPath, "done", "1")
	if err != nil {
		t.Fatalf("done failed: %v\nOutput: %s", err, out)
	}

	out, err = runTasc(t, dbPath, "undo", "--yes")
	if err != nil {
		t.Fatalf("undo --yes failed: %v\nOutput: %s", err, out)
	}
	if !strings.Contains(out, "Undid action") {
		t.Fatalf("Unexpected undo output: %s", out)
	}

	out, err = runTasc(t, dbPath, "list")
	if err != nil {
		t.Fatalf("list failed: %v\nOutput: %s", err, out)
	}
	if !strings.Contains(out, "Undo Done Task") {
		t.Fatalf("Task not restored to open list after undo done. Output:\n%s", out)
	}

	out, err = runTasc(t, dbPath, "list", "--status", "done")
	if err != nil {
		t.Fatalf("list --status done failed: %v\nOutput: %s", err, out)
	}
	if strings.Contains(out, "Undo Done Task") {
		t.Fatalf("Task still appears as done after undo. Output:\n%s", out)
	}
}
