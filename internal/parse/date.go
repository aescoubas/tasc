package parse

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/tj/go-naturaldate"
)

func Date(s string, defaultTime string) (*time.Time, error) {
	// 1. Explicit formats
	formats := []struct {
		layout  string
		hasTime bool
	}{
		{"2006-01-02", false},
		{"2006/01/02", false},
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
				return &t, nil
			}
			
			if !f.hasTime && defaultTime != "" {
				// Apply default time
				t = applyTime(t, defaultTime)
			}
			return &t, nil
		}
	}

	// 2. Natural date parsing
	processed := PreprocessDate(s)
	// We use 00:00:00 today as reference to avoid "now" time leaking into dates like "tomorrow"
	now := time.Now()
	ref := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	
	t, err := naturaldate.Parse(processed, ref)
	if err == nil {
		// Heuristic: If the input string contains time indicators, trust the result.
		// If not, apply defaultTime.
		// Check original string 's' for time patterns
		hasTime := regexp.MustCompile(`(\d{1,2}:\d{2})|(?i)(am|pm|morning|afternoon|evening|noon|midnight)`).MatchString(s)
		
		if !hasTime && defaultTime != "" {
			t = applyTime(t, defaultTime)
		}
		return &t, nil
	}

	return nil, fmt.Errorf("could not parse date %q", s)
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
	s = re.ReplaceAllString(s, "$1 $2")

	// 2. "week" -> "next week"
	if s == "week" {
		return "next week"
	}

	// 3. If it looks like a duration "10 days", "2 weeks", prepend "in " to force future
	reDuration := regexp.MustCompile(`(?i)^\d+\s+(day|days|week|weeks|month|months|year|years)$`)
	if reDuration.MatchString(s) {
		return "in " + s
	}

	return s
}
