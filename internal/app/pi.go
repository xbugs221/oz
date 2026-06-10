// Package app wraps Pi CLI JSONL execution for sealed workflow stages.
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

// PiTool adapts Pi Coding Agent CLI to the generic agent backend contract.
type PiTool struct{}

// Name returns the config name for the Pi backend.
func (PiTool) Name() string { return "pi" }

// Resolve verifies Pi is available on PATH.
func (PiTool) Resolve() error {
	_, err := resolveCommand("pi")
	return err
}

// PlanningCommand starts Pi interactively with the rendered planning prompt.
func (PiTool) PlanningCommand(ctx context.Context, _ string, prompt string, stdin io.Reader, options StageOptions) (*exec.Cmd, error) {
	path, err := resolveCommand("pi")
	if err != nil {
		return nil, err
	}
	cmd := commandContext(ctx, path, piPlanningArgs(prompt, options)...)
	cmd.Stdin = stdin
	return cmd, nil
}

// NewRunner returns a Pi sealed-run runner.
func (PiTool) NewRunner() AgentRunner {
	path, err := resolveCommand("pi")
	return &PiCLI{Path: path, ResolveErr: err}
}

// PiCLI invokes the real pi executable.
type PiCLI struct {
	Path       string
	ResolveErr error
	Progress   io.Writer
}

// SetProgress redirects concise process progress for callers that own the UI.
func (p *PiCLI) SetProgress(progress io.Writer) {
	p.Progress = progress
}

// Run executes pi in JSON mode, extracts session metadata, and waits for process exit.
func (p PiCLI) Run(ctx context.Context, repo, prompt, sessionID string, options StageOptions) (string, error) {
	if p.ResolveErr != nil {
		return "", p.ResolveErr
	}
	if p.Path == "" {
		return "", fmt.Errorf("找不到 pi 可执行文件")
	}
	args := piRunArgs(prompt, sessionID, options)
	cmd := commandContext(ctx, p.Path, args...)
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
	printAgentProcessStarted(p.Progress, "pi", cmd.Process.Pid)
	observed, drainErr := drainPiJSONL(stdout, p.Progress)
	waitErr := cmd.Wait()
	printAgentProcessExited(p.Progress, "pi", cmd.Process.Pid, cmd.ProcessState.ExitCode())
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

// piPlanningArgs builds interactive planning arguments using Pi option names.
func piPlanningArgs(prompt string, options StageOptions) []string {
	args := piCommonArgs(options)
	return append(args, prompt)
}

// piRunArgs builds shell-free sealed-run arguments for Pi JSON mode.
func piRunArgs(prompt, sessionID string, options StageOptions) []string {
	args := []string{"--mode", "json"}
	args = append(args, piCommonArgs(options)...)
	if sessionID != "" {
		args = append(args, "--session", sessionID)
	}
	return append(args, prompt)
}

// piCommonArgs maps shared stage options to Pi-supported CLI flags.
func piCommonArgs(options StageOptions) []string {
	var args []string
	if options.Model != "" {
		args = append(args, "--model", options.Model)
	}
	if options.Reasoning != "" {
		args = append(args, "--thinking", options.Reasoning)
	}
	return args
}

// piSessionEvent is the Pi JSON session header needed for workflow control.
type piSessionEvent struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

// drainPiJSONL reads stdout while best-effort extracting Pi session metadata.
func drainPiJSONL(stdout io.Reader, progress io.Writer) (sessionID string, err error) {
	reader := bufio.NewReaderSize(stdout, 64*1024)
	for {
		line, readErr := reader.ReadBytes('\n')
		if len(line) > 0 {
			if id := piSessionIDFromLine(line, progress); id != "" {
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

// piSessionIDFromLine parses Pi session header events without depending on other event shapes.
func piSessionIDFromLine(line []byte, progress io.Writer) string {
	var event piSessionEvent
	if err := json.Unmarshal(line, &event); err != nil {
		return ""
	}
	if strings.Contains(event.Type, "error") || strings.Contains(event.Type, "failed") {
		printAgentSessionFailed(progress, "pi")
	}
	if event.Type != "session" || event.ID == "" {
		return ""
	}
	printAgentSessionStarted(progress, "pi", event.ID)
	return event.ID
}
