package cmd

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/aescoubas/tasc/internal/db"
	"github.com/spf13/cobra"
)

var seedCmd = &cobra.Command{
	Use:   "seed",
	Short: "Populate the database with dummy data",
	Run: func(cmd *cobra.Command, args []string) {
		projects := []string{"Work", "Personal", "Tasc", "Health", "Finance"}
		verbs := []string{"Fix", "Buy", "Write", "Call", "Email", "Review", "Clean", "Pay", "Update", "Deploy"}
		nouns := []string{"bug", "milk", "documentation", "mom", "client", "code", "house", "bills", "dependencies", "server"}
		
		rand.Seed(time.Now().UnixNano())

		tx, err := db.DB.Begin()
		if err != nil {
			fmt.Printf("Error starting transaction: %v\n", err)
			return
		}

		for i := 0; i < 50; i++ {
			desc := fmt.Sprintf("%s %s %d", verbs[rand.Intn(len(verbs))], nouns[rand.Intn(len(nouns))], i)
			proj := projects[rand.Intn(len(projects))]
			
			// Mostly pending, some completed
			status := "pending"
			completedAt := "NULL"
			if rand.Intn(10) > 7 {
				status = "completed"
				completedAt = "CURRENT_TIMESTAMP"
			}

			dueAt := "NULL"
			if rand.Intn(10) > 7 {
				// Random due date in current month
				days := rand.Intn(28) + 1
				dueAt = fmt.Sprintf("'2026-01-%02d 12:00:00'", days) // Hardcoded for this demo context
			}

			scheduledAt := "NULL"
			if rand.Intn(10) > 8 {
				days := rand.Intn(28) + 1
				scheduledAt = fmt.Sprintf("'2026-01-%02d 09:00:00'", days)
			}

			estimate := "NULL"
			if rand.Intn(10) > 6 {
				estVals := []string{"30m", "1h", "2h", "4h", "1d"}
				estimate = fmt.Sprintf("'%s'", estVals[rand.Intn(len(estVals))])
			}

			query := fmt.Sprintf(`INSERT INTO tasks (description, project, status, completed_at, created_at, due_at, scheduled_at, estimate) 
				VALUES ('%s', '%s', '%s', %s, CURRENT_TIMESTAMP, %s, %s, %s)`, desc, proj, status, completedAt, dueAt, scheduledAt, estimate)
			
			_, err := tx.Exec(query)
			if err != nil {
				tx.Rollback()
				fmt.Printf("Error inserting task: %v\n", err)
				return
			}
		}

		// Add random dependencies
		// Collect all IDs first? No, we know IDs are 1..50 roughly (autoincrement)
		// Actually, let's just fetch IDs to be safe.
		rows, _ := tx.Query("SELECT id FROM tasks")
		var ids []int64
		for rows.Next() {
			var id int64
			rows.Scan(&id)
			ids = append(ids, id)
		}
		rows.Close()

		for i := 0; i < 20; i++ {
			blocker := ids[rand.Intn(len(ids))]
			blocked := ids[rand.Intn(len(ids))]
			if blocker != blocked {
				tx.Exec("INSERT OR IGNORE INTO task_dependencies (blocker_id, blocked_id) VALUES (?, ?)", blocker, blocked)
			}
		}

		err = tx.Commit()
		if err != nil {
			fmt.Printf("Error committing transaction: %v\n", err)
			return
		}

		fmt.Println("Successfully seeded 50 tasks.")
	},
}

func init() {
	rootCmd.AddCommand(seedCmd)
}
