package sqlite

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aescoubas/tasc/internal/models"
	"github.com/aescoubas/tasc/internal/store"
)

const claimLifetime = 10 * time.Minute

func (s *SQLiteStore) ForActor(actor string) store.Store {
	copy := *s
	copy.actor = actor
	return &copy
}

type writerTx struct{ *sql.Tx }

func (tx writerTx) Commit() error {
	if _, err := tx.Exec("UPDATE assignment_writer SET actor='', live=0 WHERE id=1"); err != nil {
		return err
	}
	return tx.Tx.Commit()
}

func (s *SQLiteStore) beginWriter() (*writerTx, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	// Take the write lock before inspecting the lease or the tasks.
	if _, err := tx.Exec("UPDATE assignment_writer SET actor=?,live=0 WHERE id=1", s.actor); err != nil {
		tx.Rollback()
		return nil, err
	}
	if s.actor != "" && s.actor != store.GuestActor {
		boot, now, err := s.clock()
		if err != nil {
			tx.Rollback()
			return nil, err
		}
		if _, err := tx.Exec(`UPDATE assignment_writer SET live=EXISTS(
     SELECT 1 FROM assignment_runs WHERE id=? AND boot=? AND expires>?) WHERE id=1`, s.actor, boot, now); err != nil {
			tx.Rollback()
			return nil, err
		}
	}
	return &writerTx{tx}, nil
}

const assignmentColumns = ", a.task_id, a.run_id, r.name, r.location, r.boot, r.expires "
const assignmentJoin = ` LEFT JOIN task_assignments a ON a.task_id=tasks.id
 LEFT JOIN assignment_runs r ON r.id=a.run_id `

const assignmentSelect = `SELECT tasks.id, tasks.title,
 CASE WHEN tasks.status NOT IN ('done','deleted') AND EXISTS(
 SELECT 1 FROM task_dependencies d JOIN tasks b ON b.id=d.blocker_id
 WHERE d.blocked_id=tasks.id AND b.status NOT IN ('done','deleted')
 ) THEN 'blocked' ELSE tasks.status END` + assignmentColumns + "FROM tasks" + assignmentJoin

type leaseTime struct {
	boot string
	now  int64
}
type leaseSource func() (string, int64, error)

// Read the awake clock once per result set, and only for an agent assignment.
func scanAssignment(row interface{ Scan(...any) error }, clock leaseSource, stamp *leaseTime, fields ...any) (*models.Assignment, error) {
	var assigned, expires sql.NullInt64
	var run, name, location, boot sql.NullString
	if err := row.Scan(append(fields, &assigned, &run, &name, &location, &boot, &expires)...); err != nil {
		return nil, err
	}
	if !assigned.Valid {
		return nil, nil
	}
	if !run.Valid {
		return &models.Assignment{Name: store.HumanOwner}, nil
	}
	if stamp.boot == "" {
		var err error
		stamp.boot, stamp.now, err = clock()
		if err != nil {
			return nil, err
		}
	}
	return &models.Assignment{Name: name.String, RunID: run.String, Location: location.String, Reclaimable: boot.String != stamp.boot || expires.Int64 <= stamp.now}, nil
}

func (s *SQLiteStore) readAssignment(row interface{ Scan(...any) error }, stamp *leaseTime) (store.AssignmentRow, error) {
	var item store.AssignmentRow
	a, err := scanAssignment(row, s.clock, stamp, &item.ID, &item.Title, &item.Status)
	item.Assignment = a
	return item, err
}

func (s *SQLiteStore) Assignments() ([]store.AssignmentRow, error) {
	rows, err := s.db.Query(assignmentSelect + " WHERE a.task_id IS NOT NULL ORDER BY r.name,r.id,tasks.id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []store.AssignmentRow{}
	var stamp leaseTime
	for rows.Next() {
		item, err := s.readAssignment(rows, &stamp)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *SQLiteStore) ChangeAssignments(change store.AssignmentChange) ([]store.AssignmentRow, error) {
	human := s.actor == ""
	claiming := change.Action == "claim" || change.Action == "extend"
	switch change.Action {
	case "claim", "extend":
		if human || s.actor == store.GuestActor || strings.TrimSpace(change.Name) == "" {
			return nil, errors.New("claim requires a named agent run")
		}
	case "renew", "end":
		if human || s.actor == store.GuestActor || len(change.IDs) != 0 {
			return nil, errors.New("renew and end require an agent run and apply to the whole run")
		}
	case "assign", "release":
		if s.actor == store.GuestActor || (!human && change.TargetRun != "") {
			return nil, errors.New("only the operator can reassign another run")
		}
	default:
		return nil, fmt.Errorf("unknown assignment action %q", change.Action)
	}
	if change.Action != "renew" && change.Action != "end" && len(change.IDs) == 0 {
		return nil, errors.New("task IDs required")
	}
	tx, err := s.beginWriter()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	boot, now, err := s.clock()
	if err != nil {
		return nil, err
	}
	switch change.Action {
	case "claim":
		// An expired run cannot resurrect all of its old assignments by claiming one.
		_, err = tx.Exec(`INSERT INTO assignment_runs(id,name,location,boot,expires)
		 VALUES(?,?,?,?,?) ON CONFLICT(id) DO NOTHING`, s.actor, change.Name, change.Location, boot, now+int64(claimLifetime))
		if err != nil {
			return nil, err
		}
		fallthrough
	case "renew", "extend":
		if err := requireUpdatedRow(tx.Exec("UPDATE assignment_runs SET expires=? WHERE id=? AND boot=? AND expires>?", now+int64(claimLifetime), s.actor, boot, now)); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, store.ErrLeaseExpired
			}
			return nil, err
		}
	case "end":
		if _, err := tx.Exec("DELETE FROM task_assignments WHERE run_id=?", s.actor); err != nil {
			return nil, err
		}
		if _, err := tx.Exec("DELETE FROM assignment_runs WHERE id=?", s.actor); err != nil {
			return nil, err
		}
	case "assign":
		if change.TargetRun != "" {
			var active bool
			if err := tx.QueryRow("SELECT EXISTS(SELECT 1 FROM assignment_runs WHERE id=? AND boot=? AND expires>?)", change.TargetRun, boot, now).Scan(&active); err != nil {
				return nil, err
			}
			if !active {
				return nil, errors.New("target run is missing or expired")
			}
		}
	}
	items := []store.AssignmentRow{}
	seen := map[int64]bool{}
	for _, id := range change.IDs {
		if seen[id] {
			continue
		}
		seen[id] = true
		read := func() (store.AssignmentRow, error) {
			return s.readAssignment(tx.QueryRow(assignmentSelect+" WHERE tasks.id=?", id), &leaseTime{boot, now})
		}
		item, err := read()
		if errors.Is(err, sql.ErrNoRows) {
			items = append(items, store.AssignmentRow{ID: id, Result: "missing"})
			continue
		}
		if err != nil {
			return nil, err
		}
		a := item.Assignment
		switch {
		case item.Status == models.StatusDone || item.Status == models.StatusDeleted:
			item.Result = "closed"
		case claiming && a != nil && a.RunID != s.actor && !a.Reclaimable:
			item.Result = "conflict"
		case !human && !claiming && (a == nil || a.RunID != s.actor || a.Reclaimable):
			item.Result = "conflict"
		default:
			result := "assigned"
			if change.Action == "release" {
				_, err = tx.Exec("DELETE FROM task_assignments WHERE task_id=?", id)
				result = "released"
			} else {
				target := change.TargetRun
				if claiming {
					target = s.actor
					result = "claimed"
				}
				_, err = tx.Exec(`INSERT INTO task_assignments(task_id,run_id) VALUES(?,NULLIF(?,''))
				 ON CONFLICT(task_id) DO UPDATE SET run_id=excluded.run_id`, id, target)
			}
			if err != nil {
				return nil, err
			}
			item, err = read()
			if err != nil {
				return nil, err
			}
			item.Result = result
			if claiming && a != nil && a.Reclaimable {
				item.Previous = a
			}
		}
		items = append(items, item)
	}
	// Keep stale owners visible until every associated task has been recovered.
	if change.Action != "renew" {
		_, err = tx.Exec(`DELETE FROM assignment_runs WHERE (boot<>? OR expires<=?)
		 AND NOT EXISTS(SELECT 1 FROM task_assignments WHERE run_id=assignment_runs.id)`, boot, now)
		if err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return items, nil
}
