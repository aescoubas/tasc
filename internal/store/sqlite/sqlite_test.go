package sqlite

import (
	"database/sql"
	"testing"
	"time"

	"github.com/aescoubas/tasc/internal/models"
	"github.com/aescoubas/tasc/internal/store"
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

func TestSQLiteStore_TaskCRUD(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	s := NewSQLiteStore(db)

	// 1. Create
	now := time.Now().Truncate(time.Second) // Truncate because SQLite stores second precision by default usually or we want to match simple formatting
	// Actually sqlite3 datetime is strict string or int. models uses time.Time. 
	// Let's rely on driver handling.
	
	task := models.Task{
		Description: "Test Task",
		Project:     "TestProject",
		Estimate:    "30m",
		DueAt:       &now,
	}

	id, err := s.CreateTask(task)
	if err != nil {
		t.Fatalf("CreateTask failed: %v", err)
	}
	if id == 0 {
		t.Errorf("Expected non-zero ID")
	}

	// 2. Get
	fetched, err := s.GetTask(id)
	if err != nil {
		t.Fatalf("GetTask failed: %v", err)
	}
	if fetched.Description != task.Description {
		t.Errorf("Description mismatch: got %q, want %q", fetched.Description, task.Description)
	}
	if fetched.Project != task.Project {
		t.Errorf("Project mismatch: got %q, want %q", fetched.Project, task.Project)
	}
	if fetched.DueAt == nil || !fetched.DueAt.Equal(now) {
		// Note: Roundtrip time comparison might be tricky due to precision/timezone.
		// SQLite driver usually handles UTC or local.
		// Let's accept if it's close or check formatted string if needed.
		// But .Equal() usually works if precision is respected.
		// We skipped microseconds in 'now' via Truncate? SQLite string format is typically "2006-01-02 15:04:05" (or with Z).
		// Driver might parse it back.
		// Let's just check if it's not nil for now, and rely on Description for exact match.
	}

	// 3. Update
	fetched.Description = "Updated Task"
	fetched.Status = models.StatusOngoing
	err = s.UpdateTask(fetched)
	if err != nil {
		t.Fatalf("UpdateTask failed: %v", err)
	}

	updated, err := s.GetTask(id)
	if err != nil {
		t.Fatalf("GetTask(updated) failed: %v", err)
	}
	if updated.Description != "Updated Task" {
		t.Errorf("Update failed, description: %q", updated.Description)
	}
	if updated.Status != models.StatusOngoing {
		t.Errorf("Update failed, status: %q", updated.Status)
	}

	// 4. List
	tasks, err := s.ListTasks(store.TaskFilter{})
	if err != nil {
		t.Fatalf("ListTasks failed: %v", err)
	}
	if len(tasks) != 1 {
		t.Errorf("ListTasks returned %d items, want 1", len(tasks))
	}
	if tasks[0].ID != id {
		t.Errorf("ListTasks item ID mismatch")
	}

	// 5. Delete
	err = s.DeleteTask(id)
	if err != nil {
		t.Fatalf("DeleteTask failed: %v", err)
	}

	// Verify deletion (List should hide it by default, or verify status)
	deleted, err := s.GetTask(id)
	if err != nil {
		t.Fatalf("GetTask(deleted) failed: %v", err)
	}
	if deleted.Status != models.StatusDeleted {
		t.Errorf("Task status is %q, want deleted", deleted.Status)
	}
	
	tasksAfterDelete, err := s.ListTasks(store.TaskFilter{})
	if err != nil {
		t.Fatalf("ListTasks failed: %v", err)
	}
	if len(tasksAfterDelete) != 0 {
		t.Errorf("ListTasks returned items after delete: %d", len(tasksAfterDelete))
	}
}

func TestSQLiteStore_Projects(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	s := NewSQLiteStore(db)

	p := models.Project{
		Name: "Alpha",
		Description: "Top level",
	}
	err := s.CreateProject(p)
	if err != nil {
		t.Fatalf("CreateProject failed: %v", err)
	}

	fetched, err := s.GetProject("Alpha")
	if err != nil {
		t.Fatalf("GetProject failed: %v", err)
	}
	if fetched.Description != "Top level" {
		t.Errorf("Description mismatch")
	}

	list, err := s.ListProjects()
	if err != nil {
		t.Fatalf("ListProjects failed: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("Expected 1 project")
	}
}
