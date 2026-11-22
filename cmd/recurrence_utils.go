package cmd

import (
	"fmt"
	"strings"
)

// The store makes the successor and edits its predecessor in one transaction.
func keepSeries(id int64, prompt, short, answer string) bool {
	task, err := CurrentStore.GetTask(id)
	if err != nil || task.Recurrence == "" {
		return false
	}
	fmt.Printf("Task %d is recurring ('%s').\n%s", task.ID, task.Recurrence, prompt)
	var response string
	if _, err := fmt.Scanln(&response); err != nil {
		return false
	}
	return strings.EqualFold(response, short) || strings.EqualFold(response, answer)
}
