package integration

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	dbpkg "github.com/aescoubas/tasc/internal/db"
	"github.com/aescoubas/tasc/internal/models"
	"github.com/aescoubas/tasc/internal/server"
	"github.com/aescoubas/tasc/internal/store/sqlite"
	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
)

func TestMCPTaskHandoff(t *testing.T) {
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
			labels := "Legacy evidence (Project: nested, Status: backlog, Due: none, Description: literal, Estimate: 1h, Recurrence: none, Time Spent: 0s). "
			preview := labels + strings.Repeat("x", 511-len(labels))
			legacy := "\r\n" + preview + "é\r\n \t\r\n" + strings.Repeat("Evidence line with literal labels.\r\n", 180)
			original := models.Task{Title: "Legacy handoff fixture", Project: "Handoffs", Estimate: "1h", Description: legacy}
			id, err := st.CreateTask(original)
			if err != nil {
				t.Fatal(err)
			}

			args := []string{"mcp"}
			var overlap atomic.Bool
			if mode == "remote" {
				handler := server.NewServer(st)
				bothWriting := make(chan struct{})
				var writers atomic.Int32
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					// Make both MCP clients reach their writes before either can
					// save. Both have already read and checked the same revision.
					if overlap.Load() && r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/description") {
						if writers.Add(1) == 2 {
							close(bothWriting)
						}
						select {
						case <-bothWriting:
						case <-r.Context().Done():
							return
						case <-time.After(5 * time.Second):
							http.Error(w, "second writer did not arrive", http.StatusGatewayTimeout)
							return
						}
					}
					handler.ServeHTTP(w, r)
				}))
				defer srv.Close()
				args = []string{"--remote", srv.URL, "mcp"}
			}

			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			connect := func() *client.Client {
				t.Helper()
				c, err := client.NewStdioMCPClient(binaryPath, []string{
					"TASC_DB_PATH=" + path, "HOME=" + dir, "TASC_REMOTE=",
				}, args...)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { c.Close() })
				if _, err := c.Initialize(ctx, mcp.InitializeRequest{Params: mcp.InitializeParams{
					ProtocolVersion: mcp.LATEST_PROTOCOL_VERSION,
					ClientInfo:      mcp.Implementation{Name: "handoff-regression", Version: "1"},
				}}); err != nil {
					t.Fatal(err)
				}
				return c
			}
			alex, agent := connect(), connect()
			call := func(c *client.Client, tool string, args map[string]any) (*mcp.CallToolResult, error) {
				return c.CallTool(ctx, mcp.CallToolRequest{Params: mcp.CallToolParams{Name: tool, Arguments: args}})
			}
			requireCall := func(c *client.Client, tool string, args map[string]any) *mcp.CallToolResult {
				t.Helper()
				result, err := call(c, tool, args)
				if err != nil || result == nil || result.IsError {
					t.Fatalf("%s failed: %+v, %v", tool, result, err)
				}
				return result
			}
			read := func(result *mcp.CallToolResult) map[string]any {
				t.Helper()
				payload, ok := result.StructuredContent.(map[string]any)
				if !ok || payload["id"] != float64(id) {
					t.Fatalf("bad handoff result: %+v", result)
				}
				fallback, ok := result.Content[0].(mcp.TextContent)
				var decoded map[string]any
				if !ok || json.Unmarshal([]byte(fallback.Text), &decoded) != nil || decoded["handoff"] != payload["handoff"] || decoded["revision"] != payload["revision"] {
					t.Fatalf("bad JSON fallback: %+v", result.Content)
				}
				return payload
			}
			getArgs := map[string]any{"id": id}
			getTool, updateTool := "tasc_get_task_handoff", "tasc_update_task_handoff"
			full := requireCall(alex, "tasc_get_task", getArgs)
			if content, ok := full.Content[0].(mcp.TextContent); !ok || !strings.Contains(content.Text, legacy) {
				t.Fatalf("legacy full read dropped literal description text: %+v", full)
			}
			short := requireCall(alex, getTool, getArgs)
			initial := read(short)
			if initial["handoff"] != preview || initial["truncated"] != true {
				t.Fatalf("expected a UTF-8-safe legacy preview: %+v", initial)
			}
			fullJSON, err := json.Marshal(full)
			if err != nil {
				t.Fatal(err)
			}
			shortJSON, err := json.Marshal(short)
			if err != nil {
				t.Fatal(err)
			}
			if len(shortJSON) >= len(fullJSON)/2 {
				t.Fatalf("handoff result has %d bytes, full result has %d", len(shortJSON), len(fullJSON))
			}
			t.Logf("MCP result JSON: bounded legacy handoff %d bytes, full task %d bytes", len(shortJSON), len(fullJSON))

			edit := func(mode, handoff string, revision any) map[string]any {
				return map[string]any{"id": id, "mode": mode, "handoff": handoff, "revision": revision}
			}
			const first = "Alex can confirm the result. The Tasc agent can then record the evidence."
			result, err := call(alex, updateTool, edit("replace", first, initial["revision"]))
			if err != nil || result == nil || !result.IsError {
				t.Fatalf("replaced a truncated paragraph: %+v, %v", result, err)
			}
			installed := read(requireCall(alex, updateTool, edit("prepend", first, initial["revision"])))
			other := read(requireCall(agent, getTool, getArgs))
			if other["handoff"] != first || other["truncated"] != false || other["revision"] != installed["revision"] {
				t.Fatalf("second reader lost the recorded actor and action: %+v", other)
			}

			type writeResult struct {
				result  *mcp.CallToolResult
				err     error
				handoff string
			}
			writes := make(chan writeResult, 2)
			start := make(chan struct{})
			write := func(c *client.Client, handoff string, revision any) {
				<-start
				result, err := call(c, updateTool, edit("replace", handoff, revision))
				writes <- writeResult{result, err, handoff}
			}
			overlap.Store(true)
			go write(alex, "Alex can review the local gate.", installed["revision"])
			go write(agent, "The Tasc agent can verify the remote MCP.", other["revision"])
			close(start)

			winner, conflicts := "", 0
			for range 2 {
				got := <-writes
				if got.err != nil || got.result == nil {
					t.Fatalf("overlapping MCP write failed: %+v, %v", got.result, got.err)
				}
				if got.result.IsError {
					message, ok := got.result.Content[0].(mcp.TextContent)
					if !ok || !strings.Contains(message.Text, "Read the handoff again") {
						t.Fatalf("missing conflict recovery message: %+v", got.result)
					}
					conflicts++
				} else {
					winner = got.handoff
				}
			}
			overlap.Store(false)
			if conflicts != 1 || winner == "" {
				t.Fatalf("expected one winner and one conflict, got winner %q and %d conflicts", winner, conflicts)
			}
			final, err := st.GetTask(id)
			if err != nil || final.Description != winner+"\n\n"+legacy || final.Title != original.Title ||
				final.Project != original.Project || final.Estimate != original.Estimate || final.Status != models.StatusBacklog {
				t.Fatalf("handoff edit changed unrelated task data: %+v, %v", final, err)
			}
			latest := read(requireCall(alex, getTool, getArgs))
			requireCall(agent, "tasc_update_task", map[string]any{"id": id, "description": final.Description + "Later evidence."})
			result, err = call(alex, updateTool, edit("replace", first, latest["revision"]))
			if err != nil || result == nil || !result.IsError {
				t.Fatalf("handoff edit accepted a revision predating a body-only edit: %+v, %v", result, err)
			}
			t.Log("Two overlapping edits produced one winner and one conflict. Legacy text and task metadata were preserved.")
		})
	}
}
