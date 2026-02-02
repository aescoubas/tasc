package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/aescoubas/tasc/internal/db"
	"github.com/aescoubas/tasc/internal/models"
	"github.com/spf13/cobra"
)

var importTwCmd = &cobra.Command{
	Use:   "import-tw",
	Short: "Import tasks from Taskwarrior export",
	Long: `Import tasks from a Taskwarrior JSON export.

Usage:
  task export | tasc import-tw
  tasc import-tw < export.json

Mismatches and Resolutions:
1. Tags: Tasc has no tags. Taskwarrior tags are appended to the description as hashtags (e.g., #work).
2. Annotations: Appended to the description.
3. Priority: Tasc uses dynamic priority. Taskwarrior priority (H/M/L) is appended to the description (e.g., [Priority: H]).
4. UDAs: User Defined Attributes are ignored.
5. UUIDs: Mapped to new integer IDs internally to preserve dependencies.
`,
	Run: runImportTw,
}

func init() {
	rootCmd.AddCommand(importTwCmd)
}

type twAnnotation struct {
	Entry       string `json:"entry"`
	Description string `json:"description"`
}

type twTask struct {
	UUID        string         `json:"uuid"`
	Description string         `json:"description"`
	Status      string         `json:"status"`
	Entry       string         `json:"entry"`
	Start       string         `json:"start"`
	End         string         `json:"end"`
	Due         string         `json:"due"`
	Scheduled   string         `json:"scheduled"`
	Wait        string         `json:"wait"`
	Project     string         `json:"project"`
	Tags        []string       `json:"tags"`
		Priority    string         `json:"priority"`
		Depends     interface{}    `json:"depends"`
		Annotations []twAnnotation `json:"annotations"`
	}
	
	func runImportTw(cmd *cobra.Command, args []string) {
		stat, _ := os.Stdin.Stat()
		if (stat.Mode() & os.ModeCharDevice) != 0 {
			fmt.Println("Please pipe Taskwarrior JSON export to this command.")
			fmt.Println("Example: task export | tasc import-tw")
			return
		}
	
		decoder := json.NewDecoder(newCleanReader(os.Stdin))
		var twTasks []twTask
		if err := decoder.Decode(&twTasks); err != nil {
			fmt.Printf("Error parsing JSON: %v\n", err)
			return
		}
	
		fmt.Printf("Found %d tasks to import.\n", len(twTasks))
	
		uuidMap := make(map[string]int64)
		dependencyMap := make(map[string][]string) // uuid -> []depends_uuids
	
		tx, err := db.DB.Begin()
		if err != nil {
			fmt.Printf("Error starting transaction: %v\n", err)
			return
		}
	
		stmt, err := tx.Prepare(`
			INSERT INTO tasks (
				title, description, project, status, created_at, 
				completed_at, due_at, scheduled_at, active_start
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		`)
		if err != nil {
			fmt.Printf("Error preparing statement: %v\n", err)
			tx.Rollback()
			return
		}
		defer stmt.Close()
	
		count := 0
		for _, tw := range twTasks {
			// 1. Map Status
			var status models.TaskStatus
			switch tw.Status {
			case "completed":
				status = models.StatusDone
			case "deleted":
				status = models.StatusDeleted
			default: // pending, waiting, recurring
				status = models.StatusBacklog
			}
	
			// 2. Map Title (Append Tags, Priority)
			title := tw.Description
			if tw.Priority != "" {
				title += fmt.Sprintf(" [Priority: %s]", tw.Priority)
			}
			if len(tw.Tags) > 0 {
				var tags []string
				for _, t := range tw.Tags {
					tags = append(tags, "#"+t)
				}
				title += " " + strings.Join(tags, " ")
			}
			
			// Map Description (Annotations)
			description := ""
			if len(tw.Annotations) > 0 {
				description = "-- Annotations --"
				for _, ann := range tw.Annotations {
					description += fmt.Sprintf("\n- %s", ann.Description)
				}
			}
	
			// 3. Parse Dates
			createdAt := parseTwDate(tw.Entry)
			if createdAt.IsZero() {
				createdAt = time.Now()
			}
			completedAt := parseTwDatePtr(tw.End)
			dueAt := parseTwDatePtr(tw.Due)
			
			scheduledAt := parseTwDatePtr(tw.Scheduled)
			if scheduledAt == nil {
				scheduledAt = parseTwDatePtr(tw.Wait) // Fallback to 'wait'
			}
	
			activeStart := parseTwDatePtr(tw.Start)
	
			// 4. Insert
			res, err := stmt.Exec(title, description, tw.Project, status, createdAt, completedAt, dueAt, scheduledAt, activeStart)
			if err != nil {
				fmt.Printf("Failed to insert task %s: %v\n", tw.UUID, err)
				continue
			}
	
			id, _ := res.LastInsertId()
			uuidMap[tw.UUID] = id
			count++
	
			// 5. Queue Dependencies
			var deps []string
			switch v := tw.Depends.(type) {
			case string:
				if v != "" {
					deps = strings.Split(v, ",")
				}
			case []interface{}:
				for _, item := range v {
					if s, ok := item.(string); ok {
						deps = append(deps, s)
					}
				}
			}
			
			if len(deps) > 0 {
				dependencyMap[tw.UUID] = deps
			}
		}	
		// 6. Insert Dependencies
		depStmt, err := tx.Prepare("INSERT INTO task_dependencies (blocker_id, blocked_id) VALUES (?, ?)")
		if err != nil {
			fmt.Printf("Error preparing dependency statement: %v\n", err)
			tx.Rollback()
			return
		}
		defer depStmt.Close()
	
		depCount := 0
		for blockedUUID, blockerUUIDs := range dependencyMap {
			blockedID, ok := uuidMap[blockedUUID]
			if !ok {
				continue
			}
			for _, blockerUUID := range blockerUUIDs {
				blockerID, ok := uuidMap[blockerUUID]
				if ok {
					_, err := depStmt.Exec(blockerID, blockedID)
					if err != nil {
						// Ignore duplicate key errors if any
						continue
					}
					depCount++
				}
			}
		}
	
		if err := tx.Commit(); err != nil {
			fmt.Printf("Error committing transaction: %v\n", err)
			return
		}
	
		fmt.Printf("Successfully imported %d tasks and %d dependencies.\n", count, depCount)
	}
	
	// cleanReader filters out invalid control characters (like escape codes) from the stream.
	type cleanReader struct {
		r io.Reader
	}
	
	func newCleanReader(r io.Reader) *cleanReader {
		return &cleanReader{r: r}
	}
	
	func (cr *cleanReader) Read(p []byte) (n int, err error) {
		n, err = cr.r.Read(p)
		if n > 0 {
			// Filter in-place
			w := 0
			for r := 0; r < n; r++ {
				b := p[r]
				// 0x1b is ESC. We skip it.
				// We could also filter other control chars if needed, but 0x1b is the main culprit here.
				if b != 0x1b {
					p[w] = b
					w++
				}
			}
			n = w
		}
		return n, err
	}
	
	func parseTwDate(s string) time.Time {	if s == "" {
		return time.Time{}
	}
	// Taskwarrior JSON format: 20210101T120000Z
	layout := "20060102T150405Z"
	t, err := time.Parse(layout, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

func parseTwDatePtr(s string) *time.Time {
	t := parseTwDate(s)
	if t.IsZero() {
		return nil
	}
	return &t
}
