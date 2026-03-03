package cmd

import (
	"database/sql"
	"fmt"

	"github.com/aescoubas/tasc/internal/db"
	"github.com/spf13/cobra"
)

var undoAutoApprove bool

var undoCmd = &cobra.Command{
	Use:   "undo",
	Short: "Undo the last mutating action",
	Run: func(cmd *cobra.Command, args []string) {
		if db.DB == nil {
			fmt.Println("Error: undo is only supported with a local SQLite database.")
			return
		}

		var actionID int64
		var actionCommand string
		var entryCount int
		query := `
			SELECT a.id, a.command, COUNT(e.id) AS entry_count
			FROM undo_actions a
			JOIN undo_entries e ON e.action_id = a.id
			WHERE a.undone = 0
			GROUP BY a.id, a.command
			ORDER BY a.id DESC
			LIMIT 1
		`
		err := db.DB.QueryRow(query).Scan(&actionID, &actionCommand, &entryCount)
		if err == sql.ErrNoRows {
			fmt.Println("Nothing to undo.")
			return
		}
		if err != nil {
			fmt.Printf("Error reading undo history: %v\n", err)
			return
		}

		if !undoAutoApprove {
			msg := fmt.Sprintf("Undo last action '%s'?", actionCommand)
			if AskConfirmation(msg) == ConfirmNo {
				fmt.Println("Undo cancelled.")
				return
			}
		}

		tx, err := db.DB.Begin()
		if err != nil {
			fmt.Printf("Error starting undo transaction: %v\n", err)
			return
		}
		defer tx.Rollback()

		if _, err := tx.Exec("UPDATE undo_context SET action_id = NULL WHERE id = 1"); err != nil {
			fmt.Printf("Error preparing undo context: %v\n", err)
			return
		}

		if _, err := tx.Exec("PRAGMA foreign_keys=OFF"); err != nil {
			fmt.Printf("Error disabling foreign keys for undo: %v\n", err)
			return
		}

		rows, err := tx.Query("SELECT id, inverse_sql FROM undo_entries WHERE action_id = ? ORDER BY id DESC", actionID)
		if err != nil {
			fmt.Printf("Error loading undo entries: %v\n", err)
			return
		}
		defer rows.Close()

		for rows.Next() {
			var entryID int64
			var inverseSQL string
			if err := rows.Scan(&entryID, &inverseSQL); err != nil {
				fmt.Printf("Error parsing undo entry: %v\n", err)
				return
			}
			if _, err := tx.Exec(inverseSQL); err != nil {
				fmt.Printf("Error applying undo entry %d: %v\n", entryID, err)
				return
			}
		}
		if err := rows.Err(); err != nil {
			fmt.Printf("Error iterating undo entries: %v\n", err)
			return
		}

		if _, err := tx.Exec("UPDATE undo_actions SET undone = 1, undone_at = CURRENT_TIMESTAMP WHERE id = ?", actionID); err != nil {
			fmt.Printf("Error finalizing undo action: %v\n", err)
			return
		}

		if _, err := tx.Exec("PRAGMA foreign_keys=ON"); err != nil {
			fmt.Printf("Error enabling foreign keys after undo: %v\n", err)
			return
		}

		checkRows, err := tx.Query("PRAGMA foreign_key_check")
		if err != nil {
			fmt.Printf("Error running foreign key check: %v\n", err)
			return
		}
		defer checkRows.Close()

		if checkRows.Next() {
			var table string
			var rowid int64
			var parent string
			var fkIndex int64
			if err := checkRows.Scan(&table, &rowid, &parent, &fkIndex); err != nil {
				fmt.Printf("Error reading foreign key check result: %v\n", err)
				return
			}
			fmt.Printf("Undo aborted: foreign key violation table=%s rowid=%d parent=%s fk=%d\n", table, rowid, parent, fkIndex)
			return
		}

		if err := tx.Commit(); err != nil {
			fmt.Printf("Error committing undo: %v\n", err)
			return
		}

		fmt.Printf("Undid action %d (%s).\n", actionID, actionCommand)
	},
}

func init() {
	rootCmd.AddCommand(undoCmd)
	undoCmd.Flags().BoolVarP(&undoAutoApprove, "yes", "y", false, "Auto-approve undo without confirmation")
}
