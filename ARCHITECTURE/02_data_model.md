# Data Model Architecture

Tasc uses SQLite as its primary persistence layer. The schema is managed by the
migrations in `internal/db/migrate.go`, which run on **every** command — see
`AGENTS.md` rules 1 and 4 before changing anything here.

## Tables

### `tasks`
The core entity.
- **`id`**: `INTEGER PRIMARY KEY AUTOINCREMENT`.
- **`title`**: `TEXT NOT NULL`. The task line. *Named `description` before migration 2.*
- **`description`**: `TEXT`. Long-form body, added by migration 2.
- **`project`**: `TEXT`. Denormalized project name (for simplicity).
- **`status`**: `TEXT`, default `backlog`. State: `backlog`, `ongoing`, `done`,
  `blocked`, `deleted`, `undefined`.
- **`created_at`**, **`completed_at`**: timestamps.
- **`due_at`**: `DATETIME`.
- **`due_date_only`**: Nullable boolean, added by migration 9. A date-only
  input stores true alongside the configured end-of-day deadline. Explicit
  timestamps store false. Existing rows keep their timestamps and a null flag.
  Clearing the deadline clears the flag. Recurrence and undo preserve it.
- **`schedule_block`**: `TEXT`. Named time block (`morning`, `afternoon`, …) resolved
  against `time_blocks` in `~/.tasc.yaml`. Added by migration 3.
- **`active_start`**: `DATETIME`. Set while a task is "started", for time tracking.
- **`time_spent`**: `INTEGER`. Accumulated seconds.
- **`estimate`**: `TEXT`. User-provided duration string (e.g. "30m").
- **`recurrence`**: `TEXT`. Rule string (e.g. "daily", "every 2 weeks").
- **`reschedule_count`**: `INTEGER`. How often the task was pushed back; feeds the
  priority "friction" boost.
- **`user_priority`**: Nullable integer, added by migration 7. Values 1–5 run from
  highest to lowest priority. `NULL` sorts with 5 and represents an unset value.
  The migration extends the task undo triggers and leaves existing rows unset.

Migration 5 dropped `scheduled_at`. The scheduler calculates start times from
the saved due date, its precision and the configured time block.

### Task assignments and leases

Migration 8 adds `task_assignments` keyed by task ID. A null run ID identifies
Alex. Each agent goal has a UUID in `assignment_runs`, a display name, a
resume location, a boot identifier and an expiry in monotonic nanoseconds.
A single heartbeat renews all assignments for a run. Linux CLOCK_MONOTONIC
and Darwin CLOCK_UPTIME_RAW exclude suspend. A different boot expires the
lease immediately. An expired heartbeat cannot revive the run.

Claims serialize on the SQLite writer lock. Each available task is claimed
in the same transaction and each conflict returns its owner. Expired owners
stay visible until release or takeover. A successful reclaim returns the old
owner and resume location. Ownership changes leave task status and the timer
alone. Completion or deletion clears ownership in the same transaction.
Undoing completion leaves the reopened task available. A recurring successor
starts unassigned.

Task mutations take the same lock before reading state. They set the caller and
lease validity in the singleton `assignment_writer` row and reset it before
commit. Triggers guard task writes, including indirect project or timer writes,
and dependency edits. SQLite serializes writers and rolls back the context on
failure. Human CLI calls can change unassigned tasks and Alex's tasks. Unclaimed
MCP sessions can change unassigned tasks. An agent run can edit its live claims.
An explicit operator assignment or release transfers ownership immediately.
These identities coordinate cooperating callers with access to the same account.

### `projects`
- **`name`**: `TEXT PRIMARY KEY`.
- **`short_name`**: `TEXT`. Abbreviation for narrow terminals, added by migration 4.
- **`description`**: `TEXT`.
- **`parent`**: `TEXT`. Self-reference for hierarchy.
- **`status`**: `TEXT`. `active` or `archived`.
- **`due_at`**: `DATETIME`. Cascading deadline.
- **`created_at`**: timestamp.

### `task_dependencies`
Blocking relationships, as a graph.
- **`blocker_id`**, **`blocked_id`**: FKs to `tasks.id`; composite primary key.

`Store.AddDependency` requires existing, distinct tasks and rejects direct and indirect
cycles. SQLite holds the writer lock through validation and insertion. Removing an
absent directed edge returns `sql.ErrNoRows`. CLI task creation and modification can
add several edges in the same transaction as the task and project changes. A missing
task, a cycle or a failed write rolls back the full operation.

### `undo_actions`, `undo_entries`, `undo_context`
Added by migration 6. An `undo_actions` row is one user command; `undo_entries` holds
the inverse SQL statements that roll it back, emitted by audit triggers on `tasks` and
`projects`; `undo_context` carries the single in-flight action id between
`beginUndoTracking` and the command's writes.

**The trigger bodies name every column explicitly.** A migration that adds or renames a
column on `tasks` or `projects` must extend them, or `tasc undo` restores a row with
that field dropped.

### `tasks_fts`
A virtual FTS5 table over `title`, `description` and `project`, kept in sync by the
`tasks_ai` / `tasks_ad` / `tasks_au` triggers.

**It is conditional.** `rebuildTaskSearchIndex` probes for the FTS5 module; when it is
absent — a binary built without `-tags fts5` — it drops `tasks_fts`, creates plain
indexes on `title` and `description`, and `SearchTasks` falls back to `LIKE`. This
degrades a real database in place, and it is why the tag is on every Makefile target.

## Storage Abstraction
The application interacts with data through the `Store` interface
(`internal/store/store.go`) for tasks, task actions, dependencies,
projects and search.
- **`SQLiteStore`** (`internal/store/sqlite`): direct database access.
- **`RemoteClient`** (`internal/store/remote`): HTTP client against the `tasc serve`
  REST API, selected when `TASC_REMOTE` is set.

Both must implement any new operation, or the remote backend loses it silently.

Task creation, logging and title edits require a nonblank title. Batch edits validate
the fields and values and resolve every distinct ID inside the write transaction.
Missing targets return an error matching `sql.ErrNoRows`. Invalid task input returns
`store.ErrInvalidTask` or a specific validation error for completion, priority or
dependencies. IDs are signed integers because renumbering assigns zero to open tasks
and negative IDs to historical tasks. An omitted stop target is represented separately.
