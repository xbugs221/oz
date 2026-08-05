// Package app wraps Claude Code CLI JSONL execution for sealed workflow stages.
package app

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// ClaudeTool adapts Claude Code CLI to the generic agent backend contract.
type ClaudeTool struct{}

// Name returns the config name for the Claude backend.
func (ClaudeTool) Name() string { return "claude" }

// Resolve verifies Claude is available on PATH.
func (ClaudeTool) Resolve() error {
	_, err := resolveCommand("claude")
	return err
}

// PlanningCommand keeps human Claude planning interactive with the rendered seed prompt.
func (ClaudeTool) PlanningCommand(ctx context.Context, _ string, prompt string, stdin io.Reader, options StageOptions) (*exec.Cmd, error) {
	path, err := resolveCommand("claude")
	if err != nil {
		return nil, err
	}
	return claudePlanningCommand(ctx, path, prompt, stdin, options), nil
}

// NewRunner returns a Claude sealed-run runner.
func (ClaudeTool) NewRunner() AgentRunner {
	return NewClaudeCLI()
}

// ClaudeCLI invokes the real claude executable.
type ClaudeCLI struct {
	Path       string
	ResolveErr error
	Progress   io.Writer
	Artifact   *artifactCapture
}

// NewClaudeCLI resolves the claude executable using the host PATH.
func NewClaudeCLI() *ClaudeCLI {
	path, err := resolveCommand("claude")
	return &ClaudeCLI{Path: path, ResolveErr: err}
}

// SetProgress redirects concise process progress for callers that own the UI.
func (c *ClaudeCLI) SetProgress(progress io.Writer) {
	c.Progress = progress
}

// SetArtifactCapture records assistant text for read-only subagent artifact materialization.
func (c *ClaudeCLI) SetArtifactCapture(capture *artifactCapture) {
	c.Artifact = capture
}

// Run executes claude in stream-json mode, extracts session metadata, and waits for process exit.
func (c ClaudeCLI) Run(ctx context.Context, repo, prompt, sessionID string, options StageOptions) (string, error) {
	if c.ResolveErr != nil {
		return "", c.ResolveErr
	}
	if c.Path == "" {
		return "", fmt.Errorf("找不到 claude 可执行文件")
	}
	args := claudeRunArgs(repo, prompt, sessionID, options)
	cmd := commandContext(ctx, c.Path, args...)
	cmd.Dir = repo
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return "", err
	}
	printAgentProcessStarted(c.Progress, "claude", cmd.Process.Pid)
	observed, drainErr, waitErr := c.waitClaudeJSONLCommand(ctx, cmd, stdout)
	if drainErr != nil {
		return observed, drainErr
	}
	if waitErr != nil {
		stderrText := strings.TrimSpace(limitAgentDiagnostics(stderr.String()))
		if stderrText == "" {
			return observed, waitErr
		}
		return observed, fmt.Errorf("%w；stderr：%s", waitErr, stderrText)
	}
	return observed, nil
}

// waitClaudeJSONLCommand drains Claude JSONL with an output-idle watchdog so stuck turns can be retried.
func (c ClaudeCLI) waitClaudeJSONLCommand(ctx context.Context, cmd *exec.Cmd, stdout io.Reader) (string, error, error) {
	type drainResult struct {
		sessionID string
		err       error
	}
	touch := make(chan struct{}, 1)
	drained := make(chan drainResult, 1)
	waited := make(chan error, 1)
	var observedMu sync.Mutex
	observed := ""
	setObserved := func(id string) {
		observedMu.Lock()
		observed = id
		observedMu.Unlock()
	}
	getObserved := func() string {
		observedMu.Lock()
		defer observedMu.Unlock()
		return observed
	}
	go func() {
		sessionID, err := drainClaudeJSONLWithCaptureNotify(stdout, c.Progress, c.Artifact, func() {
			select {
			case touch <- struct{}{}:
			default:
			}
		}, setObserved)
		drained <- drainResult{sessionID: sessionID, err: err}
	}()
	go func() {
		waited <- cmd.Wait()
	}()
	timer := time.NewTimer(agentOutputIdleTimeout)
	defer timer.Stop()
	drainDone := false
	waitDone := false
	var drainErr error
	var waitErr error
	for !drainDone || !waitDone {
		select {
		case <-ctx.Done():
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
			return getObserved(), ctx.Err(), nil
		case <-touch:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(agentOutputIdleTimeout)
		case result := <-drained:
			drainDone = true
			if result.sessionID != "" {
				setObserved(result.sessionID)
			}
			drainErr = result.err
		case err := <-waited:
			waitDone = true
			waitErr = err
			exitCode := -1
			if cmd.ProcessState != nil {
				exitCode = cmd.ProcessState.ExitCode()
			}
			printAgentProcessExited(c.Progress, "claude", cmd.Process.Pid, exitCode)
		case <-timer.C:
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
			return getObserved(), fmt.Errorf("%w: claude %s 内没有新输出，已终止本次进程并准备续跑", errGoDAGRetryableNode, agentOutputIdleTimeout), nil
		}
	}
	return getObserved(), drainErr, waitErr
}

// claudeRunArgs builds shell-free sealed-run arguments for Claude stream-json mode.
func claudeRunArgs(repo, prompt, sessionID string, options StageOptions) []string {
	args := []string{"-p", "--verbose", "--output-format", "stream-json"}
	args = append(args, claudeCommonArgs(options)...)
	if sessionID != "" {
		args = append(args, "--resume", sessionID)
	}
	return append(args, agentPromptText(prompt))
}

// claudePlanningArgs builds interactive planning arguments using Claude option names.
func claudePlanningArgs(prompt string, options StageOptions) []string {
	args := claudeCommonArgs(options)
	return append(args, agentPromptText(prompt))
}

// claudeCommonArgs maps shared stage options to Claude-supported CLI flags.
func claudeCommonArgs(options StageOptions) []string {
	var args []string
	if options.Model != "" {
		args = append(args, "--model", options.Model)
	}
	if options.Reasoning != "" {
		args = append(args, "--effort", options.Reasoning)
	}
	switch options.Permissions {
	case "danger-full-access", "default", "":
		// Claude -p print mode has no interactive approval channel, so sealed
		// stages must bypass permissions to read run-dir state and skill files
		// outside the repo working directory (mirroring codex exec's full-auto default).
		args = append(args, "--dangerously-skip-permissions")
	case "sandbox":
		args = append(args, "--allowedTools", "Read,Grep,Glob")
	}
	return args
}

// claudePlanningCommand keeps human planning interactive while passing the seed prompt directly.
func claudePlanningCommand(ctx context.Context, path, prompt string, stdin io.Reader, options StageOptions) *exec.Cmd {
	cmd := commandContext(ctx, path, claudePlanningArgs(prompt, options)...)
	cmd.Stdin = stdin
	return cmd
}

// claudeSessionEvent is the subset of Claude JSONL needed for session identification.
type claudeSessionEvent struct {
	Type      string `json:"type"`
	Subtype   string `json:"subtype"`
	SessionID string `json:"session_id"`
}

// claudeSessionIDFromLine parses one JSONL event without making workflow control depend on it.
func claudeSessionIDFromLine(line []byte, progress io.Writer) string {
	var event claudeSessionEvent
	if err := json.Unmarshal(line, &event); err != nil {
		return ""
	}
	if event.SessionID == "" {
		return ""
	}
	if event.Type == "system" && event.Subtype == "init" {
		printAgentSessionStarted(progress, "claude", event.SessionID)
		return event.SessionID
	}
	if event.Type == "result" {
		printAgentSessionStarted(progress, "claude", event.SessionID)
		return event.SessionID
	}
	return ""
}

// drainClaudeJSONL reads stdout while best-effort extracting Claude session metadata.
func drainClaudeJSONL(stdout io.Reader, progress io.Writer) (sessionID string, err error) {
	return drainClaudeJSONLWithCapture(stdout, progress, nil)
}

// drainClaudeJSONLWithCapture reads stdout while extracting Claude session metadata and assistant text.
func drainClaudeJSONLWithCapture(stdout io.Reader, progress io.Writer, capture *artifactCapture) (sessionID string, err error) {
	return drainClaudeJSONLWithCaptureNotify(stdout, progress, capture, nil, nil)
}

// drainClaudeJSONLWithCaptureNotify reports each output line and session id to the caller.
func drainClaudeJSONLWithCaptureNotify(stdout io.Reader, progress io.Writer, capture *artifactCapture, touch func(), session func(string)) (sessionID string, err error) {
	reader := bufio.NewReaderSize(stdout, 64*1024)
	for {
		line, readErr := reader.ReadBytes('\n')
		if len(line) > 0 {
			if touch != nil {
				touch()
			}
			if id := claudeSessionIDFromLine(line, progress); id != "" {
				sessionID = id
				if session != nil {
					session(id)
				}
			}
			captureClaudeText(line, capture)
		}
		if readErr == nil {
			continue
		}
		if readErr != io.EOF && !errors.Is(readErr, os.ErrClosed) && err == nil {
			err = readErr
		}
		return sessionID, err
	}
}

// captureClaudeText stores assistant message text for read-only artifact materialization.
func captureClaudeText(line []byte, capture *artifactCapture) {
	if capture == nil {
		return
	}
	var event map[string]interface{}
	if err := json.Unmarshal(line, &event); err != nil {
		return
	}
	if event["type"] != "assistant" {
		return
	}
	message, ok := event["message"].(map[string]interface{})
	if !ok {
		return
	}
	if role, _ := message["role"].(string); role != "" && role != "assistant" {
		return
	}
	content, ok := message["content"].([]interface{})
	if !ok {
		return
	}
	for _, item := range content {
		obj, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if typ, _ := obj["type"].(string); typ != "" && typ != "text" {
			continue
		}
		if text, _ := obj["text"].(string); text != "" {
			capture.Append(text)
		}
	}
}
