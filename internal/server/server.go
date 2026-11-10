package server

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/aescoubas/tasc/internal/models"
	"github.com/aescoubas/tasc/internal/store"
)

type Server struct {
	store store.Store
	mux   *http.ServeMux
}

func NewServer(s store.Store) *Server {
	srv := &Server{
		store: s,
		mux:   http.NewServeMux(),
	}
	srv.routes()
	return srv
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// CORS Middleware
	w.Header().Set("Access-Control-Allow-Origin", "*") // Allow all for local dev convenience
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Simple logger middleware
	start := time.Now()
	log.Printf("%s %s", r.Method, r.URL.Path)
	s.mux.ServeHTTP(w, r)
	log.Printf("Completed in %v", time.Since(start))
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /tasks", s.handleListTasks)
	s.mux.HandleFunc("POST /tasks", s.handleCreateTask)
	s.mux.HandleFunc("GET /tasks/{id}", s.handleGetTask)
	s.mux.HandleFunc("PUT /tasks/{id}", s.handleUpdateTask)
	s.mux.HandleFunc("DELETE /tasks/{id}", s.handleDeleteTask)

	s.mux.HandleFunc("POST /tasks/{id}/done", s.handleTaskDone)
	s.mux.HandleFunc("POST /tasks/{id}/start", s.handleTaskStart)
	s.mux.HandleFunc("POST /tasks/stop", s.handleTaskStop)
	s.mux.HandleFunc("GET /tasks/active", s.handleGetActiveTask)
	s.mux.HandleFunc("POST /tasks/batch-update", s.handleBatchUpdateTasks)

	s.mux.HandleFunc("GET /projects", s.handleListProjects)
	s.mux.HandleFunc("POST /projects", s.handleCreateProject)
	s.mux.HandleFunc("GET /projects/{name}", s.handleGetProject)
	s.mux.HandleFunc("PUT /projects/{name}", s.handleUpdateProject)
	s.mux.HandleFunc("DELETE /projects/{name}", s.handleDeleteProject)

	s.mux.HandleFunc("GET /dependencies", s.handleListDependencies)
	s.mux.HandleFunc("POST /dependencies", s.handleAddDependency)
	s.mux.HandleFunc("DELETE /dependencies/{blocker_id}/{blocked_id}", s.handleRemoveDependency)

	s.mux.HandleFunc("GET /search", s.handleSearch)
}

func (s *Server) handleGetActiveTask(w http.ResponseWriter, r *http.Request) {
	t, err := s.store.GetActiveTask()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if t == nil {
		// 404 or just null?
		// let's return 404 if no active task
		http.Error(w, "No active task", http.StatusNotFound)
		return
	}
	respondJSON(w, http.StatusOK, t)
}

func (s *Server) handleListTasks(w http.ResponseWriter, r *http.Request) {
	filter := store.TaskFilter{}
	q := r.URL.Query()

	if statuses := q.Get("status"); statuses != "" {
		for _, status := range strings.Split(statuses, ",") {
			if status = strings.TrimSpace(status); status != "" {
				filter.Status = append(filter.Status, models.TaskStatus(status))
			}
		}
	}
	if includeDeleted := q.Get("include_deleted"); includeDeleted != "" {
		var err error
		filter.IncludeDeleted, err = strconv.ParseBool(includeDeleted)
		if err != nil {
			http.Error(w, "Invalid include_deleted: expected a boolean", http.StatusBadRequest)
			return
		}
	}

	if projects := q.Get("projects"); projects != "" {
		filter.Projects = strings.Split(projects, ",")
	}
	// Support legacy 'project' param
	if p := q.Get("project"); p != "" {
		filter.Projects = append(filter.Projects, p)
	}

	if ids := q.Get("ids"); ids != "" {
		for _, idStr := range strings.Split(ids, ",") {
			id, err := strconv.ParseInt(strings.TrimSpace(idStr), 10, 64)
			if err == nil {
				filter.IDs = append(filter.IDs, id)
			}
		}
	}

	parseDate := func(param string) *time.Time {
		if val := q.Get(param); val != "" {
			t, err := time.Parse(time.RFC3339, val)
			if err == nil {
				return &t
			}
		}
		return nil
	}

	filter.DueBefore = parseDate("due_before")
	filter.DueAfter = parseDate("due_after")

	tasks, err := s.store.ListTasks(filter)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	respondJSON(w, http.StatusOK, tasks)
}

func (s *Server) handleCreateTask(w http.ResponseWriter, r *http.Request) {
	var t models.Task
	if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	id, err := s.store.CreateTask(t)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	t.ID = id
	respondJSON(w, http.StatusCreated, t)
}

func (s *Server) handleGetTask(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	t, err := s.store.GetTask(id)
	if errors.Is(err, sql.ErrNoRows) {
		// A task that is not there is not a server fault, and the client turns
		// this back into sql.ErrNoRows so callers can tell "missing" from
		// "something broke" the same way they do against sqlite.
		http.Error(w, "Task not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	respondJSON(w, http.StatusOK, t)
}

func (s *Server) handleUpdateTask(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	var t models.Task
	if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	t.ID = id // Ensure ID matches URL

	if err := s.store.UpdateTask(t); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	respondJSON(w, http.StatusOK, t)
}

func (s *Server) handleDeleteTask(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	if err := s.store.DeleteTask(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleTaskDone(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	if err := s.store.MarkDone(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "done"})
}

func (s *Server) handleTaskStart(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	if err := s.store.StartTask(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "started"})
}

func (s *Server) handleTaskStop(w http.ResponseWriter, r *http.Request) {
	id := int64(-1)
	if r.URL.Query().Has("id") {
		var err error
		id, err = strconv.ParseInt(r.URL.Query().Get("id"), 10, 64)
		if err != nil {
			http.Error(w, "Invalid ID", http.StatusBadRequest)
			return
		}
	}
	if err := s.store.StopTask(id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "stopped"})
}

func (s *Server) handleListProjects(w http.ResponseWriter, r *http.Request) {
	projects, err := s.store.ListProjects()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	respondJSON(w, http.StatusOK, projects)
}

func (s *Server) handleListDependencies(w http.ResponseWriter, r *http.Request) {
	deps, err := s.store.GetDependencies()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	respondJSON(w, http.StatusOK, deps)
}

func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		http.Error(w, "Query parameter 'q' is required", http.StatusBadRequest)
		return
	}

	tasks, err := s.store.SearchTasks(query)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	respondJSON(w, http.StatusOK, tasks)
}

func (s *Server) handleAddDependency(w http.ResponseWriter, r *http.Request) {
	var req struct {
		BlockerID *int64 `json:"blocker_id"`
		BlockedID *int64 `json:"blocked_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.BlockerID == nil || req.BlockedID == nil {
		http.Error(w, "blocker_id and blocked_id are required", http.StatusBadRequest)
		return
	}
	if err := s.store.AddDependency(*req.BlockerID, *req.BlockedID); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, store.ErrInvalidDependency) {
			status = http.StatusBadRequest
		}
		http.Error(w, err.Error(), status)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func (s *Server) handleRemoveDependency(w http.ResponseWriter, r *http.Request) {
	blockerID, err := strconv.ParseInt(r.PathValue("blocker_id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid blocker ID", http.StatusBadRequest)
		return
	}
	blockedID, err := strconv.ParseInt(r.PathValue("blocked_id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid blocked ID", http.StatusBadRequest)
		return
	}

	if err := s.store.RemoveDependency(blockerID, blockedID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleGetProject(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	p, err := s.store.GetProject(name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	respondJSON(w, http.StatusOK, p)
}

func (s *Server) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	var p models.Project
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.store.CreateProject(p); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func (s *Server) handleUpdateProject(w http.ResponseWriter, r *http.Request) {
	oldName := r.PathValue("name")
	var p models.Project
	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := s.store.UpdateProject(oldName, p); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleDeleteProject(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := s.store.DeleteProject(name); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleBatchUpdateTasks(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IDs     []int64                `json:"ids"`
		Updates map[string]interface{} `json:"updates"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := s.store.BatchUpdateTasks(req.IDs, req.Updates); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func respondJSON(w http.ResponseWriter, status int, payload interface{}) {
	response, err := json.Marshal(payload)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(err.Error()))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(response)
}
