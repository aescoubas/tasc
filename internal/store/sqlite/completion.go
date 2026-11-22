package sqlite

import (
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/aescoubas/tasc/internal/models"
	"github.com/aescoubas/tasc/internal/recurrence"
	"github.com/aescoubas/tasc/internal/store"
)

type taskQueries interface {
	Exec(string, ...interface{}) (sql.Result, error)
	QueryRow(string, ...interface{}) *sql.Row
}

func (s *SQLiteStore) completeTasks(ids []int64, updates map[string]interface{}, editCompleted bool) error {
	q, err := s.beginWriter()
	if err != nil {
		return err
	}
	defer q.Rollback()
	now := time.Now()
	seen := make(map[int64]bool)
	projects := make(map[string]bool)
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		task, err := s.readTask(q, "WHERE tasks.id=?", id)
		if err != nil {
			return store.TaskLookupError(id, err)
		}
		if task.Status == models.StatusDeleted {
			return fmt.Errorf("%w: task %d is deleted", store.ErrInvalidCompletion, id)
		}
		wasDone := task.Status == models.StatusDone
		if wasDone && s.actor != "" && s.actor != store.GuestActor {
			// Terminal tasks have no claim. An old run must not report another run's completion as its own.
			return errors.New(store.AssignmentConflict)
		}
		if wasDone && !editCompleted {
			continue
		}
		projects[task.Project] = true
		if err := applyCompletionEdits(q, id, updates); err != nil {
			return err
		}
		task, err = s.readTask(q, "WHERE tasks.id=?", id)
		if err != nil {
			return err
		}
		projects[task.Project] = true
		// Editing a completed task's metadata is still supported. Its lifecycle
		// state (completion time, timer, and successor) is not replayed.
		if wasDone {
			continue
		}

		if err := createSuccessor(q, task, now); err != nil {
			return err
		}

		elapsed := int64(0)
		if task.ActiveStart != nil && now.After(*task.ActiveStart) {
			elapsed = int64(now.Sub(*task.ActiveStart).Seconds())
		}
		if _, err := q.Exec(`UPDATE tasks SET status = 'done', completed_at = ?,
			active_start = NULL, time_spent = COALESCE(time_spent, 0) + ? WHERE id = ?`, now, elapsed, id); err != nil {
			return err
		}
	}
	for project := range projects {
		if err := checkProjectCompletion(q, project); err != nil {
			return err
		}
	}
	return q.Commit()
}

// Insert the successor in the same guarded transaction as the change to its predecessor.
func createSuccessor(q taskQueries, task models.Task, now time.Time) error {
	if task.Recurrence == "" {
		return nil
	}

	base := now
	if task.DueAt != nil {
		base = *task.DueAt
	}
	next, err := recurrence.Next(base, task.Recurrence)
	if err != nil {
		return fmt.Errorf("%w: task %d: %v", store.ErrInvalidCompletion, task.ID, err)
	}
	if !next.After(base) {
		return fmt.Errorf("%w: task %d recurrence must advance its due date", store.ErrInvalidCompletion, task.ID)
	}
	if err := ensureProjectExists(q, task.Project); err != nil {
		return err
	}
	_, err = q.Exec(`INSERT INTO tasks
				(title, description, project, due_at, schedule_block, estimate, recurrence, user_priority, created_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, task.Title, task.Description, task.Project,
		next, task.ScheduleBlock, task.Estimate, task.Recurrence, task.UserPriority, now)
	return err
}

func applyCompletionEdits(q taskQueries, id int64, updates map[string]interface{}) error {
	var fields []string
	for field := range updates {
		switch field {
		case "title", "description", "project", "due_at", "schedule_block", "estimate", "recurrence", "reschedule_count", "user_priority":
			fields = append(fields, field)
		}
	}
	if len(fields) == 0 {
		return nil
	}
	if project, ok := updates["project"].(string); ok {
		if err := ensureProjectExists(q, project); err != nil {
			return err
		}
	}
	sort.Strings(fields)
	var sets []string
	var args []interface{}
	for _, field := range fields {
		sets = append(sets, field+" = ?")
		args = append(args, updates[field])
	}
	args = append(args, id)
	_, err := q.Exec("UPDATE tasks SET "+strings.Join(sets, ", ")+" WHERE id = ?", args...)
	return err
}
