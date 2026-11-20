---
name: tasc-task-manager
description: Use when the user wants to inspect, plan, create, update, complete, reschedule, or reorganize tasks and projects in Tasc through the configured MCP server.
---

# Tasc Task Manager

Use this skill when the session has access to the `tasc` MCP server and the user wants task or project work performed in Tasc.

## Preconditions

- The client should be configured to launch `tasc mcp`.
- Only use this skill when the MCP tools or `tasc://tasks` resource are available in the session.

## Workflow

1. Read current state first.
   - Prefer `tasc://tasks` or `tasc_list_tasks`.
   - List `ongoing` tasks to review work in progress. Use `tasc_get_active_task` to identify the running timer.
2. Resolve ambiguity before mutating data.
   - Use `tasc_search_tasks` when the user refers to tasks loosely.
   - Use `tasc_get_task` before changing, completing, or deleting a specific task.
   - Use `tasc_list_projects` or `tasc_get_project` when a project name is uncertain.
3. Mutate conservatively.
   - Use `tasc_add_task` for new tasks.
   - Use `tasc_update_task` for single-task edits.
   - Use `tasc_batch_update_tasks` for bulk project, status, or due-date changes.
   - Treat `tasc_delete_task` and `tasc_delete_project` as destructive and require explicit user intent.
4. After changes, report the concrete outcome.
   - Give the number and title of each affected task.
   - Mention the resulting status, due date, or project when those changed.

## Conventions

- When asked to start work, read the task and mark it `ongoing` with `tasc_batch_update_tasks` if appropriate. Pass the single task ID in `ids` and begin the work.
- Use `tasc_start_task` when the user requests time tracking. Starting a timer stops the previous task's timer. Use `tasc_stop_task` when asked to stop timing.
- “finish” or “complete” maps to `tasc_complete_task`
- Preserve project names exactly as stored in Tasc.
- Prefer ISO dates when the user gives a fixed date. Natural-language dates are acceptable when the tool supports them.
- Inspect `tasc_get_dependencies` before changing blockers or blocked tasks.

Read `references/tools.md` for the tool map and `references/workflows.md` for common interaction patterns.
