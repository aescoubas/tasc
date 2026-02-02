package sqlite

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/aescoubas/tasc/internal/models"
	"github.com/aescoubas/tasc/internal/store"
)

type SQLiteStore struct {
	db *sql.DB
}

func NewSQLiteStore(db *sql.DB) *SQLiteStore {
	return &SQLiteStore{db: db}
}

func (s *SQLiteStore) ensureProjectExists(name string) error {
	if name == "" {
		return nil
	}
	// Insert if new, or update status to active if existing
	// SQLite UPSERT syntax:
	query := `INSERT INTO projects (name, description, status) VALUES (?, '', 'active') 
	          ON CONFLICT(name) DO UPDATE SET status = 'active'`
	_, err := s.db.Exec(query, name)
	return err
}

func (s *SQLiteStore) checkProjectCompletion(name string) error {
	if name == "" {
		return nil
	}
	// Check if any pending tasks exist
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM tasks WHERE project = ? AND status NOT IN ('done', 'deleted', 'undefined')", name).Scan(&count)
	if err != nil {
		return err
	}

	if count == 0 {
		// Mark archive
		_, err := s.db.Exec("UPDATE projects SET status = 'archived' WHERE name = ?", name)
		return err
	}
	return nil
}

func (s *SQLiteStore) CreateTask(t models.Task) (int64, error) {
	if err := s.ensureProjectExists(t.Project); err != nil {
		return 0, err
	}

	query := `INSERT INTO tasks (description, project, due_at, scheduled_at, estimate, recurrence, created_at) VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`
	stmt, err := s.db.Prepare(query)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()

	res, err := stmt.Exec(t.Description, t.Project, t.DueAt, t.ScheduledAt, t.Estimate, t.Recurrence)
	if err != nil {
		return 0, err
	}

	return res.LastInsertId()
}

func (s *SQLiteStore) GetTask(id int64) (models.Task, error) {
	var t models.Task
	var project sql.NullString
	var dueAt sql.NullTime
	var scheduledAt sql.NullTime
	var estimate sql.NullString
	var recurrence sql.NullString
	var activeStart sql.NullTime
	var timeSpent sql.NullInt64
	var completedAt sql.NullTime

	query := `SELECT 
		id, description, project, status, created_at, 
		completed_at, due_at, scheduled_at, estimate, 
		recurrence, active_start, time_spent, reschedule_count 
	FROM tasks WHERE id = ?`

	row := s.db.QueryRow(query, id)
	err := row.Scan(
		&t.ID, &t.Description, &project, &t.Status, &t.CreatedAt,
		&completedAt, &dueAt, &scheduledAt, &estimate,
		&recurrence, &activeStart, &timeSpent, &t.RescheduleCount,
	)

	if err != nil {
		return t, err
	}

	t.Project = project.String
	t.Estimate = estimate.String
	t.Recurrence = recurrence.String
	if dueAt.Valid {
		t.DueAt = &dueAt.Time
	}
	if scheduledAt.Valid {
		t.ScheduledAt = &scheduledAt.Time
	}
	if activeStart.Valid {
		t.ActiveStart = &activeStart.Time
	}
	if timeSpent.Valid {
		t.TimeSpent = timeSpent.Int64
	}
	if completedAt.Valid {
		t.CompletedAt = &completedAt.Time
	}
	
	// Check is_blocked
	// This is a separate query in GetTask generally, or could be joined. 
	// For simplicity, let's leave IsBlocked false here or do a quick check?
	// Show command does separate queries for detailed block info.
	// List command does EXISTS subquery.
	// Let's match List command behavior if possible, or just ignore for single Get unless requested.
	// t.IsBlocked logic usually happens in List.
	return t, nil
}

func (s *SQLiteStore) UpdateTask(t models.Task) error {
	// Fetch old project
	var oldProject sql.NullString
	err := s.db.QueryRow("SELECT project FROM tasks WHERE id = ?", t.ID).Scan(&oldProject)
	if err != nil {
		return err
	}

	if err := s.ensureProjectExists(t.Project); err != nil {
		return err
	}

	query := `UPDATE tasks SET description = ?, project = ?, status = ?, due_at = ?, scheduled_at = ?, estimate = ?, recurrence = ?, reschedule_count = ? WHERE id = ?`
	_, err = s.db.Exec(query, t.Description, t.Project, t.Status, t.DueAt, t.ScheduledAt, t.Estimate, t.Recurrence, t.RescheduleCount, t.ID)
	if err != nil {
		return err
	}

	if oldProject.Valid && oldProject.String != t.Project {
		if err := s.checkProjectCompletion(oldProject.String); err != nil {
			return err
		}
	}
	// Check new project too (in case the task is done/deleted, or it was the only task)
	if err := s.checkProjectCompletion(t.Project); err != nil {
		return err
	}
	return nil
}

func (s *SQLiteStore) BatchUpdateTasks(ids []int64, updates map[string]interface{}) error {
	if len(ids) == 0 {
		return nil
	}
	if len(updates) == 0 {
		return nil
	}

	// 1. Identify affected projects (Before update)
	affectedProjects := make(map[string]bool)
	
	// Helper to collect projects
	placeholders := make([]string, len(ids))
	queryArgs := make([]interface{}, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		queryArgs[i] = id
	}
	
	rows, err := s.db.Query(fmt.Sprintf("SELECT DISTINCT project FROM tasks WHERE id IN (%s)", strings.Join(placeholders, ",")), queryArgs...)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var p sql.NullString
			if err := rows.Scan(&p); err == nil && p.Valid {
				affectedProjects[p.String] = true
			}
		}
	}

	// 2. Ensure new project exists (if setting project)
	if pVal, ok := updates["project"]; ok {
		if pStr, ok := pVal.(string); ok {
			if err := s.ensureProjectExists(pStr); err != nil {
				return err
			}
			affectedProjects[pStr] = true
		}
	}

	// 3. Perform Update
	query := "UPDATE tasks SET "
	var args []interface{}
	var sets []string

	for k, v := range updates {
		// Whitelist fields
		switch k {
		case "project", "status", "estimate", "recurrence", "description":
			sets = append(sets, fmt.Sprintf("%s = ?", k))
			args = append(args, v)
		case "due_at", "scheduled_at", "completed_at":
			sets = append(sets, fmt.Sprintf("%s = ?", k))
			args = append(args, v)
		}
	}

	if len(sets) == 0 {
		return nil
	}

	query += strings.Join(sets, ", ")
	query += " WHERE id IN (" + strings.Join(placeholders, ", ") + ")"
	args = append(args, queryArgs...)

	_, err = s.db.Exec(query, args...)
	if err != nil {
		return err
	}

	// 4. Check completion for all affected projects
	for p := range affectedProjects {
		s.checkProjectCompletion(p)
	}

	return nil
}

func (s *SQLiteStore) DeleteTask(id int64) error {
	var project sql.NullString
	s.db.QueryRow("SELECT project FROM tasks WHERE id = ?", id).Scan(&project)

	query := `UPDATE tasks SET status = 'deleted' WHERE id = ?`
	_, err := s.db.Exec(query, id)
	if err != nil {
		return err
	}

	if project.Valid {
		return s.checkProjectCompletion(project.String)
	}
	return nil
}

func (s *SQLiteStore) ListTasks(filter store.TaskFilter) ([]models.Task, error) {
	baseQuery := `
		SELECT 
			id, description, project, status, created_at, due_at, scheduled_at, estimate, active_start, time_spent, reschedule_count,
			EXISTS(SELECT 1 FROM task_dependencies WHERE blocked_id = tasks.id) as is_blocked
		FROM tasks 
		WHERE 1=1
	`
	var args []interface{}

	if len(filter.Status) > 0 {
		placeholders := make([]string, len(filter.Status))
		for i, st := range filter.Status {
			placeholders[i] = "?"
			args = append(args, st)
		}
		baseQuery += fmt.Sprintf(" AND status IN (%s)", strings.Join(placeholders, ","))
	} else if !filter.IncludeDeleted {
		baseQuery += " AND status NOT IN ('done', 'deleted', 'undefined')"
	}

	if len(filter.Projects) > 0 {
		placeholders := make([]string, len(filter.Projects))
		for i, p := range filter.Projects {
			placeholders[i] = "?"
			args = append(args, p)
		}
		baseQuery += fmt.Sprintf(" AND project IN (%s)", strings.Join(placeholders, ","))
	}

	if len(filter.IDs) > 0 {
		placeholders := make([]string, len(filter.IDs))
		for i, id := range filter.IDs {
			placeholders[i] = "?"
			args = append(args, id)
		}
		baseQuery += fmt.Sprintf(" AND id IN (%s)", strings.Join(placeholders, ","))
	}

	if filter.DueBefore != nil {
		baseQuery += " AND due_at < ?"
		args = append(args, filter.DueBefore)
	}
	if filter.DueAfter != nil {
		baseQuery += " AND due_at > ?"
		args = append(args, filter.DueAfter)
	}
	if filter.ScheduledBefore != nil {
		baseQuery += " AND scheduled_at < ?"
		args = append(args, filter.ScheduledBefore)
	}
	if filter.ScheduledAfter != nil {
		baseQuery += " AND scheduled_at > ?"
		args = append(args, filter.ScheduledAfter)
	}

	rows, err := s.db.Query(baseQuery, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []models.Task
	for rows.Next() {
		var t models.Task
		var project sql.NullString
		var dueAt sql.NullTime
		var scheduledAt sql.NullTime
		var estimate sql.NullString
		var activeStart sql.NullTime
		var timeSpent sql.NullInt64
		var isBlocked bool

		err := rows.Scan(&t.ID, &t.Description, &project, &t.Status, &t.CreatedAt, &dueAt, &scheduledAt, &estimate, &activeStart, &timeSpent, &t.RescheduleCount, &isBlocked)
		if err != nil {
			continue
		}
		t.Project = project.String
		t.IsBlocked = isBlocked

		if dueAt.Valid {
			t.DueAt = &dueAt.Time
		}
		if scheduledAt.Valid {
			t.ScheduledAt = &scheduledAt.Time
		}
		if estimate.Valid {
			t.Estimate = estimate.String
		}
		if activeStart.Valid {
			t.ActiveStart = &activeStart.Time
		}
		if timeSpent.Valid {
			t.TimeSpent = timeSpent.Int64
		}
		tasks = append(tasks, t)
	}
	return tasks, nil
}

func (s *SQLiteStore) MarkDone(id int64) error {
	var project sql.NullString
	s.db.QueryRow("SELECT project FROM tasks WHERE id = ?", id).Scan(&project)

	queryUpdate := `UPDATE tasks SET status = 'done', completed_at = CURRENT_TIMESTAMP WHERE id = ?`
	_, err := s.db.Exec(queryUpdate, id)
	if err != nil {
		return err
	}
	
	if project.Valid {
		return s.checkProjectCompletion(project.String)
	}
	return nil
}

func (s *SQLiteStore) StartTask(id int64) error {
	// Logic from cmd/start.go
	// 1. Stop active
	// 2. Start new
	
	// Transaction? Ideally yes.
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Stop any active
	rows, err := tx.Query("SELECT id, active_start FROM tasks WHERE active_start IS NOT NULL")
	if err != nil {
		return err
	}
	
	var activeID int64
	var activeStart time.Time
	taskFound := false
	
	for rows.Next() {
		rows.Scan(&activeID, &activeStart)
		taskFound = true
		if activeID == id {
			rows.Close()
			return fmt.Errorf("task %d is already active", id)
		}
	}
	rows.Close()

	now := time.Now()
	if taskFound {
		elapsed := int64(now.Sub(activeStart).Seconds())
		_, err = tx.Exec("UPDATE tasks SET active_start = NULL, time_spent = time_spent + ? WHERE id = ?", elapsed, activeID)
		if err != nil {
			return err
		}
	}

	_, err = tx.Exec("UPDATE tasks SET active_start = ?, status = 'ongoing' WHERE id = ?", now, id)
	if err != nil {
		return err
	}
	
	return tx.Commit()
}

func (s *SQLiteStore) StopTask(id int64) error {
	// Logic from cmd/stop.go
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	query := "SELECT id, active_start FROM tasks WHERE active_start IS NOT NULL"
	rows, err := tx.Query(query)
	if err != nil {
		return err
	}

	var activeID int64
	var activeStart time.Time
	found := false

	for rows.Next() {
		rows.Scan(&activeID, &activeStart)
		if id != -1 && id != 0 && activeID != id {
			continue
		}
		found = true
		break
	}
	rows.Close()

	if !found {
		return fmt.Errorf("no active task found")
	}

	now := time.Now()
	elapsed := int64(now.Sub(activeStart).Seconds())

	_, err = tx.Exec("UPDATE tasks SET active_start = NULL, time_spent = time_spent + ? WHERE id = ?", elapsed, activeID)
	if err != nil {
		return err
	}

	return tx.Commit()
}

func (s *SQLiteStore) GetActiveTask() (*models.Task, error) {
	query := `SELECT 
		id, description, project, status, created_at, 
		completed_at, due_at, scheduled_at, estimate, 
		recurrence, active_start, time_spent, reschedule_count 
	FROM tasks WHERE active_start IS NOT NULL LIMIT 1`

	var t models.Task
	var project sql.NullString
	var dueAt sql.NullTime
	var scheduledAt sql.NullTime
	var estimate sql.NullString
	var recurrence sql.NullString
	var activeStart sql.NullTime
	var timeSpent sql.NullInt64
	var completedAt sql.NullTime

	row := s.db.QueryRow(query)
	err := row.Scan(
		&t.ID, &t.Description, &project, &t.Status, &t.CreatedAt,
		&completedAt, &dueAt, &scheduledAt, &estimate,
		&recurrence, &activeStart, &timeSpent, &t.RescheduleCount,
	)

	if err == sql.ErrNoRows {
		return nil, nil // No active task
	}
	if err != nil {
		return nil, err
	}

	t.Project = project.String
	t.Estimate = estimate.String
	t.Recurrence = recurrence.String
	if dueAt.Valid { t.DueAt = &dueAt.Time }
	if scheduledAt.Valid { t.ScheduledAt = &scheduledAt.Time }
	if activeStart.Valid { t.ActiveStart = &activeStart.Time }
	if timeSpent.Valid { t.TimeSpent = timeSpent.Int64 }
	if completedAt.Valid { t.CompletedAt = &completedAt.Time }

	return &t, nil
}

func (s *SQLiteStore) AddDependency(blockerID, blockedID int64) error {
	query := `INSERT INTO task_dependencies (blocker_id, blocked_id) VALUES (?, ?)`
	_, err := s.db.Exec(query, blockerID, blockedID)
	return err
}

func (s *SQLiteStore) GetDependencies() ([]models.TaskDependency, error) {
	rows, err := s.db.Query("SELECT blocker_id, blocked_id FROM task_dependencies")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var deps []models.TaskDependency
	for rows.Next() {
		var d models.TaskDependency
		if err := rows.Scan(&d.BlockerID, &d.BlockedID); err == nil {
			deps = append(deps, d)
		}
	}
	return deps, nil
}

func (s *SQLiteStore) ListProjects() ([]models.Project, error) {
	query := `
		SELECT name, description, parent, status, due_at, created_at
		FROM projects ORDER BY name
	`
	rows, err := s.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var projects []models.Project
	for rows.Next() {
		var p models.Project
		var desc, parent, status sql.NullString
		var due sql.NullTime
		
		err := rows.Scan(&p.Name, &desc, &parent, &status, &due, &p.CreatedAt)
		if err != nil {
			continue
		}
		p.Description = desc.String
		p.Parent = parent.String
		p.Status = models.ProjectStatus(status.String)
		if due.Valid {
			p.DueAt = &due.Time
		}
		projects = append(projects, p)
	}
	return projects, nil
}

func (s *SQLiteStore) GetProject(name string) (models.Project, error) {
	query := `SELECT name, description, parent, status, due_at, created_at FROM projects WHERE name = ?`
	row := s.db.QueryRow(query, name)
	
	var p models.Project
	var desc, parent, status sql.NullString
	var due sql.NullTime

	err := row.Scan(&p.Name, &desc, &parent, &status, &due, &p.CreatedAt)
	if err != nil {
		return p, err
	}

	p.Description = desc.String
	p.Parent = parent.String
	p.Status = models.ProjectStatus(status.String)
	if due.Valid {
		p.DueAt = &due.Time
	}
	return p, nil
}

func (s *SQLiteStore) CreateProject(p models.Project) error {
	query := `INSERT INTO projects (name, description, parent, due_at, status) VALUES (?, ?, ?, ?, ?)`
	var parent *string
	if p.Parent != "" {
		parent = &p.Parent
	}
	_, err := s.db.Exec(query, p.Name, p.Description, parent, p.DueAt, p.Status)
	return err
}

func (s *SQLiteStore) UpdateProject(oldName string, p models.Project) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Update project fields
	var parent *string
	if p.Parent != "" {
		parent = &p.Parent
	}

	query := `UPDATE projects SET name = ?, description = ?, parent = ?, status = ?, due_at = ? WHERE name = ?`
	_, err = tx.Exec(query, p.Name, p.Description, parent, p.Status, p.DueAt, oldName)
	if err != nil {
		return err
	}

	// If name changed, update references
	if oldName != p.Name {
		// Update tasks
		_, err = tx.Exec("UPDATE tasks SET project = ? WHERE project = ?", p.Name, oldName)
		if err != nil {
			return err
		}
		// Update child projects
		_, err = tx.Exec("UPDATE projects SET parent = ? WHERE parent = ?", p.Name, oldName)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (s *SQLiteStore) DeleteProject(name string) error {
	// Simple delete. Business logic for cascade/unlink should be handled by caller or extended method.
	// We assume caller has handled tasks if necessary.
	_, err := s.db.Exec("DELETE FROM projects WHERE name = ?", name)
	return err
}

func (s *SQLiteStore) SearchTasks(queryStr string) ([]models.Task, error) {
	query := `
		SELECT id, description, project, status, created_at 
		FROM tasks 
		WHERE id IN (
			SELECT rowid FROM tasks_fts WHERE tasks_fts MATCH ? ORDER BY rank
		)
	`
	rows, err := s.db.Query(query, queryStr)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []models.Task
	for rows.Next() {
		var t models.Task
		var project sql.NullString
		err := rows.Scan(&t.ID, &t.Description, &project, &t.Status, &t.CreatedAt)
		if err != nil {
			continue
		}
		t.Project = project.String
		tasks = append(tasks, t)
	}
	return tasks, nil
}
