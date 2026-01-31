package ui

import (
	"fmt"
	"strings"

	"github.com/aescoubas/tasc/internal/config"
	"github.com/aescoubas/tasc/internal/models"
)

// FormatTask returns a consistent one-line string representation of a task.
// Format: [Icon] ID Project: Description
func FormatTask(t models.Task, cfg config.Config) string {
	style := GetTaskStyle(t, cfg)
	
	icon := " "
	if t.Status == models.StatusDone {
		icon = "✓"
	} else if t.Status == models.StatusOngoing {
		icon = "▶"
	} else if t.IsBlocked || t.Status == models.StatusBlocked {
		icon = "🚫"
	} else {
		// Pending
		icon = "•"
	}

	proj := ""
	if t.Project != "" {
		proj = t.Project + " " // "Project "
	}
	
	// Clean description (remove newlines)
	desc := strings.ReplaceAll(t.Description, "\n", " ")
	
	// Format: Icon ID Project Description
	// Example: • 12 Work Finish report
	return style.Render(fmt.Sprintf("%s %d %s%s", icon, t.ID, proj, desc))
}

