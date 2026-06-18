// Package app wraps Agy CLI execution for sealed workflow stages.
package app

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

// AgyTool adapts Agy CLI to the generic agent backend contract.
type AgyTool struct{}

// Name returns the config name for the Agy backend.
func (AgyTool) Name() string { return "agy" }

// Resolve verifies Agy is available on PATH.
func (AgyTool) Resolve() error {
	_, err := resolveCommand("agy")
	return err
}

// PlanningCommand starts Agy interactively with the rendered planning prompt.
func (AgyTool) PlanningCommand(ctx context.Context, _ string, prompt string, stdin io.Reader, options StageOptions) (*exec.Cmd, error) {
	path, err := resolveCommand("agy")
	if err != nil {
		return nil, err
	}
	cmd := commandContext(ctx, path, agyPlanningArgs(prompt, options)...)
	cmd.Stdin = stdin
	return cmd, nil
}

// NewRunner returns an Agy sealed-run runner.
func (AgyTool) NewRunner() AgentRunner {
	path, err := resolveCommand("agy")
	return &AgyCLI{Path: path, ResolveErr: err}
}

// AgyCLI invokes the real agy executable.
type AgyCLI struct {
	Path       string
	ResolveErr error
	Progress   io.Writer
}

// SetProgress redirects concise process progress for callers that own the UI.
func (a *AgyCLI) SetProgress(progress io.Writer) {
	a.Progress = progress
}

// Run executes agy once and preserves an existing conversation id when resuming.
func (a AgyCLI) Run(ctx context.Context, repo, prompt, conversationID string, options StageOptions) (string, error) {
	if a.ResolveErr != nil {
		return "", a.ResolveErr
	}
	if a.Path == "" {
		return "", fmt.Errorf("找不到 agy 可执行文件")
	}
	args := agyRunArgs(prompt, conversationID, options)
	cmd := commandContext(ctx, a.Path, args...)
	cmd.Dir = repo
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return "", err
	}
	printAgentProcessStarted(a.Progress, "agy", cmd.Process.Pid)
	waitErr := cmd.Wait()
	printAgentProcessExited(a.Progress, "agy", cmd.Process.Pid, cmd.ProcessState.ExitCode())
	if waitErr != nil {
		stderrText := strings.TrimSpace(limitAgentDiagnostics(stderr.String()))
		if stderrText == "" {
			return conversationID, waitErr
		}
		return conversationID, fmt.Errorf("%w；stderr：%s", waitErr, stderrText)
	}
	return conversationID, nil
}

// agyPlanningArgs builds interactive planning arguments using Agy option names.
func agyPlanningArgs(prompt string, options StageOptions) []string {
	args := []string{"--prompt-interactive"}
	args = append(args, agyCommonArgs(options)...)
	return append(args, agentPromptText(prompt))
}

// agyRunArgs builds shell-free sealed-run arguments for one Agy prompt.
func agyRunArgs(prompt, conversationID string, options StageOptions) []string {
	args := []string{"--print"}
	args = append(args, agyCommonArgs(options)...)
	if conversationID != "" {
		args = append(args, "--conversation", conversationID)
	}
	return append(args, agentPromptText(prompt))
}

// agyCommonArgs maps shared stage options to Agy-supported CLI flags.
func agyCommonArgs(options StageOptions) []string {
	var args []string
	if options.Model != "" {
		args = append(args, "--model", options.Model)
	}
	switch options.Permissions {
	case "danger-full-access":
		args = append(args, "--dangerously-skip-permissions")
	case "sandbox":
		args = append(args, "--sandbox")
	}
	return args
}
