package server

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/aescoubas/tasc/internal/models"
	"github.com/aescoubas/tasc/internal/store"
)

type Server struct {
	store   store.Store
	mux     *http.ServeMux
	options Options
}

func NewServer(s store.Store) *Server {
	srv, _ := NewServerWithOptions(s, Options{})
	return srv
}

func NewServerWithOptions(s store.Store, options Options) (*Server, error) {
	if err := validateOptions(options); err != nil {
		return nil, err
	}
	options.AllowedOrigins = append([]string(nil), options.AllowedOrigins...)
	srv := &Server{
		store:   s,
		mux:     http.NewServeMux(),
		options: options,
	}
	srv.routes()
	return srv, nil
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !s.allowRequest(w, r) {
		return
	}

	// Simple logger middleware
	start := time.Now()
	log.Printf("%s %s", r.Method, r.URL.Path)
	if strings.HasPrefix(r.URL.Path, "/actors/") {
		s.serveActor(w, r)
	} else {
		s.mux.ServeHTTP(w, r)
	}
	log.Printf("Completed in %v", time.Since(start))
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /assignments", s.handleAssignments)
	s.mux.HandleFunc("POST /assignments", s.handleAssignments)
	s.mux.HandleFunc("GET /tasks", s.handleListTasks)
	s.mux.HandleFunc("POST /tasks", s.handleCreateTask)
	s.mux.HandleFunc("POST /tasks/with-links", s.handleCreateTaskWithLinks)
	s.mux.HandleFunc("POST /tasks/log", s.handleLogTask)
	s.mux.HandleFunc("GET /tasks/{id}", s.handleGetTask)
	s.mux.HandleFunc("PUT /tasks/{id}", s.handleUpdateTask)
	s.mux.HandleFunc("PUT /tasks/{id}/description", s.handleUpdateTaskDescription)
	s.mux.HandleFunc("DELETE /tasks/{id}", s.handleDeleteTask)

	s.mux.HandleFunc("POST /tasks/{id}/done", s.handleTaskDone)
	s.mux.HandleFunc("POST /tasks/{id}/vague", s.handleTaskVague)
	s.mux.HandleFunc("POST /tasks/{id}/start", s.handleTaskStart)
	s.mux.HandleFunc("POST /tasks/{id}/stop", s.handleStopTaskByID)
	s.mux.HandleFunc("POST /tasks/stop", s.handleTaskStop)
	s.mux.HandleFunc("GET /tasks/active", s.handleGetActiveTask)
	s.mux.HandleFunc("POST /tasks/batch-update", s.handleBatchUpdateTasks)
	s.mux.HandleFunc("POST /tasks/batch-update/priority", s.handleBatchUpdateTasks)
	s.mux.HandleFunc("POST /tasks/batch-update/keep-series", s.handleBatchUpdateTasks)
	s.mux.HandleFunc("POST /tasks/batch-update/with-links", s.handleBatchUpdateTasksWithLinks)

	s.mux.HandleFunc("GET /projects", s.handleListProjects)
	s.mux.HandleFunc("POST /projects", s.handleCreateProject)
	s.mux.HandleFunc("POST /projects/create", s.handleCreateProject)
	s.mux.HandleFunc("GET /projects/{name}", s.handleGetProject)
	s.mux.HandleFunc("PUT /projects/{name}", s.handleUpdateProject)
	s.mux.HandleFunc("DELETE /projects/{name}", s.handleDeleteProject)
	s.mux.HandleFunc("POST /projects/{name}/archive", s.handleArchiveProject)
	s.mux.HandleFunc("POST /projects/{name}/delete", s.handleDeleteProjectWithMode)

	s.mux.HandleFunc("GET /dependencies", s.handleListDependencies)
	s.mux.HandleFunc("POST /dependencies", s.handleAddDependency)
	s.mux.HandleFunc("DELETE /dependencies/{blocker_id}/{blocked_id}", s.handleRemoveDependency)

	s.mux.HandleFunc("GET /search", s.handleSearch)
}

func (s *Server) handleGetActiveTask(w http.ResponseWriter, r *http.Request) {
	t, err := s.requestStore(r).GetActiveTask()
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
	sortBy := q.Get("sort")
	if sortBy != "" && sortBy != "priority" {
		http.Error(w, "sort must be priority", http.StatusBadRequest)
		return
	}

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
	if excludeArchived := q.Get("exclude_archived_projects"); excludeArchived != "" {
		var err error
		filter.ExcludeArchivedProjects, err = strconv.ParseBool(excludeArchived)
		if err != nil {
			http.Error(w, "Invalid exclude_archived_projects: expected a boolean", http.StatusBadRequest)
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
			if err != nil {
				http.Error(w, "Invalid ID filter", http.StatusBadRequest)
				return
			}
			filter.IDs = append(filter.IDs, id)
		}
	}

	for _, item := range []struct {
		param string
		dest  **time.Time
	}{{"due_before", &filter.DueBefore}, {"due_after", &filter.DueAfter}} {
		if q.Has(item.param) {
			date, err := time.Parse(time.RFC3339, q.Get(item.param))
			if err != nil {
				http.Error(w, item.param+" must be an RFC3339 timestamp", http.StatusBadRequest)
				return
			}
			*item.dest = &date
		}
	}

	tasks, err := s.requestStore(r).ListTasks(filter)
	if err != nil {
		respondMutationError(w, err)
		return
	}
	if sortBy == "priority" {
		slices.SortFunc(tasks, func(a, b models.Task) int { return models.CompareTaskPriority(a, b, false) })
	}
	respondJSON(w, http.StatusOK, tasks)
}

func (s *Server) handleCreateTaskWithLinks(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Task  models.Task      `json:"task"`
		Links taskLinksRequest `json:"links"`
	}
	if err := decodeTaskRequest(r, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	links, err := req.Links.taskLinks()
	if err != nil {
		respondMutationError(w, err)
		return
	}
	id, added, err := s.requestStore(r).CreateTaskWithLinks(req.Task, links)
	if err != nil {
		respondMutationError(w, err)
		return
	}
	respondJSON(w, http.StatusCreated, map[string]any{"id": id, "links_added": added})
}

func (s *Server) handleCreateTask(w http.ResponseWriter, r *http.Request) {
	createTask(w, r, s.requestStore(r).CreateTask)
}

func (s *Server) handleLogTask(w http.ResponseWriter, r *http.Request) {
	createTask(w, r, s.requestStore(r).LogTask)
}

func createTask(w http.ResponseWriter, r *http.Request, create func(models.Task) (int64, error)) {
	var t models.Task
	if err := decodeTaskRequest(r, &t); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	id, err := create(t)
	if err != nil {
		respondMutationError(w, err)
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

	t, err := s.requestStore(r).GetTask(id)
	if err != nil {
		respondMutationError(w, err)
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
	if err := decodeTaskRequest(r, &t); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	t.ID = id // Ensure ID matches URL

	if err := s.requestStore(r).UpdateTask(t); err != nil {
		respondMutationError(w, err)
		return
	}
	// Completion owns timestamps and timer state; return the persisted result.
	t, err = s.requestStore(r).GetTask(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	respondJSON(w, http.StatusOK, t)
}

func (s *Server) handleUpdateTaskDescription(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}
	var edit struct {
		Expected    *string `json:"expected"`
		Description *string `json:"description"`
	}
	if err := decodeTaskRequest(r, &edit); err != nil || edit.Expected == nil || edit.Description == nil {
		http.Error(w, "expected and description must be strings", http.StatusBadRequest)
		return
	}
	if err := s.requestStore(r).UpdateTaskDescription(id, *edit.Expected, *edit.Description); err != nil {
		respondMutationError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleDeleteTask(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	if err := s.requestStore(r).DeleteTask(id); err != nil {
		respondMutationError(w, err)
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

	if err := s.requestStore(r).MarkDone(id); err != nil {
		respondMutationError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "done"})
}

func (s *Server) handleTaskVague(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}
	if err := s.requestStore(r).MarkVague(id); err != nil {
		respondMutationError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleTaskStart(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	if err := s.requestStore(r).StartTask(id); err != nil {
		respondMutationError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "started"})
}

func (s *Server) handleStopTaskByID(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}
	s.stopTask(w, r, &id)
}

func (s *Server) handleTaskStop(w http.ResponseWriter, r *http.Request) {
	var target *int64
	if r.URL.Query().Has("id") {
		id, err := strconv.ParseInt(r.URL.Query().Get("id"), 10, 64)
		if err != nil {
			http.Error(w, "Invalid ID", http.StatusBadRequest)
			return
		}
		target = &id
	}
	s.stopTask(w, r, target)
}

func (s *Server) stopTask(w http.ResponseWriter, r *http.Request, target *int64) {
	if err := s.requestStore(r).StopTask(target); err != nil {
		respondMutationError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "stopped"})
}

func (s *Server) handleListProjects(w http.ResponseWriter, r *http.Request) {
	projects, err := s.requestStore(r).ListProjects()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	respondJSON(w, http.StatusOK, projects)
}

func (s *Server) handleListDependencies(w http.ResponseWriter, r *http.Request) {
	deps, err := s.requestStore(r).GetDependencies()
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

	tasks, err := s.requestStore(r).SearchTasks(query)
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
	if err := decodeTaskRequest(r, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.BlockerID == nil || req.BlockedID == nil {
		http.Error(w, "blocker_id and blocked_id are required", http.StatusBadRequest)
		return
	}
	if err := s.requestStore(r).AddDependency(*req.BlockerID, *req.BlockedID); err != nil {
		respondMutationError(w, err)
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

	if err := s.requestStore(r).RemoveDependency(blockerID, blockedID); err != nil {
		respondMutationError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleGetProject(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	p, err := s.requestStore(r).GetProject(name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	respondJSON(w, http.StatusOK, p)
}

func (s *Server) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	var request struct {
		models.Project
		Options store.ProjectCreateOptions `json:"options"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.requestStore(r).CreateProject(request.Project, request.Options); err != nil {
		respondMutationError(w, err)
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

	if err := s.requestStore(r).UpdateProject(oldName, p); err != nil {
		respondMutationError(w, err)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleDeleteProject(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if err := s.requestStore(r).DeleteProject(name); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleArchiveProject(w http.ResponseWriter, r *http.Request) {
	if err := s.requestStore(r).ArchiveProject(r.PathValue("name")); err != nil {
		respondMutationError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleDeleteProjectWithMode(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Mode store.ProjectDeleteMode `json:"mode"`
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	switch request.Mode {
	case store.ProjectDeleteEmpty, store.ProjectDeleteUnlink, store.ProjectDeleteCascade:
	default:
		http.Error(w, "invalid project deletion mode", http.StatusBadRequest)
		return
	}
	result, err := s.requestStore(r).DeleteProjectWithMode(r.PathValue("name"), request.Mode)
	if err != nil {
		respondMutationError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, result)
}

func (s *Server) handleBatchUpdateTasks(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IDs     []*int64       `json:"ids"`
		Updates map[string]any `json:"updates"`
	}
	if err := decodeTaskRequest(r, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	ids, err := taskIDs(req.IDs)
	if err != nil {
		respondMutationError(w, err)
		return
	}
	if err := s.requestStore(r).BatchUpdateTasks(ids, req.Updates); err != nil {
		respondMutationError(w, err)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleBatchUpdateTasksWithLinks(w http.ResponseWriter, r *http.Request) {
	var req struct {
		IDs     []*int64         `json:"ids"`
		Updates map[string]any   `json:"updates"`
		Links   taskLinksRequest `json:"links"`
	}
	if err := decodeTaskRequest(r, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	ids, err := taskIDs(req.IDs)
	if err != nil {
		respondMutationError(w, err)
		return
	}
	links, err := req.Links.taskLinks()
	if err != nil {
		respondMutationError(w, err)
		return
	}
	added, err := s.requestStore(r).BatchUpdateTasksWithLinks(ids, req.Updates, links)
	if err != nil {
		respondMutationError(w, err)
		return
	}
	respondJSON(w, http.StatusOK, map[string]int{"links_added": added})
}

// Decode through pointers so a JSON null cannot address a real task with ID 0.
func taskIDs(values []*int64) ([]int64, error) {
	ids := make([]int64, 0, len(values))
	for _, id := range values {
		if id == nil {
			return nil, fmt.Errorf("%w: task IDs must be integers", store.ErrInvalidTask)
		}
		ids = append(ids, *id)
	}
	return ids, nil
}

type taskLinksRequest struct {
	DependsOn []*int64 `json:"depends_on"`
	Blocks    []*int64 `json:"blocks"`
}

func (r taskLinksRequest) taskLinks() (store.TaskLinks, error) {
	dependsOn, err := taskIDs(r.DependsOn)
	if err != nil {
		return store.TaskLinks{}, err
	}
	blocks, err := taskIDs(r.Blocks)
	return store.TaskLinks{DependsOn: dependsOn, Blocks: blocks}, err
}

func decodeTaskRequest(r *http.Request, dst any) error {
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(dst); err != nil {
		return err
	}
	if err := decoder.Decode(new(json.RawMessage)); err != io.EOF {
		return fmt.Errorf("request body must contain exactly one JSON value")
	}
	return nil
}

func respondMutationError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	if errors.Is(err, sql.ErrNoRows) {
		status = http.StatusNotFound
	} else if errors.Is(err, store.ErrInvalidDependency) || errors.Is(err, store.ErrInvalidTask) || errors.Is(err, store.ErrInvalidCompletion) || errors.Is(err, store.ErrInvalidUserPriority) {
		status = http.StatusBadRequest
	} else if errors.Is(err, store.ErrTaskDescriptionConflict) || errors.Is(err, store.ErrProjectNotEmpty) || err.Error() == store.AssignmentConflict {
		status = http.StatusConflict
	}
	http.Error(w, err.Error(), status)
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
