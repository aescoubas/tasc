package cmd

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/aescoubas/tasc/internal/config"
	"github.com/aescoubas/tasc/internal/models"
	"github.com/aescoubas/tasc/internal/ui"
	"github.com/spf13/cobra"
)

var showOutput string

// taskRef is the shape of a neighbouring task in both the table and the JSON
// output, so its tags are part of the `--output json` contract.
type taskRef struct {
	ID    int64  `json:"id"`
	Title string `json:"title"`
}

var showCmd = &cobra.Command{
	Use:   "show [id]",
	Short: "Show detailed information for a task",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		outputFormat, err := validateOutputFormat(showOutput)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			return
		}

		cfg, _ := config.LoadConfig()

		id, err := strconv.Atoi(args[0])
		if err != nil {
			fmt.Println("Invalid task ID")
			return
		}

		// 1. Fetch Task
		t, err := CurrentStore.GetTask(int64(id))
		if errors.Is(err, sql.ErrNoRows) {
			fmt.Printf("Task %d not found.\n", id)
			return
		} else if err != nil {
			fmt.Printf("Error fetching task: %v\n", err)
			return
		}

		// 2. Fetch Dependencies
		blockedBy, blocking, err := showNeighbours(int64(id))
		if err != nil {
			fmt.Printf("Error fetching dependencies: %v\n", err)
			return
		}

		totalDuration := t.TimeSpent
		if t.ActiveStart != nil {
			totalDuration += int64(time.Since(*t.ActiveStart).Seconds())
		}

		if outputFormat == OutputJSON {
			payload := struct {
				Task             models.Task `json:"task"`
				BlockedBy        []taskRef   `json:"blocked_by"`
				Blocking         []taskRef   `json:"blocking"`
				TimeSpentSeconds int64       `json:"time_spent_seconds"`
			}{
				Task:             t,
				BlockedBy:        blockedBy,
				Blocking:         blocking,
				TimeSpentSeconds: totalDuration,
			}

			data, err := json.MarshalIndent(payload, "", "  ")
			if err != nil {
				fmt.Printf("Error encoding JSON output: %v\n", err)
				return
			}
			fmt.Println(string(data))
			return
		}

		// 3. Display
		t.IsBlocked = len(blockedBy) > 0
		style := ui.GetTaskStyle(t, cfg)

		fmt.Println(style.Render(fmt.Sprintf("Task %d: %s", t.ID, t.Title)))
		if t.Description != "" {
			fmt.Println("------------------------------------------------")
			fmt.Println(t.Description)
		}
		fmt.Println("------------------------------------------------")
		fmt.Printf("Project:       %s\n", t.Project)
		fmt.Printf("Status:        %s\n", t.Status)
		fmt.Printf("Created:       %s\n", ui.FormatTimestamp(t.CreatedAt))

		if t.CompletedAt != nil {
			fmt.Printf("Completed:     %s\n", ui.FormatTimestamp(*t.CompletedAt))
		}

		dueStr := "-"
		if t.DueAt != nil {
			dueStr = ui.FormatTimestamp(*t.DueAt)
		}
		fmt.Printf("Due:           %s\n", dueStr)

		if t.RescheduleCount > 0 {
			fmt.Printf("Rescheduled:   %d times\n", t.RescheduleCount)
		}

		if t.Estimate != "" {
			fmt.Printf("Estimate:      %s\n", t.Estimate)
		}

		if t.Recurrence != "" {
			fmt.Printf("Recurrence:    %s\n", t.Recurrence)
		}

		// Duration calculation
		if t.ActiveStart != nil {
			fmt.Printf("Active since:  %s\n", t.ActiveStart.Format("15:04:05"))
		}
		if totalDuration > 0 {
			dur := time.Duration(totalDuration) * time.Second
			fmt.Printf("Time Spent:    %s\n", dur.String())
		}

		// Dependencies display
		if len(blockedBy) > 0 {
			fmt.Println()
			fmt.Println("Blocked By:")
			for _, b := range blockedBy {
				fmt.Printf("  - %d (%s)\n", b.ID, b.Title)
			}
		}

		if len(blocking) > 0 {
			fmt.Println()
			fmt.Println("Blocking:")
			for _, b := range blocking {
				fmt.Printf("  - %d (%s)\n", b.ID, b.Title)
			}
		}
		fmt.Println()
	},
}

// showNeighbours resolves one task's dependency edges into the two lists show
// prints: the tasks still blocking it, and the tasks it blocks.
//
// This used to be two JOINs against db.DB, which is nil whenever TASC_REMOTE is
// set — root.go only calls InitDB for the sqlite backend, so `tasc show` panicked
// against a server. Titles now come from GetTask, which applies no status filter
// on either backend, and the edges from one GetDependencies call.
func showNeighbours(id int64) (blockedBy, blocking []taskRef, err error) {
	deps, err := CurrentStore.GetDependencies()
	if err != nil {
		return nil, nil, fmt.Errorf("get dependencies: %w", err)
	}

	// One task can sit on both sides of several edges, and the two loops below
	// ask about overlapping IDs, so resolve each neighbour once. A neighbour that
	// does not resolve is dropped, which is what the old JOIN did with an edge
	// pointing at a row that is not there — foreign_keys is off on this
	// connection, so a mistyped ID can leave one behind.
	resolved := make(map[int64]models.Task)
	absent := make(map[int64]bool)
	resolve := func(nid int64) (models.Task, bool) {
		if absent[nid] {
			return models.Task{}, false
		}
		if t, ok := resolved[nid]; ok {
			return t, true
		}
		t, err := CurrentStore.GetTask(nid)
		if err != nil {
			absent[nid] = true
			return models.Task{}, false
		}
		resolved[nid] = t
		return t, true
	}

	for _, d := range deps {
		switch {
		case d.BlockedID == id:
			// A done or deleted blocker no longer blocks anything, and listing it
			// here would contradict is_blocked in every other view. The edge itself
			// stays visible in `tasc graph` and `tasc_get_dependencies`.
			t, ok := resolve(d.BlockerID)
			if !ok || t.Status == models.StatusDone || t.Status == models.StatusDeleted {
				continue
			}
			blockedBy = append(blockedBy, taskRef{ID: t.ID, Title: t.Title})
		case d.BlockerID == id:
			t, ok := resolve(d.BlockedID)
			if !ok {
				continue
			}
			blocking = append(blocking, taskRef{ID: t.ID, Title: t.Title})
		}
	}

	// Neither JOIN had an ORDER BY, so the two lists came out in whatever order
	// the rows happened to sit in. Sort them so repeated runs agree.
	sort.Slice(blockedBy, func(i, j int) bool { return blockedBy[i].ID < blockedBy[j].ID })
	sort.Slice(blocking, func(i, j int) bool { return blocking[i].ID < blocking[j].ID })

	return blockedBy, blocking, nil
}

func init() {
	showCmd.Flags().StringVar(&showOutput, "output", OutputTable, "Output format (table, json)")
	showCmd.RegisterFlagCompletionFunc("output", outputFormatCompletionFunc)
	rootCmd.AddCommand(showCmd)
}
