package sqlite

import (
	"context"
	"database/sql"
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

// A dedicated connection keeps BEGIN IMMEDIATE, every read/write, and COMMIT on
// the same SQLite connection. Acquire the write lock before reading task state:
// concurrent completions must see the previous writer's committed done status.
type completionQueries struct{ conn *sql.Conn }

func (q completionQueries) Exec(query string, args ...interface{}) (sql.Result, error) {
	return q.conn.ExecContext(context.Background(), query, args...)
}

func (q completionQueries) QueryRow(query string, args ...interface{}) *sql.Row {
	return q.conn.QueryRowContext(context.Background(), query, args...)
}

func (s *SQLiteStore) completeTasks(ids []int64, updates map[string]interface{}, editCompleted bool) error {
	conn, err := s.db.Conn(context.Background())
	if err != nil {
		return err
	}
	defer conn.Close()
	q := completionQueries{conn}
	if _, err := q.Exec("BEGIN IMMEDIATE"); err != nil {
		return err
	}
	defer q.Exec("ROLLBACK")

	now := time.Now()
	seen := make(map[int64]bool)
	projects := make(map[string]bool)
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		task, err := getTask(q, id)
		if err != nil {
			return fmt.Errorf("task %d: %w", id, err)
		}
		if task.Status == models.StatusDeleted {
			return fmt.Errorf("%w: task %d is deleted", store.ErrInvalidCompletion, id)
		}
		wasDone := task.Status == models.StatusDone
		if wasDone && !editCompleted {
			continue
		}
		projects[task.Project] = true
		if err := applyCompletionEdits(q, id, updates); err != nil {
			return err
		}
		task, err = getTask(q, id)
		if err != nil {
			return err
		}
		projects[task.Project] = true
		// Editing a completed task's metadata is still supported. Its lifecycle
		// state (completion time, timer, and successor) is not replayed.
		if wasDone {
			continue
		}

		if task.Recurrence != "" {
			base := now
			if task.DueAt != nil {
				base = *task.DueAt
			}
			next, err := recurrence.Next(base, task.Recurrence)
			if err != nil {
				return fmt.Errorf("%w: task %d: %v", store.ErrInvalidCompletion, id, err)
			}
			if !next.After(base) {
				return fmt.Errorf("%w: task %d recurrence must advance its due date", store.ErrInvalidCompletion, id)
			}
			if err := ensureProjectExists(q, task.Project); err != nil {
				return err
			}
			_, err = q.Exec(`INSERT INTO tasks
				(title, description, project, due_at, schedule_block, estimate, recurrence, created_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, task.Title, task.Description, task.Project,
				next, task.ScheduleBlock, task.Estimate, task.Recurrence, now)
			if err != nil {
				return err
			}
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
	_, err = q.Exec("COMMIT")
	return err
}

func applyCompletionEdits(q taskQueries, id int64, updates map[string]interface{}) error {
	var fields []string
	for field := range updates {
		switch field {
		case "title", "description", "project", "due_at", "schedule_block", "estimate", "recurrence", "reschedule_count":
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
