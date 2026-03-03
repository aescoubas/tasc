package cmd

import (
	"github.com/aescoubas/tasc/internal/db"
	"github.com/spf13/cobra"
)

var (
	currentUndoActionID int64
	undoTrackedCommands = map[string]bool{
		"tasc add":             true,
		"tasc log":             true,
		"tasc done":            true,
		"tasc delete":          true,
		"tasc modify":          true,
		"tasc edit":            true,
		"tasc start":           true,
		"tasc stop":            true,
		"tasc dep":             true,
		"tasc reschedule":      true,
		"tasc vague":           true,
		"tasc renumber":        true,
		"tasc project create":  true,
		"tasc project edit":    true,
		"tasc project archive": true,
		"tasc project delete":  true,
	}
)

func beginUndoTracking(cmd *cobra.Command) {
	currentUndoActionID = 0

	if db.DB == nil {
		return
	}

	_, _ = db.DB.Exec("UPDATE undo_context SET action_id = NULL WHERE id = 1")

	if !undoTrackedCommands[cmd.CommandPath()] {
		return
	}

	res, err := db.DB.Exec("INSERT INTO undo_actions(command) VALUES (?)", cmd.CommandPath())
	if err != nil {
		return
	}

	actionID, err := res.LastInsertId()
	if err != nil {
		_, _ = db.DB.Exec("DELETE FROM undo_actions WHERE id = ?", actionID)
		return
	}
	currentUndoActionID = actionID

	if _, err := db.DB.Exec("UPDATE undo_context SET action_id = ? WHERE id = 1", currentUndoActionID); err != nil {
		_, _ = db.DB.Exec("DELETE FROM undo_actions WHERE id = ?", currentUndoActionID)
		currentUndoActionID = 0
		_, _ = db.DB.Exec("UPDATE undo_context SET action_id = NULL WHERE id = 1")
	}
}

func endUndoTracking() {
	if db.DB == nil {
		return
	}

	_, _ = db.DB.Exec("UPDATE undo_context SET action_id = NULL WHERE id = 1")

	if currentUndoActionID == 0 {
		return
	}

	var entryCount int
	err := db.DB.QueryRow("SELECT COUNT(*) FROM undo_entries WHERE action_id = ?", currentUndoActionID).Scan(&entryCount)
	if err == nil && entryCount == 0 {
		_, _ = db.DB.Exec("DELETE FROM undo_actions WHERE id = ?", currentUndoActionID)
	}

	currentUndoActionID = 0
}
