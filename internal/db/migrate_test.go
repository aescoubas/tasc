package db

import (
	"database/sql"
	"path/filepath"
	"sync"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func TestUserPriorityUpgradesVersionSix(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	conn, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := conn.Exec(query, args...); err != nil {
			t.Fatalf("%s: %v", query, err)
		}
	}
	exec("CREATE TABLE schema_migrations (version INTEGER PRIMARY KEY)")
	for _, migration := range migrations {
		if migration.Version > 6 {
			break
		}
		if err := migration.Up(conn); err != nil {
			t.Fatalf("migration %d: %v", migration.Version, err)
		}
		exec("INSERT INTO schema_migrations (version) VALUES (?)", migration.Version)
	}
	exec("INSERT INTO tasks (id, title, description) VALUES (42, 'Legacy fixture', 'Preserved body')")
	exec("INSERT INTO undo_actions (id, command) VALUES (1, 'legacy edit')")
	exec("UPDATE undo_context SET action_id = 1")
	exec("UPDATE tasks SET title = 'Edited fixture' WHERE id = 42")
	exec("UPDATE undo_context SET action_id = NULL")

	var upgrades sync.WaitGroup
	upgradeErrors := make(chan error, 2)
	upgrades.Add(2)
	for range 2 {
		go func() {
			defer upgrades.Done()
			upgradeErrors <- RunMigrations(conn)
		}()
	}
	upgrades.Wait()
	close(upgradeErrors)
	for err := range upgradeErrors {
		if err != nil {
			t.Fatal(err)
		}
	}
	var version int
	if err := conn.QueryRow("SELECT MAX(version) FROM schema_migrations").Scan(&version); err != nil || version != 8 {
		t.Fatalf("schema version %d: %v", version, err)
	}
	var priority sql.NullInt64
	if err := conn.QueryRow("SELECT user_priority FROM tasks WHERE id = 42").Scan(&priority); err != nil || priority.Valid {
		t.Fatalf("legacy task priority: %+v, %v", priority, err)
	}
	for _, invalid := range []any{0, 6, 2.5, "high"} {
		if _, err := conn.Exec("UPDATE tasks SET user_priority = ? WHERE id = 42", invalid); err == nil {
			t.Fatalf("SQLite accepted %v", invalid)
		}
	}
	exec("UPDATE tasks SET user_priority = 5 WHERE id = 42")
	var undoLegacy string
	if err := conn.QueryRow("SELECT inverse_sql FROM undo_entries WHERE action_id = 1").Scan(&undoLegacy); err != nil {
		t.Fatal(err)
	}
	exec(undoLegacy)
	exec("INSERT INTO undo_actions (id, command) VALUES (2, 'delete fixture')")
	exec("UPDATE undo_context SET action_id = 2")
	exec("DELETE FROM tasks WHERE id = 42")
	exec("UPDATE undo_context SET action_id = NULL")
	var undoDelete string
	if err := conn.QueryRow("SELECT inverse_sql FROM undo_entries WHERE action_id = 2").Scan(&undoDelete); err != nil {
		t.Fatal(err)
	}
	exec(undoDelete)
	var title, description string
	err = conn.QueryRow("SELECT title, description, user_priority FROM tasks WHERE id = 42").Scan(&title, &description, &priority)
	if err != nil || title != "Legacy fixture" || description != "Preserved body" || !priority.Valid || priority.Int64 != 5 {
		t.Fatalf("undo restored title=%q description=%q priority=%+v: %v", title, description, priority, err)
	}
	t.Log("Concurrent upgrades from version 6 retained existing tasks and undo entries. Undo restored a deleted task with priority 5.")
}

func TestRunMigrations_RemovesScheduledColumn(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()

	if err := RunMigrations(db); err != nil {
		t.Fatalf("run migrations: %v", err)
	}

	rows, err := db.Query("PRAGMA table_info(tasks)")
	if err != nil {
		t.Fatalf("table_info: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			t.Fatalf("scan pragma row: %v", err)
		}
		if name == "scheduled_at" {
			t.Fatalf("tasks table still has scheduled_at column")
		}
	}
}
