# Common Workflows

## Capture a new task

1. If the user names a project, confirm it exists with `tasc_list_projects` when unsure.
2. Create the task with `tasc_add_task`.
3. Report the new task ID and any due date or recurrence that was applied.

## Update a vaguely referenced task

1. Use `tasc_search_tasks` with the user’s wording.
2. If several matches look plausible, present the candidates instead of guessing.
3. Fetch the final target with `tasc_get_task`.
4. Apply the change with `tasc_update_task`.

## Bulk reschedule or move work

1. Inspect the current scope with `tasc_list_tasks`.
2. Confirm the target set when the request could match multiple projects or statuses.
3. Apply the change with `tasc_batch_update_tasks`.
4. Summarize the IDs or count of tasks that changed.

## Complete active work

1. Use `tasc_get_active_task` if the request is about “what I’m working on now”.
2. Confirm the task ID if needed.
3. Complete it with `tasc_complete_task` or stop timing first with `tasc_stop_task` if the user asked for that explicitly.

## Manage dependencies

1. Inspect existing relationships with `tasc_get_dependencies`.
2. Fetch relevant tasks with `tasc_get_task` when titles are ambiguous.
3. Add a dependency with `tasc_add_dependency`.
4. Restate the blocker and blocked task IDs clearly.
