package parse

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/tj/go-naturaldate"
)

func Date(s string) (*time.Time, error) {
	formats := []string{
		"2006-01-02",
		"2006/01/02",
		time.RFC3339,
	}
	for _, f := range formats {
		t, err := time.Parse(f, s)
		if err == nil {
			return &t, nil
		}
	}

	// Try natural date parsing
	processed := PreprocessDate(s)
	t, err := naturaldate.Parse(processed, time.Now())
	if err == nil {
		return &t, nil
	}

	return nil, fmt.Errorf("could not parse date %q", s)
}

func PreprocessDate(s string) string {
	s = strings.TrimSpace(s)
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
