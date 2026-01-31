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
	if env := os.Getenv("TASC_DB_PATH"); env != "" {
		return env
	}
	home, err := os.UserHomeDir()
	if err != nil {
		log.Fatal(err)
	}
	return filepath.Join(home, ".tasc.db")
}

func InitDB() {
	dbPath := GetDBPath()
	// Ensure directory exists
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		log.Fatal(err)
	}

	var err error
	DB, err = sql.Open("sqlite3", dbPath)
	if err != nil {
		log.Fatal(err)
	}

	if err := RunMigrations(DB); err != nil {
		log.Fatalf("Failed to migrate database: %v", err)
	}
}
