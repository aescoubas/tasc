package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/aescoubas/tasc/internal/models"
	"github.com/aescoubas/tasc/internal/parse"
	"github.com/aescoubas/tasc/internal/store"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

type MCPServer struct {
	store  store.Store
	server *server.MCPServer
}

func NewServer(s store.Store) *MCPServer {
	mcpServer := server.NewMCPServer("tasc", "0.1.0")

	ms := &MCPServer{
		store:  s,
		server: mcpServer,
	}

	ms.registerTools()
	ms.registerResources()

	return ms
}

func (ms *MCPServer) Serve() error {
	return server.ServeStdio(ms.server)
}

func (ms *MCPServer) registerTools() {
	// 1. Add Task
	ms.server.AddTool(mcp.NewTool("add_task",
		mcp.WithDescription("Add a new task to the list"),
		mcp.WithString("description", mcp.Required(), mcp.Description("The task description")),
		mcp.WithString("project", mcp.Description("The project name (optional)")),
		mcp.WithString("due", mcp.Description("Due date (YYYY-MM-DD or natural language like 'tomorrow')")),
		mcp.WithString("scheduled", mcp.Description("Scheduled date (YYYY-MM-DD)")),
		mcp.WithString("estimate", mcp.Description("Time estimate (e.g. 30m, 2h)")),
	), ms.handleAdd)

	// 2. List Tasks
	ms.server.AddTool(mcp.NewTool("list_tasks",
		mcp.WithDescription("List tasks with optional filtering"),
		mcp.WithString("project", mcp.Description("Filter by project")),
		mcp.WithString("status", mcp.Description("Filter by status (comma separated: backlog, ongoing, done, blocked)")),
		mcp.WithBoolean("include_deleted", mcp.Description("Include deleted tasks")),
	), ms.handleList)

	// 3. Complete Task
	ms.server.AddTool(mcp.NewTool("complete_task",
		mcp.WithDescription("Mark a task as done"),
		mcp.WithNumber("id", mcp.Required(), mcp.Description("The ID of the task to complete")),
	), ms.handleComplete)

	// 4. Update Task
	ms.server.AddTool(mcp.NewTool("update_task",
		mcp.WithDescription("Update an existing task"),
		mcp.WithNumber("id", mcp.Required(), mcp.Description("The ID of the task to update")),
		mcp.WithString("description", mcp.Description("New description")),
		mcp.WithString("project", mcp.Description("New project")),
		mcp.WithString("due", mcp.Description("New due date")),
		mcp.WithString("scheduled", mcp.Description("New scheduled date")),
		mcp.WithString("estimate", mcp.Description("New time estimate")),
	), ms.handleUpdate)

	// 5. List Projects
	ms.server.AddTool(mcp.NewTool("list_projects",
		mcp.WithDescription("List all projects"),
	), ms.handleListProjects)
}

func (ms *MCPServer) registerResources() {
	ms.server.AddResource(mcp.NewResource("tasc://tasks", "All Pending Tasks",
		mcp.WithResourceDescription("A list of all currently pending tasks"),
		mcp.WithMIMEType("application/json"),
	), ms.handleResourceTasks)
}

// Handlers

func (ms *MCPServer) handleAdd(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	desc := request.GetString("description", "")
	proj := request.GetString("project", "")
	dueStr := request.GetString("due", "")
	schStr := request.GetString("scheduled", "")
	est := request.GetString("estimate", "")

	var dueAt, scheduledAt *time.Time

	if dueStr != "" {
		t, err := parse.Date(dueStr)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid due date: %v", err)), nil
		}
		dueAt = t
	}

	if schStr != "" {
		t, err := parse.Date(schStr)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid scheduled date: %v", err)), nil
		}
		scheduledAt = t
	}

	task := models.Task{
		Description: desc,
		Project:     proj,
		DueAt:       dueAt,
		ScheduledAt: scheduledAt,
		Estimate:    est,
	}

	id, err := ms.store.CreateTask(task)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Error creating task: %v", err)), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Created task %d: %s", id, desc)), nil
}

func (ms *MCPServer) handleList(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	proj := request.GetString("project", "")
	statusStr := request.GetString("status", "")
	incDel := request.GetBool("include_deleted", false)

	filter := store.TaskFilter{
		Project:        proj,
		IncludeDeleted: incDel,
	}

	if statusStr != "" {
		parts := strings.Split(statusStr, ",")
		for _, p := range parts {
			filter.Status = append(filter.Status, models.TaskStatus(strings.TrimSpace(p)))
		}
	}

	tasks, err := ms.store.ListTasks(filter)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Error listing tasks: %v", err)), nil
	}

	// Format as JSON-like text or just a clear list
	var sb strings.Builder
	if len(tasks) == 0 {
		return mcp.NewToolResultText("No tasks found."), nil
	}

	for _, t := range tasks {
		status := t.Status
		if t.IsBlocked {
			status = "blocked"
		}
		
		due := ""
		if t.DueAt != nil {
			due = fmt.Sprintf(" Due: %s", t.DueAt.Format("2006-01-02"))
		}

		sb.WriteString(fmt.Sprintf("- [%d] %s (Project: %s, Status: %s)%s\n", t.ID, t.Description, t.Project, status, due))
	}

	return mcp.NewToolResultText(sb.String()), nil
}

func (ms *MCPServer) handleComplete(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id := int64(request.GetInt("id", 0))
	if id == 0 {
		return mcp.NewToolResultError("Invalid ID"), nil
	}

	err := ms.store.MarkDone(id)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Error completing task: %v", err)), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Task %d marked as done.", id)), nil
}

func (ms *MCPServer) handleUpdate(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id := int64(request.GetInt("id", 0))
	if id == 0 {
		return mcp.NewToolResultError("Invalid ID"), nil
	}

	// Fetch existing
	task, err := ms.store.GetTask(id)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Task not found: %v", err)), nil
	}

	if d := request.GetString("description", ""); d != "" {
		task.Description = d
	}
	if p := request.GetString("project", ""); p != "" {
		task.Project = p
	}
	if est := request.GetString("estimate", ""); est != "" {
		task.Estimate = est
	}

	// For dates, we need to know if they were provided to possibly clear them?
	// The helpers return default if missing.
	// If the user wants to clear, they might send empty string.
	// If they don't send anything, we shouldn't clear.
	// But GetString returns "" if missing OR if explicitly "".
	// So we can't distinguish.
	// For now, let's assume empty string means "no change" unless we check params key existence.
	// We can use request.Params.Arguments map to check existence if needed.
	// But let's keep it simple: if provided (non-empty), update. If you want to clear, maybe use a specific flag?
	// Or check the map directly for existence.
	
	args := request.GetArguments()
	
	if _, ok := args["due"]; ok {
		dueStr := request.GetString("due", "")
		if dueStr == "" {
			task.DueAt = nil
		} else {
			t, err := parse.Date(dueStr)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Invalid due date: %v", err)), nil
			}
			task.DueAt = t
		}
	}

	if _, ok := args["scheduled"]; ok {
		schStr := request.GetString("scheduled", "")
		if schStr == "" {
			task.ScheduledAt = nil
		} else {
			t, err := parse.Date(schStr)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Invalid scheduled date: %v", err)), nil
			}
			task.ScheduledAt = t
		}
	}

	err = ms.store.UpdateTask(task)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Error updating task: %v", err)), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Task %d updated.", id)), nil
}

func (ms *MCPServer) handleListProjects(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	projects, err := ms.store.ListProjects()
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Error listing projects: %v", err)), nil
	}

	var sb strings.Builder
	for _, p := range projects {
		sb.WriteString(fmt.Sprintf("- %s: %s\n", p.Name, p.Description))
	}
	return mcp.NewToolResultText(sb.String()), nil
}

func (ms *MCPServer) handleResourceTasks(ctx context.Context, request mcp.ReadResourceRequest) ([]mcp.ResourceContents, error) {
	// List all pending tasks
	tasks, err := ms.store.ListTasks(store.TaskFilter{}) // Default filters out done/deleted
	if err != nil {
		return nil, fmt.Errorf("failed to list tasks: %w", err)
	}

	jsonData, err := json.MarshalIndent(tasks, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal tasks: %w", err)
	}

	return []mcp.ResourceContents{
		mcp.TextResourceContents{
			URI:      "tasc://tasks",
			MIMEType: "application/json",
			Text:     string(jsonData),
		},
	}, nil
}

