package cmd

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestDateCompletionFunc(t *testing.T) {
	tests := []struct {
		toComplete string
		want       []string
		contains   []string
	}{
		{
			toComplete: "sun",
			contains:   []string{"sunday"},
		},
		{
			toComplete: "ne",
			contains:   []string{"next week", "next monday", "next friday"},
		},
		{
			toComplete: "tom",
			contains:   []string{"tomorrow"},
		},
		{
			toComplete: "nonexistent",
			want:       []string(nil), // Empty slice or nil
		},
	}

	for _, tt := range tests {
		t.Run(tt.toComplete, func(t *testing.T) {
			got, _ := dateCompletionFunc(&cobra.Command{}, nil, tt.toComplete)
			
			if tt.want != nil {
				if len(got) != len(tt.want) {
					t.Errorf("got %d items, want %d", len(got), len(tt.want))
				}
			}

			if len(tt.contains) > 0 {
				for _, c := range tt.contains {
					found := false
					for _, g := range got {
						if g == c {
							found = true
							break
						}
					}
					if !found {
						t.Errorf("expected %q in results, got %v", c, got)
					}
				}
			}
		})
	}
}
