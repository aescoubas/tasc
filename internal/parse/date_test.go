package parse

import "testing"

func TestPreprocessDate(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"tomorrow", "tomorrow"},
		{"next week", "next week"},
		{"week", "next week"},
		
		// Spaced out
		{"10 days", "in 10 days"},
		{"2 weeks", "in 2 weeks"},
		{"1 month", "in 1 month"},
		{"3 years", "in 3 years"},

		// Stuck together
		{"10days", "in 10 days"},
		{"2weeks", "in 2 weeks"},
		{"5months", "in 5 months"},
		{"1year", "in 1 year"},

		// Mixed case/variations
		{"2Days", "in 2 Days"},
		
		// Unaffected
		{"in 2 days", "in 2 days"}, // Already has 'in'
		{"monday", "monday"},
		{"2023-01-01", "2023-01-01"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := PreprocessDate(tt.input); got != tt.want {
				t.Errorf("PreprocessDate(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
