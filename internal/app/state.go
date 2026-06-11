// Package app persists sealed run state and advances the workflow state machine.
package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"text/template"
	"time"
	"unicode"

	"gopkg.in/yaml.v3"
)

const (
	statusRunning           = "running"
	statusFailed            = "failed"
	statusDone              = "done"
	statusAborted           = "aborted_manual_intervention"
	statusArchived          = "archived_superseded"
	statusInterrupted       = "interrupted"
	statusBlocked           = "blocked_review_limit"
	statusValidationBlocked = "blocked_validation_limit"
	errNoInitialCommit      = "首次 git commit 前不能启动 wo run，请创建初始提交后重试"
)

var stateFileMu sync.Mutex

type lockStatus string

const (
	lockStatusNone    lockStatus = "none"
	lockStatusActive  lockStatus = "active"
	lockStatusStale   lockStatus = "stale"
	lockStatusUnknown lockStatus = "unknown"
)

// State is the durable source of truth for one sealed run.
type State struct {
	RunID        string                          `json:"run_id"`
	ChangeName   string                          `json:"change_name"`
	Sealed       bool                            `json:"sealed"`
	Status       string                          `json:"status"`
	Stage        string                          `json:"stage"`
	Engine       string                          `json:"engine,omitempty"`
	Error        string                          `json:"error"`
	BatchID      string                          `json:"batch_id,omitempty"`
	BatchIndex   int                             `json:"batch_index,omitempty"`
	BatchTotal   int                             `json:"batch_total,omitempty"`
	BaselineHead string                          `json:"baseline_head"`
	BaselineDiff string                          `json:"baseline_diff"`
	Sessions     map[string]string               `json:"sessions"`
	Stages       map[string]string               `json:"stages"`
	StageTimings map[string]StageTiming          `json:"stage_timings,omitempty"`
	DAGNodes     map[string]DAGNodeState         `json:"dag_nodes,omitempty"`
	Paths        map[string]string               `json:"paths"`
	Validation   map[string]StageValidationState `json:"validation,omitempty"`
	Workflow     WorkflowConfig                  `json:"workflow_config"`
}

// DAGNodeState records observable Go DAG node progress for human status and debugging.
type DAGNodeState struct {
	Status     string `json:"status"`
	Artifact   string `json:"artifact,omitempty"`
	StartedAt  string `json:"started_at,omitempty"`
	FinishedAt string `json:"finished_at,omitempty"`
	Error      string `json:"error,omitempty"`
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

// promptSnapshot stores the effective prompt bodies frozen for one sealed run.
type promptSnapshot struct {
	Prompts map[string]string `yaml:"prompts"`
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
	fmt.Fprintf(os.Stderr, "wo warning: %s: %v\n", action, err)
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

// stageKind collapses iteration stages to their shared prompt and config kind.
func stageKind(stage string) string {
	role, err := roleForStage(stage)
	if err != nil {
		return stage
	}
	return role.Name
}

// stageIteration returns the numeric review, QA, or fix round encoded in the stage name.
func stageIteration(stage string) (int, error) {
	for _, prefix := range []string{"review_", "qa_", "fix_"} {
		if strings.HasPrefix(stage, prefix) {
			raw := strings.TrimPrefix(stage, prefix)
			n, err := strconv.Atoi(raw)
			if err != nil || n < 1 {
				return 0, fmt.Errorf("非法迭代阶段 %q", stage)
			}
			return n, nil
		}
	}
	return 0, nil
}

type fixEscalationPlan struct {
	Enabled               bool
	ConsecutiveFailures   int
	Reasoning             string
	RepeatedFindingTitles []string
}

// fixEscalation reports whether a fix follows repeated failed reviews.
func fixEscalation(repo string, state State) (fixEscalationPlan, error) {
	if !strings.HasPrefix(state.Stage, "fix_") {
		return fixEscalationPlan{}, nil
	}
	iteration, err := stageIteration(state.Stage)
	if err != nil {
		return fixEscalationPlan{}, err
	}
	if iteration < 2 {
		return fixEscalationPlan{}, nil
	}
	reviews := make([]Review, 0, iteration)
	failures := 0
	for i := iteration; i >= 1; i-- {
		review, err := ReadReview(filepath.Join(runDir(repo, state.RunID), fmt.Sprintf("review-%d.json", i)))
		if os.IsNotExist(err) {
			break
		}
		if err != nil {
			return fixEscalationPlan{}, err
		}
		if !NeedsFix(review) {
			break
		}
		reviews = append(reviews, review)
		failures++
	}
	if failures < 2 {
		return fixEscalationPlan{}, nil
	}
	plan := fixEscalationPlan{
		Enabled:             true,
		ConsecutiveFailures: failures,
		Reasoning:           reasoningForConsecutiveFailures(failures),
	}
	if plan.Reasoning == "low" {
		return fixEscalationPlan{}, nil
	}
	if len(reviews) >= 2 {
		plan.RepeatedFindingTitles = repeatedFindingTitles(reviews[0], reviews[1])
	}
	return plan, nil
}

func reasoningForConsecutiveFailures(failures int) string {
	switch {
	case failures >= 4:
		return "xhigh"
	case failures >= 3:
		return "high"
	case failures >= 2:
		return "medium"
	default:
		return "low"
	}
}

func higherReasoning(current, target string) string {
	if reasoningRank(current) >= reasoningRank(target) {
		return current
	}
	return target
}

func reasoningRank(reasoning string) int {
	switch reasoning {
	case "low":
		return 1
	case "medium":
		return 2
	case "high":
		return 3
	case "xhigh":
		return 4
	default:
		return 0
	}
}

// repeatedFindingTitles returns current finding titles that also appeared in the previous review.
func repeatedFindingTitles(current, previous Review) []string {
	seen := map[string]bool{}
	for _, finding := range previous.Findings {
		key := findingKey(finding.Title)
		if key != "" {
			seen[key] = true
		}
	}
	var repeated []string
	for _, finding := range current.Findings {
		key := findingKey(finding.Title)
		if key != "" && seen[key] {
			repeated = append(repeated, finding.Title)
		}
	}
	return repeated
}

func findingKey(title string) string {
	var out strings.Builder
	lastSpace := true
	for _, r := range strings.ToLower(strings.TrimSpace(title)) {
		switch {
		case unicode.IsLetter(r) || unicode.IsNumber(r):
			out.WriteRune(r)
			lastSpace = false
		case unicode.IsSpace(r) || r == '-' || r == '_' || r == ':' || r == '：':
			if !lastSpace {
				out.WriteByte(' ')
				lastSpace = true
			}
		}
	}
	return strings.TrimSpace(out.String())
}

// artifactDone checks whether the current stage already produced its required artifact.
func (e *Engine) artifactDone(state State) (bool, error) {
	base := runDir(e.Repo, state.RunID)
	switch {
	case state.Stage == "execution":
		done, err := ChangeTasksDone(e.Repo, state.ChangeName)
		if err != nil || !done {
			return done, err
		}
		if err := ValidateParallelContextGate(base, state.Workflow); err != nil {
			return false, newStageArtifactGateError(err)
		}
		return true, nil
	case strings.HasPrefix(state.Stage, "review_"):
		iteration, err := stageIteration(state.Stage)
		if err != nil {
			return false, err
		}
		n := strconv.Itoa(iteration)
		review, err := ReadReview(filepath.Join(base, "review-"+n+".json"))
		if os.IsNotExist(err) {
			return false, nil
		}
		if err != nil {
			return false, newStageArtifactGateError(err)
		}
		if err := ValidateParallelReviewGate(base, state.Workflow, iteration, review); err != nil {
			return false, newStageArtifactGateError(err)
		}
		return true, nil
	case strings.HasPrefix(state.Stage, "fix_"):
		iteration, err := stageIteration(state.Stage)
		if err != nil {
			return false, err
		}
		n := strconv.Itoa(iteration)
		return fileExists(filepath.Join(base, "fix-"+n+"-summary.md")), nil
	case strings.HasPrefix(state.Stage, "qa_"):
		iteration, err := stageIteration(state.Stage)
		if err != nil {
			return false, err
		}
		n := strconv.Itoa(iteration)
		qa, err := ReadQA(filepath.Join(base, "qa-"+n+".json"))
		if os.IsNotExist(err) {
			return false, nil
		}
		if err != nil {
			return false, newStageArtifactGateError(err)
		}
		acceptance, err := readAcceptanceForState(e.Repo, state)
		if err != nil {
			return false, err
		}
		if err := ValidateQAAgainstAcceptance(qa, acceptance); err != nil {
			return false, newStageArtifactGateError(err)
		}
		if err := ValidateParallelQAGate(base, state.Workflow, iteration, qa); err != nil {
			return false, newStageArtifactGateError(err)
		}
		return true, nil
	case state.Stage == "archive":
		return fileExists(filepath.Join(base, "delivery-summary.md")) && archiveExists(e.Repo, state.ChangeName), nil
	}
	return false, fmt.Errorf("未知阶段 %q", state.Stage)
}

// advance moves state to the next linear stage, honoring review fix decisions.
func (e *Engine) advance(state *State) error {
	ensureWorkflowConfig(state)
	switch {
	case state.Stage == "execution":
		if err := e.validateExecutionParallelContextGate(*state); err != nil {
			return newStageArtifactGateError(err)
		}
		if state.Workflow.MaxReviewIterations == 0 {
			state.Stage = "archive"
		} else {
			state.Stage = "review_1"
		}
	case strings.HasPrefix(state.Stage, "review_"):
		iteration, err := stageIteration(state.Stage)
		if err != nil {
			return err
		}
		n := strconv.Itoa(iteration)
		review, err := ReadReview(filepath.Join(runDir(e.Repo, state.RunID), "review-"+n+".json"))
		if err != nil {
			return newStageArtifactGateError(err)
		}
		if err := ValidateParallelReviewGate(runDir(e.Repo, state.RunID), state.Workflow, iteration, review); err != nil {
			return newStageArtifactGateError(err)
		}
		clearStageValidationFailure(state)
		if ReviewDeclaresWorkflowFailure(review) {
			state.Status = statusFailed
			state.Error = "审核阶段判定工作流无法继续：" + strings.TrimSpace(review.WorkflowFailure.Reason)
			return nil
		}
		if NeedsFix(review) {
			state.Stage = "fix_" + n
		} else {
			state.Stage = "qa_" + n
		}
	case strings.HasPrefix(state.Stage, "qa_"):
		iteration, err := stageIteration(state.Stage)
		if err != nil {
			return err
		}
		n := strconv.Itoa(iteration)
		qa, err := ReadQA(filepath.Join(runDir(e.Repo, state.RunID), "qa-"+n+".json"))
		if err != nil {
			return newStageArtifactGateError(err)
		}
		acceptance, err := readAcceptanceForState(e.Repo, *state)
		if err != nil {
			return err
		}
		if err := ValidateQAAgainstAcceptance(qa, acceptance); err != nil {
			return newStageArtifactGateError(err)
		}
		if err := ValidateParallelQAGate(runDir(e.Repo, state.RunID), state.Workflow, iteration, qa); err != nil {
			return newStageArtifactGateError(err)
		}
		clearStageValidationFailure(state)
		if QANeedsFix(qa) {
			state.Stage = "fix_" + n
		} else {
			state.Stage = "archive"
		}
	case strings.HasPrefix(state.Stage, "fix_"):
		n, err := stageIteration(state.Stage)
		if err != nil {
			return err
		}
		if n >= state.Workflow.MaxReviewIterations {
			state.Status = statusBlocked
			state.Stage = statusBlocked
			state.Error = "审核修正达到上限，工作流已中断"
		} else {
			state.Stage = fmt.Sprintf("review_%d", n+1)
		}
	case state.Stage == "archive":
		if err := e.validateArchiveReadiness(*state); err != nil {
			return e.stageArtifactGateError(*state, err)
		}
		state.Status = statusDone
		state.Stage = "done"
	default:
		return fmt.Errorf("未知阶段 %q", state.Stage)
	}
	return nil
}

// validateArchiveReadiness blocks archive completion until the evidence chain is complete.
func (e *Engine) validateArchiveReadiness(state State) error {
	if !fileExists(filepath.Join(runDir(e.Repo, state.RunID), "delivery-summary.md")) || !archiveExists(e.Repo, state.ChangeName) {
		return fmt.Errorf("archive 阶段缺少 delivery summary 或归档目录")
	}
	if state.Workflow.MaxReviewIterations == 0 {
		return nil
	}
	iteration := latestCompletedQAIteration(state)
	if iteration == 0 {
		return fmt.Errorf("archive 阶段缺少 clean QA artifact")
	}
	review, err := ReadReview(filepath.Join(runDir(e.Repo, state.RunID), fmt.Sprintf("review-%d.json", iteration)))
	if err != nil {
		return err
	}
	if NeedsFix(review) {
		return fmt.Errorf("archive 阶段发现 review-%d 仍需修复", iteration)
	}
	qa, err := ReadQA(filepath.Join(runDir(e.Repo, state.RunID), fmt.Sprintf("qa-%d.json", iteration)))
	if err != nil {
		return err
	}
	acceptance, err := readAcceptanceForState(e.Repo, state)
	if err != nil {
		return err
	}
	if err := ValidateQAAgainstAcceptance(qa, acceptance); err != nil {
		return err
	}
	if QANeedsFix(qa) {
		return fmt.Errorf("archive 阶段发现 qa-%d 仍需修复", iteration)
	}
	if len(state.Workflow.Validation.Commands) > 0 {
		return validateArchiveValidationEvidence(state)
	}
	return nil
}

func latestCompletedQAIteration(state State) int {
	latest := 0
	for i := 1; i <= state.Workflow.MaxReviewIterations; i++ {
		if state.Stages[fmt.Sprintf("qa_%d", i)] != "" {
			latest = i
		}
	}
	return latest
}

func validateArchiveValidationEvidence(state State) error {
	for stage, status := range state.Stages {
		if status == "" {
			continue
		}
		if stage != "execution" && !strings.HasPrefix(stage, "fix_") {
			continue
		}
		if state.Validation[stage].Status != validationStatusPassed {
			return fmt.Errorf("archive 阶段缺少 %s 的 validation passed 记录", stage)
		}
	}
	return nil
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

// promptForStage reads and renders the YAML prompt for a sealed stage.
func promptForStage(repo string, state State) (string, error) {
	if state.ChangeName != "" {
		if err := validateChangeNameForPath(state.ChangeName); err != nil {
			return "", err
		}
	}
	name, err := promptNameForStage(state.Stage)
	if err != nil {
		return "", err
	}
	var templateText string
	if state.RunID != "" {
		templateText, err = runPromptTemplate(repo, state.RunID, name)
		if err != nil {
			return "", err
		}
	} else {
		config := DefaultWorkflowConfig()
		if state.Workflow.Prompts != nil {
			config = state.Workflow
		} else if loaded, loadErr := LoadWorkflowConfig(repo); loadErr == nil {
			config = loaded
		}
		templateText, err = promptForName(config, name)
		if err != nil {
			return "", err
		}
	}
	context, err := promptContext(repo, state)
	if err != nil {
		return "", err
	}
	prompt, err := renderPromptTemplate(name, templateText, context)
	if err != nil {
		return "", err
	}
	return prompt + validationFailurePrompt(repo, state), nil
}

// promptForName reads a named prompt from the effective YAML config.
func promptForName(config WorkflowConfig, name string) (string, error) {
	key, err := promptKeyForName(name)
	if err != nil {
		return "", err
	}
	body := config.Prompts[key]
	if body == "" {
		return "", fmt.Errorf("配置缺少 prompts.%s", key)
	}
	return body, nil
}

// promptNameForStage maps workflow stages to named prompt templates.
func promptNameForStage(stage string) (string, error) {
	role, err := roleForStage(stage)
	if err != nil {
		return "", err
	}
	return role.PromptName, nil
}

func promptKeyForName(name string) (string, error) {
	role, ok := roleByPromptName(name)
	if !ok {
		return "", fmt.Errorf("未知 prompt %q", name)
	}
	return role.PromptKey, nil
}

// runPromptTemplate reads the prompt snapshot saved when a sealed run starts.
func runPromptTemplate(repo, runID, name string) (string, error) {
	key, keyErr := promptKeyForName(name)
	if keyErr != nil {
		return "", keyErr
	}
	snapshotPath := filepath.Join(runDir(repo, runID), "prompt-snapshot.yaml")
	data, err := os.ReadFile(snapshotPath)
	if err == nil {
		var snapshot promptSnapshot
		if err := yaml.Unmarshal(data, &snapshot); err != nil {
			return "", fmt.Errorf("读取 prompt 快照 %s 失败: %w", snapshotPath, err)
		}
		body := snapshot.Prompts[key]
		if body == "" {
			return "", fmt.Errorf("prompt 快照缺少 prompts.%s", key)
		}
		return body, nil
	}
	if !os.IsNotExist(err) {
		return "", err
	}
	return "", fmt.Errorf("run %s 缺少 prompt 快照 prompt-snapshot.yaml", runID)
}

// snapshotRunPrompts freezes sealed-run prompts so resume cannot drift.
func snapshotRunPrompts(repo, runID string) error {
	root := runDir(repo, runID)
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	config, err := LoadWorkflowConfig(repo)
	if err != nil {
		return err
	}
	snapshot := promptSnapshot{Prompts: map[string]string{}}
	for _, key := range rolePromptKeys() {
		body := config.Prompts[key]
		if body == "" {
			return fmt.Errorf("配置缺少 prompts.%s", key)
		}
		snapshot.Prompts[key] = body
	}
	data, err := yaml.Marshal(snapshot)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(root, "prompt-snapshot.yaml"), data, 0o644)
}

// renderPromptTemplate injects run metadata and fails on unknown template variables.
func renderPromptTemplate(name, body string, context promptTemplateContext) (string, error) {
	tmpl, err := template.New(name).Option("missingkey=error").Parse(body)
	if err != nil {
		return "", err
	}
	var out strings.Builder
	if err := tmpl.Execute(&out, context); err != nil {
		return "", err
	}
	return out.String(), nil
}

// promptTemplateContext is the data exposed to named prompt templates.
type promptTemplateContext struct {
	RunID                        string
	ChangeName                   string
	Stage                        string
	StageKind                    string
	Iteration                    int
	MaxReviewIterations          int
	StatePath                    string
	ChangePath                   string
	AcceptancePath               string
	AcceptanceSummaryPath        string
	ReviewPath                   string
	QAPath                       string
	FixSummaryPath               string
	PreviousReviewPaths          []string
	PreviousQAPaths              []string
	PreviousFixSummaryPaths      []string
	PreviousReviewCount          int
	PreviousFixSummaryCount      int
	LatestPreviousReviewPath     string
	LatestPreviousFixSummaryPath string
	HasPreviousReview            bool
	HasPreviousFixSummary        bool
	PlanningContextPath          string
	ParallelContextPath          string
	ParallelReviewPath           string
	ParallelQAPath               string
	HasPlanningContext           bool
	HasParallelContext           bool
	HasParallelReview            bool
	HasParallelQA                bool
	DeliverySummaryPath          string
	BaselineHead                 string
	RoleSessionKey               string
	RoleSessionID                string
	HasRoleSession               bool
	IsFirstRoleTurn              bool
	FixEscalated                 bool
	FixEscalationReasoning       string
	ConsecutiveReviewFailures    int
	RepeatedFindingTitles        []string
}

// promptContext computes stable run paths for the current workflow stage.
func promptContext(repo string, state State) (promptTemplateContext, error) {
	ensureWorkflowConfig(&state)
	kind := stageKind(state.Stage)
	iteration, err := stageIteration(state.Stage)
	if err != nil {
		return promptTemplateContext{}, err
	}
	runPath := runDir(repo, state.RunID)
	roleSessionKey, roleSessionID := promptRoleSession(state)
	context := promptTemplateContext{
		RunID:                   state.RunID,
		ChangeName:              state.ChangeName,
		Stage:                   state.Stage,
		StageKind:               kind,
		Iteration:               iteration,
		MaxReviewIterations:     state.Workflow.MaxReviewIterations,
		StatePath:               filepath.Join(runPath, "state.json"),
		ChangePath:              "docs/changes/" + state.ChangeName,
		AcceptancePath:          acceptancePath(repo, state.ChangeName),
		AcceptanceSummaryPath:   acceptancePath(repo, state.ChangeName),
		DeliverySummaryPath:     filepath.Join(runPath, "delivery-summary.md"),
		BaselineHead:            state.BaselineHead,
		RoleSessionKey:          roleSessionKey,
		RoleSessionID:           roleSessionID,
		HasRoleSession:          roleSessionID != "",
		IsFirstRoleTurn:         roleSessionID == "",
		PreviousReviewPaths:     []string{},
		PreviousQAPaths:         []string{},
		PreviousFixSummaryPaths: []string{},
		RepeatedFindingTitles:   []string{},
	}
	if state.Workflow.Parallel.Enabled {
		context.PlanningContextPath = parallelArtifactPath(runPath, "planning_context", iteration)
		context.ParallelContextPath = parallelArtifactPath(runPath, "implementation_context", iteration)
		context.ParallelReviewPath = parallelArtifactPath(runPath, "review", iteration)
		context.ParallelQAPath = parallelArtifactPath(runPath, "qa", iteration)
		context.HasPlanningContext = parallelGroupConfigured(state.Workflow, "planning_context") && (kind == "planning" || kind == "execution" || kind == "review" || kind == "qa")
		context.HasParallelContext = parallelGroupConfigured(state.Workflow, "implementation_context") && (kind == "execution" || kind == "review" || kind == "qa")
		context.HasParallelReview = parallelGroupConfigured(state.Workflow, "review") && kind == "review"
		context.HasParallelQA = parallelGroupConfigured(state.Workflow, "qa") && kind == "qa"
	}
	escalation, err := fixEscalation(repo, state)
	if err != nil {
		return promptTemplateContext{}, err
	}
	if escalation.Enabled {
		context.FixEscalated = escalation.Enabled
		context.FixEscalationReasoning = escalation.Reasoning
		context.ConsecutiveReviewFailures = escalation.ConsecutiveFailures
		context.RepeatedFindingTitles = escalation.RepeatedFindingTitles
	}
	if iteration > 0 {
		context.ReviewPath = filepath.Join(runPath, fmt.Sprintf("review-%d.json", iteration))
		context.QAPath = filepath.Join(runPath, fmt.Sprintf("qa-%d.json", iteration))
		context.FixSummaryPath = filepath.Join(runPath, fmt.Sprintf("fix-%d-summary.md", iteration))
		for i := 1; i < iteration; i++ {
			context.PreviousReviewPaths = append(context.PreviousReviewPaths, filepath.Join(runPath, fmt.Sprintf("review-%d.json", i)))
			context.PreviousFixSummaryPaths = append(context.PreviousFixSummaryPaths, filepath.Join(runPath, fmt.Sprintf("fix-%d-summary.md", i)))
		}
		context.PreviousReviewCount = len(context.PreviousReviewPaths)
		context.PreviousFixSummaryCount = len(context.PreviousFixSummaryPaths)
		context.HasPreviousReview = context.PreviousReviewCount > 0
		context.HasPreviousFixSummary = context.PreviousFixSummaryCount > 0
		if context.HasPreviousReview {
			context.LatestPreviousReviewPath = context.PreviousReviewPaths[context.PreviousReviewCount-1]
		}
		if context.HasPreviousFixSummary {
			context.LatestPreviousFixSummaryPath = context.PreviousFixSummaryPaths[context.PreviousFixSummaryCount-1]
		}
	}
	if kind == "archive" {
		for i := 1; i <= state.Workflow.MaxReviewIterations; i++ {
			reviewStage := fmt.Sprintf("review_%d", i)
			if state.Stages[reviewStage] == "" {
				continue
			}
			context.PreviousReviewPaths = append(context.PreviousReviewPaths, filepath.Join(runPath, fmt.Sprintf("review-%d.json", i)))
			qaStage := fmt.Sprintf("qa_%d", i)
			if state.Stages[qaStage] != "" {
				context.PreviousQAPaths = append(context.PreviousQAPaths, filepath.Join(runPath, fmt.Sprintf("qa-%d.json", i)))
			}
			fixStage := fmt.Sprintf("fix_%d", i)
			if state.Stages[fixStage] != "" {
				context.PreviousFixSummaryPaths = append(context.PreviousFixSummaryPaths, filepath.Join(runPath, fmt.Sprintf("fix-%d-summary.md", i)))
			}
		}
	}
	return context, nil
}

// promptRoleSession returns the current stage role's backend-scoped session identity.
func promptRoleSession(state State) (string, string) {
	options, err := state.Workflow.StageOption(state.Stage)
	if err != nil || options.Tool == "" {
		return "", ""
	}
	key := sessionStateKey(options.Tool, stageSessionRole(state.Stage))
	return key, state.Sessions[key]
}

func parallelGroupConfigured(workflow WorkflowConfig, name string) bool {
	group, ok := workflow.Parallel.Groups[name]
	return ok && len(group.Members) > 0
}

// saveState writes durable workflow state as pretty JSON.
func saveState(repo string, state State) error {
	stateFileMu.Lock()
	defer stateFileMu.Unlock()
	if err := validateRunID(state.RunID); err != nil {
		return err
	}
	normalizeStateMaps(&state)
	return writeStateFileLocked(repo, state.RunID, state)
}

// loadState reads durable workflow state for a run id.
func loadState(repo, runID string) (State, error) {
	stateFileMu.Lock()
	defer stateFileMu.Unlock()
	if err := validateRunID(runID); err != nil {
		return State{}, err
	}
	data, err := os.ReadFile(filepath.Join(runDir(repo, runID), "state.json"))
	if err != nil {
		return State{}, err
	}
	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return State{}, err
	}
	if err := validateRunID(state.RunID); err != nil {
		return State{}, err
	}
	if state.RunID != runID {
		return State{}, fmt.Errorf("state run_id %q does not match requested run %q", state.RunID, runID)
	}
	normalizeStateMaps(&state)
	return state, nil
}

// mergeState applies a small mutation to the latest durable state under one lock.
func mergeState(repo, runID string, mutate func(*State)) error {
	stateFileMu.Lock()
	defer stateFileMu.Unlock()
	if err := validateRunID(runID); err != nil {
		return err
	}
	path := filepath.Join(runDir(repo, runID), "state.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var state State
	if err := json.Unmarshal(data, &state); err != nil {
		return err
	}
	if state.RunID != runID {
		return fmt.Errorf("state run_id %q does not match requested run %q", state.RunID, runID)
	}
	normalizeStateMaps(&state)
	if mutate != nil {
		mutate(&state)
	}
	if state.RunID != runID {
		return fmt.Errorf("state mutation changed run_id from %q to %q", runID, state.RunID)
	}
	normalizeStateMaps(&state)
	return writeStateFileLocked(repo, runID, state)
}

// writeStateFileLocked writes state.json for the explicit run while the caller holds stateFileMu.
func writeStateFileLocked(repo, runID string, state State) error {
	if state.RunID != runID {
		return fmt.Errorf("state run_id %q does not match write run %q", state.RunID, runID)
	}
	if err := os.MkdirAll(runDir(repo, runID), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(runDir(repo, runID), "state.json"), append(data, '\n'), 0o644)
}

// validateRunID rejects run identifiers that could escape the runs directory.
func validateRunID(runID string) error {
	if runID == "" || runID == "." || runID == ".." || filepath.IsAbs(runID) || strings.Contains(runID, "..") || strings.ContainsAny(runID, `/\`) {
		return fmt.Errorf("invalid run_id %q", runID)
	}
	return nil
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
	case statusFailed, statusBlocked, statusValidationBlocked, statusAborted, "aborted":
		return true
	default:
		return state.Stage == statusBlocked || state.Stage == statusValidationBlocked
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

// AbortRun marks an unfinished run aborted and interrupts its live worker when possible.
func AbortRun(repo, runID string) error {
	state, err := loadState(repo, runID)
	if err != nil {
		return err
	}
	if err := interruptLockedRun(repo, runID); err != nil {
		return err
	}
	state.Status = "aborted"
	if err := saveState(repo, state); err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(runDir(repo, runID), "lock")); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// interruptLockedRun sends the best available stop signal to the process recorded in a run lock.
func interruptLockedRun(repo, runID string) error {
	data, err := os.ReadFile(filepath.Join(runDir(repo, runID), "lock"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	lock, err := parseLockInfo(data)
	if err != nil {
		return nil
	}
	hostname, _ := os.Hostname()
	if lock.Hostname != "" && hostname != "" && lock.Hostname != hostname {
		return fmt.Errorf("run %s 被主机 %s 上的进程 %d 锁定，无法从主机 %s 中止", runID, lock.Hostname, lock.PID, hostname)
	}
	if lock.PID == os.Getpid() {
		return nil
	}
	return interruptProcessGroup(lock.PID)
}

// ArchiveSupersededRun marks a stale unfinished run as archived after the user starts a replacement.
func ArchiveSupersededRun(repo, runID string) error {
	state, err := loadState(repo, runID)
	if err != nil {
		return err
	}
	state.Status = statusArchived
	state.Error = "superseded by a newer run"
	if err := saveState(repo, state); err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(runDir(repo, runID), "lock")); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// acquireLock creates a process lock file for one run.
func acquireLock(repo, runID string) (func(), error) {
	return acquireLockForGOOS(repo, runID, runtime.GOOS)
}

// acquireLockForGOOS creates a process lock and keeps Windows unknown locks explicit.
func acquireLockForGOOS(repo, runID, goos string) (func(), error) {
	path := filepath.Join(runDir(repo, runID), "lock")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	status, err := lockFileStatus(repo, runID, goos)
	if err != nil {
		return nil, err
	}
	if status == lockStatusActive {
		return nil, newRunLockedError(runID)
	}
	if status == lockStatusUnknown {
		return nil, fmt.Errorf("run %s 的 lock 无法确认，请通过交互菜单恢复或中止", runID)
	}
	hostname, _ := os.Hostname()
	lock := LockInfo{
		PID:       os.Getpid(),
		Hostname:  hostname,
		RunID:     runID,
		StartedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	data, err := json.MarshalIndent(lock, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		return nil, err
	}
	return func() { _ = os.Remove(path) }, nil
}

// lockActive reports whether a lock file points at a live local process.
func lockActive(repo, runID string) (bool, error) {
	status, err := lockFileStatus(repo, runID, runtime.GOOS)
	return status == lockStatusActive, err
}

// lockFileStatus classifies a run lock without killing any external process.
func lockFileStatus(repo, runID, goos string) (lockStatus, error) {
	data, err := os.ReadFile(filepath.Join(runDir(repo, runID), "lock"))
	if os.IsNotExist(err) {
		return lockStatusNone, nil
	}
	if err != nil {
		return lockStatusNone, err
	}
	lock, err := parseLockInfo(data)
	if err != nil || lock.PID <= 0 {
		return lockStatusStale, nil
	}
	hostname, _ := os.Hostname()
	if lock.Hostname != "" && hostname != "" && lock.Hostname != hostname {
		return lockStatusActive, nil
	}
	if goos == "windows" {
		if lock.PID == os.Getpid() {
			return lockStatusActive, nil
		}
		return lockStatusUnknown, nil
	}
	process, err := os.FindProcess(lock.PID)
	if err != nil {
		return lockStatusStale, nil
	}
	if process.Signal(syscall.Signal(0)) == nil {
		return lockStatusActive, nil
	}
	return lockStatusStale, nil
}

// parseLockInfo accepts structured JSON lock metadata.
func parseLockInfo(data []byte) (LockInfo, error) {
	var lock LockInfo
	if err := json.Unmarshal(data, &lock); err != nil {
		return LockInfo{}, err
	}
	if lock.PID <= 0 {
		return LockInfo{}, fmt.Errorf("lock pid 不能为空")
	}
	return lock, nil
}

// gitSnapshot captures HEAD and porcelain status for intervention checks.
func gitSnapshot(repo string) (string, string, error) {
	gitPath, err := resolveCommand("git")
	if err != nil {
		return "", "", err
	}
	headCmd := commandContext(context.Background(), gitPath, "rev-parse", "--verify", "HEAD^{commit}")
	headCmd.Dir = repo
	head, err := headCmd.CombinedOutput()
	if err != nil {
		headErr := strings.TrimSpace(string(head))
		if isUnbornBranch(repo, gitPath) {
			return "", "", fmt.Errorf(errNoInitialCommit)
		}
		if headErr != "" {
			return "", "", fmt.Errorf("git rev-parse --verify HEAD 失败：%s", headErr)
		}
		return "", "", err
	}
	statusCmd := commandContext(context.Background(), gitPath, "status", "--porcelain")
	statusCmd.Dir = repo
	status, err := statusCmd.CombinedOutput()
	if err != nil {
		statusErr := strings.TrimSpace(string(status))
		if statusErr != "" {
			return "", "", fmt.Errorf("git status --porcelain 失败：%s", statusErr)
		}
		return "", "", err
	}
	return strings.TrimSpace(string(head)), filterRuntimeStatus(string(status)), nil
}

// isUnbornBranch confirms HEAD is a symbolic branch that has not received a commit yet.
func isUnbornBranch(repo, gitPath string) bool {
	cmd := commandContext(context.Background(), gitPath, "symbolic-ref", "-q", "HEAD")
	cmd.Dir = repo
	out, err := cmd.Output()
	ref := strings.TrimSpace(string(out))
	if err != nil || !strings.HasPrefix(ref, "refs/heads/") {
		return false
	}
	verifyCmd := commandContext(context.Background(), gitPath, "show-ref", "--verify", "--quiet", ref)
	verifyCmd.Dir = repo
	return verifyCmd.Run() != nil
}

// filterRuntimeStatus removes workflow-owned runtime paths from git status snapshots.
func filterRuntimeStatus(status string) string {
	var kept []string
	for _, line := range strings.Split(status, "\n") {
		if line == "" {
			continue
		}
		path := strings.TrimSpace(line[3:])
		if path == ".wo" || strings.HasPrefix(path, ".wo/") || path == "test-results" || strings.HasPrefix(path, "test-results/") {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

type gitSnapshotGuard struct {
	Blocked bool
	Paths   []string
	Allowed []string
}

// Detail formats the paths that explain a git snapshot guard decision.
func (guard gitSnapshotGuard) Detail() string {
	if len(guard.Paths) == 0 {
		return "HEAD 变化"
	}
	limit := len(guard.Paths)
	if limit > 5 {
		limit = 5
	}
	detail := strings.Join(guard.Paths[:limit], ", ")
	if len(guard.Paths) > limit {
		detail += fmt.Sprintf(" 等 %d 个路径", len(guard.Paths))
	}
	return detail
}

// classifyGitSnapshotChange separates new unrelated demand proposal edits from current-run writes.
func classifyGitSnapshotChange(repo, changeName, beforeHead, beforeDiff, afterHead, afterDiff string) (gitSnapshotGuard, error) {
	paths := changedStatusPaths(beforeDiff, afterDiff)
	if beforeHead != afterHead {
		commitPaths, err := committedPaths(repo, beforeHead, afterHead)
		if err != nil {
			return gitSnapshotGuard{Blocked: true}, err
		}
		paths = append(paths, commitPaths...)
	}
	var blocked []string
	var allowed []string
	for _, path := range uniqueSortedPaths(paths) {
		if isUnrelatedChangePath(path, changeName) {
			allowed = append(allowed, path)
			continue
		}
		blocked = append(blocked, path)
	}
	return gitSnapshotGuard{Blocked: len(blocked) > 0, Paths: blocked, Allowed: allowed}, nil
}

// changedStatusPaths returns paths whose porcelain status changed since the saved baseline.
func changedStatusPaths(before, after string) []string {
	beforeLines := statusLineByPath(before)
	afterLines := statusLineByPath(after)
	seen := map[string]bool{}
	var paths []string
	for path, line := range afterLines {
		if beforeLines[path] != line {
			paths = append(paths, path)
			seen[path] = true
		}
	}
	for path, line := range beforeLines {
		if seen[path] {
			continue
		}
		if afterLines[path] != line {
			paths = append(paths, path)
		}
	}
	return paths
}

// statusLineByPath indexes every normalized git status path by its full porcelain line.
func statusLineByPath(status string) map[string]string {
	lines := map[string]string{}
	for _, line := range strings.Split(status, "\n") {
		if line == "" {
			continue
		}
		for _, path := range porcelainLinePaths(line) {
			lines[path] = line
		}
	}
	return lines
}

// porcelainLinePaths extracts all business paths from one git status --porcelain line.
func porcelainLinePaths(line string) []string {
	if len(line) < 4 {
		return nil
	}
	path := strings.TrimSpace(line[3:])
	if renamed := strings.Split(path, " -> "); len(renamed) == 2 {
		return statusNamePaths(strings.Join(renamed, "\n"))
	}
	return statusNamePaths(path)
}

// committedPaths returns every file path touched by commits between two saved HEADs.
func committedPaths(repo, beforeHead, afterHead string) ([]string, error) {
	gitPath, err := exec.LookPath("git")
	if err != nil {
		return nil, err
	}
	cmd := commandContext(context.Background(), gitPath, "diff", "--name-status", "--find-renames", beforeHead, afterHead)
	cmd.Dir = repo
	out, err := cmd.CombinedOutput()
	if err != nil {
		detail := strings.TrimSpace(string(out))
		if detail != "" {
			return nil, fmt.Errorf("git diff --name-status 失败：%s", detail)
		}
		return nil, err
	}
	return statusNamePaths(string(out)), nil
}

// statusNamePaths normalizes paths from newline or tab separated git status output.
func statusNamePaths(output string) []string {
	var paths []string
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Split(strings.TrimSpace(line), "\t")
		if len(fields) > 1 {
			fields = fields[1:]
		}
		for _, field := range fields {
			path := strings.TrimSpace(field)
			if path != "" {
				paths = append(paths, filepath.ToSlash(path))
			}
		}
	}
	return paths
}

// uniqueSortedPaths removes duplicate path entries for stable guard messages.
func uniqueSortedPaths(paths []string) []string {
	seen := map[string]bool{}
	var unique []string
	for _, path := range paths {
		path = filepath.ToSlash(strings.TrimSpace(path))
		if path == "" || seen[path] {
			continue
		}
		seen[path] = true
		unique = append(unique, path)
	}
	sort.Strings(unique)
	return unique
}

// isUnrelatedChangePath returns true only for docs/changes entries outside the active change.
func isUnrelatedChangePath(path, changeName string) bool {
	path = strings.TrimPrefix(filepath.ToSlash(path), "./")
	const prefix = "docs/changes/"
	if !strings.HasPrefix(path, prefix) {
		return false
	}
	rest := strings.TrimPrefix(path, prefix)
	if rest == "" || strings.HasPrefix(rest, "archive/") || strings.HasPrefix(rest, ".") {
		return false
	}
	change := rest
	if slash := strings.IndexByte(rest, '/'); slash >= 0 {
		change = rest[:slash]
	}
	return change != "" && change != changeName
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
		return fmt.Errorf("解析 wo 可执行文件失败：%w", err)
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
