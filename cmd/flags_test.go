package cmd

import "testing"

func TestScheduledFlagRemoved(t *testing.T) {
	if addCmd.Flags().Lookup("scheduled") != nil {
		t.Fatalf("add command still exposes --scheduled")
	}
	if modifyCmd.Flags().Lookup("scheduled") != nil {
		t.Fatalf("modify command still exposes --scheduled")
	}
	if logCmd.Flags().Lookup("scheduled") != nil {
		t.Fatalf("log command still exposes --scheduled")
	}
}

func TestDueFlagStillPresent(t *testing.T) {
	if addCmd.Flags().Lookup("due") == nil {
		t.Fatalf("add command must expose --due")
	}
	if modifyCmd.Flags().Lookup("due") == nil {
		t.Fatalf("modify command must expose --due")
	}
	if logCmd.Flags().Lookup("due") == nil {
		t.Fatalf("log command must expose --due")
	}
}
