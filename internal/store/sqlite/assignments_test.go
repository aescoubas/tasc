package sqlite

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	dbpkg "github.com/aescoubas/tasc/internal/db"
	"github.com/aescoubas/tasc/internal/models"
	"github.com/aescoubas/tasc/internal/store"
)

func TestConcurrentClaimsAndAwakeExpiry(t *testing.T) {
	path := filepath.Join(t.TempDir(), "leases.db")
	open := func() *SQLiteStore {
		db, err := sql.Open("sqlite3", path)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { db.Close() })
		return NewSQLiteStore(db)
	}
	st, other := open(), open()
	if err := dbpkg.RunMigrations(st.db); err != nil {
		t.Fatal(err)
	}
	boot := "test boot"
	now := int64(time.Hour)
	st.clock = func() (string, int64, error) { return boot, now, nil }
	other.clock = st.clock
	id, err := st.CreateTask(models.Task{Title: "Concurrent claim"})
	if err != nil {
		t.Fatal(err)
	}
	type outcome struct {
		st   store.Store
		rows []store.AssignmentRow
		err  error
	}
	finished := make(chan outcome, 2)
	start := make(chan struct{})
	for i, actor := range []string{"first", "second"} {
		agent := []*SQLiteStore{st, other}[i].ForActor(actor)
		go func() {
			<-start
			r, err := agent.ChangeAssignments(store.AssignmentChange{Action: "claim", Name: "Same label", IDs: []int64{id}})
			finished <- outcome{agent, r, err}
		}()
	}
	close(start)
	var winner, loser store.Store
	for range 2 {
		r := <-finished
		if r.err != nil || len(r.rows) != 1 {
			t.Fatalf("race: %+v", r)
		}
		switch r.rows[0].Result {
		case "claimed":
			winner = r.st
		case "conflict":
			loser = r.st
		}
	}
	if winner == nil || loser == nil {
		t.Fatal("expected one winner and one conflict")
	}
	now += int64(9 * time.Minute)
	if _, err := winner.ChangeAssignments(store.AssignmentChange{Action: "renew"}); err != nil {
		t.Fatal(err)
	}
	now += int64(9 * time.Minute)
	if err := winner.UpdateTaskDescription(id, "", "after build"); err != nil {
		t.Fatal(err)
	}
	now += int64(time.Minute)
	if err := winner.MarkDone(id); err == nil {
		t.Fatal("expired completion accepted")
	}
	if _, err := winner.ChangeAssignments(store.AssignmentChange{Action: "renew"}); err == nil {
		t.Fatal("revived expired run")
	}
	if _, err := winner.ChangeAssignments(store.AssignmentChange{Action: "end"}); err != nil {
		t.Fatal(err)
	}
	fresh := st.ForActor("new")
	if _, err := fresh.ChangeAssignments(store.AssignmentChange{Action: "claim", Name: "New goal", IDs: []int64{id}}); err != nil {
		t.Fatal(err)
	}
	boot = "next boot"
	now = 1
	if err := fresh.DeleteTask(id); err == nil {
		t.Fatal("old boot retained lease")
	}
	rows, err := st.Assignments()
	if err != nil || len(rows) != 1 || !rows[0].Assignment.Reclaimable {
		t.Fatalf("boot change: %+v %v", rows, err)
	}
}
