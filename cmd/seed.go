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

			query := fmt.Sprintf(`INSERT INTO tasks (description, project, status, completed_at, created_at) 
				VALUES ('%s', '%s', '%s', %s, CURRENT_TIMESTAMP)`, desc, proj, status, completedAt)
			
			_, err := tx.Exec(query)
			if err != nil {
				tx.Rollback()
				fmt.Printf("Error inserting task: %v\n", err)
				return
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
