# Core Logic Architecture

## Priority Calculation (`internal/priority`)
Tasc uses a dynamic scoring system to calculate "Urgency". The `Calculator` applies a set of `Rule` functions, summing their outputs.

### Rules
1.  **`DueStalenessRule`**:
    *   Any due date: Base urgency boost (+20).
    *   Past/Today due date: Additional staleness boost (+15).
2.  **`RescheduleRule`**: +2 per reschedule event (highlights friction).
3.  **`AgeRule`**: +0.1 per day (prevents starvation of old tasks).
4.  **`DueRule`**:
    *   Overdue: +20 (+2/day).
    *   Today/Tomorrow: +15.
    *   This week: +5.
    *   Later: -5.
5.  **`EstimateRule`**: "Quick Wins" (<30m) get a small boost (+3).

### Dependency Propagation
In `cmd/list.go`, priority scores are propagated through the dependency graph. A task blocking a high-priority task inherits a portion of that priority (relaxation algorithm), ensuring blockers bubble up.

## Recurrence (`internal/recurrence`)
Handles generating the next task instance.
- **Trigger**: When a recurring task is marked `done` or `deleted` (with user choice).
- **Logic**: Parses natural language rules like "daily", "every 2 weeks", "first monday of month".
- **Implementation**: Calculates the next `DueAt` date based on the *previous* `DueAt` (or `Now`). It creates a *new* task row for the next instance, preserving history.

Completion is owned by `internal/store/sqlite/completion.go`, through `Store.MarkDone`
and status-to-done updates. A dedicated SQLite connection acquires `BEGIN IMMEDIATE`
before reading any target. The same transaction applies completion edits, creates
successors, records elapsed timer seconds, clears active timers, sets completion
timestamps, and updates affected projects. A failure rolls back the entire batch.

Repeated completion of an already-done task preserves its timestamp and time spent
and creates no successor. Missing/deleted targets and invalid or non-advancing
recurrence rules fail; duplicate IDs are processed once. A successor keeps title,
description, project, recurrence, estimate and time block, with a fresh backlog/timer
state. Metadata edits of an already-completed task remain supported without replaying
its completion. Existing completed rows are not backfilled or repaired.

CLI `done`, editor status changes, MCP completion/bulk updates, REST completion/full
updates/bulk updates, and the TUI use this Store path. TUI failures keep the task visible;
success reloads the list so the next occurrence appears. Local CLI audit triggers
record the transaction for undo. The existing deletion/skip and detach prompts remain
separate from completion.

## Date Parsing (`internal/parse`)
Robust parsing combining standard layouts (ISO 8601) and natural language processing (`go-naturaldate`).
- Preprocessing step handles shorthand inputs like "2d" -> "in 2 days" to ensure intuitive CLI usage.
