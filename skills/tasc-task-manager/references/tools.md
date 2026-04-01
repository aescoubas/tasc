# Tool Map

Use these MCP endpoints when working with Tasc.

## Resources

- `tasc://tasks`
  - Pending tasks as JSON.
  - Prefer this for a quick read of current work before making changes.

## Task Tools

- `tasc_add_task`
  - Create a task with title, optional description, project, due date, estimate, or recurrence.
- `tasc_list_tasks`
  - Filter by project or status when the user already knows the scope.
- `tasc_search_tasks`
  - Use for fuzzy lookup when the request refers to titles loosely.
- `tasc_get_task`
  - Fetch one task before editing, completing, or deleting it.
- `tasc_update_task`
  - Edit one task.
- `tasc_batch_update_tasks`
  - Bulk reschedule or move tasks by project or status.
- `tasc_complete_task`
  - Mark a task done.
- `tasc_delete_task`
  - Soft delete a task. Require explicit intent.
- `tasc_start_task`
  - Start time tracking for a task.
- `tasc_stop_task`
  - Stop the active task timer.
- `tasc_get_active_task`
  - Check current in-progress work.

## Project Tools

- `tasc_list_projects`
  - Discover project names before using them.
- `tasc_get_project`
  - Inspect one project.
- `tasc_add_project`
  - Create a project.
- `tasc_update_project`
  - Rename or edit a project.
- `tasc_delete_project`
  - Delete a project. Require explicit intent.

## Dependency Tools

- `tasc_add_dependency`
  - Create a blocker -> blocked relationship.
- `tasc_get_dependencies`
  - Inspect dependency structure before changing blockers or rescheduling blocked work.
