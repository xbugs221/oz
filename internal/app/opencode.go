// Package app wraps OpenCode CLI JSONL execution for sealed workflow stages.
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
)

// OpenCodeTool adapts OpenCode CLI to the generic agent backend contract.
type OpenCodeTool struct{}

// Name returns the config name for the OpenCode backend.
func (OpenCodeTool) Name() string { return "opencode" }

// Resolve verifies OpenCode is available on PATH.
func (OpenCodeTool) Resolve() error {
	_, err := resolveCommand("opencode")
	return err
}

// PlanningCommand starts OpenCode TUI with the rendered planning prompt.
func (OpenCodeTool) PlanningCommand(ctx context.Context, repo, prompt string, stdin io.Reader, options StageOptions) (*exec.Cmd, error) {
	path, err := resolveCommand("opencode")
	if err != nil {
		return nil, err
	}
	cmd := commandContext(ctx, path, opencodePlanningArgs(repo, prompt, options)...)
	cmd.Stdin = stdin
	return cmd, nil
}

// NewRunner returns an OpenCode sealed-run runner.
func (OpenCodeTool) NewRunner() AgentRunner {
	path, err := resolveCommand("opencode")
	return &OpenCodeCLI{Path: path, ResolveErr: err}
}

// OpenCodeCLI invokes the real opencode executable.
type OpenCodeCLI struct {
	Path       string
	ResolveErr error
	Progress   io.Writer
}

// SetProgress redirects concise process progress for callers that own the UI.
func (o *OpenCodeCLI) SetProgress(progress io.Writer) {
	o.Progress = progress
}

// Run executes opencode run, extracts session metadata, and waits for process exit.
func (o OpenCodeCLI) Run(ctx context.Context, repo, prompt, sessionID string, options StageOptions) (string, error) {
	if o.ResolveErr != nil {
		return "", o.ResolveErr
	}
	if o.Path == "" {
		return "", fmt.Errorf("找不到 opencode 可执行文件")
	}
	args := opencodeRunArgs(repo, prompt, sessionID, options)
	cmd := commandContext(ctx, o.Path, args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return "", err
	}
	printAgentProcessStarted(o.Progress, "opencode", cmd.Process.Pid)
	observed, drainErr := drainOpenCodeJSONL(stdout, o.Progress)
	waitErr := cmd.Wait()
	printAgentProcessExited(o.Progress, "opencode", cmd.Process.Pid, cmd.ProcessState.ExitCode())
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

// opencodePlanningArgs builds TUI planning arguments.
func opencodePlanningArgs(repo, prompt string, options StageOptions) []string {
	args := []string{repo}
	if options.Model != "" {
		args = append(args, "-m", options.Model)
	}
	return append(args, "--prompt", prompt)
}

// opencodeRunArgs builds shell-free sealed-run arguments.
func opencodeRunArgs(repo, prompt, sessionID string, options StageOptions) []string {
	args := []string{"run", "--format", "json", "--dir", repo}
	if sessionID != "" {
		args = append(args, "--session", sessionID)
	}
	if options.Model != "" {
		args = append(args, "-m", options.Model)
	}
	if options.Reasoning != "" {
		args = append(args, "--variant", options.Reasoning)
	}
	return append(args, prompt)
}

// drainOpenCodeJSONL reads stdout while best-effort extracting session metadata.
func drainOpenCodeJSONL(stdout io.Reader, progress io.Writer) (sessionID string, err error) {
	reader := bufio.NewReaderSize(stdout, 64*1024)
	for {
		line, readErr := reader.ReadBytes('\n')
		if len(line) > 0 {
			if id := opencodeSessionIDFromLine(line, progress); id != "" {
				sessionID = id
			}
		}
		if readErr == nil {
			continue
		}
		if readErr != io.EOF && err == nil {
			err = readErr
		}
		return sessionID, err
	}
}

// opencodeSessionIDFromLine parses common OpenCode JSONL session id shapes.
func opencodeSessionIDFromLine(line []byte, progress io.Writer) string {
	var event map[string]any
	if err := json.Unmarshal(line, &event); err != nil {
		return ""
	}
	if failed, _ := event["type"].(string); strings.Contains(failed, "error") || strings.Contains(failed, "failed") {
		printAgentSessionFailed(progress, "opencode")
	}
	if id := stringField(event, "sessionID", "sessionId", "session_id"); id != "" {
		printAgentSessionStarted(progress, "opencode", id)
		return id
	}
	for _, key := range []string{"session", "part", "message"} {
		nested, ok := event[key].(map[string]any)
		if !ok {
			continue
		}
		if id := stringField(nested, "sessionID", "sessionId", "session_id"); id != "" {
			printAgentSessionStarted(progress, "opencode", id)
			return id
		}
	}
	return ""
}

// stringField returns the first string value found under any candidate key.
func stringField(values map[string]any, keys ...string) string {
	for _, key := range keys {
		value, ok := values[key].(string)
		if ok && value != "" {
			return value
		}
	}
	return ""
}
