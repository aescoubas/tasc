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

			// 3. Rebuild FTS
			if _, err := db.Exec("DROP TABLE IF EXISTS tasks_fts"); err != nil {
				return fmt.Errorf("failed to drop tasks_fts: %w", err)
			}
			if _, err := db.Exec("CREATE VIRTUAL TABLE tasks_fts USING fts5(title, description, project, content='tasks', content_rowid='id')"); err != nil {
				return fmt.Errorf("failed to create tasks_fts: %w", err)
			}

			// 4. Update Triggers
			db.Exec("DROP TRIGGER IF EXISTS tasks_ai")
			db.Exec("DROP TRIGGER IF EXISTS tasks_ad")
			db.Exec("DROP TRIGGER IF EXISTS tasks_au")

			triggers := `
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
			if _, err := db.Exec(triggers); err != nil {
				return fmt.Errorf("failed to create triggers: %w", err)
			}

			// 5. Re-populate FTS
			if _, err := db.Exec("INSERT INTO tasks_fts(tasks_fts) VALUES('rebuild')"); err != nil {
				return fmt.Errorf("failed to rebuild fts: %w", err)
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

			if _, err := tx.Exec("DROP TABLE IF EXISTS tasks_fts"); err != nil {
				return fmt.Errorf("failed to drop tasks_fts: %w", err)
			}
			if _, err := tx.Exec("CREATE VIRTUAL TABLE tasks_fts USING fts5(title, description, project, content='tasks', content_rowid='id')"); err != nil {
				return fmt.Errorf("failed to create tasks_fts: %w", err)
			}

			tx.Exec("DROP TRIGGER IF EXISTS tasks_ai")
			tx.Exec("DROP TRIGGER IF EXISTS tasks_ad")
			tx.Exec("DROP TRIGGER IF EXISTS tasks_au")
			triggers := `
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
			if _, err := tx.Exec(triggers); err != nil {
				return fmt.Errorf("failed to create triggers: %w", err)
			}
			if _, err := tx.Exec("INSERT INTO tasks_fts(tasks_fts) VALUES('rebuild')"); err != nil {
				return fmt.Errorf("failed to rebuild tasks_fts: %w", err)
			}

			if _, err := tx.Exec("PRAGMA foreign_keys=ON"); err != nil {
				return fmt.Errorf("failed to enable foreign keys: %w", err)
			}
			return tx.Commit()
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

	// 5. FTS
	_, err = db.Exec(`CREATE VIRTUAL TABLE IF NOT EXISTS tasks_fts USING fts5(description, project, content='tasks', content_rowid='id');`)
	if err != nil {
		return err
	}

	// Triggers (Drop and Recreate to be safe)
	db.Exec("DROP TRIGGER IF EXISTS tasks_ai")
	db.Exec("DROP TRIGGER IF EXISTS tasks_ad")
	db.Exec("DROP TRIGGER IF EXISTS tasks_au")

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
	_, err = db.Exec(triggers)
	if err != nil {
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
