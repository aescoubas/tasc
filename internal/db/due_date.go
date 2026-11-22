package db

import (
	"context"
	"database/sql"
)

func addTaskDueDatePrecision(db *sql.DB) error {
	ctx := context.Background()
	conn, err := db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return err
	}
	defer conn.ExecContext(ctx, "ROLLBACK")

	// The migration runner records the version after Up commits. Allow a retry
	// if the schema transaction committed before that record was written.
	var applied bool
	if err := conn.QueryRowContext(ctx, `SELECT EXISTS (
		SELECT 1 FROM pragma_table_info('tasks') WHERE name = 'due_date_only'
	)`).Scan(&applied); err != nil {
		return err
	}
	if applied {
		return nil
	}

	_, err = conn.ExecContext(ctx, `
		ALTER TABLE tasks ADD COLUMN due_date_only INTEGER CHECK (due_date_only IN (0, 1));
		DROP TRIGGER tasks_undo_ad;
		DROP TRIGGER tasks_undo_au;
		CREATE TRIGGER tasks_undo_ad AFTER DELETE ON tasks
		WHEN (SELECT action_id FROM undo_context WHERE id = 1) IS NOT NULL
		BEGIN
			INSERT INTO undo_entries(action_id, inverse_sql)
			VALUES (
				(SELECT action_id FROM undo_context WHERE id = 1),
				printf(
					'INSERT INTO tasks (id, title, description, project, status, created_at, completed_at, due_at, schedule_block, estimate, recurrence, active_start, time_spent, reschedule_count, user_priority, due_date_only) VALUES (%d, %Q, %Q, %Q, %Q, %Q, %Q, %Q, %Q, %Q, %Q, %Q, %d, %d, %s, %s);',
					OLD.id, OLD.title, OLD.description, OLD.project, OLD.status, OLD.created_at, OLD.completed_at, OLD.due_at, OLD.schedule_block, OLD.estimate, OLD.recurrence, OLD.active_start, COALESCE(OLD.time_spent, 0), COALESCE(OLD.reschedule_count, 0), COALESCE(CAST(OLD.user_priority AS TEXT), 'NULL'), COALESCE(CAST(OLD.due_date_only AS TEXT), 'NULL')
				)
			);
		END;
		CREATE TRIGGER tasks_undo_au AFTER UPDATE ON tasks
		WHEN (SELECT action_id FROM undo_context WHERE id = 1) IS NOT NULL
		BEGIN
			INSERT INTO undo_entries(action_id, inverse_sql)
			VALUES (
				(SELECT action_id FROM undo_context WHERE id = 1),
				printf(
					'UPDATE tasks SET id = %d, title = %Q, description = %Q, project = %Q, status = %Q, created_at = %Q, completed_at = %Q, due_at = %Q, schedule_block = %Q, estimate = %Q, recurrence = %Q, active_start = %Q, time_spent = %d, reschedule_count = %d, user_priority = %s, due_date_only = %s WHERE id = %d;',
					OLD.id, OLD.title, OLD.description, OLD.project, OLD.status, OLD.created_at, OLD.completed_at, OLD.due_at, OLD.schedule_block, OLD.estimate, OLD.recurrence, OLD.active_start, COALESCE(OLD.time_spent, 0), COALESCE(OLD.reschedule_count, 0), COALESCE(CAST(OLD.user_priority AS TEXT), 'NULL'), COALESCE(CAST(OLD.due_date_only AS TEXT), 'NULL'), NEW.id
				)
			);
		END;
	`)
	if err != nil {
		return err
	}
	_, err = conn.ExecContext(ctx, "COMMIT")
	return err
}
