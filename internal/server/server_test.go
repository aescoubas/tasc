package server

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aescoubas/tasc/internal/models"
	"github.com/aescoubas/tasc/internal/store/sqlite"
	_ "github.com/mattn/go-sqlite3"
)

func setupTestDB(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open memory db: %v", err)
	}

	schema := `
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
		time_spent INTEGER DEFAULT 0,
		reschedule_count INTEGER DEFAULT 0
	);
	CREATE TABLE projects (
		name TEXT PRIMARY KEY,
		description TEXT,
		parent TEXT,
		status TEXT DEFAULT 'active',
		due_at DATETIME,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY(parent) REFERENCES projects(name) ON DELETE SET NULL
	);
	CREATE VIRTUAL TABLE tasks_fts USING fts5(description, project, content='tasks', content_rowid='id');
	CREATE TABLE task_dependencies (
		blocker_id INTEGER,
		blocked_id INTEGER,
		PRIMARY KEY (blocker_id, blocked_id),
		FOREIGN KEY(blocker_id) REFERENCES tasks(id) ON DELETE CASCADE,
		FOREIGN KEY(blocked_id) REFERENCES tasks(id) ON DELETE CASCADE
	);
	`
	_, err = db.Exec(schema)
	if err != nil {
		t.Fatalf("Failed to init schema: %v", err)
	}
	return db
}

func TestServer_CreateAndListTasks(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	st := sqlite.NewSQLiteStore(db)
	srv := NewServer(st)

	// 1. Create Task
	task := models.Task{
		Description: "API Task",
		Project:     "API",
	}
	body, _ := json.Marshal(task)
	req, _ := http.NewRequest("POST", "/tasks", bytes.NewBuffer(body))
	rr := httptest.NewRecorder()

	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Errorf("POST /tasks returned status %v", rr.Code)
	}

	var created models.Task
	json.NewDecoder(rr.Body).Decode(&created)
	if created.ID == 0 {
		t.Errorf("Expected created task to have ID")
	}

	// 2. List Tasks
	req, _ = http.NewRequest("GET", "/tasks", nil)
	rr = httptest.NewRecorder()

	srv.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("GET /tasks returned status %v", rr.Code)
	}

	var tasks []models.Task
	json.NewDecoder(rr.Body).Decode(&tasks)
	if len(tasks) != 1 {
		t.Errorf("Expected 1 task, got %d", len(tasks))
	}
	if tasks[0].Description != "API Task" {
		t.Errorf("Task description mismatch")
	}
}

func TestServer_GetActiveTask(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	st := sqlite.NewSQLiteStore(db)
	srv := NewServer(st)

	// No active task initially
	req, _ := http.NewRequest("GET", "/tasks/active", nil)
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("Expected 404 for no active task, got %v", rr.Code)
	}

	// Create and start task
	task := models.Task{Description: "Active Task"}
	id, _ := st.CreateTask(task)
	st.StartTask(id)

	// Check again
	req, _ = http.NewRequest("GET", "/tasks/active", nil)
	rr = httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("Expected 200 for active task, got %v", rr.Code)
	}
	var active models.Task
	json.NewDecoder(rr.Body).Decode(&active)
	if active.ID != id {
		t.Errorf("Active task ID mismatch")
	}
}
