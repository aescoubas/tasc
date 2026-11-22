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

## Update a task handoff

1. Call `tasc_get_task_handoff` with the task ID.
2. Write a short paragraph naming the next actor and concrete action.
3. Use `tasc_update_task_handoff` with the returned revision. Choose `replace`
   when the full opening paragraph is already the handoff. Choose `prepend`
   for an empty description, a truncated preview or an opening paragraph that
   should be kept.
4. If the revision is stale, read again and reconcile your edit with the latest text.

## Bulk reschedule or move work

1. Inspect the current scope with `tasc_list_tasks`.
2. Confirm the target set when the request could match multiple projects or statuses.
3. Apply the change with `tasc_batch_update_tasks`.
4. Summarize the IDs or count of tasks that changed.

## Prioritize a project

1. Call `tasc_list_tasks` with the project name and `sort: "priority"`
   to read its saved priorities.
2. When asked to prioritize the work, propose levels using due dates, dependencies
   and the user's stated goals.
3. When asked to save priorities, use `tasc_update_task` for a single task or
   `tasc_batch_update_tasks` for tasks sharing a level.

## Complete active work

1. Identify the task from the request and current work. If the user refers to the timed task, use `tasc_get_active_task`.
2. Confirm the task ID if needed.
3. Complete it with `tasc_complete_task` or stop timing first with `tasc_stop_task` if the user asked for that explicitly.

## Manage dependencies

1. Inspect existing relationships with `tasc_get_dependencies`.
2. Fetch relevant tasks with `tasc_get_task` when titles are ambiguous.
3. Add a dependency with `tasc_add_dependency`, or drop one with
   `tasc_remove_dependency`.
4. Restate the blocker and blocked task IDs clearly.

A blocker due after the task it blocks cannot be cleared on time, so check the two
due dates before adding an edge — and when the one that is already stored is wrong,
remove it rather than moving the dates to hide the contradiction. Removal is
undoable with `tasc undo`.

## Work a named goal

1. Call `tasc_claim_tasks` with all named IDs, the agent and goal name, and a conversation or tmux location. Supply the long-lived coding agent’s `pid` when the MCP host can survive that process. Work only on rows returned as claimed. Report conflicts with the task number, title and owner.
2. Mark each task ongoing as you begin it. On resume, recheck ownership. For reclaimed work, inspect the handoff, worktree and local commits before editing.
3. Use the claiming session for task updates. Completion releases ownership. When Alex is next, write the handoff, then call `tasc_release_tasks` with `to_alex: true`.
4. Call `tasc_end_run` before yielding or finishing, including when no tasks remain. An open idle stdio connection stays capable of renewing until the run ends. After a crash, a new run can claim expired tasks. A rejected write requires checking the current owner.
