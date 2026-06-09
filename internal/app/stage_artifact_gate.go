// Package app centralizes main-stage artifact expectations for workflow retry gates.
package app

import (
	"fmt"
	"path/filepath"
	"strings"
)

type stageArtifactExpectation struct {
	Path        string
	Description string
}

// stageArtifactExpectation describes the durable output that proves one stage completed.
func (e *Engine) stageArtifactExpectation(state State) stageArtifactExpectation {
	base := runDir(e.Repo, state.RunID)
	switch {
	case state.Stage == "execution":
		return stageArtifactExpectation{
			Path:        filepath.Join(e.Repo, "docs", "changes", state.ChangeName, "task.md"),
			Description: "oz status tasks.total > 0 且 tasks.done == tasks.total",
		}
	case strings.HasPrefix(state.Stage, "review_"):
		n := strings.TrimPrefix(state.Stage, "review_")
		return stageArtifactExpectation{
			Path:        filepath.Join(base, "review-"+n+".json"),
			Description: "review JSON schema、parallel gate 和 review 合同",
		}
	case strings.HasPrefix(state.Stage, "fix_"):
		n := strings.TrimPrefix(state.Stage, "fix_")
		return stageArtifactExpectation{
			Path:        filepath.Join(base, "fix-"+n+"-summary.md"),
			Description: "非空 fix summary",
		}
	case strings.HasPrefix(state.Stage, "qa_"):
		n := strings.TrimPrefix(state.Stage, "qa_")
		return stageArtifactExpectation{
			Path:        filepath.Join(base, "qa-"+n+".json"),
			Description: "QA JSON schema、acceptance matrix 和 parallel gate",
		}
	case state.Stage == "archive":
		return stageArtifactExpectation{
			Path:        filepath.Join(base, "delivery-summary.md"),
			Description: "delivery summary 和 docs/changes/archive/*-" + state.ChangeName + " 归档目录",
		}
	default:
		return stageArtifactExpectation{Description: "当前阶段产物"}
	}
}

// checkStageArtifactGate converts missing or invalid stage output into a same-stage retry error.
func (e *Engine) checkStageArtifactGate(state State) (bool, error) {
	done, err := e.artifactDone(state)
	if err != nil {
		return false, e.stageArtifactGateError(state, err)
	}
	if !done {
		return false, e.stageArtifactGateError(state, fmt.Errorf("%s 阶段 artifact 未完成", state.Stage))
	}
	return true, nil
}

// stageArtifactGateError adds stage and target artifact context to retriable gate failures.
func (e *Engine) stageArtifactGateError(state State, failure error) error {
	expect := e.stageArtifactExpectation(state)
	reason := failure.Error()
	if expect.Path != "" {
		reason = fmt.Sprintf("%s；stage=%s；artifact=%s；expectation=%s", reason, state.Stage, expect.Path, expect.Description)
	} else {
		reason = fmt.Sprintf("%s；stage=%s；expectation=%s", reason, state.Stage, expect.Description)
	}
	return newStageArtifactGateError(fmt.Errorf("%s", reason))
}
