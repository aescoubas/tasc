package integration

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
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
)

const missingTaskID int64 = 9999

func taskErrorFixture(t *testing.T) (*sqlite.SQLiteStore, string) {
	t.Helper()
	t.Setenv("TASC_REMOTE", "")
	t.Setenv("TASC_API_TOKEN_FILE", "")
	path := filepath.Join(t.TempDir(), "tasks.db")
	conn, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	if err := dbpkg.RunMigrations(conn); err != nil {
		t.Fatal(err)
	}
	st := sqlite.NewSQLiteStore(conn)
	for _, task := range []models.Task{
		{Title: "Recurring fixture", Recurrence: "daily"},
		{Title: "Active fixture"},
		{Title: "Other fixture"},
	} {
		if _, err := st.CreateTask(task); err != nil {
			t.Fatal(err)
		}
	}
	// Renumbering can assign ID zero to a real task.
	if _, err := conn.Exec("UPDATE tasks SET id = 0 WHERE id = 1"); err != nil {
		t.Fatal(err)
	}
	if err := st.StartTask(2); err != nil {
		t.Fatal(err)
	}
	if err := st.AddDependency(3, 0); err != nil {
		t.Fatal(err)
	}
	return st, path
}

func assertTaskErrorFixture(t *testing.T, st *sqlite.SQLiteStore) {
	t.Helper()
	tasks, err := st.ListTasks(store.TaskFilter{IncludeDeleted: true})
	if err != nil || len(tasks) != 3 {
		t.Fatalf("failed operations changed the task count: %+v, %v", tasks, err)
	}
	task, err := st.GetTask(0)
	if err != nil || task.Title != "Recurring fixture" || task.Project != "" || task.UserPriority != nil ||
		task.Status != models.StatusBacklog || task.CompletedAt != nil || task.ActiveStart != nil {
		t.Fatalf("failed operations changed task 0: %+v, %v", task, err)
	}
	active, err := st.GetActiveTask()
	if err != nil || active == nil || active.ID != 2 || active.TimeSpent != 0 {
		t.Fatalf("failed operation changed the running timer: %+v, %v", active, err)
	}
	projects, err := st.ListProjects()
	if err != nil || len(projects) != 0 {
		t.Fatalf("failed operations changed projects: %+v, %v", projects, err)
	}
	deps, err := st.GetDependencies()
	if err != nil || len(deps) != 1 || deps[0] != (models.TaskDependency{BlockerID: 3, BlockedID: 0}) {
		t.Fatalf("failed operations changed dependencies: %+v, %v", deps, err)
	}
}

func TestTaskErrorsAcrossStoresAndREST(t *testing.T) {
	st, _ := taskErrorFixture(t)
	srv := httptest.NewServer(server.NewServer(st))
	t.Cleanup(srv.Close)

	for _, backend := range []struct {
		name string
		st   store.Store
	}{{"sqlite", st}, {"remote", remote.NewClient(srv.URL)}} {
		t.Run(backend.name, func(t *testing.T) {
			s := backend.st
			requireError := func(name string, err, want error) {
				t.Helper()
				if !errors.Is(err, want) {
					t.Fatalf("%s error = %v, want %v", name, err, want)
				}
				assertTaskErrorFixture(t, st)
			}
			_, err := s.CreateTask(models.Task{Title: " \t ", Project: "Must roll back"})
			requireError("blank create", err, store.ErrInvalidTask)
			_, err = s.LogTask(models.Task{})
			requireError("blank log", err, store.ErrInvalidTask)
			requireError("blank full update", s.UpdateTask(models.Task{ID: 0, Status: models.StatusBacklog}), store.ErrInvalidTask)
			_, err = s.GetTask(missingTaskID)
			requireError("missing read", err, sql.ErrNoRows)
			requireError("missing full update", s.UpdateTask(models.Task{ID: missingTaskID, Title: "Missing", Status: models.StatusBacklog}), sql.ErrNoRows)
			requireError("missing description", s.UpdateTaskDescription(missingTaskID, "", "new"), sql.ErrNoRows)
			requireError("missing delete", s.DeleteTask(missingTaskID), sql.ErrNoRows)
			requireError("missing completion", s.MarkDone(missingTaskID), sql.ErrNoRows)
			requireError("missing vague", s.MarkVague(missingTaskID), sql.ErrNoRows)
			requireError("missing start", s.StartTask(missingTaskID), sql.ErrNoRows)
			requireError("already active", s.StartTask(2), store.ErrInvalidTask)
			explicit := int64(-1)
			requireError("missing stop", s.StopTask(&explicit), sql.ErrNoRows)
			requireError("missing dependency", s.AddDependency(0, missingTaskID), sql.ErrNoRows)
			requireError("duplicate dependency", s.AddDependency(3, 0), store.ErrInvalidDependency)
			requireError("absent dependency", s.RemoveDependency(0, 3), sql.ErrNoRows)
			for _, updates := range []map[string]any{
				{"project": "Must roll back"}, {"user_priority": 1}, {"status": "done"}, {"status": "deleted"},
			} {
				requireError("missing batch target", s.BatchUpdateTasks([]int64{0, missingTaskID}, updates), sql.ErrNoRows)
			}
			requireError("unknown batch field", s.BatchUpdateTasks([]int64{0}, map[string]any{"project": "Must roll back", "typo": "value"}), store.ErrInvalidTask)
			requireError("no batch changes", s.BatchUpdateTasks([]int64{0}, nil), store.ErrInvalidTask)
			requireError("unknown status", s.BatchUpdateTasks([]int64{0}, map[string]any{"status": "future"}), store.ErrInvalidTask)
		})
	}
	for _, tc := range []struct {
		method, path, body string
		status             int
	}{
		{"POST", "/tasks", `{}`, 400},
		{"POST", "/tasks", `{"title":"valid document"} {}`, 400},
		{"POST", "/tasks/log", `{"title":"  "}`, 400},
		{"PUT", "/tasks/0", `{"title":"  ","status":"backlog"}`, 400},
		{"PUT", "/tasks/0", `{"title":"valid","status":"typo"}`, 400},
		{"PUT", "/tasks/9999", `{"title":"valid","status":"backlog"}`, 404},
		{"DELETE", "/tasks/9999", "", 404},
		{"POST", "/tasks/9999/done", "", 404},
		{"POST", "/tasks/9999/start", "", 404},
		{"POST", "/tasks/stop?id=-1", "", 404},
		{"POST", "/tasks/stop?id=1.5", "", 400},
		{"POST", "/tasks/batch-update", `{"ids":[0,9999],"updates":{"project":"Must roll back"}}`, 404},
		{"POST", "/tasks/batch-update", `{"ids":[null],"updates":{"title":"Must not target zero"}}`, 400},
		{"POST", "/tasks/batch-update", `{"ids":[0],"updates":{"due_at":"typo"}}`, 400},
		{"POST", "/tasks/batch-update", `{"ids":[],"updates":{"title":"No targets"}}`, 400},
		{"GET", "/tasks?ids=0,typo", "", 400},
		{"GET", "/tasks?due_before=typo", "", 400},
	} {
		t.Run(tc.method+tc.path+tc.body, func(t *testing.T) {
			req, err := http.NewRequest(tc.method, srv.URL+tc.path, strings.NewReader(tc.body))
			if err != nil {
				t.Fatal(err)
			}
			resp, err := srv.Client().Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != tc.status {
				body, _ := io.ReadAll(resp.Body)
				t.Fatalf("HTTP %d, want %d: %s", resp.StatusCode, tc.status, body)
			}
			assertTaskErrorFixture(t, st)
		})
	}
}

func TestTaskErrorsAcrossCLIAndMCP(t *testing.T) {
	for _, mode := range []string{"local", "remote"} {
		t.Run(mode, func(t *testing.T) {
			st, path := taskErrorFixture(t)
			var prefix []string
			if mode == "remote" {
				srv := httptest.NewServer(server.NewServer(st))
				t.Cleanup(srv.Close)
				prefix = []string{"--remote", srv.URL}
				path = filepath.Join(t.TempDir(), "client.db")
			}
			run := func(want string, args ...string) {
				t.Helper()
				command := append(append([]string(nil), prefix...), args...)
				out, err := runTasc(t, path, command...)
				if err == nil || !strings.Contains(out, want) || strings.Contains(out, "panic:") {
					t.Fatalf("tasc %v: want error containing %q, got %v\n%s", args, want, err, out)
				}
				for _, falseSuccess := range []string{"Modified ", "Deleted ", "Created task ", "Marked ", "Started task "} {
					if strings.Contains(out, falseSuccess) {
						t.Fatalf("failed command reported success: %s", out)
					}
				}
				assertTaskErrorFixture(t, st)
			}
			run("title must not be blank", "add", "   ", "--project", "Must roll back")
			run("title must not be blank", "log", "   ")
			run("not found", "show", "9999")
			run("not found", "start", "9999")
			run("already active", "start", "2")
			run("not found", "stop", "--", "-1")
			run("not found", "done", "0", "9999")
			run("not found", "delete", "--yes", "0", "9999")
			run("invalid task ID", "delete", "--yes", "0", "typo")
			run("not found", "modify", "--yes", "--project", "Must roll back", "0", "9999")
			run("not found", "modify", "--yes", "--priority", "1", "0", "9999")
			run("cycle", "modify", "--yes", "--project", "Must roll back", "--blocks", "2,3", "0")
			run("not found", "add", "Must roll back", "--depends-on", "3,9999", "--project", "Must roll back")
			run("task 4 not found", "add", "Must roll back", "--depends-on", "4")
			run("invalid task ID filter", "list", "--id", "0,typo")
			run("not found", "reschedule", "9999", "tomorrow")

			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			c, err := client.NewStdioMCPClient(binaryPath, []string{"HOME=" + filepath.Dir(path), "TASC_DB_PATH=" + path, "TASC_REMOTE=", "TASC_API_TOKEN_FILE="},
				append(prefix, "mcp")...)
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			if _, err := c.Initialize(ctx, mcp.InitializeRequest{Params: mcp.InitializeParams{
				ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION, ClientInfo: mcp.Implementation{Name: "task-errors", Version: "1"},
			}}); err != nil {
				t.Fatal(err)
			}
			call := func(name string, args map[string]any, want string, wantError bool) {
				t.Helper()
				result, err := c.CallTool(ctx, mcp.CallToolRequest{Params: mcp.CallToolParams{Name: name, Arguments: args}})
				if err != nil || result == nil || result.IsError != wantError || !strings.Contains(fmt.Sprint(result.Content), want) {
					t.Fatalf("%s %v: want error=%t containing %q, got %+v, %v", name, args, wantError, want, result, err)
				}
				if wantError {
					assertTaskErrorFixture(t, st)
				}
			}
			for _, tc := range []struct {
				tool string
				args map[string]any
				want string
			}{
				{"add_task", map[string]any{"title": "   "}, "title must not be blank"},
				{"update_task", map[string]any{"id": 0, "title": ""}, "title must not be blank"},
				{"delete_task", map[string]any{"id": 9999}, "not found"},
				{"complete_task", map[string]any{"id": 0.5}, "invalid task ID"},
				{"complete_task", map[string]any{"id": 9999}, "not found"},
				{"start_task", map[string]any{"id": 9999}, "not found"},
				{"start_task", map[string]any{"id": 2}, "already active"},
				{"stop_task", map[string]any{"id": -1}, "not found"},
				{"batch_update_tasks", map[string]any{"ids": "0,9999", "status": "done"}, "not found"},
				{"batch_update_tasks", map[string]any{"ids": "0,9999", "project": "Must roll back"}, "not found"},
				{"batch_update_tasks", map[string]any{"ids": "0,9999", "user_priority": 1}, "not found"},
				{"batch_update_tasks", map[string]any{"ids": "0,", "status": "done"}, "Invalid ID"},
				{"batch_update_tasks", map[string]any{"ids": "0"}, "no task updates specified"},
			} {
				call("tasc_"+tc.tool, tc.args, tc.want, true)
			}
			call("tasc_get_task", map[string]any{"id": 0}, "Recurring fixture", false)
			call("tasc_batch_update_tasks", map[string]any{"ids": "0,0", "user_priority": 1}, "Updated 1 tasks.", false)
			task, err := st.GetTask(0)
			if err != nil || task.UserPriority == nil || *task.UserPriority != 1 {
				t.Fatalf("deduplicated update did not persist: %+v, %v", task, err)
			}
			out, err := runTasc(t, path, append(prefix, "delete", "--yes", "0", "0")...)
			if err != nil || !strings.Contains(out, "Task 0 deleted.") {
				t.Fatalf("CLI duplicate IDs should report one deletion: %v, %s", err, out)
			}
		})
	}
}
