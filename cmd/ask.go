package cmd

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/aescoubas/tasc/internal/db"
	"github.com/aescoubas/tasc/internal/models"
	"github.com/google/generative-ai-go/genai"
	"github.com/spf13/cobra"
	"google.golang.org/api/option"
)

var askCmd = &cobra.Command{
	Use:   "ask [prompt]",
	Short: "Ask Gemini about your tasks",
	Long:  `Send your current task list to Google's Gemini model and ask for advice, prioritization, or general queries.`,
	Run: func(cmd *cobra.Command, args []string) {
		apiKey := os.Getenv("GEMINI_API_KEY")
		if apiKey == "" {
			fmt.Println("Error: GEMINI_API_KEY environment variable not set.")
			return
		}

		userPrompt := "What should I prioritize?"
		if len(args) > 0 {
			userPrompt = strings.Join(args, " ")
		}

		// 1. Fetch all pending tasks
		query := "SELECT id, title, project, created_at, due_at FROM tasks WHERE status != 'done' AND status != 'deleted'"
		rows, err := db.DB.Query(query)
		if err != nil {
			log.Fatalf("Error querying tasks: %v", err)
		}
		defer rows.Close()

		var sb strings.Builder
		sb.WriteString("Here is my current task list:\n")

		count := 0
		for rows.Next() {
			var t models.Task
			var project sql.NullString
			var dueAt sql.NullTime // use NullTime for scanning
			if err := rows.Scan(&t.ID, &t.Title, &project, &t.CreatedAt, &dueAt); err != nil {
				continue
			}
			due := "No due date"
			if dueAt.Valid {
				due = dueAt.Time.Format("2006-01-02")
			}

			sb.WriteString(fmt.Sprintf("- [ID: %d] %s (Project: %s, Created: %s, Due: %s)\n",
				t.ID, t.Title, project.String, t.CreatedAt.Format("2006-01-02"), due))
			count++
		}

		if count == 0 {
			fmt.Println("No pending tasks to analyze.")
			return
		}

		sb.WriteString("\nRequest: " + userPrompt)

		// 2. Call Gemini
		ctx := context.Background()
		client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
		if err != nil {
			log.Fatal(err)
		}
		defer client.Close()

		model := client.GenerativeModel("gemini-1.5-flash")
		resp, err := model.GenerateContent(ctx, genai.Text(sb.String()))
		if err != nil {
			log.Fatal(err)
		}

		// 3. Print response
		for _, cand := range resp.Candidates {
			if cand.Content != nil {
				for _, part := range cand.Content.Parts {
					fmt.Println(part)
				}
			}
		}
	},
}

func init() {
	rootCmd.AddCommand(askCmd)
}
