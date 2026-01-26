package db

import (
	"database/sql"
	"log"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
)

var DB *sql.DB

func InitDB() {
	home, err := os.UserHomeDir()
	if err != nil {
		log.Fatal(err)
	}

	dbPath := filepath.Join(home, ".tasc.db")
	var newDb bool
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		newDb = true
	}

	DB, err = sql.Open("sqlite3", dbPath)
	if err != nil {
		log.Fatal(err)
	}

	if newDb {
		createTable()
	}
}

func createTable() {
	query := `
	CREATE TABLE tasks (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		description TEXT NOT NULL,
		project TEXT,
		status TEXT DEFAULT 'pending',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		completed_at DATETIME,
		due_at DATETIME
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
	`
	_, err := DB.Exec(query)
	if err != nil {
		log.Fatal(err)
	}
}
