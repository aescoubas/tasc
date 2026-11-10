package server

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	dbpkg "github.com/aescoubas/tasc/internal/db"
	"github.com/aescoubas/tasc/internal/models"
	"github.com/aescoubas/tasc/internal/store"
	"github.com/aescoubas/tasc/internal/store/remote"
	"github.com/aescoubas/tasc/internal/store/sqlite"
	_ "github.com/mattn/go-sqlite3"
)

func setupTestDB(t *testing.T) *sql.DB {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("Failed to open memory db: %v", err)
	}

	if err := dbpkg.RunMigrations(db); err != nil {
		t.Fatalf("Failed to run migrations: %v", err)
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
		Title:   "API Task",
		Project: "API",
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
	if tasks[0].Title != "API Task" {
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
	task := models.Task{Title: "Active Task"}
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

func TestRemoteStopRespectsRequestedID(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	st := sqlite.NewSQLiteStore(db)
	first, err := st.CreateTask(models.Task{Title: "Active timer"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := st.CreateTask(models.Task{Title: "Inactive timer"})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.StartTask(first); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(NewServer(st))
	defer srv.Close()
	client := remote.NewClient(srv.URL)
	if err := client.StopTask(second); err == nil {
		t.Fatal("stopping an inactive ID must fail")
	}
	for _, id := range []string{"not-an-id", ""} {
		resp, err := srv.Client().Post(srv.URL+"/tasks/stop?id="+id, "application/json", nil)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("invalid stop ID %q returned %d", id, resp.StatusCode)
		}
	}
	active, err := st.GetActiveTask()
	if err != nil || active == nil || active.ID != first {
		t.Fatalf("unsuccessful stops changed the active timer: %+v, %v", active, err)
	}
	if err := client.StopTask(first); err != nil {
		t.Fatal(err)
	}
	if err := st.StartTask(first); err != nil {
		t.Fatal(err)
	}
	if err := client.StopTask(-1); err != nil {
		t.Fatal(err)
	}
	active, err = st.GetActiveTask()
	if err != nil || active != nil {
		t.Fatalf("default stop left an active timer: %+v, %v", active, err)
	}
}

func TestServer_AddAndRemoveDependency(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	st := sqlite.NewSQLiteStore(db)
	srv := NewServer(st)

	blocker, _ := st.CreateTask(models.Task{Title: "Blocker"})
	blocked, _ := st.CreateTask(models.Task{Title: "Blocked"})

	body, _ := json.Marshal(map[string]int64{"blocker_id": blocker, "blocked_id": blocked})
	req, _ := http.NewRequest("POST", "/dependencies", bytes.NewBuffer(body))
	rr := httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("POST /dependencies returned status %v", rr.Code)
	}

	req, _ = http.NewRequest("DELETE", fmt.Sprintf("/dependencies/%d/%d", blocker, blocked), nil)
	rr = httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("DELETE /dependencies returned status %v", rr.Code)
	}

	req, _ = http.NewRequest("GET", "/dependencies", nil)
	rr = httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("GET /dependencies returned status %v", rr.Code)
	}

	var deps []models.TaskDependency
	if err := json.NewDecoder(rr.Body).Decode(&deps); err != nil {
		t.Fatalf("decode dependencies: %v", err)
	}
	if len(deps) != 0 {
		t.Fatalf("GET /dependencies returned %d rows after delete, want 0", len(deps))
	}

	req, _ = http.NewRequest("DELETE", "/dependencies/not-an-id/3", nil)
	rr = httptest.NewRecorder()
	srv.ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("DELETE /dependencies with a bad ID returned status %v, want 400", rr.Code)
	}
}

func TestServer_DependencyValidation(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()
	st := sqlite.NewSQLiteStore(db)
	for _, title := range []string{"Zero", "Second", "Third", "Fourth"} {
		if _, err := st.CreateTask(models.Task{Title: title}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec("UPDATE tasks SET id = 0 WHERE id = 1"); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(NewServer(st))
	defer srv.Close()
	c := remote.NewClient(srv.URL)
	for _, edge := range [][2]int64{{0, 2}, {2, 3}} {
		if err := c.AddDependency(edge[0], edge[1]); err != nil {
			t.Fatalf("valid dependency rejected: %v", err)
		}
	}
	for _, tt := range []struct {
		name string
		body string
	}{
		{"self", `{"blocker_id":2,"blocked_id":2}`},
		{"missing blocker task", `{"blocker_id":9999,"blocked_id":2}`},
		{"missing dependent task", `{"blocker_id":2,"blocked_id":9999}`},
		{"direct cycle", `{"blocker_id":2,"blocked_id":0}`},
		{"indirect cycle", `{"blocker_id":3,"blocked_id":0}`},
		{"missing blocker field", `{"blocked_id":4}`},
		{"missing dependent field", `{"blocker_id":4}`},
		{"null blocker field", `{"blocker_id":null,"blocked_id":4}`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			response, err := srv.Client().Post(srv.URL+"/dependencies", "application/json", bytes.NewBufferString(tt.body))
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			if response.StatusCode != http.StatusBadRequest {
				t.Fatalf("invalid dependency returned HTTP %d, want 400", response.StatusCode)
			}
			deps, err := st.GetDependencies()
			if err != nil || len(deps) != 2 {
				t.Fatalf("invalid request changed graph: deps=%+v err=%v", deps, err)
			}
		})
	}
	if err := c.AddDependency(3, 0); !errors.Is(err, store.ErrInvalidDependency) {
		t.Fatalf("remote validation error = %v, want ErrInvalidDependency", err)
	}
	// Store failures must remain server errors rather than being called bad input.
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	response, err := srv.Client().Post(srv.URL+"/dependencies", "application/json", bytes.NewBufferString(`{"blocker_id":0,"blocked_id":4}`))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusInternalServerError {
		t.Fatalf("closed database returned HTTP %d, want 500", response.StatusCode)
	}
}
