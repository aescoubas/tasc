package integration

import (
	"context"
	"database/sql"
	"encoding/json"
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

// Exercise two independent stdio processes and the operator's real CLI.
func TestAssignmentLifecycle(t *testing.T) {
	for _, mode := range []string{"local", "remote"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "tasks.db")
			db, err := sql.Open("sqlite3", path)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			if err := dbpkg.RunMigrations(db); err != nil {
				t.Fatal(err)
			}
			st := sqlite.NewSQLiteStore(db)
			for _, title := range []string{"Implement search", "Check results", "Review output", "Record evidence", "Close release"} {
				if _, err := st.CreateTask(models.Task{Title: title}); err != nil {
					t.Fatal(err)
				}
			}
			var prefix []string
			if mode == "remote" {
				srv := httptest.NewServer(server.NewServer(st))
				defer srv.Close()
				prefix = []string{"--remote", srv.URL}
			}
			cli := func(args ...string) string {
				t.Helper()
				out, err := runTasc(t, path, append(prefix, args...)...)
				if err != nil {
					t.Fatalf("%v: %s %v", args, out, err)
				}
				return out
			}
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			connect := func() *client.Client {
				t.Helper()
				c, err := client.NewStdioMCPClient(binaryPath, []string{"HOME=" + dir, "TASC_DB_PATH=" + path, "TASC_REMOTE="}, append(prefix, "mcp")...)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := c.Initialize(ctx, mcp.InitializeRequest{Params: mcp.InitializeParams{ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION, ClientInfo: mcp.Implementation{Name: "assignments-test", Version: "1"}}}); err != nil {
					t.Fatal(err)
				}
				return c
			}
			call := func(c *client.Client, name string, args map[string]any, wantError bool) string {
				t.Helper()
				r, err := c.CallTool(ctx, mcp.CallToolRequest{Params: mcp.CallToolParams{Name: name, Arguments: args}})
				if err != nil || r == nil || r.IsError != wantError {
					t.Fatalf("%s: %+v %v", name, r, err)
				}
				return r.Content[0].(mcp.TextContent).Text
			}
			type result struct {
				RunID string                `json:"run_id"`
				Tasks []store.AssignmentRow `json:"tasks"`
			}
			claim := func(c *client.Client, ids string) result {
				t.Helper()
				raw := call(c, "tasc_claim_tasks", map[string]any{"ids": ids, "name": "Codex search", "location": "conversation ABC"}, false)
				var batch result
				if err := json.Unmarshal([]byte(raw), &batch); err != nil || batch.RunID == "" {
					t.Fatalf("%s: %v", raw, err)
				}
				return batch
			}
			// A failed edit of a recurring instance must roll back its successor too.
			if err := st.BatchUpdateTasks([]int64{2}, map[string]any{"recurrence": "daily"}); err != nil {
				t.Fatal(err)
			}
			cli("assign", "3")
			first := connect()
			defer first.Close()
			batch := claim(first, "1,2,3,4,4")
			if len(batch.Tasks) != 4 || batch.Tasks[0].Status != "backlog" || batch.Tasks[0].Result != "claimed" || batch.Tasks[2].Result != "conflict" || batch.Tasks[2].Assignment.Name != "Alex" {
				t.Fatalf("initial batch: %+v", batch)
			}
			for _, out := range []string{cli("assignments"), cli("list", "--all"), call(first, "tasc_list_tasks", nil, false), call(first, "tasc_search_tasks", map[string]any{"query": "Implement"}, false)} {
				if !strings.Contains(out, batch.RunID) || !strings.Contains(out, "Implement search") {
					t.Fatalf("assignment missing from list: %s", out)
				}
			}
			if out := cli("assignments"); !strings.Contains(out, "conversation ABC") || strings.Count(out, batch.RunID) != 1 {
				t.Fatalf("grouped CLI: %s", out)
			}

			second := connect()
			defer second.Close()
			call(second, "tasc_complete_task", map[string]any{"id": 3}, true)
			competing := claim(second, "2,5")
			if batch.RunID == competing.RunID || competing.Tasks[0].Result != "conflict" || competing.Tasks[0].Title != "Check results" || competing.Tasks[0].Assignment.RunID != batch.RunID || competing.Tasks[1].Result != "claimed" {
				t.Fatalf("overlap: %+v", competing)
			}
			call(first, "tasc_update_task", map[string]any{"id": 1, "status": "ongoing"}, false)
			call(second, "tasc_batch_update_tasks", map[string]any{"ids": "5,1", "status": "done"}, true)
			if task, err := st.GetTask(5); err != nil || task.Status != models.StatusBacklog {
				t.Fatalf("partial batch: %+v %v", task, err)
			}
			handoff := call(first, "tasc_get_task_handoff", map[string]any{"id": 4}, false)
			var edit map[string]any
			if err := json.Unmarshal([]byte(handoff), &edit); err != nil {
				t.Fatal(err)
			}
			edit["mode"] = "prepend"
			edit["handoff"] = "Alex can verify the release."
			call(first, "tasc_update_task_handoff", edit, false)
			call(first, "tasc_release_tasks", map[string]any{"ids": "4", "to_alex": true}, false)
			call(first, "tasc_complete_task", map[string]any{"id": 4}, true)

			cli("assign", "--to", competing.RunID, "2")
			call(first, "tasc_update_task", map[string]any{"id": 2, "title": "Stale edit"}, true)
			var backend store.Store = st
			if mode == "remote" {
				backend = remote.NewClient(prefix[1])
			}
			detach := map[string]any{"recurrence": "", "keep_series": true}
			if err := backend.ForActor(batch.RunID).BatchUpdateTasks([]int64{2}, detach); err == nil {
				t.Fatal("stale run detached another run's series")
			}
			if err := backend.ForActor(competing.RunID).BatchUpdateTasks([]int64{2}, detach); err != nil {
				t.Fatal(err)
			}
			var successors int
			if err := db.QueryRow("SELECT COUNT(*) FROM tasks WHERE title='Check results' AND recurrence='daily'").Scan(&successors); err != nil || successors != 1 {
				t.Fatalf("got %d successors: %v", successors, err)
			}
			call(second, "tasc_complete_task", map[string]any{"id": 2}, false)
			call(first, "tasc_complete_task", map[string]any{"id": 2}, true)
			// Move only the persisted deadline. This simulates ten elapsed awake minutes
			// without making every invocation of the integration gate wait ten minutes.
			if _, err := db.Exec("UPDATE assignment_runs SET expires=0 WHERE id=?", batch.RunID); err != nil {
				t.Fatal(err)
			}
			if out := cli("assignments"); !strings.Contains(out, "reclaimable") {
				t.Fatalf("expired run invisible: %s", out)
			}
			call(first, "tasc_complete_task", map[string]any{"id": 1}, true)
			reclaimed := claim(second, "1")
			if len(reclaimed.Tasks) != 1 || reclaimed.Tasks[0].Previous == nil || reclaimed.Tasks[0].Previous.RunID != batch.RunID || !reclaimed.Tasks[0].Previous.Reclaimable {
				t.Fatalf("lost recovery context: %+v", reclaimed)
			}
			call(first, "tasc_complete_task", map[string]any{"id": 1}, true)
			call(first, "tasc_claim_tasks", map[string]any{"ids": "5", "name": "Codex search"}, true)
			call(first, "tasc_end_run", nil, false)
			call(second, "tasc_complete_task", map[string]any{"id": 1}, false)
			call(second, "tasc_end_run", nil, false)
			rows, err := st.Assignments()
			if err != nil || len(rows) != 2 || rows[0].ID != 3 || rows[1].ID != 4 || rows[1].Assignment.Name != "Alex" {
				t.Fatalf("remaining owners: %+v %v", rows, err)
			}
			if row, err := st.GetTask(4); err != nil || row.Description != "Alex can verify the release." {
				t.Fatalf("handoff: %+v %v", row, err)
			}
			cli("release", "3", "4")
			if out := cli("assignments"); !strings.Contains(out, "No assigned tasks.") {
				t.Fatal(out)
			}
			t.Logf("%s: distinct runs, partial claim, atomic conflict, takeover, recovery and human handoff verified", mode)
		})
	}
}
