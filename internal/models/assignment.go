package models

import "fmt"

// A blank RunID denotes the durable human assignment.
type Assignment struct {
	Name        string `json:"name"`
	RunID       string `json:"run_id,omitempty"`
	Location    string `json:"location,omitempty"`
	Reclaimable bool   `json:"reclaimable,omitempty"`
}

func (a *Assignment) String() string {
	if a == nil {
		return ""
	}
	label := a.Name
	if a.RunID != "" {
		label += " [" + a.RunID + "]"
	}
	if a.Reclaimable {
		label += " (reclaimable)"
	}
	return label
}

func (t Task) AssignmentSuffix() string {
	if t.Assignment == nil {
		return ""
	}
	return fmt.Sprintf(" · %s", t.Assignment)
}
