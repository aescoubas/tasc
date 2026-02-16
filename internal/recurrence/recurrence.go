package recurrence

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Next calculates the next occurrence date based on the last date and a recurrence rule.
func Next(last time.Time, rule string) (time.Time, error) {
	rule = strings.ToLower(strings.TrimSpace(rule))

	if rule == "daily" || rule == "every day" {
		return last.AddDate(0, 0, 1), nil
	}
	if rule == "weekly" || rule == "every week" {
		return last.AddDate(0, 0, 7), nil
	}
	if rule == "monthly" || rule == "every month" {
		return last.AddDate(0, 1, 0), nil
	}
	if rule == "yearly" || rule == "every year" {
		return last.AddDate(1, 0, 0), nil
	}

	// Regex for "every N units"
	// e.g. "every 2 days", "every 3 weeks"
	reSimple := regexp.MustCompile(`^every (\d+)\s+(day|days|week|weeks|month|months|year|years)$`)
	matches := reSimple.FindStringSubmatch(rule)
	if len(matches) == 3 {
		n, err := strconv.Atoi(matches[1])
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid number in recurrence: %v", err)
		}
		unit := matches[2]
		switch {
		case strings.HasPrefix(unit, "day"):
			return last.AddDate(0, 0, n), nil
		case strings.HasPrefix(unit, "week"):
			return last.AddDate(0, 0, n*7), nil
		case strings.HasPrefix(unit, "month"):
			return last.AddDate(0, n, 0), nil
		case strings.HasPrefix(unit, "year"):
			return last.AddDate(n, 0, 0), nil
		}
	}

	// Complex recurrence: "Second monday of each month"
	// Structure: [ordinal] [weekday] of [each] month
	reComplex := regexp.MustCompile(`^(first|second|third|fourth|last)\s+(monday|tuesday|wednesday|thursday|friday|saturday|sunday)\s+of\s+(each\s+)?month$`)
	complexMatches := reComplex.FindStringSubmatch(rule)
	if len(complexMatches) >= 3 {
		ordinal := complexMatches[1]
		weekdayStr := complexMatches[2]

		targetWeekday := parseWeekday(weekdayStr)

		// Start looking from the next month of the 'last' date
		// Or should we verify if the 'last' date matches the rule?
		// The requirement usually implies: give me the NEXT one after 'last'.
		// If 'last' was "Jan 13 (2nd Mon)", next should be "Feb 10 (2nd Mon)".
		// If 'last' was some random date, we find the next occurrence strictly after 'last'.

		next := last
		for {
			// Move to next month (or initially just ensuring we are looking forward)
			// But careful: if 'last' is strictly before the occurrence in the SAME month, we should return that.
			// However, usually 'last' IS the previous occurrence. So we want the occurrence in the NEXT month?
			// Let's iterate months starting from 'last' month.

			// Actually, easiest is:
			// Calculate occurrence for Current Month of 'last'.
			// If it is > last, return it.
			// Else, Calculate occurrence for Next Month.

			// We need a loop because "First Monday" might have already passed in the current month.

			// Let's start checking from the month of 'last'
			candidate := getNthWeekdayOfMonth(next.Year(), next.Month(), ordinal, targetWeekday)

			// If candidate is strictly after last, we found it.
			// Note: We use strictly after because 'last' is presumably the previous execution.
			// Unless 'last' is just "now" when setting it up?
			// But for "Next()", we assume 'last' is the reference point.
			if candidate.After(last) {
				// Preserve time? Usually recurrence sets date.
				// Set the next due date to that exact calendar date.
				// 'last' might have a time component (e.g. 9:00 AM).
				// We should probably preserve the time of day from 'last'.
				return time.Date(candidate.Year(), candidate.Month(), candidate.Day(),
					last.Hour(), last.Minute(), last.Second(), last.Nanosecond(), last.Location()), nil
			}

			// Move to next month
			// Safest way: Set day to 1, THEN add 1 month to avoid normalization issues (e.g. Jan 31 + 1mo -> Mar 3)
			next = time.Date(next.Year(), next.Month(), 1, 0, 0, 0, 0, next.Location()).AddDate(0, 1, 0)
		}
	}

	return time.Time{}, fmt.Errorf("unsupported recurrence rule: %q", rule)
}

func parseWeekday(s string) time.Weekday {
	switch strings.ToLower(s) {
	case "sunday":
		return time.Sunday
	case "monday":
		return time.Monday
	case "tuesday":
		return time.Tuesday
	case "wednesday":
		return time.Wednesday
	case "thursday":
		return time.Thursday
	case "friday":
		return time.Friday
	case "saturday":
		return time.Saturday
	}
	return time.Sunday
}

func getNthWeekdayOfMonth(year int, month time.Month, ordinal string, target time.Weekday) time.Time {
	firstDay := time.Date(year, month, 1, 0, 0, 0, 0, time.Local)

	if ordinal == "last" {
		// Start from next month's first day and go back
		nextMonth := firstDay.AddDate(0, 1, 0)
		lastDay := nextMonth.AddDate(0, 0, -1)

		// Rewind until we hit target weekday
		for lastDay.Weekday() != target {
			lastDay = lastDay.AddDate(0, 0, -1)
		}
		return lastDay
	}

	// Forward search
	// Find first occurrence of weekday
	// Difference: (Target - FirstDay + 7) % 7
	diff := (int(target) - int(firstDay.Weekday()) + 7) % 7
	firstOccurrence := firstDay.AddDate(0, 0, diff)

	n := 1
	switch ordinal {
	case "first":
		n = 1
	case "second":
		n = 2
	case "third":
		n = 3
	case "fourth":
		n = 4
	}

	return firstOccurrence.AddDate(0, 0, (n-1)*7)
}
