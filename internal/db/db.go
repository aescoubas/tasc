package db

import (
	"database/sql"
	"log"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
)

var DB *sql.DB

func GetDBPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		log.Fatal(err)
	}
	return filepath.Join(home, ".tasc.db")
}

func InitDB() {
	dbPath := GetDBPath()
	var newDb bool
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		newDb = true
	}

	var err error
	DB, err = sql.Open("sqlite3", dbPath)
	if err != nil {
		log.Fatal(err)
	}

	if newDb {
		createTable()
	} else {
		migrate()
	}
}

func migrate() {
	// Attempt to add new columns for existing databases.
	// We ignore errors here because the columns might already exist.
	_, _ = DB.Exec("ALTER TABLE tasks ADD COLUMN scheduled_at DATETIME")
	_, _ = DB.Exec("ALTER TABLE tasks ADD COLUMN estimate TEXT")
	_, _ = DB.Exec("ALTER TABLE tasks ADD COLUMN recurrence TEXT")
	_, _ = DB.Exec("ALTER TABLE tasks ADD COLUMN active_start DATETIME")
	_, _ = DB.Exec("ALTER TABLE tasks ADD COLUMN time_spent INTEGER DEFAULT 0")

	// Migrate status values
	_, _ = DB.Exec("UPDATE tasks SET status = 'backlog' WHERE status = 'pending'")
	_, _ = DB.Exec("UPDATE tasks SET status = 'done' WHERE status = 'completed'")
	_, _ = DB.Exec("UPDATE tasks SET status = 'undefined' WHERE status = 'poorly_defined'")
	_, _ = DB.Exec("UPDATE tasks SET status = 'ongoing' WHERE active_start IS NOT NULL AND status = 'backlog'")
}

func createTable() {
	query := `
	CREATE TABLE tasks (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		description TEXT NOT NULL,
		project TEXT,
		status TEXT DEFAULT 'backlog',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		completed_at DATETIME,
		due_at DATETIME,
		scheduled_at DATETIME,
		estimate TEXT,
		recurrence TEXT,
		active_start DATETIME,
		time_spent INTEGER DEFAULT 0
	);

	CREATE VIRTUAL TABLE tasks_fts USING fts5(description, project, content='tasks', content_rowid='id');

	-- Triggers to keep FTS index in sync
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

	CREATE TABLE task_dependencies (
		blocker_id INTEGER,
		blocked_id INTEGER,
		PRIMARY KEY (blocker_id, blocked_id),
		FOREIGN KEY(blocker_id) REFERENCES tasks(id) ON DELETE CASCADE,
		FOREIGN KEY(blocked_id) REFERENCES tasks(id) ON DELETE CASCADE
	);
	`
	_, err := DB.Exec(query)
	if err != nil {
		log.Fatal(err)
	}
}
