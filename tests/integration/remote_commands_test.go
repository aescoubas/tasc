package integration

import (
	"database/sql"
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	dbpkg "github.com/aescoubas/tasc/internal/db"
	"github.com/aescoubas/tasc/internal/models"
	"github.com/aescoubas/tasc/internal/server"
	"github.com/aescoubas/tasc/internal/store/sqlite"
)

func TestCLIProjectActions(t *testing.T) {
	for _, remote := range []bool{false, true} {
		name := "local"
		if remote {
			name = "remote"
		}
		t.Run(name, func(t *testing.T) {
			st, conn, run := projectAndTaskCLI(t, remote)
			const root = "R&D / Europe"
			run(true, "created.", "project", "create", root, "--desc", "Root", "--due", "2026-11-30")
			var shortName sql.NullString
			if err := conn.QueryRow("SELECT short_name FROM projects WHERE name = ?", root).Scan(&shortName); err != nil || shortName.Valid {
				t.Fatalf("CLI project creation must leave the short name unset: %+v, %v", shortName, err)
			}
			run(true, "created.", "project", "create", "Child", "--parent", root)
			run(true, "created.", "project", "create", "Grandchild", "--parent", "Child")
			run(false, "not found", "project", "create", "Orphan", "--parent", "Missing")
			run(false, "no rows", "project", "archive", "Missing")
			run(false, "no rows", "project", "delete", "Missing")
			run(true, "Created task 1.", "add", "Keep this", "--project", root)
			run(true, "Created task 2.", "add", "Child task", "--project", "Child")
			run(true, "Created task 3.", "add", "Grandchild task", "--project", "Grandchild")

			run(true, "archived.", "project", "archive", root)
			project, err := st.GetProject(root)
			if err != nil || project.Description != "Root" || project.DueAt == nil ||
				project.DueAt.Format("2006-01-02") != "2026-11-30" || project.Status != models.ProjectStatusArchived {
				t.Fatalf("archive must preserve project metadata: %+v, %v", project, err)
			}
			run(false, "has 1 tasks and 2 subprojects", "project", "delete", root)
			run(false, "if any flags in the group", "project", "delete", root, "--unlink", "--cascade")
			assertProjectParent(t, st, "Child", root)
			assertTaskProjectAndStatus(t, st, 1, root, models.StatusBacklog)

			run(true, "Unlinked 1 tasks.", "project", "delete", root, "--unlink")
			assertTaskProjectAndStatus(t, st, 1, "", models.StatusBacklog)
			assertProjectParent(t, st, "Child", "")
			assertProjectParent(t, st, "Grandchild", "Child")

			if _, err := conn.Exec(`CREATE TRIGGER reject_grandchild_delete BEFORE DELETE ON projects
				WHEN OLD.name = 'Grandchild' BEGIN SELECT RAISE(ABORT, 'fixture refuses deletion'); END`); err != nil {
				t.Fatal(err)
			}
			run(false, "fixture refuses deletion", "project", "delete", "Child", "--cascade")
			assertProjectParent(t, st, "Child", "")
			assertProjectParent(t, st, "Grandchild", "Child")
			assertTaskProjectAndStatus(t, st, 2, "Child", models.StatusBacklog)
			assertTaskProjectAndStatus(t, st, 3, "Grandchild", models.StatusBacklog)
			if _, err := conn.Exec("DROP TRIGGER reject_grandchild_delete"); err != nil {
				t.Fatal(err)
			}
			run(true, "Deleted 2 associated tasks and 1 subprojects.", "project", "delete", "Child", "--cascade")
			for _, project := range []string{root, "Child", "Grandchild", "Orphan"} {
				if _, err := st.GetProject(project); !errors.Is(err, sql.ErrNoRows) {
					t.Fatalf("project %q should be absent: %v", project, err)
				}
			}
			assertTaskProjectAndStatus(t, st, 2, "Child", models.StatusDeleted)
			assertTaskProjectAndStatus(t, st, 3, "Grandchild", models.StatusDeleted)

			if !remote {
				run(true, "Undid", "undo", "--yes")
				assertProjectParent(t, st, "Grandchild", "Child")
				assertTaskProjectAndStatus(t, st, 3, "Grandchild", models.StatusBacklog)
			}

			run(true, "created.", "project", "create", ".")
			run(true, "archived.", "project", "archive", ".")
			run(true, "deleted.", "project", "delete", ".")
		})
	}
}

func TestRemoteCLIEditLogVagueAndAsk(t *testing.T) {
	st, _, run := projectAndTaskCLI(t, true)
	run(true, "created.", "project", "create", "Archive")
	run(true, "archived.", "project", "archive", "Archive")
	run(true, "Logged task 1.", "log", "Finished before using Tasc", "--project", "Archive", "--due", "2026-11-30", "--estimate", "30m")
	task, err := st.GetTask(1)
	if err != nil || task.Status != models.StatusDone || task.CompletedAt == nil ||
		!task.CreatedAt.Equal(*task.CompletedAt) || task.Estimate != "30m" || task.DueAt == nil ||
		task.DueAt.Format("2006-01-02") != "2026-11-30" {
		t.Fatalf("remote log failed to preserve completed-history fields: %+v, %v", task, err)
	}
	project, err := st.GetProject("Archive")
	if err != nil || project.Status != models.ProjectStatusArchived {
		t.Fatalf("logging completed work must keep the project archived: %+v, %v", project, err)
	}
	run(true, "Logged task 2.", "log", "Historical project", "--project", "Uncatalogued")
	if _, err := st.GetProject("Uncatalogued"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("log unexpectedly created a project: %v", err)
	}
	t.Setenv("GEMINI_API_KEY", "unused-fixture-key")
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:1")
	run(true, "No pending tasks to analyze.", "ask")

	run(true, "Created task 3.", "add", "Edit this title", "--project", "Editing")
	editor := filepath.Join(t.TempDir(), "editor")
	if err := os.WriteFile(editor, []byte("#!/bin/sh\nsed -i 's/^title: .*/title: Edited remotely/' \"$1\"\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EDITOR", editor)
	run(true, "Task 3 updated.", "edit", "3")
	run(true, "marked as undefined.", "vague", "3")
	run(false, "invalid task ID", "vague", "not-a-number")
	run(false, "no rows", "vague", "999999")
	task = assertTaskProjectAndStatus(t, st, 3, "Editing", models.StatusUndefined)
	project, err = st.GetProject("Editing")
	if task.Title != "Edited remotely" || err != nil || project.Status != models.ProjectStatusActive {
		t.Fatalf("edit or vague changed unrelated state: task=%+v project=%+v err=%v", task, project, err)
	}
}

func projectAndTaskCLI(t *testing.T, remote bool) (*sqlite.SQLiteStore, *sql.DB, func(bool, string, ...string)) {
	t.Helper()
	t.Setenv("TASC_REMOTE", "")
	t.Setenv("TASC_API_TOKEN_FILE", "")
	dir := t.TempDir()
	path := filepath.Join(dir, "server.db")
	conn, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	if err := dbpkg.RunMigrations(conn); err != nil {
		t.Fatal(err)
	}
	st := sqlite.NewSQLiteStore(conn)
	var prefix []string
	if remote {
		srv := httptest.NewServer(server.NewServer(st))
		t.Cleanup(srv.Close)
		prefix = []string{"--remote", srv.URL}
		path = filepath.Join(dir, "client.db")
		t.Cleanup(func() {
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Errorf("remote commands initialized a client database: %v", err)
			}
		})
	}
	run := func(success bool, want string, args ...string) {
		t.Helper()
		command := append(append([]string(nil), prefix...), args...)
		out, err := runTasc(t, path, command...)
		if (err == nil) != success || !strings.Contains(out, want) || strings.Contains(out, "panic:") {
			t.Fatalf("tasc %v: err=%v, expected success=%t and %q\n%s", args, err, success, want, out)
		}
	}
	return st, conn, run
}

func assertProjectParent(t *testing.T, st *sqlite.SQLiteStore, name, parent string) {
	t.Helper()
	project, err := st.GetProject(name)
	if err != nil || project.Parent != parent {
		t.Fatalf("project %q: expected parent %q, got %+v, %v", name, parent, project, err)
	}
}

func assertTaskProjectAndStatus(t *testing.T, st *sqlite.SQLiteStore, id int64, project string, status models.TaskStatus) models.Task {
	t.Helper()
	task, err := st.GetTask(id)
	if err != nil || task.Project != project || task.Status != status {
		t.Fatalf("task %d: expected project %q and status %q, got %+v, %v", id, project, status, task, err)
	}
	return task
}
