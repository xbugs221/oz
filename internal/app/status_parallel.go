// Package app renders run-local parallel helper artifacts in human status output.
package app

import (
	"fmt"
	"os"
	"path/filepath"
)

type parallelStatusSummary struct {
	group    string
	file     string
	total    int
	passed   int
	status   string
	members  []ParallelMemberResult
	artifact bool
}

// parallelStatusLinesForRole returns status sublines for parallel groups owned by a workflow role.
func parallelStatusLinesForRole(repo string, state State, role string, indent string) []string {
	ensureWorkflowConfig(&state)
	if !state.Workflow.Parallel.Enabled || len(state.Workflow.Parallel.Groups) == 0 || state.RunID == "" {
		return nil
	}
	var lines []string
	for _, group := range parallelGroupsForRole(role) {
		summary, ok := parallelStatusSummaryForGroup(repo, state, group)
		if !ok {
			continue
		}
		line := fmt.Sprintf("%s- 并行 %s %d/%d %s", indent, summary.group, summary.passed, summary.total, summary.status)
		if !summary.artifact {
			line += " " + summary.file
		}
		lines = append(lines, line)
		if summary.artifact {
			for _, member := range summary.members {
				lines = append(lines, fmt.Sprintf("%s  - %s %s", indent, member.Name, member.Status))
			}
		}
	}
	return lines
}

// parallelGroupsForRole maps public status role rows to their parallel artifact groups.
func parallelGroupsForRole(role string) []string {
	switch role {
	case "planner":
		return []string{"planning_context"}
	case "executor":
		return []string{"implementation_context"}
	case "reviewer":
		return []string{"review"}
	case "qa":
		return []string{"qa"}
	default:
		return nil
	}
}

// parallelStatusSummaryForGroup validates one configured group without failing the whole status command.
func parallelStatusSummaryForGroup(repo string, state State, group string) (parallelStatusSummary, bool) {
	config, ok := state.Workflow.Parallel.Groups[group]
	if !ok || len(config.Members) == 0 {
		return parallelStatusSummary{}, false
	}
	iteration := parallelStatusIteration(state, group)
	path := parallelArtifactPath(runDir(repo, state.RunID), group, iteration)
	file := filepath.Base(path)
	if !parallelStatusGroupReached(path, state, group) {
		return parallelStatusSummary{}, false
	}
	summary := parallelStatusSummary{group: group, file: file, total: len(config.Members)}
	artifact, err := ReadParallelArtifact(path)
	if err != nil {
		if os.IsNotExist(err) {
			summary.status = "missing"
		} else {
			summary.status = "invalid"
		}
		return summary, true
	}
	if err := ValidateParallelArtifactForGroup(artifact, group, config); err != nil {
		summary.status = "invalid"
		return summary, true
	}
	summary.artifact = true
	summary.members = artifact.Members
	for _, member := range artifact.Members {
		if memberStatusSucceeded(member.Status) {
			summary.passed++
		}
	}
	if summary.passed == summary.total {
		summary.status = "success"
	} else {
		summary.status = "failed"
	}
	return summary, true
}

// parallelStatusGroupReached hides future default groups until the run reached them or produced evidence.
func parallelStatusGroupReached(path string, state State, group string) bool {
	if _, err := os.Stat(path); err == nil {
		return true
	}
	if state.Status == statusDone || state.Stage == "done" {
		return false
	}
	kind := stageKind(state.Stage)
	switch group {
	case "planning_context", "implementation_context":
		return stageAtLeast(kind, "execution")
	case "review":
		return kind == "review"
	case "qa":
		return kind == "qa"
	default:
		return false
	}
}

// parallelStatusIteration selects the artifact round for iterative review and QA groups.
func parallelStatusIteration(state State, group string) int {
	switch group {
	case "review", "qa":
		iteration := stageIteration(state.Stage)
		if iteration > 0 {
			return iteration
		}
		return 1
	default:
		return 0
	}
}

// stageAtLeast reports whether a stage kind has reached a configured status milestone.
func stageAtLeast(kind string, minimum string) bool {
	order := map[string]int{
		"planning":  1,
		"execution": 2,
		"review":    3,
		"qa":        4,
		"archive":   5,
		"done":      6,
	}
	return order[kind] >= order[minimum]
}
