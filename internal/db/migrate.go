package db

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"
)

type Migration struct {
	Version     int
	Description string
	Up          func(*sql.DB) error
}

type sqlExecQuerier interface {
	Exec(query string, args ...interface{}) (sql.Result, error)
	QueryRow(query string, args ...interface{}) *sql.Row
}

var migrations = []Migration{
	{
		Version:     1,
		Description: "Baseline Schema",
		Up:          baselineSchema,
	},
	{
		Version:     2,
		Description: "Rename description to title, add rich description",
		Up: func(db *sql.DB) error {
			// 1. Rename column
			if _, err := db.Exec("ALTER TABLE tasks RENAME COLUMN description TO title"); err != nil {
				return fmt.Errorf("failed to rename column: %w", err)
			}
			// 2. Add new description column
			if _, err := db.Exec("ALTER TABLE tasks ADD COLUMN description TEXT"); err != nil {
				return fmt.Errorf("failed to add description column: %w", err)
			}

			// 3. Rebuild search index (FTS5 when available, fallback otherwise).
			if err := rebuildTaskSearchIndex(db, true); err != nil {
				return fmt.Errorf("failed to setup search index: %w", err)
			}

			return nil
		},
	},
	{
		Version:     3,
		Description: "Add schedule_block column",
		Up: func(db *sql.DB) error {
			if _, err := db.Exec("ALTER TABLE tasks ADD COLUMN schedule_block TEXT"); err != nil {
				return fmt.Errorf("failed to add schedule_block column: %w", err)
			}
			return nil
		},
	},
	{
		Version:     4,
		Description: "Add short_name to projects",
		Up: func(db *sql.DB) error {
			// 1. Add Column
			if _, err := db.Exec("ALTER TABLE projects ADD COLUMN short_name TEXT"); err != nil {
				return fmt.Errorf("failed to add short_name column: %w", err)
			}

			// 2. Backfill
			rows, err := db.Query("SELECT name FROM projects ORDER BY created_at")
			if err != nil {
				return err
			}
			var names []string
			for rows.Next() {
				var n string
				rows.Scan(&n)
				names = append(names, n)
			}
			rows.Close()

			used := make(map[string]bool)

			// Helper to generate
			gen := func(n string) string {
				clean := strings.Map(func(r rune) rune {
					if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
						return r
					}
					if r >= 'A' && r <= 'Z' {
						return r + 32 // to lower
					}
					return -1
				}, n)

				if len(clean) == 0 {
					clean = "unk"
				}

				// If short enough, try as is first
				if len(clean) <= 3 {
					if !used[clean] {
						return clean
					}
				}

				// Try 3 chars
				if len(clean) >= 3 {
					c := clean[:3]
					if !used[c] {
						return c
					}
				}

				// Try 4 chars
				if len(clean) >= 4 {
					c := clean[:4]
					if !used[c] {
						return c
					}
				}

				// Try 1st + 3rd + 4th (skip vowels strategy simulation)
				// Fallback to appended numbers
				for i := 1; i < 100; i++ {
					c := fmt.Sprintf("%s%d", clean[:2], i)
					if !used[c] {
						return c
					}
				}

				// Final fallback: unique by full name hash/append?
				// Just append more numbers
				return clean[:3] + fmt.Sprintf("%d", time.Now().UnixNano()%1000)
			}

			tx, err := db.Begin()
			if err != nil {
				return err
			}
			defer tx.Rollback()

			for _, name := range names {
				sn := gen(name)
				used[sn] = true
				if _, err := tx.Exec("UPDATE projects SET short_name = ? WHERE name = ?", sn, name); err != nil {
					return err
				}
			}

			return tx.Commit()
		},
	},
	{
		Version:     5,
		Description: "Drop scheduled_at column from tasks",
		Up: func(db *sql.DB) error {
			rows, err := db.Query("PRAGMA table_info(tasks)")
			if err != nil {
				return fmt.Errorf("failed to inspect tasks schema: %w", err)
			}

			hasScheduled := false
			for rows.Next() {
				var cid int
				var name, ctype string
				var notnull, pk int
				var dflt sql.NullString
				if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
					rows.Close()
					return fmt.Errorf("failed to scan table_info: %w", err)
				}
				if name == "scheduled_at" {
					hasScheduled = true
					break
				}
			}
			if err := rows.Close(); err != nil {
				return fmt.Errorf("failed to close table_info rows: %w", err)
			}
			if err := rows.Err(); err != nil {
				return fmt.Errorf("table_info iteration error: %w", err)
			}
			if !hasScheduled {
				return nil
			}

			tx, err := db.Begin()
			if err != nil {
				return err
			}
			defer tx.Rollback()

			if _, err := tx.Exec("PRAGMA foreign_keys=OFF"); err != nil {
				return fmt.Errorf("failed to disable foreign keys: %w", err)
			}

			createTasks := `
			CREATE TABLE tasks_new (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				title TEXT NOT NULL,
				description TEXT,
				project TEXT,
				status TEXT DEFAULT 'backlog',
				created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
				completed_at DATETIME,
				due_at DATETIME,
				schedule_block TEXT,
				estimate TEXT,
				recurrence TEXT,
				active_start DATETIME,
				time_spent INTEGER DEFAULT 0,
				reschedule_count INTEGER DEFAULT 0
			);`
			if _, err := tx.Exec(createTasks); err != nil {
				return fmt.Errorf("failed to create tasks_new: %w", err)
			}

			copyData := `
			INSERT INTO tasks_new (
				id, title, description, project, status, created_at, completed_at,
				due_at, schedule_block, estimate, recurrence, active_start, time_spent, reschedule_count
			)
			SELECT
				id, title, description, project, status, created_at, completed_at,
				COALESCE(due_at, scheduled_at), schedule_block, estimate, recurrence, active_start, time_spent, reschedule_count
			FROM tasks`
			if _, err := tx.Exec(copyData); err != nil {
				return fmt.Errorf("failed to copy tasks data: %w", err)
			}

			if _, err := tx.Exec("DROP TABLE tasks"); err != nil {
				return fmt.Errorf("failed to drop tasks: %w", err)
			}
			if _, err := tx.Exec("ALTER TABLE tasks_new RENAME TO tasks"); err != nil {
				return fmt.Errorf("failed to rename tasks_new: %w", err)
			}

			if err := rebuildTaskSearchIndex(tx, true); err != nil {
				return fmt.Errorf("failed to setup search index: %w", err)
			}

			if _, err := tx.Exec("PRAGMA foreign_keys=ON"); err != nil {
				return fmt.Errorf("failed to enable foreign keys: %w", err)
			}
			return tx.Commit()
		},
	},
	{
		Version:     6,
		Description: "Add undo infrastructure and audit triggers",
		Up: func(db *sql.DB) error {
			return setupUndoInfrastructure(db)
		},
	},
}

// RunMigrations executes pending migrations.
func RunMigrations(db *sql.DB) error {
	// 1. Create migrations table
	query := `CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);`
	if _, err := db.Exec(query); err != nil {
		return fmt.Errorf("failed to create schema_migrations: %w", err)
	}

	// 2. Get current version
	var currentVersion int
	err := db.QueryRow("SELECT COALESCE(MAX(version), 0) FROM schema_migrations").Scan(&currentVersion)
	if err != nil {
		return fmt.Errorf("failed to query version: %w", err)
	}

	// 3. Special Case: Detect Legacy DB (tasks exists but no version)
	if currentVersion == 0 {
		var tasksExists int
		err := db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='tasks'").Scan(&tasksExists)
		if err != nil {
			return err
		}

		if tasksExists > 0 {
			// Legacy DB found.
			// We treat the current state as Version 1.
			// But we must ensure it strictly matches Version 1 schema (e.g. missing columns from old versions).
			// The baselineSchema function is designed to be idempotent for this reason.
			log.Println("Detected legacy database. applying baseline fixes...")
		}
	}

	// 4. Apply migrations
	for _, m := range migrations {
		if m.Version > currentVersion {
			log.Printf("Applying migration %d: %s...", m.Version, m.Description)
			if err := m.Up(db); err != nil {
				return fmt.Errorf("migration %d failed: %w", m.Version, err)
			}

			if _, err := db.Exec("INSERT INTO schema_migrations (version) VALUES (?)", m.Version); err != nil {
				return fmt.Errorf("failed to record migration %d: %w", m.Version, err)
			}
		}
	}

	return nil
}

func setupUndoInfrastructure(db *sql.DB) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS undo_actions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			command TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			undone INTEGER NOT NULL DEFAULT 0,
			undone_at DATETIME
		);`,
		`CREATE TABLE IF NOT EXISTS undo_entries (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			action_id INTEGER NOT NULL,
			inverse_sql TEXT NOT NULL,
			FOREIGN KEY(action_id) REFERENCES undo_actions(id) ON DELETE CASCADE
		);`,
		`CREATE INDEX IF NOT EXISTS idx_undo_entries_action_id ON undo_entries(action_id);`,
		`CREATE TABLE IF NOT EXISTS undo_context (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			action_id INTEGER,
			FOREIGN KEY(action_id) REFERENCES undo_actions(id) ON DELETE SET NULL
		);`,
		`INSERT OR IGNORE INTO undo_context(id, action_id) VALUES(1, NULL);`,
		`UPDATE undo_context SET action_id = NULL WHERE id = 1;`,
	}

	for _, stmt := range statements {
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}

	dropTriggers := []string{
		"DROP TRIGGER IF EXISTS tasks_undo_ai",
		"DROP TRIGGER IF EXISTS tasks_undo_ad",
		"DROP TRIGGER IF EXISTS tasks_undo_au",
		"DROP TRIGGER IF EXISTS deps_undo_ai",
		"DROP TRIGGER IF EXISTS deps_undo_ad",
		"DROP TRIGGER IF EXISTS deps_undo_au",
		"DROP TRIGGER IF EXISTS projects_undo_ai",
		"DROP TRIGGER IF EXISTS projects_undo_ad",
		"DROP TRIGGER IF EXISTS projects_undo_au",
	}
	for _, stmt := range dropTriggers {
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}

	undoTriggers := `
	CREATE TRIGGER tasks_undo_ai AFTER INSERT ON tasks
	WHEN (SELECT action_id FROM undo_context WHERE id = 1) IS NOT NULL
	BEGIN
		INSERT INTO undo_entries(action_id, inverse_sql)
		VALUES (
			(SELECT action_id FROM undo_context WHERE id = 1),
			printf('DELETE FROM tasks WHERE id = %d;', NEW.id)
		);
	END;

	CREATE TRIGGER tasks_undo_ad AFTER DELETE ON tasks
	WHEN (SELECT action_id FROM undo_context WHERE id = 1) IS NOT NULL
	BEGIN
		INSERT INTO undo_entries(action_id, inverse_sql)
		VALUES (
			(SELECT action_id FROM undo_context WHERE id = 1),
			printf(
				'INSERT INTO tasks (id, title, description, project, status, created_at, completed_at, due_at, schedule_block, estimate, recurrence, active_start, time_spent, reschedule_count) VALUES (%d, %Q, %Q, %Q, %Q, %Q, %Q, %Q, %Q, %Q, %Q, %Q, %d, %d);',
				OLD.id, OLD.title, OLD.description, OLD.project, OLD.status, OLD.created_at, OLD.completed_at, OLD.due_at, OLD.schedule_block, OLD.estimate, OLD.recurrence, OLD.active_start, COALESCE(OLD.time_spent, 0), COALESCE(OLD.reschedule_count, 0)
			)
		);
	END;

	CREATE TRIGGER tasks_undo_au AFTER UPDATE ON tasks
	WHEN (SELECT action_id FROM undo_context WHERE id = 1) IS NOT NULL
	BEGIN
		INSERT INTO undo_entries(action_id, inverse_sql)
		VALUES (
			(SELECT action_id FROM undo_context WHERE id = 1),
			printf(
				'UPDATE tasks SET id = %d, title = %Q, description = %Q, project = %Q, status = %Q, created_at = %Q, completed_at = %Q, due_at = %Q, schedule_block = %Q, estimate = %Q, recurrence = %Q, active_start = %Q, time_spent = %d, reschedule_count = %d WHERE id = %d;',
				OLD.id, OLD.title, OLD.description, OLD.project, OLD.status, OLD.created_at, OLD.completed_at, OLD.due_at, OLD.schedule_block, OLD.estimate, OLD.recurrence, OLD.active_start, COALESCE(OLD.time_spent, 0), COALESCE(OLD.reschedule_count, 0), NEW.id
			)
		);
	END;

	CREATE TRIGGER deps_undo_ai AFTER INSERT ON task_dependencies
	WHEN (SELECT action_id FROM undo_context WHERE id = 1) IS NOT NULL
	BEGIN
		INSERT INTO undo_entries(action_id, inverse_sql)
		VALUES (
			(SELECT action_id FROM undo_context WHERE id = 1),
			printf('DELETE FROM task_dependencies WHERE blocker_id = %d AND blocked_id = %d;', NEW.blocker_id, NEW.blocked_id)
		);
	END;

	CREATE TRIGGER deps_undo_ad AFTER DELETE ON task_dependencies
	WHEN (SELECT action_id FROM undo_context WHERE id = 1) IS NOT NULL
	BEGIN
		INSERT INTO undo_entries(action_id, inverse_sql)
		VALUES (
			(SELECT action_id FROM undo_context WHERE id = 1),
			printf('INSERT INTO task_dependencies (blocker_id, blocked_id) VALUES (%d, %d);', OLD.blocker_id, OLD.blocked_id)
		);
	END;

	CREATE TRIGGER deps_undo_au AFTER UPDATE ON task_dependencies
	WHEN (SELECT action_id FROM undo_context WHERE id = 1) IS NOT NULL
	BEGIN
		INSERT INTO undo_entries(action_id, inverse_sql)
		VALUES (
			(SELECT action_id FROM undo_context WHERE id = 1),
			printf(
				'UPDATE task_dependencies SET blocker_id = %d, blocked_id = %d WHERE blocker_id = %d AND blocked_id = %d;',
				OLD.blocker_id, OLD.blocked_id, NEW.blocker_id, NEW.blocked_id
			)
		);
	END;

	CREATE TRIGGER projects_undo_ai AFTER INSERT ON projects
	WHEN (SELECT action_id FROM undo_context WHERE id = 1) IS NOT NULL
	BEGIN
		INSERT INTO undo_entries(action_id, inverse_sql)
		VALUES (
			(SELECT action_id FROM undo_context WHERE id = 1),
			printf('DELETE FROM projects WHERE name = %Q;', NEW.name)
		);
	END;

	CREATE TRIGGER projects_undo_ad AFTER DELETE ON projects
	WHEN (SELECT action_id FROM undo_context WHERE id = 1) IS NOT NULL
	BEGIN
		INSERT INTO undo_entries(action_id, inverse_sql)
		VALUES (
			(SELECT action_id FROM undo_context WHERE id = 1),
			printf(
				'INSERT INTO projects (name, short_name, description, parent, status, due_at, created_at) VALUES (%Q, %Q, %Q, %Q, %Q, %Q, %Q);',
				OLD.name, OLD.short_name, OLD.description, OLD.parent, OLD.status, OLD.due_at, OLD.created_at
			)
		);
	END;

	CREATE TRIGGER projects_undo_au AFTER UPDATE ON projects
	WHEN (SELECT action_id FROM undo_context WHERE id = 1) IS NOT NULL
	BEGIN
		INSERT INTO undo_entries(action_id, inverse_sql)
		VALUES (
			(SELECT action_id FROM undo_context WHERE id = 1),
			printf(
				'UPDATE projects SET name = %Q, short_name = %Q, description = %Q, parent = %Q, status = %Q, due_at = %Q, created_at = %Q WHERE name = %Q;',
				OLD.name, OLD.short_name, OLD.description, OLD.parent, OLD.status, OLD.due_at, OLD.created_at, NEW.name
			)
		);
	END;
	`

	if _, err := db.Exec(undoTriggers); err != nil {
		return err
	}
	return nil
}

func isFTS5UnavailableErr(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "no such module: fts5")
}

func sqliteHasFTS5(q sqlExecQuerier) bool {
	_, _ = q.Exec("DROP TABLE IF EXISTS temp.tasc_fts5_probe")
	_, err := q.Exec("CREATE VIRTUAL TABLE temp.tasc_fts5_probe USING fts5(content)")
	if err != nil {
		return false
	}
	_, _ = q.Exec("DROP TABLE IF EXISTS temp.tasc_fts5_probe")
	return true
}

func createTaskSearchFallbackIndexes(q sqlExecQuerier, hasTitle bool) error {
	statements := []string{
		"CREATE INDEX IF NOT EXISTS idx_tasks_description_search ON tasks(description)",
		"CREATE INDEX IF NOT EXISTS idx_tasks_project_search ON tasks(project)",
	}
	if hasTitle {
		statements = append(statements, "CREATE INDEX IF NOT EXISTS idx_tasks_title_search ON tasks(title)")
	}

	for _, stmt := range statements {
		if _, err := q.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

func rebuildTaskSearchIndex(q sqlExecQuerier, hasTitle bool) error {
	if _, err := q.Exec("DROP TRIGGER IF EXISTS tasks_ai"); err != nil {
		return err
	}
	if _, err := q.Exec("DROP TRIGGER IF EXISTS tasks_ad"); err != nil {
		return err
	}
	if _, err := q.Exec("DROP TRIGGER IF EXISTS tasks_au"); err != nil {
		return err
	}

	if !sqliteHasFTS5(q) {
		if _, err := q.Exec("DROP TABLE IF EXISTS tasks_fts"); err != nil && !isFTS5UnavailableErr(err) {
			return err
		}
		log.Printf("SQLite FTS5 module unavailable; using LIKE-based search fallback.")
		return createTaskSearchFallbackIndexes(q, hasTitle)
	}

	if _, err := q.Exec("DROP TABLE IF EXISTS tasks_fts"); err != nil {
		return err
	}

	createFTS := "CREATE VIRTUAL TABLE tasks_fts USING fts5(description, project, content='tasks', content_rowid='id')"
	triggers := `
	CREATE TRIGGER tasks_ai AFTER INSERT ON tasks BEGIN
	  INSERT INTO tasks_fts(rowid, description, project) VALUES (new.id, new.description, new.project);
	END;

	CREATE TRIGGER tasks_ad AFTER DELETE ON tasks BEGIN
	  INSERT INTO tasks_fts(tasks_fts, rowid, description, project) VALUES('delete', old.id, old.description, old.project);
	END;

	CREATE TRIGGER tasks_au AFTER UPDATE ON tasks BEGIN
	  INSERT INTO tasks_fts(tasks_fts, rowid, description, project) VALUES('delete', old.id, old.description, old.project);
	  INSERT INTO tasks_fts(rowid, description, project) VALUES (new.id, new.description, new.project);
	END;
	`
	if hasTitle {
		createFTS = "CREATE VIRTUAL TABLE tasks_fts USING fts5(title, description, project, content='tasks', content_rowid='id')"
		triggers = `
		CREATE TRIGGER tasks_ai AFTER INSERT ON tasks BEGIN
		  INSERT INTO tasks_fts(rowid, title, description, project) VALUES (new.id, new.title, new.description, new.project);
		END;

		CREATE TRIGGER tasks_ad AFTER DELETE ON tasks BEGIN
		  INSERT INTO tasks_fts(tasks_fts, rowid, title, description, project) VALUES('delete', old.id, old.title, old.description, old.project);
		END;

		CREATE TRIGGER tasks_au AFTER UPDATE ON tasks BEGIN
		  INSERT INTO tasks_fts(tasks_fts, rowid, title, description, project) VALUES('delete', old.id, old.title, old.description, old.project);
		  INSERT INTO tasks_fts(rowid, title, description, project) VALUES (new.id, new.title, new.description, new.project);
		END;
		`
	}

	if _, err := q.Exec(createFTS); err != nil {
		if isFTS5UnavailableErr(err) {
			log.Printf("SQLite FTS5 module unavailable; using LIKE-based search fallback.")
			return createTaskSearchFallbackIndexes(q, hasTitle)
		}
		return err
	}

	if _, err := q.Exec(triggers); err != nil {
		return err
	}
	if _, err := q.Exec("INSERT INTO tasks_fts(tasks_fts) VALUES('rebuild')"); err != nil {
		return err
	}

	return nil
}

// baselineSchema ensures the DB has the schema corresponding to Tasc v1.0 state.
// It is idempotent to handle legacy upgrades safely.
func baselineSchema(db *sql.DB) error {
	// 1. Tasks Table
	_, err := db.Exec(`
	CREATE TABLE IF NOT EXISTS tasks (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		description TEXT NOT NULL,
		project TEXT,
		status TEXT DEFAULT 'backlog',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		completed_at DATETIME,
		due_at DATETIME,
		estimate TEXT,
		recurrence TEXT,
		active_start DATETIME,
		time_spent INTEGER DEFAULT 0,
		reschedule_count INTEGER DEFAULT 0
	);
	`)
	if err != nil {
		return err
	}

	// 2. Ensure Columns exist (for legacy DBs that might be half-migrated)
	// SQLite ADD COLUMN IF NOT EXISTS is not standard in older versions, so we ignore errors.
	columns := []string{
		"ALTER TABLE tasks ADD COLUMN estimate TEXT",
		"ALTER TABLE tasks ADD COLUMN recurrence TEXT",
		"ALTER TABLE tasks ADD COLUMN active_start DATETIME",
		"ALTER TABLE tasks ADD COLUMN time_spent INTEGER DEFAULT 0",
		"ALTER TABLE tasks ADD COLUMN reschedule_count INTEGER DEFAULT 0",
	}
	for _, col := range columns {
		_, _ = db.Exec(col)
	}

	// 3. Projects Table
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS projects (
			name TEXT PRIMARY KEY,
			description TEXT,
			parent TEXT,
			status TEXT DEFAULT 'active',
			due_at DATETIME,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			FOREIGN KEY(parent) REFERENCES projects(name) ON DELETE SET NULL
		)
	`)
	if err != nil {
		return err
	}

	// Ensure Project columns
	projCols := []string{
		"ALTER TABLE projects ADD COLUMN parent TEXT",
		"ALTER TABLE projects ADD COLUMN status TEXT DEFAULT 'active'",
		"ALTER TABLE projects ADD COLUMN due_at DATETIME",
	}
	for _, col := range projCols {
		_, _ = db.Exec(col)
	}

	// 4. Data Fixups
	fixups := []string{
		"UPDATE tasks SET status = 'backlog' WHERE status = 'pending'",
		"UPDATE tasks SET status = 'done' WHERE status = 'completed'",
		"UPDATE tasks SET status = 'undefined' WHERE status = 'poorly_defined'",
		"UPDATE tasks SET status = 'ongoing' WHERE active_start IS NOT NULL AND status = 'backlog'",
		"INSERT OR IGNORE INTO projects (name) SELECT DISTINCT project FROM tasks WHERE project IS NOT NULL AND project != ''",
	}
	for _, q := range fixups {
		_, _ = db.Exec(q)
	}

	// 5. Search index (FTS5 when available, fallback otherwise).
	if err := rebuildTaskSearchIndex(db, false); err != nil {
		return err
	}

	// 6. Dependencies
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS task_dependencies (
			blocker_id INTEGER,
			blocked_id INTEGER,
			PRIMARY KEY (blocker_id, blocked_id),
			FOREIGN KEY(blocker_id) REFERENCES tasks(id) ON DELETE CASCADE,
			FOREIGN KEY(blocked_id) REFERENCES tasks(id) ON DELETE CASCADE
		);
	`)
	return err
}
