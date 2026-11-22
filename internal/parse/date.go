package parse

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/tj/go-naturaldate"
)

func Date(s string, defaultTime string) (*time.Time, error) {
	due, _, err := DateWithPrecision(s, defaultTime)
	return due, err
}

var clockInput = regexp.MustCompile(`(?i)\d{1,2}:\d{2}|\d\s*(?:am|pm)\b|\b(?:morning|afternoon|evening|noon|midnight)\b|\b0{1,2}$`)
var subdayInput = regexp.MustCompile(`\b(?:hours?|minutes?)\b`)
var twelveClock = regexp.MustCompile(`\b12((?::\d{1,2}){0,2})\s*(am|pm)\b`)
var namedClock = regexp.MustCompile(`\b(noon|midnight)\b`)

// DateWithPrecision reports whether the input is date-only, even when a
// default clock time is attached to the returned deadline.
func DateWithPrecision(s string, defaultTime string) (*time.Time, bool, error) {
	s = strings.TrimSpace(s)
	// 1. Explicit formats
	formats := []struct {
		layout  string
		hasTime bool
	}{
		{"2006-01-02", false},
		{"2006/01/02", false},
		{"02.01.2006", false},
		{"02/01/2006", false},
		{"02-01-2006", false},
		{"2006-01-02 15:04", true},
		{"15:04", true},
		{time.RFC3339, true},
	}

	for _, f := range formats {
		t, err := time.ParseInLocation(f.layout, s, time.Local)
		if err == nil {
			if f.layout == "15:04" {
				now := time.Now()
				t = time.Date(now.Year(), now.Month(), now.Day(), t.Hour(), t.Minute(), 0, 0, time.Local)
				return &t, false, nil
			}

			if !f.hasTime && defaultTime != "" {
				// Apply default time
				t = applyTime(t, defaultTime)
			}
			return &t, !f.hasTime, nil
		}
	}

	// 2. Natural date parsing
	processed := PreprocessDate(s)
	// go-naturaldate applies AM/PM as a raw offset, which misreads 12 AM
	// and rolls 12 PM into the next day. Supply a 24-hour clock for these.
	processed = twelveClock.ReplaceAllStringFunc(processed, func(clock string) string {
		parts := twelveClock.FindStringSubmatch(clock)
		hour, rest := "12", parts[1]
		if parts[2] == "am" {
			hour = "00"
		}
		if rest == "" {
			rest = ":00"
		}
		return hour + rest
	})
	processed = namedClock.ReplaceAllStringFunc(processed, func(clock string) string {
		if clock == "noon" {
			return "12:00"
		}
		return "00:00"
	})
	// Keep date-only expressions anchored to the local calendar day. Relative
	// hours and minutes, and "now" itself, are timed inputs.
	now := time.Now()
	ref := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	relativeClock := subdayInput.MatchString(processed) || processed == "now" || processed == "right now"
	if relativeClock {
		ref = now
	}

	t, err := naturaldate.Parse(processed, ref)
	if err == nil {
		// The parser also accepts bare clock hours. A midnight result needs an
		// explicit clock marker to distinguish it from the date-only anchor.
		dateOnly := !relativeClock && !clockInput.MatchString(s) &&
			t.Hour() == 0 && t.Minute() == 0 && t.Second() == 0
		if dateOnly && defaultTime != "" {
			t = applyTime(t, defaultTime)
		}
		return &t, dateOnly, nil
	}

	return nil, false, fmt.Errorf("could not parse date %q", s)
}

func applyTime(t time.Time, hm string) time.Time {
	parts := strings.Split(hm, ":")
	if len(parts) != 2 {
		return t
	}
	// Parse HH:MM loosely
	var h, m int
	fmt.Sscanf(parts[0], "%d", &h)
	fmt.Sscanf(parts[1], "%d", &m)
	return time.Date(t.Year(), t.Month(), t.Day(), h, m, 0, 0, t.Location())
}

func PreprocessDate(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ToLower(s)

	// Map words to digits for common durations
	wordMap := map[string]string{
		"one": "1", "two": "2", "three": "3", "four": "4", "five": "5",
		"six": "6", "seven": "7", "eight": "8", "nine": "9", "ten": "10",
	}

	parts := strings.Fields(s)
	if len(parts) > 0 {
		if val, ok := wordMap[parts[0]]; ok {
			parts[0] = val
			s = strings.Join(parts, " ")
		}
	}

	// 1. Split number and unit if stuck together: "10days" -> "10 days"
	re := regexp.MustCompile(`^(\d+)([a-zA-Z]+)$`)
	if matches := re.FindStringSubmatch(s); len(matches) == 3 {
		num := matches[1]
		unit := matches[2]

		if num != "1" {
			// Normalize singular units to plural for consistency if > 1
			switch strings.ToLower(unit) {
			case "day":
				unit = "days"
			case "week":
				unit = "weeks"
			case "month":
				unit = "months"
			case "year":
				unit = "years"
			}
		}
		s = num + " " + unit
	} else {
		s = re.ReplaceAllString(s, "$1 $2")
	}

	// 2. "week" -> "next week", "month" -> "next month"
	if s == "week" {
		return "next week"
	}
	if s == "month" {
		return "next month"
	}

	// 3. If it looks like a duration "10 days", "2 weeks", prepend "in " to force future
	reDuration := regexp.MustCompile(`(?i)^\d+\s+(day|days|week|weeks|month|months|year|years)$`)
	if reDuration.MatchString(s) {
		return "in " + s
	}

	// 4. "monday" -> "next monday"
	// We want to force next occurrence for weekdays, avoiding "past" or "today" ambiguity
	reWeekday := regexp.MustCompile(`^(on\s+)?(monday|tuesday|wednesday|thursday|friday|saturday|sunday)\b`)
	if reWeekday.MatchString(s) {
		// Replace the matching part (optional "on " + weekday) with "next weekday"
		// $2 refers to the weekday capture group
		return reWeekday.ReplaceAllString(s, "next $2")
	}

	return s
}
