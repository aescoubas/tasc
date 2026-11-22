package cmd

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/aescoubas/tasc/internal/store"
)

var (
	dependsOn []string
	blocks    []string
)

type dependencySelection struct {
	dependsOn []int64
	blocks    []int64
}

func buildDependencySelection(dependsOnValues, blockValues []string) (dependencySelection, error) {
	dependsOnIDs, err := parseDependencyIDs(dependsOnValues)
	if err != nil {
		return dependencySelection{}, err
	}

	blockIDs, err := parseDependencyIDs(blockValues)
	if err != nil {
		return dependencySelection{}, err
	}

	return dependencySelection{
		dependsOn: dependsOnIDs,
		blocks:    blockIDs,
	}, nil
}

func parseDependencyIDs(values []string) ([]int64, error) {
	var ids []int64
	seen := make(map[int64]struct{})

	for _, value := range values {
		for _, rawPart := range strings.Split(value, ",") {
			part := strings.TrimSpace(rawPart)
			if part == "" {
				continue
			}

			id, err := strconv.ParseInt(part, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("invalid task ID %q", part)
			}

			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			ids = append(ids, id)
		}
	}

	return ids, nil
}

func (s dependencySelection) hasLinks() bool {
	return len(s.dependsOn) > 0 || len(s.blocks) > 0
}

func (s dependencySelection) links() store.TaskLinks {
	return store.TaskLinks{DependsOn: s.dependsOn, Blocks: s.blocks}
}

func (s dependencySelection) validate(targetIDs []int64) error {
	paired := make(map[int64]struct{}, len(s.dependsOn))
	for _, id := range s.dependsOn {
		paired[id] = struct{}{}
	}
	for _, id := range s.blocks {
		if _, ok := paired[id]; ok {
			return fmt.Errorf("task %d cannot both block and be blocked by the same task", id)
		}
	}

	targetSet := make(map[int64]struct{}, len(targetIDs))
	for _, id := range targetIDs {
		targetSet[id] = struct{}{}
	}

	for _, id := range s.dependsOn {
		if _, ok := targetSet[id]; ok {
			return fmt.Errorf("task %d cannot depend on itself", id)
		}
	}
	for _, id := range s.blocks {
		if _, ok := targetSet[id]; ok {
			return fmt.Errorf("task %d cannot block itself", id)
		}
	}

	return nil
}

func ensureTasksExist(ids []int64) error {
	seen := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}

		if _, err := CurrentStore.GetTask(id); err != nil {
			return fmt.Errorf("fetch task %d: %w", id, err)
		}
	}

	return nil
}

// dependencyExists reports whether the exact blocker -> blocked edge is stored.
func dependencyExists(blockerID, blockedID int64) (bool, error) {
	deps, err := CurrentStore.GetDependencies()
	if err != nil {
		return false, fmt.Errorf("get dependencies: %w", err)
	}

	for _, dep := range deps {
		if dep.BlockerID == blockerID && dep.BlockedID == blockedID {
			return true, nil
		}
	}

	return false, nil
}

func uniqueInt64s(values []int64) []int64 {
	var out []int64
	seen := make(map[int64]struct{}, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
