package cmd

import (
	"fmt"
	"strconv"
	"strings"
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

func (s dependencySelection) referencedIDs() []int64 {
	ids := make([]int64, 0, len(s.dependsOn)+len(s.blocks))
	ids = append(ids, s.dependsOn...)
	ids = append(ids, s.blocks...)
	return ids
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
			return fmt.Errorf("task %d not found", id)
		}
	}

	return nil
}

func addDependencyLinks(targetIDs []int64, selection dependencySelection) (int, error) {
	if !selection.hasLinks() {
		return 0, nil
	}

	existingDeps, err := CurrentStore.GetDependencies()
	if err != nil {
		return 0, fmt.Errorf("get dependencies: %w", err)
	}

	existing := make(map[[2]int64]struct{}, len(existingDeps))
	for _, dep := range existingDeps {
		existing[[2]int64{dep.BlockerID, dep.BlockedID}] = struct{}{}
	}

	added := 0
	for _, targetID := range uniqueInt64s(targetIDs) {
		for _, blockerID := range selection.dependsOn {
			key := [2]int64{blockerID, targetID}
			if _, ok := existing[key]; ok {
				continue
			}
			if err := CurrentStore.AddDependency(blockerID, targetID); err != nil {
				return added, fmt.Errorf("add dependency %d -> %d: %w", blockerID, targetID, err)
			}
			existing[key] = struct{}{}
			added++
		}

		for _, blockedID := range selection.blocks {
			key := [2]int64{targetID, blockedID}
			if _, ok := existing[key]; ok {
				continue
			}
			if err := CurrentStore.AddDependency(targetID, blockedID); err != nil {
				return added, fmt.Errorf("add dependency %d -> %d: %w", targetID, blockedID, err)
			}
			existing[key] = struct{}{}
			added++
		}
	}

	return added, nil
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
