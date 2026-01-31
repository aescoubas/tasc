package ui

import (
	"time"

	"github.com/aescoubas/tasc/internal/config"
	"github.com/aescoubas/tasc/internal/models"
	"github.com/charmbracelet/lipgloss"
)

type UrgencyTier int

const (
	TierBlocked UrgencyTier = iota
	TierOverdue
	TierToday
	TierTomorrow
	TierWeek
	TierDefault
)

// GetUrgencyTier determines the urgency level of a task based on its DueAt and ScheduledAt times.
func GetUrgencyTier(t models.Task) UrgencyTier {
	if t.IsBlocked || t.Status == models.StatusBlocked {
		return TierBlocked
	}

	now := time.Now()
	// Standardize "Today" boundaries
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	tomorrowStart := startOfDay.AddDate(0, 0, 1)
	weekStart := startOfDay.AddDate(0, 0, 2)
	weekEnd := startOfDay.AddDate(0, 0, 9)

	checkTier := func(dt *time.Time) UrgencyTier {
		if dt == nil {
			return TierDefault
		}
		if dt.Before(now) {
			return TierOverdue
		}
		if dt.Before(tomorrowStart) {
			return TierToday
		}
		if dt.Before(weekStart) {
			return TierTomorrow
		}
		if dt.Before(weekEnd) {
			return TierWeek
		}
		return TierDefault
	}

	tierDue := checkTier(t.DueAt)
	tierSch := checkTier(t.ScheduledAt)

	// Lower enum value = higher urgency
	if tierDue < tierSch {
		return tierDue
	}
	return tierSch
}

// GetTaskStyle returns the lipgloss Style for a task based on its urgency and the user config.
func GetTaskStyle(t models.Task, cfg config.Config) lipgloss.Style {
	tier := GetUrgencyTier(t)
	return GetStyleForTier(tier, cfg)
}

// GetStyleForTier returns the lipgloss Style for a specific UrgencyTier.
func GetStyleForTier(tier UrgencyTier, cfg config.Config) lipgloss.Style {
	makeStyle := func(fg, bg string) lipgloss.Style {
		s := lipgloss.NewStyle().Foreground(lipgloss.Color(fg))
		if bg != "" {
			s = s.Background(lipgloss.Color(bg))
		}
		return s
	}

	switch tier {
	case TierBlocked:
		return makeStyle(cfg.Colors.Blocked, cfg.Colors.BlockedBg)
	case TierOverdue:
		return makeStyle(cfg.Colors.Overdue, cfg.Colors.OverdueBg)
	case TierToday:
		return makeStyle(cfg.Colors.Today, cfg.Colors.TodayBg)
	case TierTomorrow:
		return makeStyle(cfg.Colors.Tomorrow, cfg.Colors.TomorrowBg)
	case TierWeek:
		return makeStyle(cfg.Colors.Week, cfg.Colors.WeekBg)
	default:
		return makeStyle(cfg.Colors.Default, cfg.Colors.DefaultBg)
	}
}
