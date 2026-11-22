package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/aescoubas/tasc/internal/config"
	"github.com/aescoubas/tasc/internal/models"
	"github.com/aescoubas/tasc/internal/parse"
	"github.com/aescoubas/tasc/internal/store"
	"github.com/aescoubas/tasc/internal/version"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

type MCPServer struct {
	run    agentRun
	store  store.Store
	server *server.MCPServer
	config config.Config
}

func NewServer(s store.Store) *MCPServer {

	cfg, err := config.LoadConfig()
	if err != nil {
		cfg = config.DefaultConfig()
	}

	ms := &MCPServer{
		store: s,

		config: cfg,
	}

	ms.server = server.NewMCPServer("tasc", version.Version, server.WithToolHandlerMiddleware(ms.bindCaller))
	ms.registerTools()
	ms.registerAssignmentTools()
	ms.registerHandoffTools()
	ms.registerResources()

	return ms
}

func (ms *MCPServer) Serve() error {
	defer func() {
		ms.run.mu.Lock()
		defer ms.run.mu.Unlock()
		if ms.run.cancel != nil {
			ms.run.cancel()
		}
	}()
	return server.ServeStdio(ms.server)
}

func (ms *MCPServer) registerTools() {
	// 1. Add Task
	ms.server.AddTool(mcp.NewTool("tasc_add_task",
		mcp.WithDescription("Add a new task to the list"),
		mcp.WithString("title", mcp.Required(), mcp.Description("The task title")),
		mcp.WithString("description", mcp.Description("Optional detailed description")),
		mcp.WithString("project", mcp.Description("The project name (optional)")),
		mcp.WithString("due", mcp.Description("Due date (YYYY-MM-DD, DD.MM.YYYY, DD/MM/YYYY, DD-MM-YYYY, or natural language like 'tomorrow')")),
		mcp.WithString("estimate", mcp.Description("Time estimate (e.g. 30m, 2h)")),
		mcp.WithString("recurrence", mcp.Description("Recurrence rule (e.g. 'daily', 'every 2 weeks')")),
	), ms.handleAdd)

	// 2. List Tasks
	ms.server.AddTool(mcp.NewTool("tasc_list_tasks",
		mcp.WithDescription("List tasks with optional filtering"),
		mcp.WithString("project", mcp.Description("Filter by project")),
		mcp.WithString("status", mcp.Description("Filter by status (comma separated: backlog, ongoing, done, blocked)")),
		mcp.WithBoolean("include_deleted", mcp.Description("Include deleted tasks")),
		mcp.WithBoolean("exclude_archived_projects", mcp.Description("Exclude tasks assigned to archived projects. Unassigned tasks are still included.")),
		mcp.WithString("sort", mcp.Enum("priority"), mcp.Description("Order by user priority (1 highest, unset sorts like 5), then task ID")),
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
		mcp.WithString("title", mcp.Description("New title")),
		mcp.WithString("description", mcp.Description("New detailed description")),
		mcp.WithString("project", mcp.Description("New project")),
		mcp.WithString("due", mcp.Description("New due date")),
		mcp.WithString("estimate", mcp.Description("New time estimate")),
		mcp.WithString("recurrence", mcp.Description("New recurrence rule")),
		mcp.WithNumber("user_priority", mcp.Min(1), mcp.Max(5), mcp.Description("Set user priority to an integer from 1 (highest) to 5 (lowest)")),
		mcp.WithBoolean("clear_user_priority", mcp.Description("Clear user priority")),
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
		mcp.WithBoolean("clear_due", mcp.Description("Clear due date")),
		mcp.WithNumber("user_priority", mcp.Min(1), mcp.Max(5), mcp.Description("Set user priority to an integer from 1 (highest) to 5 (lowest)")),
		mcp.WithBoolean("clear_user_priority", mcp.Description("Clear user priority")),
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

	// 15. Get Task
	ms.server.AddTool(mcp.NewTool("tasc_get_task",
		mcp.WithDescription("Get a single task by ID"),
		mcp.WithNumber("id", mcp.Required(), mcp.Description("The ID of the task to retrieve")),
	), ms.handleGetTask)

	// 16. Get Active Task
	ms.server.AddTool(mcp.NewTool("tasc_get_active_task",
		mcp.WithDescription("Get the currently active time-tracked task"),
	), ms.handleGetActiveTask)

	// 17. Get Dependencies
	ms.server.AddTool(mcp.NewTool("tasc_get_dependencies",
		mcp.WithDescription("List all task dependencies"),
	), ms.handleGetDependencies)

	// 18. Get Project
	ms.server.AddTool(mcp.NewTool("tasc_get_project",
		mcp.WithDescription("Get a single project's details"),
		mcp.WithString("name", mcp.Required(), mcp.Description("The project name")),
	), ms.handleGetProject)

	// 19. Remove Dependency
	ms.server.AddTool(mcp.NewTool("tasc_remove_dependency",
		mcp.WithDescription("Remove a dependency between two tasks (Blocker no longer blocks Blocked)"),
		mcp.WithNumber("blocker_id", mcp.Required(), mcp.Description("The ID of the blocking task")),
		mcp.WithNumber("blocked_id", mcp.Required(), mcp.Description("The ID of the blocked task")),
	), ms.handleRemoveDependency)
}

func (ms *MCPServer) registerResources() {
	ms.server.AddResource(mcp.NewResource("tasc://tasks", "All Pending Tasks",
		mcp.WithResourceDescription("A list of all currently pending tasks"),
		mcp.WithMIMEType("application/json"),
	), ms.handleResourceTasks)
}

// Handlers

func (ms *MCPServer) handleAdd(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if err := validateTaskStringArgs(request, "title", "description", "project", "due", "estimate", "recurrence"); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	title := request.GetString("title", "")
	desc := request.GetString("description", "")
	proj := request.GetString("project", "")
	dueStr := request.GetString("due", "")
	est := request.GetString("estimate", "")
	rec := request.GetString("recurrence", "")

	var dueAt *time.Time
	var precision *bool

	if dueStr != "" {
		t, dateOnly, err := parse.DateWithPrecision(dueStr, ms.config.EndOfDay)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid due date: %v", err)), nil
		}
		dueAt = t
		precision = &dateOnly
	}

	task := models.Task{
		Title:       title,
		Description: desc,
		Project:     proj,
		DueAt:       dueAt,
		DueDateOnly: precision,
		Estimate:    est,
		Recurrence:  rec,
	}

	id, err := ms.store.CreateTask(task)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Error creating task: %v", err)), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Created task %d: %s", id, title)), nil
}

func (ms *MCPServer) handleList(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if err := validateTaskStringArgs(request, "project", "status", "sort"); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	for _, field := range []string{"include_deleted", "exclude_archived_projects"} {
		if value, exists := request.GetArguments()[field]; exists {
			if _, ok := value.(bool); !ok {
				return mcp.NewToolResultError(field + " must be a boolean"), nil
			}
		}
	}
	proj := request.GetString("project", "")
	statusStr := request.GetString("status", "")
	incDel := request.GetBool("include_deleted", false)
	sortBy, sortRequested := request.GetArguments()["sort"]
	if sortRequested && sortBy != "priority" {
		return mcp.NewToolResultError("sort must be priority"), nil
	}

	filter := store.TaskFilter{
		IncludeDeleted:          incDel,
		ExcludeArchivedProjects: request.GetBool("exclude_archived_projects", false),
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
	if sortRequested {
		slices.SortFunc(tasks, func(a, b models.Task) int { return models.CompareTaskPriority(a, b, false) })
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

		fmt.Fprintf(&sb, "- [%d] %s (Project: %s, Status: %s)%s%s\n", t.ID, t.Title, t.Project, status, due, formatUserPriority(t)+t.AssignmentSuffix())
	}

	return mcp.NewToolResultText(sb.String()), nil
}

// MCP decodes JSON numbers as float64. Refuse fractional or imprecise
// numeric IDs so a bad argument cannot mutate a different task.
func taskID(request mcp.CallToolRequest, field string) (int64, error) {
	switch value := request.GetArguments()[field].(type) {
	case float64:
		if math.Abs(value) < 1<<53 && math.Trunc(value) == value {
			return int64(value), nil
		}
	case int:
		return int64(value), nil
	case string:
		if id, err := strconv.ParseInt(value, 10, 64); err == nil {
			return id, nil
		}
	}
	return 0, fmt.Errorf("%w: invalid task ID (%s)", store.ErrInvalidTask, field)
}

func validateTaskStringArgs(request mcp.CallToolRequest, fields ...string) error {
	for _, field := range fields {
		if value, exists := request.GetArguments()[field]; exists {
			if _, ok := value.(string); !ok {
				return fmt.Errorf("%w: %s must be a string", store.ErrInvalidTask, field)
			}
		}
	}
	return nil
}

func (ms *MCPServer) handleGetTask(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id, err := taskID(request, "id")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	task, err := ms.store.GetTask(id)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Error fetching task: %v", err)), nil
	}

	return mcp.NewToolResultText(formatTaskDetails(task)), nil
}

func (ms *MCPServer) handleGetActiveTask(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	task, err := ms.store.GetActiveTask()
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Error getting active task: %v", err)), nil
	}
	if task == nil {
		return mcp.NewToolResultText("No active task."), nil
	}

	return mcp.NewToolResultText(formatTaskDetails(*task)), nil
}

func (ms *MCPServer) handleGetDependencies(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	deps, err := ms.store.GetDependencies()
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Error listing dependencies: %v", err)), nil
	}
	if len(deps) == 0 {
		return mcp.NewToolResultText("No dependencies found."), nil
	}

	var sb strings.Builder
	for _, dep := range deps {
		sb.WriteString(fmt.Sprintf("- Task %d blocks Task %d\n", dep.BlockerID, dep.BlockedID))
	}

	return mcp.NewToolResultText(sb.String()), nil
}

func (ms *MCPServer) handleGetProject(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name := request.GetString("name", "")
	if name == "" {
		return mcp.NewToolResultError("name is required"), nil
	}

	project, err := ms.store.GetProject(name)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Project not found: %v", err)), nil
	}

	return mcp.NewToolResultText(formatProjectDetails(project)), nil
}

func (ms *MCPServer) handleComplete(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id, err := taskID(request, "id")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	err = ms.caller(ctx).MarkDone(id)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Error completing task: %v", err)), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Task %d marked as done.", id)), nil
}

func (ms *MCPServer) handleUpdate(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id, err := taskID(request, "id")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	if err := validateTaskStringArgs(request, "title", "description", "project", "due", "estimate", "recurrence"); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	args := request.GetArguments()
	updates := make(map[string]interface{})
	if err := addUserPriorityEdit(args, updates); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	for _, field := range []string{"title", "description", "project", "estimate", "recurrence"} {
		if value, ok := args[field]; ok {
			updates[field] = value
		}
	}

	if _, ok := args["due"]; ok {
		dueStr := request.GetString("due", "")
		if dueStr == "" {
			updates["due_at"] = nil
		} else {
			t, dateOnly, err := parse.DateWithPrecision(dueStr, ms.config.EndOfDay)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Invalid due date: %v", err)), nil
			}
			updates["due_at"] = t
			updates["due_date_only"] = dateOnly
		}
	}

	if err := ms.caller(ctx).BatchUpdateTasks([]int64{id}, updates); err != nil {
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
	if err := validateTaskStringArgs(request, "ids", "project", "status", "due"); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	args := request.GetArguments()
	clearDue, clearDueValid := args["clear_due"].(bool)
	if _, exists := args["clear_due"]; exists && !clearDueValid {
		return mcp.NewToolResultError("clear_due must be a boolean"), nil
	}
	if clearDue && request.GetString("due", "") != "" {
		return mcp.NewToolResultError("choose either due or clear_due"), nil
	}
	idsStr := request.GetString("ids", "")
	if idsStr == "" {
		return mcp.NewToolResultError("IDs are required"), nil
	}

	var ids []int64
	seen := make(map[int64]bool)
	parts := strings.Split(idsStr, ",")
	for _, p := range parts {
		p = strings.TrimSpace(p)
		id, err := strconv.ParseInt(p, 10, 64)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid ID: %s", p)), nil
		}
		if !seen[id] {
			ids = append(ids, id)
			seen[id] = true
		}
	}

	updates := make(map[string]interface{})
	if err := addUserPriorityEdit(request.GetArguments(), updates); err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	for _, field := range []string{"project", "status"} {
		if value, exists := args[field]; exists {
			updates[field] = value
		}
	}

	if clearDue {
		updates["due_at"] = nil
	} else if d := request.GetString("due", ""); d != "" {
		t, dateOnly, err := parse.DateWithPrecision(d, ms.config.EndOfDay)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid due date: %v", err)), nil
		}
		updates["due_at"] = t
		updates["due_date_only"] = dateOnly
	}

	err := ms.caller(ctx).BatchUpdateTasks(ids, updates)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Error batch updating tasks: %v", err)), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Updated %d tasks.", len(ids))), nil
}

func addUserPriorityEdit(args, updates map[string]any) error {
	value, set := args["user_priority"]
	clear := false
	if arg, ok := args["clear_user_priority"]; ok {
		clear, ok = arg.(bool)
		if !ok {
			return fmt.Errorf("clear_user_priority must be a boolean")
		}
	}
	if clear && set {
		return fmt.Errorf("choose either user_priority or clear_user_priority")
	}
	if clear {
		updates["user_priority"] = nil
	}
	if set {
		if value == nil {
			return store.ErrInvalidUserPriority
		}
		if err := store.ValidateUserPriority(value); err != nil {
			return err
		}
		updates["user_priority"] = value
	}
	return nil
}

func (ms *MCPServer) handleDelete(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id, err := taskID(request, "id")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	err = ms.caller(ctx).DeleteTask(id)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Error deleting task: %v", err)), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Task %d deleted.", id)), nil
}

func (ms *MCPServer) handleStart(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id, err := taskID(request, "id")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	err = ms.caller(ctx).StartTask(id)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Error starting task: %v", err)), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Started task %d.", id)), nil
}

func (ms *MCPServer) handleStop(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	var targetID *int64
	if _, ok := request.GetArguments()["id"]; ok {
		id, err := taskID(request, "id")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		targetID = &id
	}

	err := ms.caller(ctx).StopTask(targetID)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Error stopping task: %v", err)), nil
	}

	return mcp.NewToolResultText("Stopped active task."), nil
}

func (ms *MCPServer) handleAddDependency(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	blockerID, err := taskID(request, "blocker_id")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	blockedID, err := taskID(request, "blocked_id")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	err = ms.caller(ctx).AddDependency(blockerID, blockedID)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Error adding dependency: %v", err)), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Task %d now blocks task %d.", blockerID, blockedID)), nil
}

func (ms *MCPServer) handleRemoveDependency(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	blockerID, err := taskID(request, "blocker_id")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}
	blockedID, err := taskID(request, "blocked_id")
	if err != nil {
		return mcp.NewToolResultError(err.Error()), nil
	}

	deps, err := ms.store.GetDependencies()
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Error listing dependencies: %v", err)), nil
	}

	exists := false
	reversed := false
	for _, dep := range deps {
		if dep.BlockerID == blockerID && dep.BlockedID == blockedID {
			exists = true
		}
		if dep.BlockerID == blockedID && dep.BlockedID == blockerID {
			reversed = true
		}
	}

	if !exists {
		msg := fmt.Sprintf("No dependency from task %d to task %d.", blockerID, blockedID)
		if reversed {
			msg += fmt.Sprintf(" Task %d blocks task %d — the arguments may be the wrong way round.", blockedID, blockerID)
		}
		return mcp.NewToolResultError(msg), nil
	}

	if err := ms.caller(ctx).RemoveDependency(blockerID, blockedID); err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Error removing dependency: %v", err)), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Task %d no longer blocks task %d.", blockerID, blockedID)), nil
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
		t, err := parse.Date(dueStr, ms.config.EndOfDay)
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

	err := ms.store.CreateProject(p, store.ProjectCreateOptions{})
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
			t, err := parse.Date(d, ms.config.EndOfDay)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Invalid due date: %v", err)), nil
			}
			p.DueAt = t
		}
	}

	err = ms.caller(ctx).UpdateProject(name, p)
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

	err := ms.caller(ctx).DeleteProject(name)
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Error deleting project: %v", err)), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf("Project '%s' deleted.", name)), nil
}

// ... (handleSearch)
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
		sb.WriteString(fmt.Sprintf("- [%d] %s (Project: %s, Status: %s)\n", t.ID, t.Title, t.Project, string(t.Status)+t.AssignmentSuffix()))
	}

	return mcp.NewToolResultText(sb.String()), nil
}

func formatTaskDetails(t models.Task) string {
	status := t.Status
	if t.IsBlocked {
		status = models.StatusBlocked
	}

	due := "none"
	if t.DueAt != nil {
		due = t.DueAt.Format("2006-01-02")
	}

	description := t.Description
	if description == "" {
		description = "none"
	}

	estimate := t.Estimate
	if estimate == "" {
		estimate = "none"
	}

	recurrence := t.Recurrence
	if recurrence == "" {
		recurrence = "none"
	}

	return fmt.Sprintf(
		"- [%d] %s (Project: %s, Status: %s, Due: %s, Description: %s, Estimate: %s, Recurrence: %s, Time Spent: %ds)%s",
		t.ID,
		t.Title,
		t.Project,
		status,
		due,
		description,
		estimate,
		recurrence,
		t.TimeSpent,
		formatUserPriority(t)+t.AssignmentSuffix(),
	)
}

func formatUserPriority(t models.Task) string {
	if t.UserPriority == nil {
		return ""
	}
	return fmt.Sprintf(" User priority: %d", *t.UserPriority)
}

func formatProjectDetails(p models.Project) string {
	shortName := p.ShortName
	if shortName == "" {
		shortName = "none"
	}

	description := p.Description
	if description == "" {
		description = "none"
	}

	parent := p.Parent
	if parent == "" {
		parent = "none"
	}

	due := "none"
	if p.DueAt != nil {
		due = p.DueAt.Format("2006-01-02")
	}

	createdAt := p.CreatedAt.Format(time.RFC3339)
	if p.CreatedAt.IsZero() {
		createdAt = "0001-01-01T00:00:00Z"
	}

	return fmt.Sprintf(
		"Name: %s\nShort Name: %s\nDescription: %s\nParent: %s\nStatus: %s\nDue: %s\nCreated At: %s",
		p.Name,
		shortName,
		description,
		parent,
		p.Status,
		due,
		createdAt,
	)
}
