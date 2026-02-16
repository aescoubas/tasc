package cmd

import (
	"fmt"
	"math/rand"
	"strings"
	"time"

	"github.com/aescoubas/tasc/internal/db"
	"github.com/spf13/cobra"
)

var seedCmd = &cobra.Command{
	Use:   "seed",
	Short: "Populate the database with dummy data",
	Run: func(cmd *cobra.Command, args []string) {
		projects := []string{"Work", "Personal", "Tasc", "Health", "Finance", "Learning", "Home"}
		verbs := []string{"Fix", "Buy", "Write", "Call", "Email", "Review", "Clean", "Pay", "Update", "Deploy", "Research", "Design", "Test", "Meeting with"}
		nouns := []string{"bug", "milk", "documentation", "mom", "client", "code", "house", "bills", "dependencies", "server", "proposal", "architecture", "gym", "groceries"}

		// Seed random number generator
		rand.Seed(time.Now().UnixNano())

		tx, err := db.DB.Begin()
		if err != nil {
			fmt.Printf("Error starting transaction: %v\n", err)
			return
		}

		fmt.Println("Seeding database with 100 tasks over the next 60 days...")

		now := time.Now()

		for i := 0; i < 100; i++ {
			desc := fmt.Sprintf("%s %s %d", verbs[rand.Intn(len(verbs))], nouns[rand.Intn(len(nouns))], i)
			proj := projects[rand.Intn(len(projects))]

			// Status: mostly backlog
			status := "backlog"
			var completedAt interface{} = nil // database/sql uses nil for NULL

			if rand.Intn(100) > 90 { // 10% done
				status = "done"
				completedAt = now.Format("2006-01-02 15:04:05")
			}

			// Due Date: mostly in the next 60 days
			var dueAt interface{} = nil
			if rand.Intn(10) > 1 { // 80% have due date
				daysOffset := rand.Intn(60)
				hour := 9 + rand.Intn(9)
				dueTime := now.AddDate(0, 0, daysOffset)
				dueTime = time.Date(dueTime.Year(), dueTime.Month(), dueTime.Day(), hour, 0, 0, 0, dueTime.Location())
				dueAt = dueTime.Format("2006-01-02 15:04:05")
			}

			// Estimate (Always present now)
			estVals := []string{"30m", "1h", "2h", "4h", "1d"}
			estStr := estVals[rand.Intn(len(estVals))]
			estimate := estStr

			// Time Spent (Actual)
			// Only meaningful if we have an estimate and status is done (or ongoing, but stats checks done)
			var timeSpent int64 = 0
			if status == "done" {
				// Parse estimate to get base duration
				d, _ := time.ParseDuration(estStr)
				if strings.HasSuffix(estStr, "d") {
					d = 24 * time.Hour // Simple fallback for "1d" since ParseDuration might fail on "d" depending on version/logic, but let's stick to simple
				}

				baseSeconds := d.Seconds()

				// Variance: +/- 50%
				variance := (rand.Float64() - 0.5) // -0.5 to 0.5
				actualSeconds := baseSeconds * (1.0 + variance)
				timeSpent = int64(actualSeconds)
			}

			query := `INSERT INTO tasks (title, project, status, completed_at, created_at, due_at, estimate, time_spent) 
				VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP, ?, ?, ?)`

			_, err := tx.Exec(query, desc, proj, status, completedAt, dueAt, estimate, timeSpent)
			if err != nil {
				tx.Rollback()
				fmt.Printf("Error inserting task: %v\n", err)
				return
			}
		}

		// Ensure projects exist in 'projects' table (if we enforced FKs, but for now we just auto-migrate or ignore)
		// Assuming 'projects' table is populated by 'tasc project' logic or implicitly by 'tasc list' (which creates project entries? No, 'tasc project' manages them).
		// We should probably insert projects into 'projects' table if they don't exist to be clean.
		for _, p := range projects {
			tx.Exec("INSERT OR IGNORE INTO projects (name, description) VALUES (?, ?)", p, "Seeded project")
		}

		// Add random dependencies

		rows, _ := tx.Query("SELECT id FROM tasks ORDER BY id DESC LIMIT 100") // Get recent ones
		var ids []int64
		for rows.Next() {
			var id int64
			rows.Scan(&id)
			ids = append(ids, id)
		}
		rows.Close()

		if len(ids) > 0 {
			for i := 0; i < 30; i++ {
				blocker := ids[rand.Intn(len(ids))]
				blocked := ids[rand.Intn(len(ids))]
				if blocker != blocked {
					tx.Exec("INSERT OR IGNORE INTO task_dependencies (blocker_id, blocked_id) VALUES (?, ?)", blocker, blocked)
				}
			}
		}

		err = tx.Commit()
		if err != nil {
			fmt.Printf("Error committing transaction: %v\n", err)
			return
		}

		fmt.Println("Successfully seeded 100 tasks.")
	},
}

func init() {
	rootCmd.AddCommand(seedCmd)
}
