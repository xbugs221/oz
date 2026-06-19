// Package app centralizes main-stage artifact expectations for workflow retry gates.
package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type stageArtifactExpectation struct {
	Path        string
	Description string
}

type stageArtifactResult struct {
	Review Review
	QA     QA
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
			Description: "review JSON schema 和 review decision 合同",
		}
	case strings.HasPrefix(state.Stage, "fix_"):
		n := strings.TrimPrefix(state.Stage, "fix_")
		return stageArtifactExpectation{
			Path:        filepath.Join(base, "fix-"+n+"-summary.md"),
			Description: "fix 阶段进度记录",
		}
	case strings.HasPrefix(state.Stage, "qa_"):
		n := strings.TrimPrefix(state.Stage, "qa_")
		return stageArtifactExpectation{
			Path:        filepath.Join(base, "qa-"+n+".json"),
			Description: "QA JSON schema、acceptance matrix 和 QA helper 输入检查",
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

// validateStageArtifact performs the single durable-output check for the active stage.
func (e *Engine) validateStageArtifact(state State) (stageArtifactResult, bool, error) {
	base := runDir(e.Repo, state.RunID)
	switch {
	case state.Stage == "execution":
		done, err := ChangeTasksDone(e.Repo, state.ChangeName)
		return stageArtifactResult{}, done, err
	case strings.HasPrefix(state.Stage, "review_"):
		iteration, err := stageIteration(state.Stage)
		if err != nil {
			return stageArtifactResult{}, false, err
		}
		review, err := ReadReview(filepath.Join(base, "review-"+strconv.Itoa(iteration)+".json"))
		if os.IsNotExist(err) {
			return stageArtifactResult{}, false, nil
		}
		if err != nil {
			return stageArtifactResult{}, false, err
		}
		return stageArtifactResult{Review: review}, true, nil
	case strings.HasPrefix(state.Stage, "fix_"):
		iteration, err := stageIteration(state.Stage)
		if err != nil {
			return stageArtifactResult{}, false, err
		}
		return stageArtifactResult{}, fileExists(filepath.Join(base, "fix-"+strconv.Itoa(iteration)+"-summary.md")), nil
	case strings.HasPrefix(state.Stage, "qa_"):
		iteration, err := stageIteration(state.Stage)
		if err != nil {
			return stageArtifactResult{}, false, err
		}
		qa, err := ReadQA(filepath.Join(base, "qa-"+strconv.Itoa(iteration)+".json"))
		if os.IsNotExist(err) {
			return stageArtifactResult{}, false, nil
		}
		if err != nil {
			return stageArtifactResult{}, false, err
		}
		acceptance, err := readAcceptanceForState(e.Repo, state)
		if err != nil {
			return stageArtifactResult{}, false, err
		}
		if err := ValidateQAAgainstAcceptance(qa, acceptance); err != nil {
			return stageArtifactResult{}, false, err
		}
		return stageArtifactResult{QA: qa}, true, nil
	case state.Stage == "archive":
		if !fileExists(filepath.Join(base, "delivery-summary.md")) || !archiveExists(e.Repo, state.ChangeName) {
			return stageArtifactResult{}, false, nil
		}
		if err := e.validateArchiveReadiness(state); err != nil {
			return stageArtifactResult{}, false, err
		}
		return stageArtifactResult{}, true, nil
	}
	return stageArtifactResult{}, false, fmt.Errorf("未知阶段 %q", state.Stage)
}

// artifactDone reports whether the current stage already has a valid durable output.
func (e *Engine) artifactDone(state State) (bool, error) {
	_, done, err := e.validateStageArtifact(state)
	return done, err
}

// checkStageArtifactGate converts missing or invalid stage output into a same-stage retry error.
func (e *Engine) checkStageArtifactGate(state State) (bool, error) {
	_, done, err := e.validateStageArtifact(state)
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
	return stageArtifactGateError{Reason: reason, Cause: failure}
}
