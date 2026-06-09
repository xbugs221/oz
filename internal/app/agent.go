// Package app defines agent tool backends used by planning and sealed stages.
package app

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"sort"
	"strings"
)

// AgentRunner executes one agent turn and returns the observed session id.
type AgentRunner interface {
	Run(ctx context.Context, repo, prompt, sessionID string, options StageOptions) (string, error)
}

// AgentTool owns CLI-specific command construction and sealed-run execution.
type AgentTool interface {
	Name() string
	Resolve() error
	PlanningCommand(ctx context.Context, repo, prompt string, stdin io.Reader, options StageOptions) (*exec.Cmd, error)
	NewRunner() AgentRunner
}

// AgentRegistry maps configured tool names to concrete backends.
type AgentRegistry struct {
	tools map[string]AgentTool
}

// NewAgentRegistry returns the built-in agent tool registry.
func NewAgentRegistry() *AgentRegistry {
	registry := &AgentRegistry{tools: map[string]AgentTool{}}
	registry.Register(CodexTool{})
	registry.Register(OpenCodeTool{})
	registry.Register(PiTool{})
	return registry
}

// Register adds or replaces one named agent tool.
func (r *AgentRegistry) Register(tool AgentTool) {
	if r.tools == nil {
		r.tools = map[string]AgentTool{}
	}
	r.tools[tool.Name()] = tool
}

// Tool returns the backend for a configured stage option.
func (r *AgentRegistry) Tool(name string) (AgentTool, error) {
	if r == nil {
		r = NewAgentRegistry()
	}
	tool, ok := r.tools[name]
	if !ok {
		return nil, fmt.Errorf("未知 agent tool %q", name)
	}
	return tool, nil
}

// ResolveForWorkflow checks only tools used by sealed-run stages.
func (r *AgentRegistry) ResolveForWorkflow(config WorkflowConfig) error {
	normalizeWorkflowConfig(&config)
	for _, name := range requiredAgentTools(config) {
		tool, err := r.Tool(name)
		if err != nil {
			return err
		}
		if err := tool.Resolve(); err != nil {
			return err
		}
	}
	return nil
}

// validAgentTool reports whether a config value names a supported backend.
func validAgentTool(name string) bool {
	return name == "codex" || name == "opencode" || name == "pi"
}

// requiredAgentTools returns sorted unique backend names used by sealed-run stages.
func requiredAgentTools(config WorkflowConfig) []string {
	seen := map[string]bool{}
	for _, stage := range workflowStagesForConfig(config) {
		options, ok := config.Stages[stage]
		if !ok {
			continue
		}
		seen[options.Tool] = true
	}
	var names []string
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// limitAgentDiagnostics keeps process error messages useful without recreating log files.
func limitAgentDiagnostics(text string) string {
	const limit = 4096
	text = strings.TrimSpace(text)
	if len(text) <= limit {
		return text
	}
	return text[:limit] + "\n... truncated"
}

// printAgentSessionStarted reports a durable session id in the generic progress format.
func printAgentSessionStarted(progress io.Writer, tool, sessionID string) {
	if progress == nil || sessionID == "" {
		return
	}
	fmt.Fprintf(progress, "agent session started: tool=%s session=%s\n", tool, sessionID)
}

// printAgentSessionFailed reports a backend failure in the generic progress format.
func printAgentSessionFailed(progress io.Writer, tool string) {
	if progress == nil {
		return
	}
	fmt.Fprintf(progress, "agent session failed: tool=%s\n", tool)
}

// printAgentProcessStarted reports a spawned backend process boundary.
func printAgentProcessStarted(progress io.Writer, tool string, pid int) {
	if progress == nil {
		return
	}
	fmt.Fprintf(progress, "agent process started: tool=%s pid=%d\n", tool, pid)
}

// printAgentProcessExited reports a backend process exit boundary.
func printAgentProcessExited(progress io.Writer, tool string, pid, exitCode int) {
	if progress == nil {
		return
	}
	fmt.Fprintf(progress, "agent process exited: tool=%s pid=%d exit=%d\n", tool, pid, exitCode)
}
