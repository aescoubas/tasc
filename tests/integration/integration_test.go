package integration

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	dbpkg "github.com/aescoubas/tasc/internal/db"
	"github.com/aescoubas/tasc/internal/models"
	"github.com/aescoubas/tasc/internal/server"
	"github.com/aescoubas/tasc/internal/store"
	"github.com/aescoubas/tasc/internal/store/remote"
	"github.com/aescoubas/tasc/internal/store/sqlite"
	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
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

func TestAddSupportsDayFirstDueDateFormats(t *testing.T) {
	tests := []struct {
		name string
		due  string
	}{
		{name: "dot", due: "20.04.2026"},
		{name: "slash", due: "20/04/2026"},
		{name: "dash", due: "20-04-2026"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			dbPath := filepath.Join(tmpDir, "day-first-due.db")

			out, err := runTasc(t, dbPath, "add", "Day-first due task", "--due", tt.due)
			if err != nil {
				t.Fatalf("Add with due %q failed: %v\nOutput: %s", tt.due, err, out)
			}
			if !strings.Contains(out, "Created task") {
				t.Fatalf("Unexpected add output for due %q: %s", tt.due, out)
			}

			db, err := sql.Open("sqlite3", dbPath)
			if err != nil {
				t.Fatalf("Open db failed: %v", err)
			}
			defer db.Close()

			var dueAt time.Time
			if err := db.QueryRow(`SELECT due_at FROM tasks WHERE title = ?`, "Day-first due task").Scan(&dueAt); err != nil {
				t.Fatalf("Query due_at failed: %v", err)
			}

			if dueAt.Year() != 2026 || dueAt.Month() != time.April || dueAt.Day() != 20 {
				t.Fatalf("due_at = %s, want 2026-04-20", dueAt.Format(time.RFC3339))
			}
			if dueAt.Hour() != 20 || dueAt.Minute() != 0 {
				t.Fatalf("due_at stored as %02d:%02d, want 20:00", dueAt.Hour(), dueAt.Minute())
			}
		})
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

func TestShowDisplaysCreatedAndDueWithTimestampFormat(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "show-format.db")

	out, err := runTasc(t, dbPath, "add", "Show formatted dates", "--due", "15.01.2030")
	if err != nil {
		t.Fatalf("Add failed: %v\nOutput: %s", err, out)
	}

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("Open db failed: %v", err)
	}
	defer db.Close()

	var createdAt time.Time
	if err := db.QueryRow(`SELECT created_at FROM tasks WHERE title = ?`, "Show formatted dates").Scan(&createdAt); err != nil {
		t.Fatalf("Query created_at failed: %v", err)
	}

	out, err = runTasc(t, dbPath, "show", "1")
	if err != nil {
		t.Fatalf("show failed: %v\nOutput: %s", err, out)
	}

	wantCreated := fmt.Sprintf("Created:       %s", createdAt.Format("2006-01-02 15:04"))
	if !strings.Contains(out, wantCreated) {
		t.Fatalf("show output missing created timestamp %q\nOutput:\n%s", wantCreated, out)
	}

	wantDue := "Due:           2030-01-15 20:00"
	if !strings.Contains(out, wantDue) {
		t.Fatalf("show output missing due timestamp %q\nOutput:\n%s", wantDue, out)
	}

	dueLinePattern := regexp.MustCompile(`(?m)^Due:\s+\d{4}-\d{2}-\d{2} \d{2}:\d{2}$`)
	if !dueLinePattern.MatchString(out) {
		t.Fatalf("show output due line is not in YYYY-MM-DD HH:MM format\nOutput:\n%s", out)
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

func TestUndepRemovesDependencyAndIsUndoable(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "undep.db")

	if out, err := runTasc(t, dbPath, "add", "Undep Blocker"); err != nil {
		t.Fatalf("add blocker failed: %v\nOutput: %s", err, out)
	}
	if out, err := runTasc(t, dbPath, "add", "Undep Blocked"); err != nil {
		t.Fatalf("add blocked failed: %v\nOutput: %s", err, out)
	}

	if out, err := runTasc(t, dbPath, "dep", "1", "2"); err != nil {
		t.Fatalf("dep failed: %v\nOutput: %s", err, out)
	}

	out, err := runTasc(t, dbPath, "show", "2")
	if err != nil {
		t.Fatalf("show failed: %v\nOutput: %s", err, out)
	}
	if !strings.Contains(out, "Blocked By") {
		t.Fatalf("show 2 does not report the dependency. Output:\n%s", out)
	}

	// Naming the pair the wrong way round must leave the edge in place.
	out, err = runTasc(t, dbPath, "undep", "2", "1")
	if err != nil {
		t.Fatalf("undep (reversed) failed: %v\nOutput: %s", err, out)
	}
	if !strings.Contains(out, "No dependency from task 2 to task 1") {
		t.Fatalf("undep (reversed) output unexpected: %s", out)
	}
	if countDependencies(t, dbPath) != 1 {
		t.Fatalf("undep (reversed) changed the dependency table")
	}

	out, err = runTasc(t, dbPath, "undep", "1", "2")
	if err != nil {
		t.Fatalf("undep failed: %v\nOutput: %s", err, out)
	}
	if !strings.Contains(out, "no longer blocks") {
		t.Fatalf("undep output unexpected: %s", out)
	}

	if got := countDependencies(t, dbPath); got != 0 {
		t.Fatalf("task_dependencies has %d rows after undep, want 0", got)
	}

	out, err = runTasc(t, dbPath, "show", "2")
	if err != nil {
		t.Fatalf("show after undep failed: %v\nOutput: %s", err, out)
	}
	if strings.Contains(out, "Blocked By") {
		t.Fatalf("show 2 still reports a blocker after undep. Output:\n%s", out)
	}

	// Migration 6 installs an AFTER DELETE trigger on task_dependencies, so the
	// removal is undoable like any other tracked command.
	out, err = runTasc(t, dbPath, "undo", "--yes")
	if err != nil {
		t.Fatalf("undo --yes failed: %v\nOutput: %s", err, out)
	}
	if !strings.Contains(out, "Undid action") {
		t.Fatalf("Unexpected undo output: %s", out)
	}
	if got := countDependencies(t, dbPath); got != 1 {
		t.Fatalf("task_dependencies has %d rows after undo, want 1", got)
	}
}

func countDependencies(t *testing.T, dbPath string) int {
	t.Helper()

	conn, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer conn.Close()

	var count int
	if err := conn.QueryRow("SELECT COUNT(*) FROM task_dependencies").Scan(&count); err != nil {
		t.Fatalf("count dependencies: %v", err)
	}
	return count
}

func TestRemoteListTaskFiltersMatchLocal(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "remote-filters.db")
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := dbpkg.RunMigrations(db); err != nil {
		t.Fatal(err)
	}
	local := sqlite.NewSQLiteStore(db)
	due := time.Date(2030, 1, 10, 0, 0, 0, 0, time.UTC)
	create := func(title, project string, status models.TaskStatus) int64 {
		t.Helper()
		id, err := local.CreateTask(models.Task{Title: title, Project: project, DueAt: &due})
		if err != nil {
			t.Fatal(err)
		}
		if err := local.BatchUpdateTasks([]int64{id}, map[string]interface{}{"status": status}); err != nil {
			t.Fatal(err)
		}
		return id
	}
	blocker := create("Remote blocker", "Blockers", models.StatusBacklog)
	backlog := create("Remote backlog", "Filtered", models.StatusBacklog)
	ongoing := create("Remote ongoing", "Filtered", models.StatusOngoing)
	blocked := create("Remote dependent", "Filtered", models.StatusBacklog)
	manual := create("Remote manually blocked", "Other", models.StatusBlocked)
	done := create("Remote done", "Filtered", models.StatusDone)
	deleted := create("Remote deleted", "Filtered", models.StatusDeleted)
	undefined := create("Remote undefined", "Filtered", models.StatusUndefined)
	if err := local.AddDependency(blocker, blocked); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(server.NewServer(local))
	defer srv.Close()
	remoteClient := remote.NewClient(srv.URL)
	before, after := due.Add(time.Hour), due.Add(-time.Hour)

	check := func(t *testing.T, tasks []models.Task, want []int64) {
		t.Helper()
		if len(tasks) != len(want) {
			t.Fatalf("got %d tasks, want %d: %+v", len(tasks), len(want), tasks)
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
			if task.IsBlocked != (task.ID == blocked) {
				t.Errorf("task %d — %s: is_blocked=%v", task.ID, task.Title, task.IsBlocked)
			}
		}
		if len(expected) != 0 {
			t.Errorf("missing task IDs: %v", expected)
		}
	}

	for _, tt := range []struct {
		name   string
		query  string
		filter store.TaskFilter
		want   []int64
	}{
		{"default", "", store.TaskFilter{}, []int64{blocker, backlog, ongoing, blocked, manual}},
		{"blocked", "?status=blocked", store.TaskFilter{Status: []models.TaskStatus{models.StatusBlocked}}, []int64{blocked, manual}},
		{"backlog", "?status=backlog", store.TaskFilter{Status: []models.TaskStatus{models.StatusBacklog}}, []int64{blocker, backlog}},
		{"ongoing", "?status=ongoing", store.TaskFilter{Status: []models.TaskStatus{models.StatusOngoing}}, []int64{ongoing}},
		{"done", "?status=done", store.TaskFilter{Status: []models.TaskStatus{models.StatusDone}}, []int64{done}},
		{"deleted", "?status=deleted", store.TaskFilter{Status: []models.TaskStatus{models.StatusDeleted}}, []int64{deleted}},
		{"undefined", "?status=undefined", store.TaskFilter{Status: []models.TaskStatus{models.StatusUndefined}}, []int64{undefined}},
		{"mixed and whitespace", "?status=blocked,%20done", store.TaskFilter{Status: []models.TaskStatus{models.StatusBlocked, models.StatusDone}}, []int64{blocked, manual, done}},
		{"include finished", "?include_deleted=true", store.TaskFilter{IncludeDeleted: true}, []int64{blocker, backlog, ongoing, blocked, manual, done, deleted, undefined}},
		{"exclude finished", "?include_deleted=false", store.TaskFilter{}, []int64{blocker, backlog, ongoing, blocked, manual}},
		{"explicit status with include finished", "?status=done&include_deleted=true", store.TaskFilter{Status: []models.TaskStatus{models.StatusDone}, IncludeDeleted: true}, []int64{done}},
		{"unknown status", "?status=unknown", store.TaskFilter{Status: []models.TaskStatus{"unknown"}}, nil},
		{"empty status tokens", "?status=%20,,", store.TaskFilter{}, []int64{blocker, backlog, ongoing, blocked, manual}},
		{"combined filters", fmt.Sprintf("?status=blocked&projects=Filtered&ids=%d,%d&due_after=2030-01-09T23:00:00Z&due_before=2030-01-10T01:00:00Z", blocked, manual), store.TaskFilter{Status: []models.TaskStatus{models.StatusBlocked}, Projects: []string{"Filtered"}, IDs: []int64{blocked, manual}, DueBefore: &before, DueAfter: &after}, []int64{blocked}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			for name, backend := range map[string]store.Store{"local": local, "remote": remoteClient} {
				t.Run(name, func(t *testing.T) {
					tasks, err := backend.ListTasks(tt.filter)
					if err != nil {
						t.Fatal(err)
					}
					check(t, tasks, tt.want)
				})
			}
			t.Run("HTTP query", func(t *testing.T) {
				response, err := srv.Client().Get(srv.URL + "/tasks" + tt.query)
				if err != nil {
					t.Fatal(err)
				}
				defer response.Body.Close()
				if response.StatusCode != http.StatusOK {
					t.Fatalf("GET /tasks returned %d", response.StatusCode)
				}
				var tasks []models.Task
				if err := json.NewDecoder(response.Body).Decode(&tasks); err != nil {
					t.Fatal(err)
				}
				check(t, tasks, tt.want)
			})
			// The CLI has no include_deleted flag; exercise its status filters.
			if tt.filter.IncludeDeleted || tt.filter.DueBefore != nil {
				return
			}
			t.Run("CLI", func(t *testing.T) {
				args := []string{"list", "--output", "json"}
				if len(tt.filter.Status) > 0 {
					var statuses []string
					for _, status := range tt.filter.Status {
						statuses = append(statuses, string(status))
					}
					args = append(args, "--status", strings.Join(statuses, ","))
				}
				for _, prefix := range [][]string{nil, {"--remote", srv.URL}} {
					out, err := runTasc(t, dbPath, append(prefix, args...)...)
					if err != nil {
						t.Fatalf("list %v: %v\n%s", prefix, err, out)
					}
					var rows []struct {
						Task models.Task `json:"task"`
					}
					if err := json.Unmarshal([]byte(out), &rows); err != nil {
						t.Fatalf("decode list: %v\n%s", err, out)
					}
					var tasks []models.Task
					for _, row := range rows {
						tasks = append(tasks, row.Task)
					}
					check(t, tasks, tt.want)
				}
			})
		})
	}
	response, err := srv.Client().Get(srv.URL + "/tasks?include_deleted=maybe")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusBadRequest {
		t.Fatalf("invalid include_deleted returned %d, want 400", response.StatusCode)
	}
}

func TestMCPBlockedStatusFilter(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "mcp-blocked.db")
	for _, args := range [][]string{
		{"add", "MCP Open Blocker", "--project", "Blockers", "--due", "2999-01-01"},
		{"add", "MCP Blocked Backlog", "--project", "Dependents", "--due", "2999-01-01"},
		{"add", "MCP Blocked Ongoing", "--project", "Dependents", "--due", "2999-01-01"},
		{"start", "3"},
		{"dep", "1", "2"},
		{"dep", "1", "3"},
	} {
		if out, err := runTasc(t, dbPath, args...); err != nil {
			t.Fatalf("%v failed: %v\n%s", args, err, out)
		}
	}

	c, err := client.NewStdioMCPClient(binaryPath, []string{
		"TASC_DB_PATH=" + dbPath, "HOME=" + tmpDir, "TASC_REMOTE=",
	}, "mcp")
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if _, err := c.Initialize(ctx, mcp.InitializeRequest{Params: mcp.InitializeParams{
		ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
		ClientInfo:      mcp.Implementation{Name: "tasc-integration", Version: "1"},
	}}); err != nil {
		t.Fatal(err)
	}

	call := func(name string, args map[string]any) string {
		t.Helper()
		result, err := c.CallTool(ctx, mcp.CallToolRequest{Params: mcp.CallToolParams{
			Name: name, Arguments: args,
		}})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if result.IsError || len(result.Content) != 1 {
			t.Fatalf("%s returned unexpected result: %+v", name, result)
		}
		content, ok := result.Content[0].(mcp.TextContent)
		if !ok {
			t.Fatalf("%s returned non-text content: %+v", name, result.Content)
		}
		return content.Text
	}
	checkList := func(args map[string]any, want string) {
		t.Helper()
		got := call("tasc_list_tasks", args)
		if got != want {
			t.Fatalf("list %v:\ngot: %q\nwant: %q", args, got, want)
		}
		t.Logf("list %v: %s", args, got)
	}
	blocker := "- [1] MCP Open Blocker (Project: Blockers, Status: backlog) Due: 2999-01-01\n"
	dependents := "- [2] MCP Blocked Backlog (Project: Dependents, Status: blocked) Due: 2999-01-01\n" +
		"- [3] MCP Blocked Ongoing (Project: Dependents, Status: blocked) Due: 2999-01-01\n"
	checkList(nil, blocker+dependents)
	checkList(map[string]any{"status": "blocked"}, dependents)
	checkList(map[string]any{"status": "backlog"}, blocker)
	checkList(map[string]any{"status": "ongoing"}, "No tasks found.")
	checkList(map[string]any{"status": "backlog, blocked"}, blocker+dependents)
	checkList(map[string]any{"status": "blocked", "project": "Dependents"}, dependents)
	checkList(map[string]any{"status": "blocked", "project": "Blockers"}, "No tasks found.")
	call("tasc_complete_task", map[string]any{"id": 1})
	checkList(map[string]any{"status": "blocked"}, "No tasks found.")
	checkList(map[string]any{"status": "backlog"}, "- [2] MCP Blocked Backlog (Project: Dependents, Status: backlog) Due: 2999-01-01\n")
	checkList(map[string]any{"status": "ongoing"}, "- [3] MCP Blocked Ongoing (Project: Dependents, Status: ongoing) Due: 2999-01-01\n")
}

func TestCompletingABlockerClearsTheDependent(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "satisfied-blocker.db")

	if out, err := runTasc(t, dbPath, "add", "Satisfied Blocker"); err != nil {
		t.Fatalf("add blocker failed: %v\nOutput: %s", err, out)
	}
	if out, err := runTasc(t, dbPath, "add", "Dependent Task"); err != nil {
		t.Fatalf("add dependent failed: %v\nOutput: %s", err, out)
	}
	if out, err := runTasc(t, dbPath, "dep", "1", "2"); err != nil {
		t.Fatalf("dep failed: %v\nOutput: %s", err, out)
	}

	out, err := runTasc(t, dbPath, "show", "2")
	if err != nil {
		t.Fatalf("show failed: %v\nOutput: %s", err, out)
	}
	if !strings.Contains(out, "Blocked By") {
		t.Fatalf("show 2 does not report the open blocker. Output:\n%s", out)
	}

	if out, err := runTasc(t, dbPath, "done", "1"); err != nil {
		t.Fatalf("done failed: %v\nOutput: %s", err, out)
	}

	out, err = runTasc(t, dbPath, "show", "2")
	if err != nil {
		t.Fatalf("show after done failed: %v\nOutput: %s", err, out)
	}
	if strings.Contains(out, "Blocked By") {
		t.Fatalf("show 2 still reports a blocker that is done. Output:\n%s", out)
	}

	out, err = runTasc(t, dbPath, "list", "--output", "json")
	if err != nil {
		t.Fatalf("list --output json failed: %v\nOutput: %s", err, out)
	}

	var rows []struct {
		Task struct {
			ID        int64 `json:"id"`
			IsBlocked bool  `json:"is_blocked"`
		} `json:"task"`
	}
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("decode list json: %v\nOutput: %s", err, out)
	}
	if len(rows) != 1 {
		t.Fatalf("list returned %d open tasks, want 1", len(rows))
	}
	if rows[0].Task.ID != 2 || rows[0].Task.IsBlocked {
		t.Fatalf("list reports is_blocked=%v for task %d, want false", rows[0].Task.IsBlocked, rows[0].Task.ID)
	}

	// The edge is still there — only its effect is gone.
	if got := countDependencies(t, dbPath); got != 1 {
		t.Fatalf("task_dependencies has %d rows, want 1", got)
	}
}

func TestDepRefusesEdgesThatCannotHold(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "dep-guards.db")

	if out, err := runTasc(t, dbPath, "add", "Dep Guard Blocker"); err != nil {
		t.Fatalf("add blocker failed: %v\nOutput: %s", err, out)
	}
	if out, err := runTasc(t, dbPath, "add", "Dep Guard Blocked"); err != nil {
		t.Fatalf("add blocked failed: %v\nOutput: %s", err, out)
	}

	// foreign_keys is off on the connection, so without the existence check this
	// would store an edge pointing at a task that does not exist.
	out, err := runTasc(t, dbPath, "dep", "1", "999999")
	if err != nil {
		t.Fatalf("dep with an unknown ID failed: %v\nOutput: %s", err, out)
	}
	if !strings.Contains(out, "task 999999 not found") {
		t.Fatalf("dep did not report the unknown task: %s", out)
	}
	if got := countDependencies(t, dbPath); got != 0 {
		t.Fatalf("task_dependencies has %d rows after a bad dep, want 0", got)
	}

	if out, err := runTasc(t, dbPath, "dep", "1", "2"); err != nil {
		t.Fatalf("dep failed: %v\nOutput: %s", err, out)
	}

	out, err = runTasc(t, dbPath, "dep", "2", "1")
	if err != nil {
		t.Fatalf("dep (reverse) failed: %v\nOutput: %s", err, out)
	}
	if !strings.Contains(out, "deadlock") {
		t.Fatalf("dep did not refuse the reverse edge: %s", out)
	}
	if got := countDependencies(t, dbPath); got != 1 {
		t.Fatalf("task_dependencies has %d rows, want 1", got)
	}
}
