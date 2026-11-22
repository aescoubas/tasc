package mcp

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/aescoubas/tasc/internal/agent"
	"github.com/aescoubas/tasc/internal/store"
	"github.com/google/uuid"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// One stdio connection is one agent session. Bind each tool call to the
// current goal when it enters, so an in-flight request can't adopt a new goal.
type agentRun struct {
	mu     sync.Mutex
	id     string
	name   string
	cancel context.CancelFunc
}
type callerKey struct{}

func (ms *MCPServer) bindCaller(next server.ToolHandlerFunc) server.ToolHandlerFunc {
	return func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		ms.run.mu.Lock()
		actor := ms.run.id
		ms.run.mu.Unlock()
		if actor == "" {
			actor = store.GuestActor
		}
		return next(context.WithValue(ctx, callerKey{}, actor), r)
	}
}
func (ms *MCPServer) caller(ctx context.Context) store.Store {
	return ms.store.ForActor(ctx.Value(callerKey{}).(string))
}

func (ms *MCPServer) registerAssignmentTools() {
	ms.server.AddTool(mcp.NewTool("tasc_list_assignments", mcp.WithDescription("List assigned tasks by owner and run, including reclaimable tasks and resume locations")), func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		rows, err := ms.store.Assignments()
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultJSON(struct {
			Tasks []store.AssignmentRow `json:"tasks"`
		}{rows})
	})
	ms.server.AddTool(mcp.NewTool("tasc_claim_tasks", mcp.WithDescription("Claim the available tasks for this goal in one transaction. Check each result before working. This stdio session renews the goal until tasc_end_run or disconnect."),
		mcp.WithString("ids", mcp.Required(), mcp.Description("Comma-separated task IDs")),
		mcp.WithString("name", mcp.Required(), mcp.Description("Readable provider and goal, e.g. Codex · Tasc search")),
		mcp.WithNumber("pid", mcp.Min(2), mcp.MultipleOf(1), mcp.Description("Linux coding agent PID if the MCP host can outlive it")),
		mcp.WithString("location", mcp.Description("Resumable conversation ID or tmux location"))), ms.claimTasks)
	ms.server.AddTool(mcp.NewTool("tasc_release_tasks", mcp.WithDescription("Release your claimed tasks, or assign them to Alex after recording the handoff"),
		mcp.WithString("ids", mcp.Required(), mcp.Description("Comma-separated task IDs")),
		mcp.WithBoolean("to_alex", mcp.Description("Assign to Alex instead of releasing"))), func(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		ids, err := store.ParseTaskIDs([]string{r.GetString("ids", "")})
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		action := "release"
		if r.GetBool("to_alex", false) {
			action = "assign"
		}
		rows, err := ms.caller(ctx).ChangeAssignments(store.AssignmentChange{Action: action, IDs: ids})
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultJSON(struct {
			Tasks []store.AssignmentRow `json:"tasks"`
		}{rows})
	})
	ms.server.AddTool(mcp.NewTool("tasc_end_run", mcp.WithDescription("End this goal on finish, pause, or yield. Release all its remaining tasks and stop its heartbeat.")), ms.endRun)
}

func (ms *MCPServer) claimTasks(ctx context.Context, r mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	ids, err := store.ParseTaskIDs([]string{r.GetString("ids", "")})
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	name := strings.TrimSpace(r.GetString("name", ""))
	location := strings.TrimSpace(r.GetString("location", ""))
	if location == "" {
		location = os.Getenv("CODEX_THREAD_ID")
	}
	if pane := os.Getenv("TMUX_PANE"); pane != "" {
		location = strings.TrimSpace(location + " tmux " + pane)
	}
	ms.run.mu.Lock()
	defer ms.run.mu.Unlock()
	id := ms.run.id
	action := "extend"
	if id == "" && ctx.Value(callerKey{}) == store.GuestActor {
		id = uuid.NewString()
		action = "claim"
	} else if id == "" || ctx.Value(callerKey{}) != id || name != ms.run.name {
		return mcp.NewToolResultError("run changed or another goal uses this session; recheck the assignments"), nil
	}
	var heartbeat context.Context
	var cancel context.CancelFunc
	if action == "claim" {
		pid := 0
		if _, provided := r.GetArguments()["pid"]; provided {
			pid = r.GetInt("pid", 0)
			if pid < 2 || float64(pid) != r.GetFloat("pid", 0) {
				return mcp.NewToolResultError("invalid agent PID"), nil
			}
		}
		heartbeat, cancel, err = agent.RunContext(context.Background(), pid)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

	}
	st := ms.store.ForActor(id)
	rows, err := st.ChangeAssignments(store.AssignmentChange{Action: action, IDs: ids, Name: name, Location: location})
	if err != nil {
		if cancel != nil {
			cancel()
		}
		return mcp.NewToolResultError(err.Error()), nil
	}
	if ms.run.id == "" {
		ms.run.id = id
		ms.run.name = name
		ms.run.cancel = cancel
		go store.MaintainRun(heartbeat, st)
	}
	return mcp.NewToolResultJSON(store.ClaimResult{RunID: id, Tasks: rows})
}

func (ms *MCPServer) endRun(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	ms.run.mu.Lock()
	defer ms.run.mu.Unlock()
	if ms.run.id == "" {
		return mcp.NewToolResultText("No active run."), nil
	}
	if ctx.Value(callerKey{}) != ms.run.id {
		return mcp.NewToolResultError("run changed; recheck the assignments"), nil
	}
	// A failed remote release must still stop renewal. Keep the run ID for a retry.
	ms.run.cancel()
	if _, err := ms.caller(ctx).ChangeAssignments(store.AssignmentChange{Action: "end"}); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	msg := fmt.Sprintf("Ended run %s and released its remaining tasks.", ms.run.id)
	ms.run.id = ""
	ms.run.name = ""
	ms.run.cancel = nil
	return mcp.NewToolResultText(msg), nil
}
