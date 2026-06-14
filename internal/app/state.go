// Package app persists sealed run state and advances the workflow state machine.
package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	statusRunning                   = "running"
	statusFailed                    = "failed"
	statusStale                     = "stale"
	statusDone                      = "done"
	statusAborted                   = "aborted_manual_intervention"
	statusArchived                  = "archived_superseded"
	statusInterrupted               = "interrupted"
	statusBlocked                   = "blocked_review_limit"
	statusValidationBlocked         = "blocked_validation_limit"
	statusAcceptanceContractBlocked = "blocked_acceptance_contract"
	errNoInitialCommit              = "首次 git commit 前不能启动 oz flow run，请创建初始提交后重试"
)

// State is the durable source of truth for one sealed run.
type State struct {
	RunID               string                          `json:"run_id"`
	ChangeName          string                          `json:"change_name"`
	Sealed              bool                            `json:"sealed"`
	Status              string                          `json:"status"`
	Stage               string                          `json:"stage"`
	Engine              string                          `json:"engine,omitempty"`
	Error               string                          `json:"error"`
	BatchID             string                          `json:"batch_id,omitempty"`
	BatchIndex          int                             `json:"batch_index,omitempty"`
	BatchTotal          int                             `json:"batch_total,omitempty"`
	BaselineHead        string                          `json:"baseline_head"`
	BaselineDiff        string                          `json:"baseline_diff"`
	Sessions            map[string]string               `json:"sessions"`
	Stages              map[string]string               `json:"stages"`
	StageTimings        map[string]StageTiming          `json:"stage_timings,omitempty"`
	DAGNodes            map[string]DAGNodeState         `json:"dag_nodes,omitempty"`
	Processes           []ProcessState                  `json:"processes,omitempty"`
	Paths               map[string]string               `json:"paths"`
	Validation          map[string]StageValidationState `json:"validation,omitempty"`
	ArtifactGates       map[string]StageValidationState `json:"artifact_gates,omitempty"`
	AcceptancePreflight AcceptancePreflightState        `json:"acceptance_preflight,omitempty"`
	Workflow            WorkflowConfig                  `json:"workflow_config"`
}

// DAGNodeState records observable Go DAG node progress for human status and debugging.
type DAGNodeState struct {
	Status     string `json:"status"`
	Artifact   string `json:"artifact,omitempty"`
	StartedAt  string `json:"started_at,omitempty"`
	FinishedAt string `json:"finished_at,omitempty"`
	Error      string `json:"error,omitempty"`
}

// ProcessState is the flattened process view consumed by external status renderers.
type ProcessState struct {
	Stage     string `json:"stage"`
	Role      string `json:"role"`
	Provider  string `json:"provider"`
	Status    string `json:"status"`
	SessionID string `json:"session_id,omitempty"`
}

// StageTiming records the wall-clock interval for one actually executed stage.
type StageTiming struct {
	StartedAt  string `json:"started_at,omitempty"`
	FinishedAt string `json:"finished_at,omitempty"`
}

// LockInfo records conservative process ownership for a run lock file.
type LockInfo struct {
	PID       int    `json:"pid"`
	Hostname  string `json:"hostname"`
	RunID     string `json:"run_id"`
	StartedAt string `json:"started_at"`
}

// Engine coordinates state persistence, lock ownership, and agent turns.
type Engine struct {
	Repo     string
	Registry *AgentRegistry
	Output   io.Writer

	PlanningTool      string
	PlanningSessionID string
	progressMu        sync.Mutex
	lastProgressState string
	progressLines     int
	stageRuntime      map[string]stageRuntime
	inPlaceProgress   bool
}

// stageRuntime is transient process metadata shown beside the current stage.
type stageRuntime struct {
	PID    string
	Thread string
	Exit   string
	Failed bool
}

type progressSetter interface {
	SetProgress(io.Writer)
}

var (
	currentExecutable         = os.Executable
	startDetachedCommand      = startDetachedResumeCommand
	startDetachedBatchCommand = startDetachedBatchResumeCommand
)

// NewEngine creates a workflow engine rooted at a git repository.
func NewEngine(repo string, registry *AgentRegistry) *Engine {
	if registry == nil {
		registry = NewAgentRegistry()
	}
	return &Engine{Repo: repo, Registry: registry, stageRuntime: map[string]stageRuntime{}}
}

// Start creates a sealed run for an active change and runs it to completion.
func (e *Engine) Start(ctx context.Context, changeName string) error {
	state, err := e.createRun(changeName)
	if err != nil {
		return err
	}
	return e.run(ctx, state)
}

// Submit creates a sealed run and starts a detached worker to advance it.
func (e *Engine) Submit(ctx context.Context, changeName string) error {
	_ = ctx
	state, err := e.createRun(changeName)
	if err != nil {
		return err
	}
	if err := startDetachedCommand(e.Repo, state.RunID); err != nil {
		return err
	}
	e.printProgress(state, "submitted")
	return nil
}

// StartJSON creates a sealed run, emits its runner DTO, then runs the default Go DAG engine.
func (e *Engine) StartJSON(ctx context.Context, changeName string, stdout io.Writer) error {
	return e.StartGoDAGJSON(ctx, changeName, stdout)
}

// createRun validates a change and persists the initial sealed run state.
func (e *Engine) createRun(changeName string) (State, error) {
	if err := ValidateChange(e.Repo, changeName); err != nil {
		return State{}, err
	}
	head, diff, err := gitSnapshot(e.Repo)
	if err != nil {
		return State{}, err
	}
	acceptanceSource := acceptancePath(e.Repo, changeName)
	if _, err := ReadAcceptance(acceptanceSource); err != nil {
		return State{}, err
	}
	workflow, err := LoadWorkflowConfig(e.Repo)
	if err != nil {
		return State{}, err
	}
	if err := e.Registry.ResolveForWorkflow(workflow); err != nil {
		return State{}, err
	}
	state := State{
		RunID:        newRunID(),
		ChangeName:   changeName,
		Sealed:       true,
		Status:       statusRunning,
		Stage:        "execution",
		Engine:       workflow.Engine,
		BaselineHead: head,
		BaselineDiff: diff,
		Sessions:     map[string]string{},
		Stages:       map[string]string{},
		Paths:        map[string]string{},
		Workflow:     workflow,
	}
	if e.PlanningSessionID != "" {
		tool := e.PlanningTool
		if tool == "" {
			tool = "codex"
		}
		state.Sessions[sessionStateKey(tool, "planner")] = e.PlanningSessionID
	}
	if err := snapshotRunPrompts(e.Repo, state.RunID); err != nil {
		return State{}, err
	}
	if err := snapshotRunAcceptance(e.Repo, state.RunID, acceptanceSource); err != nil {
		return State{}, err
	}
	if err := saveState(e.Repo, state); err != nil {
		return State{}, err
	}
	return state, nil
}

// Resume loads the newest unfinished run and continues from its current stage.
func (e *Engine) Resume(ctx context.Context) error {
	return e.resume(ctx, false)
}

// ResumeAfterUserChoice resumes after the interactive menu made the lock decision explicit.
func (e *Engine) ResumeAfterUserChoice(ctx context.Context) error {
	return e.resume(ctx, true)
}

// ResumeDetachedAfterUserChoice starts an unfinished run in the background after an explicit menu choice.
func (e *Engine) ResumeDetachedAfterUserChoice(ctx context.Context, runID string) error {
	_ = ctx
	state, err := loadState(e.Repo, runID)
	if err != nil {
		return err
	}
	if state.Status == statusBlocked || state.Stage == statusBlocked {
		e.printProgress(state, "blocked")
		return fmt.Errorf("run %s 已到达 blocked_review_limit，无法自动继续", runID)
	}
	if state.Status == statusValidationBlocked || state.Stage == statusValidationBlocked {
		e.printProgress(state, "blocked")
		return fmt.Errorf("run %s 已到达 blocked_validation_limit，无法自动继续", runID)
	}
	status, err := lockFileStatus(e.Repo, runID, runtime.GOOS)
	if err != nil {
		return err
	}
	if status == lockStatusActive {
		return newRunLockedError(runID)
	}
	if status == lockStatusUnknown {
		if err := os.Remove(filepath.Join(runDir(e.Repo, runID), "lock")); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	if err := startDetachedCommand(e.Repo, runID); err != nil {
		return err
	}
	if e.stageRuntime == nil {
		e.stageRuntime = map[string]stageRuntime{}
	}
	e.stageRuntime[state.Stage] = stageRuntime{}
	e.printProgress(state, "submitted")
	return nil
}

// ResumeRunJSON resumes a specific run, emits its runner DTO, then continues the workflow.
func (e *Engine) ResumeRunJSON(ctx context.Context, runID string, stdout io.Writer) error {
	return e.resumeRun(ctx, runID, false, stdout)
}

// resume loads the newest recoverable run and handles lock policy before continuing.
func (e *Engine) resume(ctx context.Context, allowUnknownLock bool) error {
	runID, err := FindUnfinishedRun(e.Repo)
	if err != nil {
		return err
	}
	if runID == "" {
		return fmt.Errorf("没有未完成 run")
	}
	return e.resumeRun(ctx, runID, allowUnknownLock, nil)
}

// resumeRun loads a specific recoverable run and handles lock policy before continuing.
func (e *Engine) resumeRun(ctx context.Context, runID string, allowUnknownLock bool, startupJSON io.Writer) error {
	state, err := loadState(e.Repo, runID)
	if err != nil {
		return err
	}
	if state.Status == statusBlocked || state.Stage == statusBlocked {
		e.printProgress(state, "blocked")
		return fmt.Errorf("run %s 已到达 blocked_review_limit，无法自动继续", runID)
	}
	if state.Status == statusValidationBlocked || state.Stage == statusValidationBlocked {
		e.printProgress(state, "blocked")
		return fmt.Errorf("run %s 已到达 blocked_validation_limit，无法自动继续", runID)
	}
	status, err := lockFileStatus(e.Repo, runID, runtime.GOOS)
	if err != nil {
		return err
	}
	if status == lockStatusActive {
		return newRunLockedError(runID)
	}
	if status == lockStatusUnknown {
		if !allowUnknownLock {
			return fmt.Errorf("run %s 存在无法确认的 lock，请通过交互菜单恢复或中止", runID)
		}
		if err := os.Remove(filepath.Join(runDir(e.Repo, runID), "lock")); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	unlock, err := acquireLock(e.Repo, runID)
	if err != nil {
		return err
	}
	defer unlock()
	if !hasWorkflowConfig(state) {
		return fmt.Errorf("run %s 缺少 workflow_config 快照", runID)
	}
	normalizeWorkflowConfig(&state.Workflow)
	if err := e.Registry.ResolveForWorkflow(state.Workflow); err != nil {
		return err
	}
	if err := saveState(e.Repo, state); err != nil {
		return err
	}
	if startupJSON != nil {
		if err := writeRunnerState(startupJSON, state); err != nil {
			return err
		}
		flushWriter(startupJSON)
	}
	if state.Engine == "go-dag" {
		return e.runGoDAGLocked(ctx, state)
	}
	return e.runLoop(ctx, state)
}

// run advances stages until the workflow is done or aborted.
func (e *Engine) run(ctx context.Context, state State) error {
	if !hasWorkflowConfig(state) {
		return fmt.Errorf("run %s 缺少 workflow_config 快照", state.RunID)
	}
	if state.Engine != "go-dag" {
		unlock, err := acquireLock(e.Repo, state.RunID)
		if err != nil {
			return err
		}
		defer unlock()
		return e.runLoop(ctx, state)
	}
	return e.runGoDAG(ctx, state)
}

// runLoop advances stages while the caller holds the run lock.
func (e *Engine) runLoop(ctx context.Context, state State) error {
	e.printProgress(state, "resuming")
	for state.Status == statusRunning {
		forceRun := shouldForceStageRerun(state)
		done := false
		var err error
		if !forceRun {
			done, err = e.artifactDone(state)
			if err != nil {
				gateErr := e.stageArtifactGateError(state, err)
				if handled, handleErr := e.handleStageArtifactGateFailure(&state, gateErr); handleErr != nil {
					return handleErr
				} else if handled {
					continue
				}
				return gateErr
			}
		}
		if !done || forceRun {
			if err := e.detectManualIntervention(&state); err != nil {
				return err
			}
			if err := e.runStage(ctx, &state); err != nil {
				return err
			}
		} else {
			state.Stages[state.Stage] = "completed"
			e.printProgress(state, "skipped")
		}
		done, err = e.checkStageArtifactGate(state)
		if err != nil {
			if handled, handleErr := e.handleStageArtifactGateFailure(&state, err); handleErr != nil {
				return handleErr
			} else if handled {
				continue
			}
			return err
		}
		if !done {
			continue
		}
		clearStageArtifactGateFailure(&state)
		if state.Stage == "execution" {
			preflightPassed, err := e.runAcceptancePreflight(&state)
			if err != nil {
				return err
			}
			if !preflightPassed {
				if err := saveState(e.Repo, state); err != nil {
					return err
				}
				e.printProgress(state, "blocked")
				continue
			}
		}
		validationPassed, err := e.validateStage(ctx, &state)
		if err != nil {
			return err
		}
		if !validationPassed {
			if err := saveState(e.Repo, state); err != nil {
				return err
			}
			if state.Status == statusValidationBlocked {
				e.printProgress(state, "blocked")
			} else {
				e.printProgress(state, "validation_failed")
			}
			continue
		}
		if state.Status != statusRunning {
			if err := saveState(e.Repo, state); err != nil {
				return err
			}
			e.printProgress(state, "blocked")
			continue
		}
		if err := e.advance(&state); err != nil {
			if handled, handleErr := e.handleStageArtifactGateFailure(&state, err); handleErr != nil {
				return handleErr
			} else if handled {
				continue
			}
			return err
		}
		if err := saveState(e.Repo, state); err != nil {
			return err
		}
		e.printProgress(state, "next")
	}
	return nil
}

// handleStageArtifactGateFailure records retriable artifact failures before the workflow fails.
func (e *Engine) handleStageArtifactGateFailure(state *State, failure error) (bool, error) {
	if !isStageArtifactGateError(failure) {
		return false, nil
	}
	if err := recordStageArtifactGateFailure(e.Repo, state, failure); err != nil {
		return true, err
	}
	if err := saveState(e.Repo, *state); err != nil {
		return true, err
	}
	if state.Status == statusValidationBlocked {
		e.printProgress(*state, "blocked")
	} else {
		e.printProgress(*state, "validation_failed")
	}
	return true, nil
}

// runStage builds the stage prompt and invokes the proper agent session.
func (e *Engine) runStage(ctx context.Context, state *State) error {
	if state.Sessions == nil {
		state.Sessions = map[string]string{}
	}
	role := stageSessionRole(state.Stage)
	if state.Paths == nil {
		state.Paths = map[string]string{}
	}
	if state.Stages == nil {
		state.Stages = map[string]string{}
	}
	if state.StageTimings == nil {
		state.StageTimings = map[string]StageTiming{}
	}
	prompt, err := promptForStage(e.Repo, *state)
	if err != nil {
		return err
	}
	options, err := e.stageOptionsForRun(state)
	if err != nil {
		return err
	}
	tool, err := e.Registry.Tool(options.Tool)
	if err != nil {
		return err
	}
	runner := tool.NewRunner()
	if e.stageRuntime == nil {
		e.stageRuntime = map[string]stageRuntime{}
	}
	e.stageRuntime[state.Stage] = stageRuntime{}
	timing := state.StageTimings[state.Stage]
	timing.StartedAt = time.Now().UTC().Format(time.RFC3339Nano)
	timing.FinishedAt = ""
	state.StageTimings[state.Stage] = timing
	state.Stages[state.Stage] = "running"
	if err := saveState(e.Repo, *state); err != nil {
		return err
	}
	e.printProgress(*state, "running")
	sessionKey := sessionStateKey(options.Tool, role)
	if runner, ok := runner.(progressSetter); ok {
		runner.SetProgress(&stageProgressWriter{engine: e, state: state, sessionKey: sessionKey})
	}
	sessionID, err := runner.Run(ctx, e.Repo, prompt, state.Sessions[sessionKey], options)
	if err != nil {
		timing := state.StageTimings[state.Stage]
		timing.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
		state.StageTimings[state.Stage] = timing
		if ctx.Err() != nil {
			state.Stages[state.Stage] = statusInterrupted
			saveErr := saveState(e.Repo, *state)
			warnWorkflowWrite("save interrupted stage state", saveErr)
			return errors.Join(err, saveErr)
		} else {
			saveErr := saveState(e.Repo, *state)
			warnWorkflowWrite("save failed stage state", saveErr)
			return errors.Join(err, saveErr)
		}
	}
	if sessionID != "" {
		state.Sessions[sessionKey] = sessionID
		meta := e.stageRuntime[state.Stage]
		if meta.Thread == "" {
			meta.Thread = sessionID
			e.stageRuntime[state.Stage] = meta
		}
	}
	state.Stages[state.Stage] = "completed"
	timing = state.StageTimings[state.Stage]
	timing.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
	state.StageTimings[state.Stage] = timing
	head, diff, snapshotErr := gitSnapshot(e.Repo)
	if snapshotErr != nil {
		return snapshotErr
	}
	state.BaselineHead = head
	state.BaselineDiff = diff
	e.printProgress(*state, "completed")
	return saveState(e.Repo, *state)
}

func warnWorkflowWrite(action string, err error) {
	if err == nil {
		return
	}
	fmt.Fprintf(os.Stderr, "oz flow warning: %s: %v\n", action, err)
}

// stageOptionsForRun resolves dynamic stage options and persists automatic escalations.
func (e *Engine) stageOptionsForRun(state *State) (StageOptions, error) {
	options, err := state.Workflow.StageOption(state.Stage)
	if err != nil {
		return StageOptions{}, err
	}
	escalation, err := fixEscalation(e.Repo, *state)
	if err != nil {
		return StageOptions{}, err
	}
	if !escalation.Enabled {
		return options, nil
	}
	options.Reasoning = higherReasoning(options.Reasoning, escalation.Reasoning)
	options.Fast = false
	state.Workflow.Stages[state.Stage] = options
	if err := saveState(e.Repo, *state); err != nil {
		return StageOptions{}, err
	}
	return options, nil
}

// validateStage runs configured deterministic checks before a stage may advance.
func (e *Engine) validateStage(ctx context.Context, state *State) (bool, error) {
	ensureWorkflowConfig(state)
	if !shouldValidateStage(*state) {
		return true, nil
	}
	if state.Validation == nil {
		state.Validation = map[string]StageValidationState{}
	}
	current := state.Validation[state.Stage]
	current.Attempts++
	current.Kind = validationKindCommands
	attempt := runValidationCommands(ctx, e.Repo, state.Stage, current.Attempts, state.Workflow.Validation)
	artifactPath, err := writeValidationAttempt(e.Repo, state.RunID, attempt)
	if err != nil {
		return false, err
	}
	current.LastArtifact = artifactPath
	current.Status = attempt.Status
	current.LastError = firstValidationError(attempt)
	state.Validation[state.Stage] = current
	if attempt.Status == validationStatusPassed {
		clearStageValidationFailure(state)
		return true, nil
	}
	if current.Attempts >= state.Workflow.Validation.MaxAttemptsPerStage {
		state.Status = statusValidationBlocked
		state.Stage = statusValidationBlocked
		state.Error = current.LastError
		return false, nil
	}
	state.Stages[state.Stage] = "validation_failed"
	return false, nil
}

// printProgress writes human-readable state-machine progress without affecting durable state.
func (e *Engine) printProgress(state State, action string) {
	if e.Output == nil {
		return
	}
	e.progressMu.Lock()
	defer e.progressMu.Unlock()
	if state.Status == statusDone || state.Stage == "done" {
		e.printStageChecklistOnceLocked(state)
		return
	}
	e.printStageChecklistOnceLocked(state)
}

// printStageChecklistOnceLocked suppresses duplicate terminal status blocks while progressMu is held.
func (e *Engine) printStageChecklistOnceLocked(state State) {
	signature := e.stageChecklistSignature(state)
	if signature == e.lastProgressState {
		return
	}
	e.lastProgressState = signature
	e.printStageChecklist(state)
}

// stageChecklistSignature identifies visible state including transient process metadata.
func (e *Engine) stageChecklistSignature(state State) string {
	parts := []string{stageChecklistSignatureWithRuntime(state, e.stageRuntime)}
	for _, stage := range workflowStagesForState(state) {
		meta := e.stageRuntime[stage]
		parts = append(parts, stage+"="+meta.Thread+"/"+strconv.FormatBool(meta.Failed))
	}
	return strings.Join(parts, "|")
}

// printStageChecklist renders a stable workflow block, refreshing in place on terminals.
func (e *Engine) printStageChecklist(state State) {
	lines := stageChecklistLines(state, e.stageRuntime)
	if e.inPlaceProgress && e.progressLines > 0 {
		fmt.Fprintf(e.Output, "\x1b[%dA\x1b[J", e.progressLines)
	}
	for _, line := range lines {
		fmt.Fprintln(e.Output, line)
	}
	e.progressLines = len(lines)
}

// stageProgressWriter folds agent process events into the stable workflow checklist.
type stageProgressWriter struct {
	engine     *Engine
	state      *State
	sessionKey string
	pending    string
}

// Write consumes line-oriented agent progress and updates the current stage metadata.
func (w *stageProgressWriter) Write(p []byte) (int, error) {
	w.pending += string(p)
	for {
		line, rest, ok := strings.Cut(w.pending, "\n")
		if !ok {
			break
		}
		w.pending = rest
		if err := w.apply(strings.TrimSpace(line)); err != nil {
			return len(p), err
		}
	}
	return len(p), nil
}

// apply parses one concise agent progress line.
func (w *stageProgressWriter) apply(line string) error {
	if line == "" || w.engine == nil || w.state == nil {
		return nil
	}
	w.engine.progressMu.Lock()
	defer w.engine.progressMu.Unlock()
	if w.engine.stageRuntime == nil {
		w.engine.stageRuntime = map[string]stageRuntime{}
	}
	meta := w.engine.stageRuntime[w.state.Stage]
	switch {
	case strings.HasPrefix(line, "agent process started: "):
		meta.PID = valueAfter(line, "pid=")
		meta.Exit = ""
	case strings.HasPrefix(line, "agent session started: "):
		sessionID := valueAfter(line, "session=")
		meta.Thread = sessionID
		if err := w.persistSessionID(sessionID); err != nil {
			return err
		}
	case strings.HasPrefix(line, "agent process exited: "):
		meta.PID = valueAfter(line, "pid=")
		meta.Exit = valueAfter(line, "exit=")
	case strings.HasPrefix(line, "agent session failed: "):
		meta.Failed = true
	default:
		return nil
	}
	w.engine.stageRuntime[w.state.Stage] = meta
	if w.engine.Output != nil {
		w.engine.printStageChecklistOnceLocked(*w.state)
	}
	return nil
}

// persistSessionID makes a started agent session visible before the turn exits.
func (w *stageProgressWriter) persistSessionID(sessionID string) error {
	if sessionID == "" || w.sessionKey == "" || w.engine == nil || w.state == nil {
		return nil
	}
	if w.state.Sessions == nil {
		w.state.Sessions = map[string]string{}
	}
	if w.state.Sessions[w.sessionKey] == sessionID {
		return nil
	}
	w.state.Sessions[w.sessionKey] = sessionID
	return mergeState(w.engine.Repo, w.state.RunID, func(latest *State) {
		latest.Sessions[w.sessionKey] = sessionID
	})
}

// subagentProgressWriter persists helper sessions without changing parent stage progress.
type subagentProgressWriter struct {
	engine     *Engine
	state      *State
	sessionKey string
	pending    string
}

// Write consumes line-oriented helper progress and persists only session started events.
func (w *subagentProgressWriter) Write(p []byte) (int, error) {
	w.pending += string(p)
	for {
		line, rest, ok := strings.Cut(w.pending, "\n")
		if !ok {
			break
		}
		w.pending = rest
		if err := w.apply(strings.TrimSpace(line)); err != nil {
			return len(p), err
		}
	}
	return len(p), nil
}

// apply extracts subagent session started events while leaving stageRuntime untouched.
func (w *subagentProgressWriter) apply(line string) error {
	if line == "" || w.engine == nil || w.state == nil {
		return nil
	}
	if !strings.HasPrefix(line, "agent session started: ") {
		return nil
	}
	sessionID := valueAfter(line, "session=")
	return persistStateSessionID(w.engine.Repo, w.state, w.sessionKey, sessionID)
}

// persistStateSessionID makes a session visible through state.json without replacing sibling keys.
func persistStateSessionID(repo string, state *State, sessionKey, sessionID string) error {
	if sessionID == "" || sessionKey == "" || state == nil {
		return nil
	}
	if state.Sessions == nil {
		state.Sessions = map[string]string{}
	}
	if state.Sessions[sessionKey] == sessionID {
		return nil
	}
	state.Sessions[sessionKey] = sessionID
	return mergeState(repo, state.RunID, func(latest *State) {
		latest.Sessions[sessionKey] = sessionID
	})
}

// valueAfter extracts the first whitespace-delimited value following a key.
func valueAfter(line, key string) string {
	_, rest, ok := strings.Cut(line, key)
	if !ok {
		return ""
	}
	value, _, _ := strings.Cut(rest, " ")
	return value
}

// sessionStateKey isolates resumable sessions by backend and workflow role.
func sessionStateKey(tool, role string) string {
	return tool + ":" + role
}

// stageSessionRole maps internal workflow stages to durable agent session roles.
func stageSessionRole(stage string) string {
	role, err := roleForStage(stage)
	if err != nil {
		return "executor"
	}
	return role.Session
}

// workflowStagesForState returns the sealed stage list from the state snapshot.
func workflowStagesForState(state State) []string {
	ensureWorkflowConfig(&state)
	return workflowStagesForConfig(state.Workflow)
}

// ensureWorkflowConfig normalizes the workflow snapshot used by state checklists.
func ensureWorkflowConfig(state *State) {
	normalizeWorkflowConfig(&state.Workflow)
}

// hasWorkflowConfig reports whether durable state contains an effective workflow snapshot.
func hasWorkflowConfig(state State) bool {
	normalizeWorkflowConfig(&state.Workflow)
	return state.Workflow.Stages != nil
}

// detectManualIntervention aborts if current-run paths change outside the recorded stage flow.
func (e *Engine) detectManualIntervention(state *State) error {
	head, diff, err := gitSnapshot(e.Repo)
	if err != nil {
		return err
	}
	if head == state.BaselineHead && diff == state.BaselineDiff {
		return nil
	}
	guard, err := classifyGitSnapshotChange(e.Repo, state.ChangeName, state.BaselineHead, state.BaselineDiff, head, diff)
	if err != nil {
		return err
	}
	if guard.Blocked {
		state.Status = statusAborted
		if err := saveState(e.Repo, *state); err != nil {
			return err
		}
		return fmt.Errorf("在 %s 阶段前检测到当前 run 相关路径或源码变化：%s", state.Stage, guard.Detail())
	}
	state.BaselineHead = head
	state.BaselineDiff = diff
	return nil
}

// promptNameForStage maps workflow stages to named prompt templates.
func promptNameForStage(stage string) (string, error) {
	role, err := roleForStage(stage)
	if err != nil {
		return "", err
	}
	return role.PromptName, nil
}

func parallelGroupConfigured(workflow WorkflowConfig, name string) bool {
	group, ok := workflow.Parallel.Groups[name]
	return ok && len(group.Members) > 0
}

// refreshStateProcesses derives a stage-aware process list from sessions and DAG nodes.
func refreshStateProcesses(state *State) {
	if state == nil || state.RunID == "" {
		return
	}
	ensureWorkflowConfig(state)
	if !state.Workflow.Parallel.Enabled || len(state.Workflow.Parallel.Groups) == 0 {
		state.Processes = nil
		return
	}
	spec := BuildWorkflowSpec(state.ChangeName, state.Workflow)
	var processes []ProcessState
	for _, node := range spec.Nodes {
		if node.Type != "subagent" {
			continue
		}
		groupName := configGroupName(node.Group)
		group, ok := state.Workflow.Parallel.Groups[groupName]
		if !ok {
			continue
		}
		member, ok := parallelMemberByName(group, node.Member)
		if !ok {
			continue
		}
		provider := nonEmpty(member.Tool, "pi")
		role := "subagent:" + groupName + ":" + member.Name + ":" + strconv.Itoa(node.Iteration)
		sessionID := state.Sessions[sessionStateKey(provider, role)]
		if sessionID == "" && provider != "pi" {
			sessionID = state.Sessions[sessionStateKey("pi", role)]
		}
		nodeState, hasNode := state.DAGNodes[node.ID]
		status := processStatusFromNode(nodeState, hasNode, sessionID)
		if status == "" {
			continue
		}
		processes = append(processes, ProcessState{
			Stage:     nonEmpty(node.Stage, workflowNodeRunStage(node)),
			Role:      role,
			Provider:  provider,
			Status:    status,
			SessionID: sessionID,
		})
	}
	state.Processes = processes
}

// parallelMemberByName returns the configured helper member for process metadata.
func parallelMemberByName(group ParallelGroupConfig, name string) (ParallelMemberConfig, bool) {
	for _, member := range group.Members {
		if member.Name == name {
			return member, true
		}
	}
	return ParallelMemberConfig{}, false
}

// processStatusFromNode converts internal DAG progress into the public process status vocabulary.
func processStatusFromNode(node DAGNodeState, hasNode bool, sessionID string) string {
	if !hasNode {
		if sessionID != "" {
			return statusRunning
		}
		return ""
	}
	switch node.Status {
	case "success", "completed", statusDone:
		return "completed"
	case statusRunning:
		return statusRunning
	case statusFailed, "error", "validation_failed":
		return statusFailed
	default:
		if node.Status != "" {
			return node.Status
		}
		if sessionID != "" {
			return statusRunning
		}
		return ""
	}
}

// FindUnfinishedRun returns the newest run whose state is not done.
func FindUnfinishedRun(repo string) (string, error) {
	root, err := runsRoot(repo)
	if err != nil {
		return "", err
	}
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	for i := len(entries) - 1; i >= 0; i-- {
		entry := entries[i]
		if !entry.IsDir() {
			continue
		}
		state, err := loadState(repo, entry.Name())
		if err == nil && state.BatchID == "" && state.Status == statusRunning {
			return entry.Name(), nil
		}
	}
	return "", nil
}

// FindStartupRuns returns the newest resumable and stopped runs for the interactive menu.
func FindStartupRuns(repo string) (string, []State, error) {
	root, err := runsRoot(repo)
	if err != nil {
		return "", nil, err
	}
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return "", nil, nil
	}
	if err != nil {
		return "", nil, err
	}
	var running string
	var stopped []State
	for i := len(entries) - 1; i >= 0; i-- {
		entry := entries[i]
		if !entry.IsDir() {
			continue
		}
		state, err := loadState(repo, entry.Name())
		if err != nil || state.BatchID != "" {
			continue
		}
		if isStoppedRunState(state) {
			stopped = append(stopped, state)
			continue
		}
		if running == "" && state.Status == statusRunning {
			running = entry.Name()
		}
	}
	return running, stopped, nil
}

// isStoppedRunState reports terminal states that should not be shown as running work.
func isStoppedRunState(state State) bool {
	switch state.Status {
	case statusFailed, statusBlocked, statusValidationBlocked, statusAcceptanceContractBlocked, statusAborted, "aborted":
		return true
	default:
		return state.Stage == statusBlocked || state.Stage == statusValidationBlocked || state.Stage == statusAcceptanceContractBlocked
	}
}

// FindCurrentRun returns the newest readable run, regardless of whether older runs are unfinished.
func FindCurrentRun(repo string) (string, error) {
	root, err := runsRoot(repo)
	if err != nil {
		return "", err
	}
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	var newest string
	for i := len(entries) - 1; i >= 0; i-- {
		entry := entries[i]
		if !entry.IsDir() {
			continue
		}
		if newest == "" {
			newest = entry.Name()
		}
		if _, err := loadState(repo, entry.Name()); err == nil {
			return entry.Name(), nil
		}
	}
	return newest, nil
}

// snapshotRunAcceptance stores the sealed acceptance contract inside the run.
func snapshotRunAcceptance(repo, runID, sourcePath string) error {
	data, err := os.ReadFile(sourcePath)
	if err != nil {
		return err
	}
	base := runDir(repo, runID)
	if err := os.MkdirAll(base, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(base, "acceptance.json"), data, 0o644)
}

// readAcceptanceForState reads the immutable run contract, with legacy fallbacks.
func readAcceptanceForState(repo string, state State) (Acceptance, error) {
	if err := validateChangeNameForPath(state.ChangeName); err != nil {
		return Acceptance{}, err
	}
	runPath := filepath.Join(runDir(repo, state.RunID), "acceptance.json")
	if acceptance, err := ReadAcceptance(runPath); err == nil {
		return acceptance, nil
	} else if !os.IsNotExist(err) {
		return Acceptance{}, err
	}

	activePath := acceptancePath(repo, state.ChangeName)
	if acceptance, err := ReadAcceptance(activePath); err == nil {
		return acceptance, nil
	} else if !os.IsNotExist(err) {
		return Acceptance{}, err
	}

	archivedPath, err := archivedAcceptancePath(repo, state.ChangeName)
	if err != nil {
		return Acceptance{}, err
	}
	return ReadAcceptance(archivedPath)
}

// archivedAcceptancePath locates acceptance.json after oz archive moves a change.
func archivedAcceptancePath(repo, changeName string) (string, error) {
	if err := validateChangeNameForPath(changeName); err != nil {
		return "", err
	}
	matches, err := filepath.Glob(filepath.Join(repo, "docs", "changes", "archive", "*-"+changeName, "acceptance.json"))
	if err != nil {
		return "", err
	}
	if len(matches) == 0 {
		return "", os.ErrNotExist
	}
	return matches[len(matches)-1], nil
}

// archiveExists checks for an archived change directory with the date prefix.
func archiveExists(repo, changeName string) bool {
	if err := validateChangeNameForPath(changeName); err != nil {
		return false
	}
	matches, _ := filepath.Glob(filepath.Join(repo, "docs", "changes", "archive", "*-"+changeName))
	return len(matches) > 0
}

// fileExists reports whether a path exists and is a regular file.
func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// newRunID produces a sortable run id.
func newRunID() string {
	return time.Now().UTC().Format("20060102T150405.000000000Z")
}

// startDetachedResumeCommand runs the sealed workflow worker without streaming output to the terminal.
func startDetachedResumeCommand(repo, runID string) error {
	exe, err := currentExecutable()
	if err != nil {
		return fmt.Errorf("解析 oz flow 可执行文件失败：%w", err)
	}
	base := runDir(repo, runID)
	if err := os.MkdirAll(base, 0o755); err != nil {
		return err
	}
	cmd := exec.Command(exe, "resume", "--run-id", runID, "--json")
	cmd.Dir = repo
	configureDetachedCommand(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}
