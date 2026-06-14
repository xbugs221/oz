// Package app starts and records human planning sessions for wo.
package app

import (
	"context"
	"io"
	"os"
	"os/exec"
	"strings"
)

// runPlanning starts a normal agent TUI session for human planning.
func runPlanning(ctx context.Context, repo string) (string, string, error) {
	rendered, options, err := planningPrompt(repo)
	if err != nil {
		return "", "", err
	}
	registry := NewAgentRegistry()
	tool, err := registry.Tool(options.Tool)
	if err != nil {
		return "", "", err
	}
	if err := tool.Resolve(); err != nil {
		return "", "", err
	}
	cmd, err := tool.PlanningCommand(ctx, repo, rendered, os.Stdin, options)
	if err != nil {
		return "", "", err
	}
	sessionFile, err := os.CreateTemp("", "wo-planning-session-*")
	if err != nil {
		return "", "", err
	}
	sessionPath := sessionFile.Name()
	_ = sessionFile.Close()
	defer os.Remove(sessionPath)
	env := cmd.Env
	if env == nil {
		env = os.Environ()
	}
	cmd.Env = append(env, "WO_PLANNING_SESSION_FILE="+sessionPath)
	cmd.Dir = repo
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", "", err
	}
	return options.Tool, planningSessionIDFromFile(sessionPath), nil
}

// planningSessionIDFromFile reads the session id captured by a planning backend.
func planningSessionIDFromFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// planningPrompt renders the human planning prompt through the public template name.
func planningPrompt(repo string) (string, StageOptions, error) {
	workflow, err := LoadWorkflowConfig(repo)
	if err != nil {
		return "", StageOptions{}, err
	}
	name, err := promptNameForStage("planning")
	if err != nil {
		return "", StageOptions{}, err
	}
	prompt, err := promptForName(workflow, name)
	if err != nil {
		return "", StageOptions{}, err
	}
	context, err := promptContext(repo, State{RunID: "planning", Stage: "planning", Workflow: workflow})
	if err != nil {
		return "", StageOptions{}, err
	}
	rendered, err := renderPromptTemplate(name, prompt, context)
	if err != nil {
		return "", StageOptions{}, err
	}
	options, err := workflow.StageOption("planning")
	if err != nil {
		return "", StageOptions{}, err
	}
	return rendered, options, nil
}

// codexPlanningCommand keeps human planning interactive while passing the seed prompt directly.
func codexPlanningCommand(ctx context.Context, path, prompt string, stdin io.Reader, options StageOptions) *exec.Cmd {
	args := []string{}
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
	args = append(args, prompt)
	cmd := commandContext(ctx, path, args...)
	cmd.Stdin = stdin
	return cmd
}
