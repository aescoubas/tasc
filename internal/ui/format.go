package ui

import (
	"fmt"
	"strings"
	"time"

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
	desc := strings.ReplaceAll(t.Title, "\n", " ")
	
	// Format: Icon ID Project Description
	// Example: • 12 Work Finish report
	return style.Render(fmt.Sprintf("%s %d %s%s", icon, t.ID, proj, desc))
}

func FormatRelative(t time.Time) string {
    now := time.Now()
    diff := t.Sub(now)
    
    suffix := ""
    prefix := ""
    if diff < 0 {
        prefix = "-"
        diff = -diff
    }
    
    if diff < time.Minute {
        return prefix + fmt.Sprintf("%ds", int(diff.Seconds())) + suffix
    }
    if diff < time.Hour {
        return prefix + fmt.Sprintf("%dm", int(diff.Minutes())) + suffix
    }
    if diff < 24*time.Hour {
        return prefix + fmt.Sprintf("%dh", int(diff.Hours())) + suffix
    }
    	return prefix + fmt.Sprintf("%dd", int(diff.Hours()/24)) + suffix
    }
    
    func FormatDate(t time.Time, cfg config.Config) string {
    	format := cfg.DateFormat
    	if format == "" {
    		format = "02-01-2006"
    	}
    	
    	// Map common user-friendly formats to Go layout
    	switch format {
    	case "YYYY-MM-DD":
    		format = "2006-01-02"
    	case "DD-MM-YYYY":
    		format = "02-01-2006"
    	case "MM-DD-YYYY":
    		format = "01-02-2006"
    	case "YYYY/MM/DD":
    		format = "2006/01/02"
    	case "DD/MM/YYYY":
    		format = "02/01/2006"
    	}
    	
    	return t.Format(format)
    }
