# Data Model Architecture

Tasc uses SQLite as its primary persistence layer. The schema is managed by the six
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
- **`schedule_block`**: `TEXT`. Named time block (`morning`, `afternoon`, …) resolved
  against `time_blocks` in `~/.tasc.yaml`. Added by migration 3.
- **`active_start`**: `DATETIME`. Set while a task is "started", for time tracking.
- **`time_spent`**: `INTEGER`. Accumulated seconds.
- **`estimate`**: `TEXT`. User-provided duration string (e.g. "30m").
- **`recurrence`**: `TEXT`. Rule string (e.g. "daily", "every 2 weeks").
- **`reschedule_count`**: `INTEGER`. How often the task was pushed back; feeds the
  priority "friction" boost.

Migration 5 dropped `scheduled_at`. `schedule_block` is the only scheduling column.

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
cycles. SQLite validates and inserts in one atomic write statement, including across
connections. REST returns HTTP 400 for validation failures; the remote client preserves
`store.ErrInvalidDependency` for callers.

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
(`internal/store/store.go`), 18 methods over tasks, task actions, dependencies,
projects and search.
- **`SQLiteStore`** (`internal/store/sqlite`): direct database access.
- **`RemoteClient`** (`internal/store/remote`): HTTP client against the `tasc serve`
  REST API, selected when `TASC_REMOTE` is set.

Both must implement any new operation, or the remote backend loses it silently.
