package integration

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/aescoubas/tasc/internal/db"
	"github.com/aescoubas/tasc/internal/models"
	"github.com/aescoubas/tasc/internal/server"
	"github.com/aescoubas/tasc/internal/store/sqlite"
	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
)

func TestUserPriorityAcrossInterfaces(t *testing.T) {
	t.Setenv("TASC_REMOTE", "")
	t.Setenv("TASC_API_TOKEN_FILE", "")
	dir := t.TempDir()
	path := filepath.Join(dir, "tasks.db")
	conn, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := db.RunMigrations(conn); err != nil {
		t.Fatal(err)
	}
	st := sqlite.NewSQLiteStore(conn)
	for _, task := range []models.Task{
		{Title: "Urgent blocker", Project: "Triage"},
		{Title: "High priority", Project: "Triage", Estimate: "20m"},
		{Title: "Explicit lowest priority", Project: "Triage", Estimate: "40m"},
		{Title: "Second priority", Project: "Triage", Estimate: "40m"},
		{Title: "Edit through MCP", Project: "Other"},
		{Title: "Batch edit through MCP", Project: "Other"},
	} {
		if _, err := st.CreateTask(task); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := conn.Exec("UPDATE tasks SET reschedule_count = CASE id WHEN 1 THEN 8 WHEN 3 THEN 1 WHEN 4 THEN 3 ELSE 0 END"); err != nil {
		t.Fatal(err)
	}
	if err := st.AddDependency(1, 2); err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(server.NewServer(st))
	defer srv.Close()
	run := func(args ...string) string {
		t.Helper()
		out, err := runTasc(t, path, args...)
		if err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
		return out
	}
	type scoredTask struct {
		Task  models.Task `json:"task"`
		Score float64     `json:"score"`
	}
	list := func(prefix, args []string) []scoredTask {
		t.Helper()
		out := run(slices.Concat(prefix, []string{"list", "--project", "Triage", "--output", "json"}, args)...)
		var tasks []scoredTask
		if err := json.Unmarshal([]byte(out), &tasks); err != nil {
			t.Fatalf("decode CLI list: %v\n%s", err, out)
		}
		return tasks
	}
	post := func(body string, want int) {
		t.Helper()
		res, err := srv.Client().Post(srv.URL+"/tasks/batch-update", "application/json", strings.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		if res.StatusCode != want {
			t.Fatalf("POST %s: got %s, want %d", body, res.Status, want)
		}
	}

	before := list(nil, nil)
	run("modify", "2", "--priority", "1", "--yes")
	run("--remote", srv.URL, "modify", "4", "--priority", "2", "--yes")
	post(`{"ids":[3],"updates":{"user_priority":5}}`, http.StatusOK)
	for _, invalid := range []string{"0", "6", "2.5", `"2"`, "true"} {
		post(fmt.Sprintf(`{"ids":[2],"updates":{"user_priority":%s,"title":"Invalid edit"}}`, invalid), http.StatusBadRequest)
	}
	for _, args := range [][]string{{"--priority", "6"}, {"--priority", "2.5"}, {"--priority", "2", "--clear-priority"}} {
		if out, err := runTasc(t, path, append([]string{"modify", "2", "--yes"}, args...)...); err == nil {
			t.Fatalf("CLI accepted invalid priority flags %v: %s", args, out)
		}
	}
	post(`{"ids":[4],"updates":{"user_priority":null}}`, http.StatusOK)
	if task, err := st.GetTask(4); err != nil || task.UserPriority != nil {
		t.Fatalf("REST clear: %+v, %v", task, err)
	}
	run("--remote", srv.URL, "modify", "4", "--priority", "2", "--yes")
	after := list(nil, nil)
	if len(before) != 4 || len(after) != 4 || before[0].Task.ID != 1 {
		t.Fatalf("default listing before=%+v after=%+v", before, after)
	}
	for i := range before {
		if before[i].Task.ID != after[i].Task.ID || math.Abs(before[i].Score-after[i].Score) > 0.001 {
			t.Fatalf("default urgency changed: before=%+v after=%+v", before, after)
		}
	}
	want := []int64{2, 4, 1, 3}
	for _, prefix := range [][]string{nil, {"--remote", srv.URL}} {
		for _, tc := range []struct {
			flags []string
			ids   []int64
		}{
			{[]string{"--sort", "priority"}, want},
			{[]string{"--sort", "priority", "--desc"}, []int64{1, 3, 4, 2}},
		} {
			var got []int64
			for _, task := range list(prefix, tc.flags) {
				got = append(got, task.Task.ID)
				if task.Task.ID == 2 && (task.Task.Title != "High priority" || !task.Task.IsBlocked) {
					t.Fatalf("invalid edit changed title or blocked state: %+v", task)
				}
			}
			if !slices.Equal(got, tc.ids) {
				t.Fatalf("CLI order %v, want %v", got, tc.ids)
			}
		}
	}
	res, err := srv.Client().Get(srv.URL + "/tasks?project=Triage&sort=priority")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var restTasks []models.Task
	if err := json.NewDecoder(res.Body).Decode(&restTasks); err != nil || res.StatusCode != http.StatusOK {
		t.Fatalf("REST list: %s, %v", res.Status, err)
	}
	var restIDs []int64
	for _, task := range restTasks {
		restIDs = append(restIDs, task.ID)
	}
	if !slices.Equal(restIDs, want) || restTasks[2].UserPriority != nil ||
		restTasks[3].UserPriority == nil || *restTasks[3].UserPriority != 5 {
		t.Fatalf("REST order %v or priority 5 and unset lost: %+v", restIDs, restTasks)
	}

	for _, mode := range []string{"local", "remote"} {
		t.Run(mode, func(t *testing.T) {
			args := []string{"mcp"}
			if mode == "remote" {
				args = []string{"--remote", srv.URL, "mcp"}
			}
			c, err := client.NewStdioMCPClient(binaryPath, []string{"TASC_DB_PATH=" + path, "HOME=" + dir, "TASC_REMOTE="}, args...)
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			if _, err := c.Initialize(ctx, mcp.InitializeRequest{Params: mcp.InitializeParams{
				ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
				ClientInfo:      mcp.Implementation{Name: "priority-regression", Version: "1"},
			}}); err != nil {
				t.Fatal(err)
			}
			call := func(name string, args map[string]any, wantError bool) string {
				t.Helper()
				res, err := c.CallTool(ctx, mcp.CallToolRequest{Params: mcp.CallToolParams{Name: name, Arguments: args}})
				if err != nil || res == nil || res.IsError != wantError || len(res.Content) != 1 {
					t.Fatalf("%s %v: %+v, %v", name, args, res, err)
				}
				return res.Content[0].(mcp.TextContent).Text
			}
			params := map[string]any{"project": "Triage", "sort": "priority"}
			listing := call("tasc_list_tasks", params, false)
			lines := strings.Split(strings.TrimSpace(listing), "\n")
			if len(lines) != len(want) {
				t.Fatalf("MCP list returned %d rows, want %d: %s", len(lines), len(want), listing)
			}
			for i, id := range want {
				if !strings.HasPrefix(lines[i], fmt.Sprintf("- [%d] ", id)) {
					t.Fatalf("MCP priority order: %s", listing)
				}
			}
			if !strings.Contains(lines[0], "Status: blocked) User priority: 1") ||
				strings.Contains(lines[2], "User priority") || !strings.HasSuffix(lines[3], "User priority: 5") {
				t.Fatalf("MCP lost priority or blocked state: %s", listing)
			}
			call("tasc_update_task", map[string]any{"id": 5, "user_priority": 3, "description": "Preserve this"}, false)
			call("tasc_update_task", map[string]any{"id": 5, "user_priority": 2.5, "description": "Invalid edit"}, true)
			call("tasc_update_task", map[string]any{"id": 5, "user_priority": 4, "clear_user_priority": true}, true)
			details := call("tasc_get_task", map[string]any{"id": 5}, false)
			if !strings.Contains(details, "User priority: 3") || !strings.Contains(details, "Description: Preserve this") {
				t.Fatalf("MCP update: %s", details)
			}
			call("tasc_batch_update_tasks", map[string]any{"ids": "5,6", "user_priority": 4}, false)
			for _, id := range []int64{5, 6} {
				task, err := st.GetTask(id)
				if err != nil || task.UserPriority == nil || *task.UserPriority != 4 {
					t.Fatalf("MCP batch set: %+v, %v", task, err)
				}
			}
			call("tasc_update_task", map[string]any{"id": 5, "clear_user_priority": true}, false)
			call("tasc_batch_update_tasks", map[string]any{"ids": "6", "clear_user_priority": true}, false)
			for _, id := range []int64{5, 6} {
				task, err := st.GetTask(id)
				if err != nil || task.UserPriority != nil {
					t.Fatalf("MCP clear: %+v, %v", task, err)
				}
			}
			t.Logf("MCP returned %d tasks in priority order %v", len(lines), want)
		})
	}
}

func TestUserPriorityUndoAndRecurrence(t *testing.T) {
	t.Setenv("TASC_REMOTE", "")
	path := filepath.Join(t.TempDir(), "tasks.db")
	run := func(args ...string) string {
		t.Helper()
		out, err := runTasc(t, path, args...)
		if err != nil {
			t.Fatalf("%v: %v\n%s", args, err, out)
		}
		return out
	}
	run("add", "Repeat the review", "--recurrence", "daily")
	assertPriority := func(id string, want int) {
		t.Helper()
		var result struct{ Task models.Task }
		out := run("show", id, "--output", "json")
		if err := json.Unmarshal([]byte(out), &result); err != nil {
			t.Fatalf("show %s: %v\n%s", id, err, out)
		}
		got := result.Task.UserPriority
		if (want == 0 && got != nil) || (want != 0 && (got == nil || *got != want)) {
			t.Fatalf("task %s priority, wanted %d (0 means unset): %s", id, want, out)
		}
	}
	assertPriority("1", 0)
	run("modify", "1", "--priority", "5", "--yes")
	assertPriority("1", 5)
	run("modify", "1", "--clear-priority", "--yes")
	assertPriority("1", 0)
	run("undo", "--yes")
	assertPriority("1", 5)
	run("undo", "--yes")
	assertPriority("1", 0)
	run("modify", "1", "--priority", "2", "--yes")
	run("reschedule", "1", "in 2 days")
	assertPriority("1", 2)
	run("done", "1")
	assertPriority("2", 2)
	run("undo", "--yes")
	assertPriority("1", 2)
	t.Log("Set, clear and both undo transitions persisted across CLI invocations. The recurring successor inherited priority 2.")
}
