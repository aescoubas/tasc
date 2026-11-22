package store

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/aescoubas/tasc/internal/models"
)

const GuestActor = "guest"
const HumanOwner = "Alex"
const AssignmentConflict = "task assignment conflict"

type AssignmentRow struct {
	ID         int64              `json:"id"`
	Title      string             `json:"title"`
	Status     models.TaskStatus  `json:"status,omitempty"`
	Assignment *models.Assignment `json:"assignment,omitempty"`
	Previous   *models.Assignment `json:"previous,omitempty"`
	Result     string             `json:"result,omitempty"`
}

type ClaimResult struct {
	RunID string          `json:"run_id"`
	Tasks []AssignmentRow `json:"tasks"`
}

type AssignmentChange struct {
	Action    string  `json:"action"`
	IDs       []int64 `json:"ids,omitempty"`
	Name      string  `json:"name,omitempty"`
	Location  string  `json:"location,omitempty"`
	TargetRun string  `json:"target_run,omitempty"`
}

func ParseTaskIDs(raw []string) ([]int64, error) {
	var ids []int64
	seen := map[int64]bool{}
	for _, arg := range raw {
		for _, part := range strings.Split(arg, ",") {
			id, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
			if err != nil {
				return nil, fmt.Errorf("invalid task ID %q", part)
			}
			if !seen[id] {
				ids = append(ids, id)
				seen[id] = true
			}
		}
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("task IDs required")
	}
	return ids, nil
}

var ErrLeaseExpired = errors.New("agent lease expired or ended; begin a new run and recheck ownership")

// MaintainRun renews a goal independently of tool calls and long-running builds.
func MaintainRun(ctx context.Context, st Store) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_, err := st.ChangeAssignments(AssignmentChange{Action: "renew"})
			if errors.Is(err, ErrLeaseExpired) {
				return
			}
			if err != nil {
				log.Printf("assignment heartbeat: %v", err)
			}
		}
	}
}
