package sqlite

import (
	"database/sql"
	"fmt"

	"github.com/aescoubas/tasc/internal/models"
	"github.com/aescoubas/tasc/internal/store"
)

func (s *SQLiteStore) ArchiveProject(name string) error {
	return requireUpdatedRow(s.db.Exec("UPDATE projects SET status = ? WHERE name = ?", models.ProjectStatusArchived, name))
}

// UNION visits each project once, including in legacy data with parent cycles.
const projectTree = `WITH RECURSIVE project_tree(name) AS (
	SELECT name FROM projects WHERE name = ?
	UNION
	SELECT projects.name FROM projects JOIN project_tree ON projects.parent = project_tree.name
)`

func (s *SQLiteStore) DeleteProjectWithMode(name string, mode store.ProjectDeleteMode) (store.ProjectDeleteResult, error) {
	var result store.ProjectDeleteResult
	tx, err := s.db.Begin()
	if err != nil {
		return result, err
	}
	defer tx.Rollback()

	var projectCount int
	err = tx.QueryRow(projectTree+`
		SELECT COUNT(*), (SELECT COUNT(*) FROM tasks WHERE project = ? AND status != 'deleted')
		FROM project_tree`, name, name).Scan(&projectCount, &result.Tasks)
	if err != nil {
		return result, err
	}
	if projectCount == 0 {
		return result, fmt.Errorf("project %q: %w", name, sql.ErrNoRows)
	}
	result.Subprojects = projectCount - 1

	switch mode {
	case store.ProjectDeleteEmpty:
		if result.Tasks > 0 || result.Subprojects > 0 {
			return result, fmt.Errorf("project %q has %d tasks and %d subprojects. Use --unlink to keep tasks or --cascade to delete them: %w",
				name, result.Tasks, result.Subprojects, store.ErrProjectNotEmpty)
		}
	case store.ProjectDeleteUnlink:
		if _, err := tx.Exec("UPDATE tasks SET project = NULL WHERE project = ?", name); err != nil {
			return result, err
		}
		if _, err := tx.Exec("UPDATE projects SET parent = NULL WHERE parent = ?", name); err != nil {
			return result, err
		}
	case store.ProjectDeleteCascade:
		affected, err := tx.Exec(projectTree+`
			UPDATE tasks SET status = 'deleted'
			WHERE project IN (SELECT name FROM project_tree) AND status != 'deleted'`, name)
		if err != nil {
			return result, err
		}
		count, err := affected.RowsAffected()
		if err != nil {
			return result, err
		}
		result.Tasks = int(count)
		if _, err := tx.Exec(projectTree+" DELETE FROM projects WHERE name IN (SELECT name FROM project_tree)", name); err != nil {
			return result, err
		}
		return result, tx.Commit()
	default:
		return result, fmt.Errorf("invalid project deletion mode %q", mode)
	}

	if _, err := tx.Exec("DELETE FROM projects WHERE name = ?", name); err != nil {
		return result, err
	}
	return result, tx.Commit()
}
