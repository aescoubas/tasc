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

func TestDependencyFlagsPresent(t *testing.T) {
	if addCmd.Flags().Lookup("depends-on") == nil {
		t.Fatalf("add command must expose --depends-on")
	}
	if addCmd.Flags().Lookup("blocks") == nil {
		t.Fatalf("add command must expose --blocks")
	}
	if modifyCmd.Flags().Lookup("depends-on") == nil {
		t.Fatalf("modify command must expose --depends-on")
	}
	if modifyCmd.Flags().Lookup("blocks") == nil {
		t.Fatalf("modify command must expose --blocks")
	}
}

func TestAutoApproveFlagsPresent(t *testing.T) {
	if modifyCmd.Flags().Lookup("yes") == nil {
		t.Fatalf("modify command must expose --yes")
	}
	if deleteCmd.Flags().Lookup("yes") == nil {
		t.Fatalf("delete command must expose --yes")
	}
}

func TestOutputFlagsPresent(t *testing.T) {
	listOutput := listCmd.Flags().Lookup("output")
	if listOutput == nil {
		t.Fatalf("list command must expose --output")
	}
	if listOutput.DefValue != "table" {
		t.Fatalf("list --output default must be table, got %q", listOutput.DefValue)
	}

	showOutput := showCmd.Flags().Lookup("output")
	if showOutput == nil {
		t.Fatalf("show command must expose --output")
	}
	if showOutput.DefValue != "table" {
		t.Fatalf("show --output default must be table, got %q", showOutput.DefValue)
	}
}
