// Package app manages one subagent execution attempt and retry boundaries.
package app

import (
	"context"
	"fmt"
	"time"
)

var subagentAttemptTimeout = 10 * time.Minute

// subagentAttemptRequest carries the immutable inputs for one backend runner call.
type subagentAttemptRequest struct {
	Tool           AgentTool
	State          State
	GroupName      string
	Member         ParallelMemberConfig
	Attempt        int
	SessionID      string
	SessionKey     string
	Prompt         string
	ArtifactPath   string
	SchemaErr      error
	PromptContext  subagentContext
	Options        StageOptions
	AttemptContext context.Context
}

// subagentAttemptsRequest carries retry-level execution inputs for one helper member.
type subagentAttemptsRequest struct {
	Tool          AgentTool
	State         State
	GroupName     string
	ConfigName    string
	Member        ParallelMemberConfig
	ArtifactPath  string
	SessionID     string
	SessionKey    string
	Prompt        string
	PromptContext subagentContext
	Options       StageOptions
	Context       context.Context
}

// subagentAttemptResult records one backend runner call result.
type subagentAttemptResult struct {
	SessionID string
	Capture   *artifactCapture
	Err       error
}

// subagentAttemptsResult records the final retry flow result for one helper member.
type subagentAttemptsResult struct {
	SessionID string
	Member    ParallelMemberResult
}

// runSubagentAttempts owns retry flow for member execution and artifact delivery.
func (e *Engine) runSubagentAttempts(request subagentAttemptsRequest) (subagentAttemptsResult, error) {
	var result ParallelMemberResult
	var schemaErr error
	var boundaryRepair subagentBoundaryRepair
	sessionID := request.SessionID
	for attempt := 1; attempt <= 3; attempt++ {
		if attempt > 1 {
			if err := removeStaleMemberArtifact(request.ArtifactPath); err != nil {
				return subagentAttemptsResult{}, e.failNodeState(request.State, err)
			}
		}
		attemptHead, attemptDiff, err := gitSnapshot(e.Repo)
		if err != nil {
			return subagentAttemptsResult{}, err
		}
		attemptContent, err := gitChangeContentSnapshot(e.Repo)
		if err != nil {
			return subagentAttemptsResult{}, err
		}
		attemptRunFiles, err := runArtifactFileSnapshot(runDir(e.Repo, request.State.RunID))
		if err != nil {
			return subagentAttemptsResult{}, e.failNodeState(request.State, err)
		}
		attemptResult := e.runSubagentAttempt(subagentAttemptRequest{
			Tool:           request.Tool,
			State:          request.State,
			GroupName:      request.GroupName,
			Member:         request.Member,
			Attempt:        attempt,
			SessionID:      sessionID,
			SessionKey:     request.SessionKey,
			Prompt:         request.Prompt,
			ArtifactPath:   request.ArtifactPath,
			SchemaErr:      schemaErr,
			PromptContext:  request.PromptContext,
			Options:        request.Options,
			AttemptContext: request.Context,
		})
		sessionID = attemptResult.SessionID
		attemptRepair, boundaryErr := e.checkSubagentReadOnlyBoundary(request.State, request.Member, attempt, request.ArtifactPath, attemptHead, attemptDiff, attemptContent, attemptRunFiles)
		boundaryRepair.Reverted = append(boundaryRepair.Reverted, attemptRepair.Reverted...)
		if boundaryErr != nil {
			return subagentAttemptsResult{}, boundaryErr
		}
		if attemptResult.Err != nil {
			return subagentAttemptsResult{}, e.failNodeState(request.State, attemptResult.Err)
		}
		if fileExists(request.ArtifactPath) {
			result, schemaErr = readNormalizeValidateMemberArtifact(request.ArtifactPath, request.ConfigName, request.Member, request.State.ChangeName)
			if schemaErr == nil {
				break
			}
		} else {
			if err := materializeCapturedMemberArtifact(request.ArtifactPath, attemptResult.Capture, request.Member, request.State.ChangeName); err != nil {
				schemaErr = err
				if attempt == 3 {
					result = subagentArtifactFailureResult(request.Member, request.State.ChangeName, request.ArtifactPath, schemaErr)
					break
				}
				continue
			}
			result, schemaErr = readNormalizeValidateMemberArtifact(request.ArtifactPath, request.ConfigName, request.Member, request.State.ChangeName)
			if schemaErr == nil {
				break
			}
		}
		if attempt == 3 {
			result = subagentArtifactFailureResult(request.Member, request.State.ChangeName, request.ArtifactPath, schemaErr)
			break
		}
	}
	result = resultWithBoundaryRepairEvidence(result, boundaryRepair)
	return subagentAttemptsResult{SessionID: sessionID, Member: result}, nil
}

// runSubagentAttempt executes exactly one helper process attempt.
func (e *Engine) runSubagentAttempt(request subagentAttemptRequest) subagentAttemptResult {
	runner := request.Tool.NewRunner()
	if runner, ok := runner.(progressSetter); ok {
		runner.SetProgress(&subagentProgressWriter{engine: e, state: &request.State, sessionKey: request.SessionKey})
	}
	capture := &artifactCapture{}
	if runner, ok := runner.(artifactCaptureSetter); ok {
		runner.SetArtifactCapture(capture)
	}
	attemptCtx, cancelAttempt := subagentAttemptContext(request.AttemptContext)
	defer cancelAttempt()

	prompt := request.Prompt
	sessionID := request.SessionID
	if request.Attempt > 1 {
		prompt = artifactRetryPrompt(request.SchemaErr, request.PromptContext)
	}
	sessionID, err := runner.Run(attemptCtx, e.Repo, prompt, sessionID, request.Options)
	if attemptCtx.Err() == context.DeadlineExceeded {
		err = fmt.Errorf("%w: subagent %s 第 %d 次执行超过 %s，可由系统重试", errGoDAGRetryableNode, request.Member.Name, request.Attempt, subagentAttemptTimeout)
	} else if err != nil {
		err = fmt.Errorf("%w: subagent %s 第 %d 次执行失败，可由系统重试：%v", errGoDAGRetryableNode, request.Member.Name, request.Attempt, err)
	}
	return subagentAttemptResult{SessionID: sessionID, Capture: capture, Err: err}
}

// subagentAttemptContext creates the timeout scope for one helper attempt.
func subagentAttemptContext(parent context.Context) (context.Context, context.CancelFunc) {
	if subagentAttemptTimeout <= 0 {
		return context.WithCancel(parent)
	}
	return context.WithTimeout(parent, subagentAttemptTimeout)
}

// resultWithBoundaryRepairEvidence records reverted yolo-helper writes for the main agent.
func resultWithBoundaryRepairEvidence(result ParallelMemberResult, repair subagentBoundaryRepair) ParallelMemberResult {
	for _, path := range uniqueSortedPaths(repair.Reverted) {
		result.Evidence = append(result.Evidence, "boundary reverted: "+path)
	}
	return result
}
