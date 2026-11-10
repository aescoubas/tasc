package ui

import (
	"database/sql"
	"testing"

	"github.com/aescoubas/tasc/internal/db"
	"github.com/aescoubas/tasc/internal/models"
	"github.com/aescoubas/tasc/internal/store/sqlite"
	tea "github.com/charmbracelet/bubbletea"
	_ "github.com/mattn/go-sqlite3"
)

func TestTUICompletionKeepsFailuresAndLoadsSuccessors(t *testing.T) {
	for _, rule := range []string{"daily", "not-a-rule"} {
		t.Run(rule, func(t *testing.T) {
			conn, err := sql.Open("sqlite3", ":memory:")
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			if err := db.RunMigrations(conn); err != nil {
				t.Fatal(err)
			}
			previous := db.DB
			db.DB = conn
			defer func() { db.DB = previous }()
			st := sqlite.NewSQLiteStore(conn)
			id, err := st.CreateTask(models.Task{Title: "TUI completion probe", Recurrence: rule, ScheduleBlock: "morning"})
			if err != nil {
				t.Fatal(err)
			}
			m := InitialModel()
			updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
			next := updated.(Model)
			if len(next.list.Items()) != 1 {
				t.Fatalf("TUI must retain the failed task or show its successor: %d items", len(next.list.Items()))
			}
			task, err := st.GetTask(id)
			if err != nil {
				t.Fatal(err)
			}
			selected := next.list.Items()[0].(item).task
			if rule == "daily" {
				if task.Status != models.StatusDone || selected.ID == id {
					t.Fatalf("successful completion did not refresh the successor: original=%+v selected=%+v", task, selected)
				}
			} else if task.Status != models.StatusBacklog || selected.ID != id {
				t.Fatalf("failed completion removed or modified the task: original=%+v selected=%+v", task, selected)
			}
		})
	}
}
