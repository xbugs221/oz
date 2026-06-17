// Package app contains workflow engine state and execution boundaries.
package app

import (
	"fmt"
	"strconv"
	"strings"
)

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
	lines := compactStatusLines(buildHumanStatusView(e.Repo, state, state.RunID, "→"))
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
