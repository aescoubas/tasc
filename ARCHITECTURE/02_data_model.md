# Data Model Architecture

Tasc uses SQLite as its primary persistence layer. The database schema is managed via migrations in `internal/db/migrate.go`.

## Tables

### `tasks`
The core entity.
- **`id`**: `INTEGER PRIMARY KEY AUTOINCREMENT`.
- **`description`**: `TEXT NOT NULL`. The task content.
- **`project`**: `TEXT`. Denormalized project name (for simplicity).
- **`status`**: `TEXT`. State: `backlog`, `ongoing`, `done`, `blocked`, `deleted`, `undefined`.
- **`created_at`, `completed_at`**: Timestamps.
- **`due_at`, `scheduled_at`**: `DATETIME`.
- **`active_start`**: `DATETIME`. Used for tracking time (when a task is "started").
- **`time_spent`**: `INTEGER`. Accumulated seconds spent on the task.
- **`estimate`**: `TEXT`. User-provided duration string (e.g., "30m").
- **`recurrence`**: `TEXT`. Rule string (e.g., "daily", "every 2 weeks").
- **`reschedule_count`**: `INTEGER`. Tracks how often a task is pushed back, used for priority boosting ("friction").

### `projects`
Metadata for projects.
- **`name`**: `TEXT PRIMARY KEY`.
- **`description`**: `TEXT`.
- **`parent`**: `TEXT`. Self-reference FK for hierarchy.
- **`status`**: `TEXT`. `active` or `archived`.
- **`due_at`**: `DATETIME`. Cascading deadline for the project.

### `task_dependencies`
Manages blocking relationships (Graph).
- **`blocker_id`**: FK to `tasks.id`.
- **`blocked_id`**: FK to `tasks.id`.
- **PK**: Composite `(blocker_id, blocked_id)`.

### `tasks_fts`
A virtual table using SQLite FTS5 for full-text search.
- Triggers (`tasks_ai`, `tasks_ad`, `tasks_au`) keep this table in sync with `tasks`.

## Storage Abstraction
The application interacts with data via the `Store` interface (`internal/store/store.go`).
- **`SQLiteStore`**: Direct database access.
- **`RemoteClient`**: HTTP client implementing `Store` to talk to the `tasc serve` REST API.
This allows the CLI to operate either locally (direct DB) or remotely (client-server).
