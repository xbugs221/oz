// Package app wraps Codex CLI JSONL execution for sealed workflow stages.
package app

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// CodexTool adapts Codex CLI to the generic agent backend contract.
type CodexTool struct{}

// Name returns the config name for the Codex backend.
func (CodexTool) Name() string { return "codex" }

// Resolve verifies Codex is available on PATH.
func (CodexTool) Resolve() error {
	_, err := resolveCommand("codex")
	return err
}

// PlanningCommand keeps human Codex planning interactive with the rendered seed prompt.
func (CodexTool) PlanningCommand(ctx context.Context, _ string, prompt string, stdin io.Reader, options StageOptions) (*exec.Cmd, error) {
	path, err := resolveCommand("codex")
	if err != nil {
		return nil, err
	}
	return codexPlanningCommand(ctx, path, prompt, stdin, options), nil
}

// NewRunner returns a Codex sealed-run runner.
func (CodexTool) NewRunner() AgentRunner {
	return NewCodexCLI()
}

// CodexCLI invokes the real codex executable.
type CodexCLI struct {
	Path       string
	ResolveErr error
	Progress   io.Writer
	Artifact   *artifactCapture
}

// NewCodexCLI resolves the codex executable using the host PATH.
func NewCodexCLI() *CodexCLI {
	path, err := resolveCommand("codex")
	return &CodexCLI{Path: path, ResolveErr: err}
}

// SetProgress redirects concise process progress for callers that own the UI.
func (c *CodexCLI) SetProgress(progress io.Writer) {
	c.Progress = progress
}

// SetArtifactCapture records assistant text for read-only subagent artifact materialization.
func (c *CodexCLI) SetArtifactCapture(capture *artifactCapture) {
	c.Artifact = capture
}

// Run executes codex exec/resume, extracts session metadata, and waits for process exit.
func (c CodexCLI) Run(ctx context.Context, repo, prompt, threadID string, options StageOptions) (string, error) {
	if c.ResolveErr != nil {
		return "", c.ResolveErr
	}
	if c.Path == "" {
		return "", fmt.Errorf("找不到 codex 可执行文件")
	}
	args := codexExecArgs(repo, threadID, options)
	cmd := commandContext(ctx, c.Path, args...)
	cmd.Stdin = stringsReader(prompt)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return "", err
	}
	printCodexProcessStarted(c.Progress, cmd.Process.Pid)
	observed, drainErr, waitErr := c.waitCodexJSONLCommand(ctx, cmd, stdout)
	if drainErr != nil {
		return observed, drainErr
	}
	if waitErr != nil {
		return observed, codexCommandError(waitErr, stderr.String())
	}
	return observed, nil
}

// waitCodexJSONLCommand drains Codex JSONL with an output-idle watchdog so stuck turns can be retried.
func (c CodexCLI) waitCodexJSONLCommand(ctx context.Context, cmd *exec.Cmd, stdout io.Reader) (string, error, error) {
	type drainResult struct {
		threadID string
		err      error
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
		threadID, err := drainCodexJSONLWithCaptureNotify(stdout, c.Progress, c.Artifact, func() {
			select {
			case touch <- struct{}{}:
			default:
			}
		}, setObserved)
		drained <- drainResult{threadID: threadID, err: err}
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
			if result.threadID != "" {
				setObserved(result.threadID)
			}
			drainErr = result.err
		case err := <-waited:
			waitDone = true
			waitErr = err
			exitCode := -1
			if cmd.ProcessState != nil {
				exitCode = cmd.ProcessState.ExitCode()
			}
			printCodexProcessExited(c.Progress, cmd.Process.Pid, exitCode)
		case <-timer.C:
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
			return getObserved(), fmt.Errorf("%w: codex %s 内没有新输出，已终止本次进程并准备续跑", errGoDAGRetryableNode, agentOutputIdleTimeout), nil
		}
	}
	return getObserved(), drainErr, waitErr
}

// codexCommandError summarizes Codex failures and includes bounded captured diagnostics.
func codexCommandError(err error, stderrText string) error {
	stderrText = strings.TrimSpace(limitAgentDiagnostics(stderrText))
	if stderrText == "" {
		return err
	}
	return fmt.Errorf("%w；stderr：%s", err, stderrText)
}

// stringsReader creates an io.Reader without importing shell-specific behavior.
func stringsReader(text string) io.Reader {
	return strings.NewReader(agentPromptText(text))
}

// codexExecArgs builds shell-free arguments while keeping prompt content on stdin.
func codexExecArgs(repo, threadID string, options StageOptions) []string {
	var args []string
	if options.Permissions == "danger-full-access" {
		args = append(args, "--dangerously-bypass-approvals-and-sandbox")
	}
	args = append(args, "exec", "--json", "--cd", repo)
	if options.Permissions == "sandbox" {
		args = append(args, "--sandbox", "workspace-write")
	}
	if options.Model != "" {
		args = append(args, "-m", options.Model)
	}
	if options.Reasoning != "" {
		args = append(args, "-c", "model_reasoning_effort="+options.Reasoning)
	}
	if options.Fast {
		args = append(args, "--enable", "fast_mode")
	} else {
		args = append(args, "--disable", "fast_mode")
	}
	if threadID != "" {
		args = append(args, "resume", threadID)
	}
	return append(args, "-")
}

// codexEvent is the subset of Codex JSONL needed for workflow control.
type codexEvent struct {
	Type     string `json:"type"`
	ThreadID string `json:"thread_id"`
	Thread   struct {
		ID string `json:"id"`
	} `json:"thread"`
}

// drainCodexJSONL reads stdout while best-effort extracting session metadata.
func drainCodexJSONL(stdout io.Reader, progress io.Writer) (threadID string, err error) {
	return drainCodexJSONLWithCapture(stdout, progress, nil)
}

// drainCodexJSONLWithCapture reads stdout while extracting session metadata and final text.
func drainCodexJSONLWithCapture(stdout io.Reader, progress io.Writer, capture *artifactCapture) (threadID string, err error) {
	return drainCodexJSONLWithCaptureNotify(stdout, progress, capture, nil, nil)
}

// drainCodexJSONLWithCaptureNotify reports each output line and thread id to the caller.
func drainCodexJSONLWithCaptureNotify(stdout io.Reader, progress io.Writer, capture *artifactCapture, touch func(), session func(string)) (threadID string, err error) {
	reader := bufio.NewReaderSize(stdout, 64*1024)
	for {
		line, readErr := reader.ReadBytes('\n')
		if len(line) > 0 {
			if touch != nil {
				touch()
			}
			if id := codexThreadIDFromLine(line, progress); id != "" {
				threadID = id
				if session != nil {
					session(id)
				}
			}
			capturePiText(line, capture)
		}
		if readErr == nil {
			continue
		}
		if readErr != io.EOF && err == nil {
			err = readErr
		}
		return threadID, err
	}
}

// codexThreadIDFromLine parses one JSONL event without making workflow control depend on it.
func codexThreadIDFromLine(line []byte, progress io.Writer) string {
	var event codexEvent
	if err := json.Unmarshal(line, &event); err != nil {
		return ""
	}
	printCodexProgress(progress, event)
	if event.Type != "thread.started" {
		return ""
	}
	if event.ThreadID != "" {
		return event.ThreadID
	}
	return event.Thread.ID
}

// printCodexProgress mirrors concise Codex JSONL progress to the terminal.
func printCodexProgress(progress io.Writer, event codexEvent) {
	if progress == nil {
		return
	}
	switch event.Type {
	case "thread.started":
		id := event.ThreadID
		if id == "" {
			id = event.Thread.ID
		}
		if id != "" {
			fmt.Fprintf(progress, "agent session started: tool=codex session=%s\n", id)
		}
	case "turn.failed":
		fmt.Fprintln(progress, "agent session failed: tool=codex")
	}
}

// printCodexProcessStarted reports only the spawned Codex process boundary.
func printCodexProcessStarted(progress io.Writer, pid int) {
	if progress == nil {
		return
	}
	fmt.Fprintf(progress, "agent process started: tool=codex pid=%d\n", pid)
}

// printCodexProcessExited reports only the Codex process exit boundary.
func printCodexProcessExited(progress io.Writer, pid, exitCode int) {
	if progress == nil {
		return
	}
	fmt.Fprintf(progress, "agent process exited: tool=codex pid=%d exit=%d\n", pid, exitCode)
}
