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
   - Use `tasc_get_active_task` when active work matters.
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
   - Mention affected task IDs or project names.
   - Mention the resulting status, due date, or project when those changed.

## Conventions

- “start working on” maps to `tasc_start_task`
- “stop working” maps to `tasc_stop_task`
- “finish” or “complete” maps to `tasc_complete_task`
- Preserve project names exactly as stored in Tasc.
- Prefer ISO dates when the user gives a fixed date. Natural-language dates are acceptable when the tool supports them.
- Inspect `tasc_get_dependencies` before changing blockers or blocked tasks.

Read `references/tools.md` for the tool map and `references/workflows.md` for common interaction patterns.
