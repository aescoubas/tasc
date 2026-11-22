package db

import "database/sql"

func addAssignments(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.Exec(`
CREATE TABLE IF NOT EXISTS assignment_runs (
 id TEXT PRIMARY KEY, name TEXT NOT NULL, location TEXT NOT NULL,
 boot TEXT NOT NULL, expires INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS task_assignments (
 task_id INTEGER PRIMARY KEY, run_id TEXT REFERENCES assignment_runs(id)
);
CREATE INDEX IF NOT EXISTS task_assignments_run ON task_assignments(run_id);

-- Each writer sets and clears this row in the same SQLite write transaction.
-- Other connections only ever see the default human context.
CREATE TABLE IF NOT EXISTS assignment_writer (
 id INTEGER PRIMARY KEY CHECK(id=1), actor TEXT NOT NULL DEFAULT '', live INTEGER NOT NULL DEFAULT 0
);
INSERT OR IGNORE INTO assignment_writer(id) VALUES(1);
CREATE VIEW IF NOT EXISTS assignment_denied AS
SELECT t.id FROM tasks t CROSS JOIN assignment_writer w
 LEFT JOIN task_assignments a ON a.task_id=t.id
 WHERE CASE w.actor
  WHEN '' THEN a.run_id IS NOT NULL
  WHEN 'guest' THEN a.task_id IS NOT NULL
  ELSE w.live=0 OR a.run_id IS NOT w.actor END;

CREATE TRIGGER IF NOT EXISTS assignment_guard_update BEFORE UPDATE ON tasks
 WHEN EXISTS(SELECT 1 FROM assignment_denied WHERE id=OLD.id)
 BEGIN SELECT RAISE(ABORT,'task assignment conflict'); END;
CREATE TRIGGER IF NOT EXISTS assignment_guard_delete BEFORE DELETE ON tasks
 WHEN EXISTS(SELECT 1 FROM assignment_denied WHERE id=OLD.id)
 BEGIN SELECT RAISE(ABORT,'task assignment conflict'); END;
CREATE TRIGGER IF NOT EXISTS assignment_guard_link BEFORE INSERT ON task_dependencies
 WHEN EXISTS(SELECT 1 FROM assignment_denied WHERE id IN (NEW.blocker_id, NEW.blocked_id))
 BEGIN SELECT RAISE(ABORT,'task assignment conflict'); END;
CREATE TRIGGER IF NOT EXISTS assignment_guard_unlink BEFORE DELETE ON task_dependencies
 WHEN EXISTS(SELECT 1 FROM assignment_denied WHERE id IN (OLD.blocker_id, OLD.blocked_id))
 BEGIN SELECT RAISE(ABORT,'task assignment conflict'); END;
CREATE TRIGGER IF NOT EXISTS assignment_closed AFTER UPDATE OF status ON tasks
 WHEN NEW.status IN ('done','deleted')
 BEGIN DELETE FROM task_assignments WHERE task_id=NEW.id; END;
CREATE TRIGGER IF NOT EXISTS assignment_deleted AFTER DELETE ON tasks
 BEGIN DELETE FROM task_assignments WHERE task_id=OLD.id; END;
CREATE TRIGGER IF NOT EXISTS assignment_renumber AFTER UPDATE OF id ON tasks
 BEGIN UPDATE task_assignments SET task_id=NEW.id WHERE task_id=OLD.id; END;
`)
	if err != nil {
		return err
	}
	return tx.Commit()
}
