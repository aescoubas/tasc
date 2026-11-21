package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/aescoubas/tasc/internal/models"
	"github.com/aescoubas/tasc/internal/store"
	"github.com/google/generative-ai-go/genai"
	"github.com/spf13/cobra"
	"google.golang.org/api/option"
)

var askCmd = &cobra.Command{
	Use:   "ask [prompt]",
	Short: "Ask Gemini about your tasks",
	Long:  `Send your current task list to Google's Gemini model and ask for advice, prioritization, or general queries.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		apiKey := os.Getenv("GEMINI_API_KEY")
		if apiKey == "" {
			return fmt.Errorf("GEMINI_API_KEY environment variable not set")
		}

		userPrompt := "What should I prioritize?"
		if len(args) > 0 {
			userPrompt = strings.Join(args, " ")
		}

		// 1. Fetch all pending tasks
		tasks, err := CurrentStore.ListTasks(store.TaskFilter{Status: []models.TaskStatus{
			models.StatusBacklog, models.StatusOngoing, models.StatusBlocked, models.StatusUndefined,
		}})
		if err != nil {
			return fmt.Errorf("list tasks: %w", err)
		}
		if len(tasks) == 0 {
			fmt.Println("No pending tasks to analyze.")
			return nil
		}

		var sb strings.Builder
		sb.WriteString("Here is my current task list:\n")

		for _, t := range tasks {
			due := "No due date"
			if t.DueAt != nil {
				due = t.DueAt.Format("2006-01-02")
			}

			fmt.Fprintf(&sb, "- [ID: %d] %s (Project: %s, Created: %s, Due: %s)\n",
				t.ID, t.Title, t.Project, t.CreatedAt.Format("2006-01-02"), due)
		}

		sb.WriteString("\nRequest: " + userPrompt)

		// 2. Call Gemini
		ctx := cmd.Context()
		client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
		if err != nil {
			return err
		}
		defer client.Close()

		model := client.GenerativeModel("gemini-1.5-flash")
		resp, err := model.GenerateContent(ctx, genai.Text(sb.String()))
		if err != nil {
			return err
		}

		// 3. Print response
		for _, cand := range resp.Candidates {
			if cand.Content != nil {
				for _, part := range cand.Content.Parts {
					fmt.Println(part)
				}
			}
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(askCmd)
}
