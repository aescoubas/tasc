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
	`
	_, err := DB.Exec(query)
	if err != nil {
		log.Fatal(err)
	}
}
