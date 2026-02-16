package integration

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
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
