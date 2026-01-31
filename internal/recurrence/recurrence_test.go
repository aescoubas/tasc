package recurrence

import (
	"testing"
	"time"
)

func TestNext(t *testing.T) {
	// Fixed start date for deterministic testing: Jan 1st, 2026 (Thursday)
	baseDate := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)

	tests := []struct {
		name    string
		last    time.Time
		rule    string
		want    time.Time
		wantErr bool
	}{
		// Simple rules
		{
			name: "Daily",
			last: baseDate,
			rule: "daily",
			want: baseDate.AddDate(0, 0, 1),
		},
		{
			name: "Every Day",
			last: baseDate,
			rule: "every day",
			want: baseDate.AddDate(0, 0, 1),
		},
		{
			name: "Weekly",
			last: baseDate,
			rule: "weekly",
			want: baseDate.AddDate(0, 0, 7),
		},
		{
			name: "Monthly",
			last: baseDate,
			rule: "monthly",
			want: baseDate.AddDate(0, 1, 0),
		},
		{
			name: "Yearly",
			last: baseDate,
			rule: "yearly",
			want: baseDate.AddDate(1, 0, 0),
		},

		// "Every N units"
		{
			name: "Every 2 days",
			last: baseDate,
			rule: "every 2 days",
			want: baseDate.AddDate(0, 0, 2),
		},
		{
			name: "Every 3 weeks",
			last: baseDate,
			rule: "every 3 weeks",
			want: baseDate.AddDate(0, 0, 21),
		},
		{
			name: "Every 6 months",
			last: baseDate,
			rule: "every 6 months",
			want: baseDate.AddDate(0, 6, 0),
		},
		{
			name: "Every 1 year",
			last: baseDate,
			rule: "every 1 year",
			want: baseDate.AddDate(1, 0, 0),
		},

		// Complex: "Ordinal Weekday of Month"
		// baseDate is Jan 1 2026 (Thursday)
		// "First Friday of month"
		// In Jan 2026, First Friday is Jan 2.
		// Since last is Jan 1, Jan 2 is > last. So it should return Jan 2.
		{
			name: "First Friday of month (Current month candidate)",
			last: baseDate, // Jan 1
			rule: "first friday of month",
			want: time.Date(2026, 1, 2, 10, 0, 0, 0, time.UTC),
		},
		// "First Wednesday of month"
		// In Jan 2026, First Wed is Jan 7.
		{
			name: "First Wednesday of month",
			last: baseDate, // Jan 1
			rule: "first wednesday of month",
			want: time.Date(2026, 1, 7, 10, 0, 0, 0, time.UTC),
		},
		// "Last Friday of month"
		// Jan 2026: Jan 30 is last Friday.
		{
			name: "Last Friday of month",
			last: baseDate,
			rule: "last friday of month",
			want: time.Date(2026, 1, 30, 10, 0, 0, 0, time.UTC),
		},
		// "Second Monday of month"
		// Jan 2026: Jan 12 is second Monday.
		{
			name: "Second Monday of month",
			last: baseDate,
			rule: "second monday of month",
			want: time.Date(2026, 1, 12, 10, 0, 0, 0, time.UTC),
		},

		// Transition to next month
		// If last is Jan 15, and rule is "First Monday of month".
		// Jan 2026 First Monday was Jan 5. That is < Jan 15.
		// So it should find Feb 2026 First Monday -> Feb 2.
		{
			name: "First Monday (Next month)",
			last: time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC),
			rule: "first monday of month",
			want: time.Date(2026, 2, 2, 10, 0, 0, 0, time.UTC),
		},

		// Error cases
		{
			name:    "Invalid rule",
			last:    baseDate,
			rule:    "invalid rule",
			wantErr: true,
		},
		{
			name:    "Bad number",
			last:    baseDate,
			rule:    "every X days",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Next(tt.last, tt.rule)
			if (err != nil) != tt.wantErr {
				t.Errorf("Next() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && !got.Equal(tt.want) {
				t.Errorf("Next() = %v, want %v", got, tt.want)
			}
		})
	}
}
