package sqlite

import (
	"database/sql"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/aescoubas/tasc/internal/models"
	"github.com/aescoubas/tasc/internal/store"
)

type SQLiteStore struct {
	db    *sql.DB
	actor string
	clock leaseSource
}

func NewSQLiteStore(db *sql.DB) *SQLiteStore {
	return &SQLiteStore{db: db, clock: leaseClock}
}

func (s *SQLiteStore) generateUniqueShortName(name string) (string, error) {
	return generateUniqueShortName(s.db, name)
}

func generateUniqueShortName(q taskQueries, name string) (string, error) {
	clean := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			return r
		}
		if r >= 'A' && r <= 'Z' {
			return r + 32 // to lower
		}
		return -1
	}, name)

	if len(clean) == 0 {
		clean = "unk"
	}

	candidates := []string{}

	// If short, try as is
	if len(clean) <= 3 {
		candidates = append(candidates, clean)
	} else {
		// Try prefix
		candidates = append(candidates, clean[:3])
	}
	if len(clean) >= 4 {
		candidates = append(candidates, clean[:4])
	}
	// Fallback numbers
	prefix := clean[:min(len(clean), 2)]
	for i := 1; i <= 20; i++ {
		candidates = append(candidates, fmt.Sprintf("%s%d", prefix, i))
	}

	for _, cand := range candidates {
		var exists int
		err := q.QueryRow("SELECT COUNT(*) FROM projects WHERE short_name = ?", cand).Scan(&exists)
		if err != nil {
			return "", err
		}
		if exists == 0 {
			return cand, nil
		}
	}

	// Ultimate fallback
	return fmt.Sprintf("%s%d", prefix, time.Now().Unix()%1000), nil
}

func (s *SQLiteStore) ensureProjectExists(name string) error {
	return ensureProjectExists(s.db, name)
}

func ensureProjectExists(q taskQueries, name string) error {
	if name == "" {
		return nil
	}

	// Check if exists first to avoid generating short name unnecessarily
	var exists int
	err := q.QueryRow("SELECT COUNT(*) FROM projects WHERE name = ?", name).Scan(&exists)
	if err != nil {
		return err
	}
	if exists > 0 {
		// Just ensure it is active
		_, err := q.Exec("UPDATE projects SET status = 'active' WHERE name = ?", name)
		return err
	}

	// Create new
	shortName, err := generateUniqueShortName(q, name)
	if err != nil {
		return err
	}

	query := `INSERT INTO projects (name, short_name, description, status) VALUES (?, ?, '', 'active')`
	_, err = q.Exec(query, name, shortName)
	return err
}

func (s *SQLiteStore) checkProjectCompletion(name string) error {
	return checkProjectCompletion(s.db, name)
}

func checkProjectCompletion(q taskQueries, name string) error {
	if name == "" {
		return nil
	}
	// Check if any pending tasks exist
	var count int
	err := q.QueryRow("SELECT COUNT(*) FROM tasks WHERE project = ? AND status NOT IN ('done', 'deleted', 'undefined')", name).Scan(&count)
	if err != nil {
		return err
	}

	if count == 0 {
		// Mark archive
		_, err := q.Exec("UPDATE projects SET status = 'archived' WHERE name = ?", name)
		return err
	}
	return nil
}

func (s *SQLiteStore) CreateTask(t models.Task) (int64, error) {
	if t.UserPriority != nil {
		if err := store.ValidateUserPriority(*t.UserPriority); err != nil {
			return 0, err
		}
	}
	if err := s.ensureProjectExists(t.Project); err != nil {
		return 0, err
	}

	query := `INSERT INTO tasks (title, description, project, due_at, schedule_block, estimate, recurrence, user_priority, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`
	stmt, err := s.db.Prepare(query)
	if err != nil {
		return 0, err
	}
	defer stmt.Close()

	res, err := stmt.Exec(t.Title, t.Description, t.Project, t.DueAt, t.ScheduleBlock, t.Estimate, t.Recurrence, t.UserPriority)
	if err != nil {
		return 0, err
	}

	return res.LastInsertId()
}

func (s *SQLiteStore) LogTask(t models.Task) (int64, error) {
	if t.UserPriority != nil {
		if err := store.ValidateUserPriority(*t.UserPriority); err != nil {
			return 0, err
		}
	}
	// Logging records past work without reopening or creating its project.
	res, err := s.db.Exec(`INSERT INTO tasks
		(title, project, due_at, estimate, user_priority, status, created_at, completed_at)
		VALUES (?, ?, ?, ?, ?, 'done', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		t.Title, t.Project, t.DueAt, t.Estimate, t.UserPriority)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *SQLiteStore) MarkVague(id int64) error {
	tx, err := s.beginWriter()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := requireUpdatedRow(tx.Exec("UPDATE tasks SET status = ? WHERE id = ?", models.StatusUndefined, id)); err != nil {
		return err
	}
	return tx.Commit()
}

func requireUpdatedRow(result sql.Result, err error) error {
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err == nil && count == 0 {
		return sql.ErrNoRows
	}
	return err
}

func (s *SQLiteStore) GetTask(id int64) (models.Task, error) {
	return s.readTask(s.db, "WHERE tasks.id=?", id)
}

func (s *SQLiteStore) readTask(q taskQueries, selection string, args ...any) (models.Task, error) {
	var t models.Task
	var project sql.NullString
	var description sql.NullString
	var dueAt sql.NullTime
	var scheduleBlock sql.NullString
	var estimate sql.NullString
	var recurrence sql.NullString
	var activeStart sql.NullTime
	var timeSpent sql.NullInt64
	var completedAt sql.NullTime

	query := `SELECT 
		tasks.id, title, description, project, status, created_at,
		completed_at, due_at, schedule_block, estimate, 
		recurrence, active_start, time_spent, reschedule_count, user_priority
	` + assignmentColumns + "FROM tasks" + assignmentJoin + selection

	row := q.QueryRow(query, args...)
	a, err := scanAssignment(row, s.clock, &leaseTime{},
		&t.ID, &t.Title, &description, &project, &t.Status, &t.CreatedAt,
		&completedAt, &dueAt, &scheduleBlock, &estimate,
		&recurrence, &activeStart, &timeSpent, &t.RescheduleCount, &t.UserPriority,
	)

	if err != nil {
		return t, err
	}

	t.Assignment = a
	t.Description = description.String
	t.Project = project.String
	t.ScheduleBlock = scheduleBlock.String
	t.Estimate = estimate.String
	t.Recurrence = recurrence.String
	if dueAt.Valid {
		t.DueAt = &dueAt.Time
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

	return t, nil
}

func (s *SQLiteStore) UpdateTask(t models.Task) error {
	if t.Status == models.StatusDone {
		return s.completeTasks([]int64{t.ID}, map[string]interface{}{
			"title": t.Title, "description": t.Description, "project": t.Project,
			"due_at": t.DueAt, "schedule_block": t.ScheduleBlock, "estimate": t.Estimate,
			"recurrence": t.Recurrence, "reschedule_count": t.RescheduleCount,
		}, true)
	}
	tx, err := s.beginWriter()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// Fetch old project
	var oldProject sql.NullString
	err = tx.QueryRow("SELECT project FROM tasks WHERE id = ?", t.ID).Scan(&oldProject)
	if err != nil {
		return err
	}

	if err := ensureProjectExists(tx, t.Project); err != nil {
		return err
	}

	query := `UPDATE tasks SET title = ?, description = ?, project = ?, status = ?, due_at = ?, schedule_block = ?, estimate = ?, recurrence = ?, reschedule_count = ? WHERE id = ?`
	_, err = tx.Exec(query, t.Title, t.Description, t.Project, t.Status, t.DueAt, t.ScheduleBlock, t.Estimate, t.Recurrence, t.RescheduleCount, t.ID)
	if err != nil {
		return err
	}

	if oldProject.Valid && oldProject.String != t.Project {
		if err := checkProjectCompletion(tx, oldProject.String); err != nil {
			return err
		}
	}
	// Check new project too (in case the task is done/deleted, or it was the only task)
	if err := checkProjectCompletion(tx, t.Project); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *SQLiteStore) UpdateTaskDescription(id int64, expected, description string) error {
	tx, err := s.beginWriter()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// Check and edit in one statement so writers on other connections cannot
	// slip in between them. Legacy NULL descriptions read back as empty text.
	result, err := tx.Exec(`UPDATE tasks SET description = ?
		WHERE id = ? AND status <> 'deleted' AND COALESCE(description, '') = ?`, description, id, expected)
	if err != nil {
		return err
	}
	if changed, err := result.RowsAffected(); err != nil {
		return err
	} else if changed == 0 {
		return store.ErrTaskDescriptionConflict
	}
	return tx.Commit()
}

func (s *SQLiteStore) BatchUpdateTasks(ids []int64, updates map[string]interface{}) error {
	if len(ids) == 0 {
		return nil
	}
	if len(updates) == 0 {
		return nil
	}
	userPriority, hasPriority := updates["user_priority"]
	if hasPriority {
		if err := store.ValidateUserPriority(userPriority); err != nil {
			return err
		}
	}
	if status, ok := updates["status"]; ok && fmt.Sprint(status) == string(models.StatusDone) {
		return s.completeTasks(ids, updates, false)
	}

	tx, err := s.beginWriter()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if keep, ok := updates["keep_series"]; ok {
		if keep != true || len(ids) != 1 || (updates["status"] != "deleted" && updates["recurrence"] != "") {
			return fmt.Errorf("%w: keeping a series requires deleting or detaching one instance", store.ErrInvalidCompletion)
		}
		task, err := s.readTask(tx, "WHERE tasks.id=?", ids[0])
		if err != nil {
			return err
		}
		if task.Status != models.StatusDeleted && task.Status != models.StatusDone {
			if err := createSuccessor(tx, task, time.Now()); err != nil {
				return err
			}
		}
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
	if hasPriority && len(updates) == 1 {
		if err := requireUpdatedRow(tx.Exec("UPDATE tasks SET user_priority = ? WHERE id IN ("+
			strings.Join(placeholders, ",")+")", append([]interface{}{userPriority}, queryArgs...)...)); err != nil {
			return err
		}
		return tx.Commit()
	}

	rows, err := tx.Query(fmt.Sprintf("SELECT DISTINCT project FROM tasks WHERE id IN (%s)", strings.Join(placeholders, ",")), queryArgs...)
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
			if err := ensureProjectExists(tx, pStr); err != nil {
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
		case "project", "status", "estimate", "recurrence", "title", "description", "schedule_block", "user_priority":
			sets = append(sets, fmt.Sprintf("%s = ?", k))
			args = append(args, v)
		case "due_at", "completed_at":
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

	_, err = tx.Exec(query, args...)
	if err != nil {
		return err
	}

	// 4. Check completion for all affected projects
	for p := range affectedProjects {
		if err := checkProjectCompletion(tx, p); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (s *SQLiteStore) DeleteTask(id int64) error {
	tx, err := s.beginWriter()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var project sql.NullString
	tx.QueryRow("SELECT project FROM tasks WHERE id = ?", id).Scan(&project)

	query := `UPDATE tasks SET status = 'deleted' WHERE id = ?`
	_, err = tx.Exec(query, id)
	if err != nil {
		return err
	}

	if project.Valid {
		if err := checkProjectCompletion(tx, project.String); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *SQLiteStore) ListTasks(filter store.TaskFilter) ([]models.Task, error) {
	baseQuery := `
		SELECT 
			tasks.id, title, description, project, status, created_at, completed_at, due_at, schedule_block, estimate, recurrence, active_start, time_spent, reschedule_count,
			user_priority, is_blocked` + assignmentColumns + `FROM (
			SELECT tasks.*, status NOT IN ('done', 'deleted') AND EXISTS(
				SELECT 1 FROM task_dependencies td
				JOIN tasks blocker ON blocker.id = td.blocker_id
				WHERE td.blocked_id = tasks.id AND blocker.status NOT IN ('done', 'deleted')
			) AS is_blocked
			FROM tasks
		) AS tasks` + assignmentJoin + `
		WHERE 1=1
	`
	var args []interface{}

	if len(filter.Status) > 0 {
		placeholders := make([]string, len(filter.Status))
		for i, st := range filter.Status {
			placeholders[i] = "?"
			args = append(args, st)
		}
		// Match the displayed status without replacing the saved workflow status.
		baseQuery += fmt.Sprintf(" AND CASE WHEN is_blocked THEN 'blocked' ELSE status END IN (%s)", strings.Join(placeholders, ","))
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
	if filter.ExcludeArchivedProjects {
		baseQuery += ` AND NOT EXISTS (
			SELECT 1 FROM projects
			WHERE name = NULLIF(tasks.project, '') AND status = ?
		)`
		args = append(args, models.ProjectStatusArchived)
	}

	if len(filter.IDs) > 0 {
		placeholders := make([]string, len(filter.IDs))
		for i, id := range filter.IDs {
			placeholders[i] = "?"
			args = append(args, id)
		}
		baseQuery += fmt.Sprintf(" AND tasks.id IN (%s)", strings.Join(placeholders, ","))
	}

	if filter.DueBefore != nil {
		baseQuery += " AND due_at < ?"
		args = append(args, filter.DueBefore)
	}
	if filter.DueAfter != nil {
		baseQuery += " AND due_at > ?"
		args = append(args, filter.DueAfter)
	}
	rows, err := s.db.Query(baseQuery, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []models.Task
	var stamp leaseTime
	for rows.Next() {
		var t models.Task
		var project sql.NullString
		var description sql.NullString
		var completedAt sql.NullTime
		var dueAt sql.NullTime
		var scheduleBlock sql.NullString
		var estimate sql.NullString
		var recurrence sql.NullString
		var activeStart sql.NullTime
		var timeSpent sql.NullInt64
		var isBlocked bool

		a, err := scanAssignment(rows, s.clock, &stamp, &t.ID, &t.Title, &description, &project, &t.Status, &t.CreatedAt, &completedAt, &dueAt, &scheduleBlock, &estimate, &recurrence, &activeStart, &timeSpent, &t.RescheduleCount, &t.UserPriority, &isBlocked)
		if err != nil {
			return nil, err
		}
		t.Description = description.String
		t.Project = project.String
		t.ScheduleBlock = scheduleBlock.String
		t.Recurrence = recurrence.String
		t.Assignment = a
		t.IsBlocked = isBlocked
		if completedAt.Valid {
			t.CompletedAt = &completedAt.Time
		}

		if dueAt.Valid {
			t.DueAt = &dueAt.Time
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
	return tasks, rows.Err()
}

func (s *SQLiteStore) MarkDone(id int64) error {
	return s.completeTasks([]int64{id}, nil, false)
}

func (s *SQLiteStore) StartTask(id int64) error {
	// Logic from cmd/start.go
	// 1. Stop active
	// 2. Start new

	// Transaction? Ideally yes.
	tx, err := s.beginWriter()
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
	tx, err := s.beginWriter()
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
		if id != -1 && activeID != id {
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

func (s *SQLiteStore) RenumberTasks(start int64) (int64, error) {
	if start < 0 {
		return 0, fmt.Errorf("start must be >= 0")
	}

	rows, err := s.db.Query("SELECT id, status FROM tasks ORDER BY id")
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	type taskID struct {
		id     int64
		status models.TaskStatus
	}

	var all []taskID
	for rows.Next() {
		var t taskID
		if err := rows.Scan(&t.id, &t.status); err != nil {
			return 0, err
		}
		all = append(all, t)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	if len(all) == 0 {
		return 0, nil
	}

	type idMap struct {
		oldID int64
		temp  int64
		newID int64
	}

	var openIDs []int64
	var closedIDs []int64

	for _, task := range all {
		switch task.status {
		case models.StatusDone, models.StatusDeleted:
			closedIDs = append(closedIDs, task.id)
		default:
			openIDs = append(openIDs, task.id)
		}
	}

	openCount := int64(len(openIDs))

	if len(all) > math.MaxInt64-1 {
		return 0, fmt.Errorf("too many tasks to renumber safely")
	}
	var maxOldID int64 = all[0].id
	for _, task := range all[1:] {
		if task.id > maxOldID {
			maxOldID = task.id
		}
	}
	if maxOldID > math.MaxInt64-int64(len(all))-1 {
		return 0, fmt.Errorf("task IDs are too large to renumber safely")
	}

	tempBase := maxOldID + int64(len(all)) + 1

	mappings := make([]idMap, 0, len(all))
	tempIdx := int64(0)

	for i, oldID := range openIDs {
		mappings = append(mappings, idMap{
			oldID: oldID,
			temp:  tempBase + tempIdx,
			newID: start + int64(i),
		})
		tempIdx++
	}

	closedCount := int64(len(closedIDs))
	for i, oldID := range closedIDs {
		// Keep closed/deleted tasks in a separate negative ID space.
		// Using -closedCount..-1 preserves their current order if sorted by ID.
		mappings = append(mappings, idMap{
			oldID: oldID,
			temp:  tempBase + tempIdx,
			newID: -closedCount + int64(i),
		})
		tempIdx++
	}

	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	if _, err := tx.Exec("PRAGMA foreign_keys=OFF"); err != nil {
		return 0, err
	}

	for _, m := range mappings {
		if _, err := tx.Exec("UPDATE tasks SET id = ? WHERE id = ?", m.temp, m.oldID); err != nil {
			return 0, err
		}
		if _, err := tx.Exec("UPDATE task_dependencies SET blocker_id = ? WHERE blocker_id = ?", m.temp, m.oldID); err != nil {
			return 0, err
		}
		if _, err := tx.Exec("UPDATE task_dependencies SET blocked_id = ? WHERE blocked_id = ?", m.temp, m.oldID); err != nil {
			return 0, err
		}
	}

	for _, m := range mappings {
		if _, err := tx.Exec("UPDATE tasks SET id = ? WHERE id = ?", m.newID, m.temp); err != nil {
			return 0, err
		}
		if _, err := tx.Exec("UPDATE task_dependencies SET blocker_id = ? WHERE blocker_id = ?", m.newID, m.temp); err != nil {
			return 0, err
		}
		if _, err := tx.Exec("UPDATE task_dependencies SET blocked_id = ? WHERE blocked_id = ?", m.newID, m.temp); err != nil {
			return 0, err
		}
	}

	if _, err := tx.Exec("DELETE FROM sqlite_sequence WHERE name = 'tasks'"); err != nil {
		return 0, err
	}

	seq := int64(-1)
	if openCount > 0 {
		seq = start + openCount - 1
	}
	if _, err := tx.Exec("INSERT INTO sqlite_sequence(name, seq) VALUES('tasks', ?)", seq); err != nil {
		return 0, err
	}

	if _, err := tx.Exec("PRAGMA foreign_keys=ON"); err != nil {
		return 0, err
	}

	checkRows, err := tx.Query("PRAGMA foreign_key_check")
	if err != nil {
		return 0, err
	}
	defer checkRows.Close()

	if checkRows.Next() {
		var table string
		var rowid int64
		var parent string
		var fkIndex int64
		if scanErr := checkRows.Scan(&table, &rowid, &parent, &fkIndex); scanErr != nil {
			return 0, scanErr
		}
		return 0, fmt.Errorf("foreign key check failed: table=%s rowid=%d parent=%s fk=%d", table, rowid, parent, fkIndex)
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return openCount, nil
}

func (s *SQLiteStore) GetActiveTask() (*models.Task, error) {
	t, err := s.readTask(s.db, "WHERE active_start IS NOT NULL LIMIT 1")
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &t, nil
}

func (s *SQLiteStore) AddDependency(blockerID, blockedID int64) error {
	if blockerID == blockedID {
		return fmt.Errorf("%w: a task cannot block itself", store.ErrInvalidDependency)
	}
	tx, err := s.beginWriter()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// Check and insert in one write statement so concurrent callers cannot both
	// pass validation and then create a cycle. UNION terminates on legacy cycles.
	query := `
		WITH RECURSIVE descendants(id) AS (
			SELECT ?
			UNION
			SELECT d.blocked_id FROM task_dependencies d
			JOIN descendants ON d.blocker_id = descendants.id
		)
		INSERT INTO task_dependencies (blocker_id, blocked_id)
		SELECT ?, ?
		WHERE EXISTS (SELECT 1 FROM tasks WHERE id = ?)
		  AND EXISTS (SELECT 1 FROM tasks WHERE id = ?)
		  AND NOT EXISTS (SELECT 1 FROM descendants WHERE id = ?)
	`
	result, err := tx.Exec(query, blockedID, blockerID, blockedID, blockerID, blockedID, blockerID)
	if err != nil {
		return err
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if inserted == 0 {
		return fmt.Errorf("%w: both tasks must exist and the link must not create a cycle", store.ErrInvalidDependency)
	}
	return tx.Commit()
}

// RemoveDependency drops one blocker -> blocked edge. The primary key makes it
// exact, and a missing edge is not an error: callers that need to report whether
// anything was there check GetDependencies first.
func (s *SQLiteStore) RemoveDependency(blockerID, blockedID int64) error {
	tx, err := s.beginWriter()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	query := `DELETE FROM task_dependencies WHERE blocker_id = ? AND blocked_id = ?`
	_, err = tx.Exec(query, blockerID, blockedID)
	if err != nil {
		return err
	}
	return tx.Commit()
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
		SELECT name, description, parent, status, due_at, created_at, short_name
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
		var desc, parent, status, sn sql.NullString
		var due sql.NullTime

		err := rows.Scan(&p.Name, &desc, &parent, &status, &due, &p.CreatedAt, &sn)
		if err != nil {
			continue
		}
		p.Description = desc.String
		p.Parent = parent.String
		p.ShortName = sn.String
		p.Status = models.ProjectStatus(status.String)
		if due.Valid {
			p.DueAt = &due.Time
		}
		projects = append(projects, p)
	}
	return projects, nil
}

func (s *SQLiteStore) GetProject(name string) (models.Project, error) {
	query := `SELECT name, description, parent, status, due_at, created_at, short_name FROM projects WHERE name = ?`
	row := s.db.QueryRow(query, name)

	var p models.Project
	var desc, parent, status, sn sql.NullString
	var due sql.NullTime

	err := row.Scan(&p.Name, &desc, &parent, &status, &due, &p.CreatedAt, &sn)
	if err != nil {
		return p, err
	}

	p.Description = desc.String
	p.Parent = parent.String
	p.ShortName = sn.String
	p.Status = models.ProjectStatus(status.String)
	if due.Valid {
		p.DueAt = &due.Time
	}
	return p, nil
}

func (s *SQLiteStore) CreateProject(p models.Project, options store.ProjectCreateOptions) error {
	if p.ShortName == "" && !options.KeepShortNameEmpty {
		var err error
		p.ShortName, err = s.generateUniqueShortName(p.Name)
		if err != nil {
			return err
		}
	}

	var shortName *string
	if p.ShortName != "" {
		shortName = &p.ShortName
	}
	err := requireUpdatedRow(s.db.Exec(`INSERT INTO projects
		(name, short_name, description, parent, due_at, status)
		SELECT ?, ?, ?, NULLIF(?, ''), ?, ?
		WHERE NOT ? OR ? = '' OR EXISTS (SELECT 1 FROM projects WHERE name = ?)`,
		p.Name, shortName, p.Description, p.Parent, p.DueAt, p.Status, options.RequireParent, p.Parent, p.Parent))
	if err == sql.ErrNoRows {
		return fmt.Errorf("parent project %q not found: %w", p.Parent, err)
	}
	return err
}

func (s *SQLiteStore) UpdateProject(oldName string, p models.Project) error {
	tx, err := s.beginWriter()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Update project fields
	var parent *string
	if p.Parent != "" {
		parent = &p.Parent
	}

	if p.ShortName == "" {
		// Keep old if not provided? Or regenerate?
		// Assuming we keep old if empty is safer for simple renames unless explicit reset requested
		// But let's fetch old if needed. For now, let's assume it's passed or we fetch it.
		// Actually, let's just update what we have.
		// Use a subquery or fetch-before-update pattern if we wanted partial updates, but this method replaces usually.
		// Let's generate if missing to be safe, or just allow empty (DB allows null? No, should be strict).
		// Let's generate if empty.
		p.ShortName, _ = generateUniqueShortName(tx, p.Name)
	}

	query := `UPDATE projects SET name = ?, short_name = ?, description = ?, parent = ?, status = ?, due_at = ? WHERE name = ?`
	_, err = tx.Exec(query, p.Name, p.ShortName, p.Description, parent, p.Status, p.DueAt, oldName)
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
	ftsQuery := `
		SELECT tasks.id, tasks.title, tasks.description, tasks.project, tasks.status, tasks.created_at, tasks.user_priority` + assignmentColumns +
		`FROM tasks` + assignmentJoin + ` JOIN tasks_fts ON tasks_fts.rowid = tasks.id
		WHERE tasks_fts MATCH ? ORDER BY rank
	`
	rows, err := s.db.Query(ftsQuery, queryStr)
	if err != nil {
		if !isTaskSearchFTSError(err) {
			return nil, err
		}
		return s.searchTasksLike(queryStr)
	}
	defer rows.Close()

	return s.scanSearchTaskRows(rows)
}

func (s *SQLiteStore) searchTasksLike(queryStr string) ([]models.Task, error) {
	pattern := "%" + strings.ToLower(strings.TrimSpace(queryStr)) + "%"
	query := `
		SELECT tasks.id, title, description, project, status, created_at, user_priority` + assignmentColumns +
		`FROM tasks` + assignmentJoin + `
		WHERE lower(COALESCE(title, '')) LIKE ?
		   OR lower(COALESCE(description, '')) LIKE ?
		   OR lower(COALESCE(project, '')) LIKE ?
		ORDER BY tasks.id
	`

	rows, err := s.db.Query(query, pattern, pattern, pattern)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return s.scanSearchTaskRows(rows)
}

func (s *SQLiteStore) scanSearchTaskRows(rows *sql.Rows) ([]models.Task, error) {
	var stamp leaseTime
	var tasks []models.Task
	for rows.Next() {
		var t models.Task
		var project sql.NullString
		var description sql.NullString
		a, err := scanAssignment(rows, s.clock, &stamp, &t.ID, &t.Title, &description, &project, &t.Status, &t.CreatedAt, &t.UserPriority)
		if err != nil {
			return nil, err
		}
		t.Assignment = a
		t.Description = description.String
		t.Project = project.String
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}

func isTaskSearchFTSError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "no such module: fts5") ||
		strings.Contains(msg, "no such table: tasks_fts") ||
		strings.Contains(msg, "unable to use function match")
}
