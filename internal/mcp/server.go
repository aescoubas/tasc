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
	ms.server.AddTool(mcp.NewTool("tasc_add_task",
		mcp.WithDescription("Add a new task to the list"),
		mcp.WithString("description", mcp.Required(), mcp.Description("The task description")),
		mcp.WithString("project", mcp.Description("The project name (optional)")),
		mcp.WithString("due", mcp.Description("Due date (YYYY-MM-DD or natural language like 'tomorrow')")),
		mcp.WithString("scheduled", mcp.Description("Scheduled date (YYYY-MM-DD)")),
		mcp.WithString("estimate", mcp.Description("Time estimate (e.g. 30m, 2h)")),
		mcp.WithString("recurrence", mcp.Description("Recurrence rule (e.g. 'daily', 'every 2 weeks')")),
	), ms.handleAdd)

	// 2. List Tasks
	ms.server.AddTool(mcp.NewTool("tasc_list_tasks",
		mcp.WithDescription("List tasks with optional filtering"),
		mcp.WithString("project", mcp.Description("Filter by project")),
		mcp.WithString("status", mcp.Description("Filter by status (comma separated: backlog, ongoing, done, blocked)")),
		mcp.WithBoolean("include_deleted", mcp.Description("Include deleted tasks")),
	), ms.handleList)

	// 3. Complete Task
	ms.server.AddTool(mcp.NewTool("tasc_complete_task",
		mcp.WithDescription("Mark a task as done"),
		mcp.WithNumber("id", mcp.Required(), mcp.Description("The ID of the task to complete")),
	), ms.handleComplete)

	// 4. Update Task
	ms.server.AddTool(mcp.NewTool("tasc_update_task",
		mcp.WithDescription("Update an existing task"),
		mcp.WithNumber("id", mcp.Required(), mcp.Description("The ID of the task to update")),
		mcp.WithString("description", mcp.Description("New description")),
		mcp.WithString("project", mcp.Description("New project")),
		mcp.WithString("due", mcp.Description("New due date")),
		mcp.WithString("scheduled", mcp.Description("New scheduled date")),
		mcp.WithString("estimate", mcp.Description("New time estimate")),
		mcp.WithString("recurrence", mcp.Description("New recurrence rule")),
	), ms.handleUpdate)

	// 5. List Projects
	ms.server.AddTool(mcp.NewTool("tasc_list_projects",
		mcp.WithDescription("List all projects"),
	), ms.handleListProjects)

	// 6. Batch Update Tasks
	ms.server.AddTool(mcp.NewTool("tasc_batch_update_tasks",
		mcp.WithDescription("Batch update multiple tasks"),
		mcp.WithString("ids", mcp.Required(), mcp.Description("Comma-separated list of IDs (e.g. '1,2,3')")),
		mcp.WithString("project", mcp.Description("New project")),
		mcp.WithString("status", mcp.Description("New status")),
		mcp.WithString("due", mcp.Description("New due date")),
		mcp.WithString("scheduled", mcp.Description("New scheduled date")),
		mcp.WithBoolean("clear_due", mcp.Description("Clear due date")),
		mcp.WithBoolean("clear_scheduled", mcp.Description("Clear scheduled date")),
	), ms.handleBatchUpdate)

	// 7. Delete Task
	ms.server.AddTool(mcp.NewTool("tasc_delete_task",
		mcp.WithDescription("Delete a task (soft delete)"),
		mcp.WithNumber("id", mcp.Required(), mcp.Description("The ID of the task to delete")),
	), ms.handleDelete)

	// 8. Start Task
	ms.server.AddTool(mcp.NewTool("tasc_start_task",
		mcp.WithDescription("Start tracking time for a task"),
		mcp.WithNumber("id", mcp.Required(), mcp.Description("The ID of the task to start")),
	), ms.handleStart)

	// 9. Stop Task
	ms.server.AddTool(mcp.NewTool("tasc_stop_task",
		mcp.WithDescription("Stop tracking time for the active task"),
		mcp.WithNumber("id", mcp.Description("Optional: The ID of the task to stop. If omitted, stops the currently active task.")),
	), ms.handleStop)

	// 10. Add Dependency
	ms.server.AddTool(mcp.NewTool("tasc_add_dependency",
		mcp.WithDescription("Add a dependency between two tasks (Blocker blocks Blocked)"),
		mcp.WithNumber("blocker_id", mcp.Required(), mcp.Description("The ID of the blocking task")),
		mcp.WithNumber("blocked_id", mcp.Required(), mcp.Description("The ID of the blocked task")),
	), ms.handleAddDependency)

	// 11. Add Project
	ms.server.AddTool(mcp.NewTool("tasc_add_project",
		mcp.WithDescription("Create a new project"),
		mcp.WithString("name", mcp.Required(), mcp.Description("Project name")),
		mcp.WithString("description", mcp.Description("Project description")),
		mcp.WithString("parent", mcp.Description("Parent project name")),
		mcp.WithString("due", mcp.Description("Due date")),
	), ms.handleAddProject)

	// 12. Update Project
	ms.server.AddTool(mcp.NewTool("tasc_update_project",
		mcp.WithDescription("Update a project"),
		mcp.WithString("name", mcp.Required(), mcp.Description("Current project name")),
		mcp.WithString("new_name", mcp.Description("New project name")),
		mcp.WithString("description", mcp.Description("New description")),
		mcp.WithString("parent", mcp.Description("New parent project")),
		mcp.WithString("status", mcp.Description("New status (active, archived)")),
		mcp.WithString("due", mcp.Description("New due date")),
	), ms.handleUpdateProject)

	// 13. Delete Project
	ms.server.AddTool(mcp.NewTool("tasc_delete_project",
		mcp.WithDescription("Delete a project"),
		mcp.WithString("name", mcp.Required(), mcp.Description("Project name")),
	), ms.handleDeleteProject)

	// 14. Search Tasks
	ms.server.AddTool(mcp.NewTool("tasc_search_tasks",
		mcp.WithDescription("Fuzzy search tasks by description or project"),
		mcp.WithString("query", mcp.Required(), mcp.Description("Search query")),
	), ms.handleSearch)
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
	rec := request.GetString("recurrence", "")

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
		Recurrence:  rec,
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
		IncludeDeleted: incDel,
	}
	if proj != "" {
		filter.Projects = []string{proj}
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

	args := request.GetArguments()

	if d := request.GetString("description", ""); d != "" {
		task.Description = d
	}
	if p := request.GetString("project", ""); p != "" {
		task.Project = p
	}
	
	if _, ok := args["estimate"]; ok {
		task.Estimate = request.GetString("estimate", "")
	}
	if _, ok := args["recurrence"]; ok {
		task.Recurrence = request.GetString("recurrence", "")
	}

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

func (ms *MCPServer) handleBatchUpdate(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	idsStr := request.GetString("ids", "")
	if idsStr == "" {
		return mcp.NewToolResultError("IDs are required"), nil
	}

	var ids []int64
	parts := strings.Split(idsStr, ",")
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		var id int64
		_, err := fmt.Sscanf(p, "%d", &id)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid ID: %s", p)), nil
		}
		ids = append(ids, id)
	}

	if len(ids) == 0 {
		return mcp.NewToolResultError("No valid IDs provided"), nil
	}

	updates := make(map[string]interface{})

	if p := request.GetString("project", ""); p != "" {
		updates["project"] = p
	}
	if s := request.GetString("status", ""); s != "" {
		updates["status"] = s
	}

	if request.GetBool("clear_due", false) {
		updates["due_at"] = nil
	} else if d := request.GetString("due", ""); d != "" {
		t, err := parse.Date(d)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid due date: %v", err)), nil
		}
		updates["due_at"] = t
	}

	if request.GetBool("clear_scheduled", false) {
		updates["scheduled_at"] = nil
	} else if s := request.GetString("scheduled", ""); s != "" {
		t, err := parse.Date(s)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid scheduled date: %v", err)), nil
		}
		updates["scheduled_at"] = t
	}

	err := ms.store.BatchUpdateTasks(ids, updates)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Error batch updating tasks: %v", err)), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Updated %d tasks.", len(ids))), nil
}

func (ms *MCPServer) handleDelete(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id := int64(request.GetInt("id", 0))
	if id == 0 {
		return mcp.NewToolResultError("Invalid ID"), nil
	}

	err := ms.store.DeleteTask(id)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Error deleting task: %v", err)), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Task %d deleted.", id)), nil
}

func (ms *MCPServer) handleStart(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id := int64(request.GetInt("id", 0))
	if id == 0 {
		return mcp.NewToolResultError("Invalid ID"), nil
	}

	err := ms.store.StartTask(id)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Error starting task: %v", err)), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Started task %d.", id)), nil
}

func (ms *MCPServer) handleStop(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := request.GetArguments()
	targetID := int64(-1)
	if _, ok := args["id"]; ok {
		targetID = int64(request.GetInt("id", 0))
	}

	err := ms.store.StopTask(targetID)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Error stopping task: %v", err)), nil
	}

	return mcp.NewToolResultText("Stopped active task."), nil
}

func (ms *MCPServer) handleAddDependency(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	blockerID := int64(request.GetInt("blocker_id", 0))
	blockedID := int64(request.GetInt("blocked_id", 0))
	
	if blockerID == 0 || blockedID == 0 {
		return mcp.NewToolResultError("Both blocker_id and blocked_id are required"), nil
	}

	err := ms.store.AddDependency(blockerID, blockedID)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Error adding dependency: %v", err)), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Task %d now blocks task %d.", blockerID, blockedID)), nil
}

func (ms *MCPServer) handleAddProject(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name := request.GetString("name", "")
	if name == "" {
		return mcp.NewToolResultError("name is required"), nil
	}
	desc := request.GetString("description", "")
	parent := request.GetString("parent", "")
	dueStr := request.GetString("due", "")
	
	var dueAt *time.Time
	if dueStr != "" {
		t, err := parse.Date(dueStr)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid due date: %v", err)), nil
		}
		dueAt = t
	}

	p := models.Project{
		Name:        name,
		Description: desc,
		Parent:      parent,
		DueAt:       dueAt,
		Status:      models.ProjectStatusActive,
	}

	err := ms.store.CreateProject(p)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Error creating project: %v", err)), nil
	}
	
	return mcp.NewToolResultText(fmt.Sprintf("Project '%s' created.", name)), nil
}

func (ms *MCPServer) handleUpdateProject(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name := request.GetString("name", "")
	if name == "" {
		return mcp.NewToolResultError("name is required"), nil
	}

	p, err := ms.store.GetProject(name)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Project not found: %v", err)), nil
	}
	
	args := request.GetArguments()

	if newName := request.GetString("new_name", ""); newName != "" {
		p.Name = newName
	}
	if _, ok := args["description"]; ok {
		p.Description = request.GetString("description", "")
	}
	if _, ok := args["parent"]; ok {
		p.Parent = request.GetString("parent", "")
	}
	if s := request.GetString("status", ""); s != "" {
		p.Status = models.ProjectStatus(s)
	}
	if _, ok := args["due"]; ok {
		d := request.GetString("due", "")
		if d == "" {
			p.DueAt = nil
		} else {
			t, err := parse.Date(d)
			if err != nil { return mcp.NewToolResultError(fmt.Sprintf("Invalid due date: %v", err)), nil }
			p.DueAt = t
		}
	}

	err = ms.store.UpdateProject(name, p)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Error updating project: %v", err)), nil
	}
	return mcp.NewToolResultText(fmt.Sprintf("Project '%s' updated.", name)), nil
}

func (ms *MCPServer) handleDeleteProject(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name := request.GetString("name", "")
	if name == "" {
		return mcp.NewToolResultError("name is required"), nil
	}

	err := ms.store.DeleteProject(name)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Error deleting project: %v", err)), nil
	}
	
	return mcp.NewToolResultText(fmt.Sprintf("Project '%s' deleted.", name)), nil
}

func (ms *MCPServer) handleSearch(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	query := request.GetString("query", "")
	if query == "" {
		return mcp.NewToolResultError("Query is required"), nil
	}

	tasks, err := ms.store.SearchTasks(query)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Error searching tasks: %v", err)), nil
	}
	
	if len(tasks) == 0 {
		return mcp.NewToolResultText("No matches found."), nil
	}

	var sb strings.Builder
	for _, t := range tasks {
		sb.WriteString(fmt.Sprintf("- [%d] %s (Project: %s, Status: %s)\n", t.ID, t.Description, t.Project, t.Status))
	}

	return mcp.NewToolResultText(sb.String()), nil
}