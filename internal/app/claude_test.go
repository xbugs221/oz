// Package app tests Claude CLI argument building and stream-json session drain.
package app

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestClaudeRunArgsMapSealedStreamJSONAndModel verifies Claude sealed-run syntax.
// Contract evidence: claude -p --verbose --output-format stream-json with model and effort pairs,
// and prompt kept as the final positional argv.
func TestClaudeRunArgsMapSealedStreamJSONAndModel(t *testing.T) {
	args := claudeRunArgs("/tmp/repo", "prompt with spaces", "", StageOptions{Model: "claude-sonnet-5", Reasoning: "high"})
	for _, want := range []string{"-p", "--verbose", "--output-format", "stream-json"} {
		if !containsArg(args, want) {
			t.Fatalf("args = %v, missing %s", args, want)
		}
	}
	if !containsArgPair(args, "--model", "claude-sonnet-5") {
		t.Fatalf("args = %v, missing --model claude-sonnet-5", args)
	}
	if !containsArgPair(args, "--effort", "high") {
		t.Fatalf("args = %v, missing --effort high", args)
	}
	if args[len(args)-1] != "prompt with spaces" {
		t.Fatalf("args = %v, want prompt as final argv", args)
	}
}

// TestClaudeRunArgsMapDangerPermissionsBeforeModel keeps danger-full-access non-interactive.
// Contract evidence: danger-full-access maps to --dangerously-skip-permissions.
func TestClaudeRunArgsMapDangerPermissionsBeforeModel(t *testing.T) {
	args := claudeRunArgs("/tmp/repo", "prompt", "", StageOptions{Permissions: "danger-full-access"})
	if !containsArg(args, "--dangerously-skip-permissions") {
		t.Fatalf("args = %v, missing --dangerously-skip-permissions", args)
	}
}

// TestClaudeRunArgsDefaultPermissionsBypassApproval proves the default (empty) permission
// mode still bypasses approvals, because Claude -p print mode has no interactive approval
// channel and sealed stages must read run-dir state and skill files outside the repo.
// Contract evidence: default/empty maps to --dangerously-skip-permissions (mirrors codex exec).
func TestClaudeRunArgsDefaultPermissionsBypassApproval(t *testing.T) {
	for _, perm := range []string{"", "default"} {
		args := claudeRunArgs("/tmp/repo", "prompt", "", StageOptions{Permissions: perm})
		if !containsArg(args, "--dangerously-skip-permissions") {
			t.Fatalf("permissions=%q args = %v, missing --dangerously-skip-permissions", perm, args)
		}
	}
}

// TestClaudeRunArgsMapSandboxToReadOnlyTools keeps sandbox tool calls read-only.
// Contract evidence: sandbox maps to --allowedTools Read,Grep,Glob.
func TestClaudeRunArgsMapSandboxToReadOnlyTools(t *testing.T) {
	args := claudeRunArgs("/tmp/repo", "prompt", "", StageOptions{Permissions: "sandbox"})
	if !containsArgPair(args, "--allowedTools", "Read,Grep,Glob") {
		t.Fatalf("args = %v, missing --allowedTools Read,Grep,Glob", args)
	}
}

// TestClaudeRunArgsResumeSession verifies an existing session id is resumed.
// Contract evidence: sessionID maps to --resume <sid>.
func TestClaudeRunArgsResumeSession(t *testing.T) {
	args := claudeRunArgs("/tmp/repo", "prompt", "sess-9", StageOptions{})
	if !containsArgPair(args, "--resume", "sess-9") {
		t.Fatalf("args = %v, missing --resume sess-9", args)
	}
}

// TestClaudePlanningArgsInteractive verifies human planning uses Claude interactive mode.
// Contract evidence: planning omits -p/--print, keeps model, and prompt is the final argv.
func TestClaudePlanningArgsInteractive(t *testing.T) {
	args := claudePlanningArgs("planning prompt", StageOptions{Model: "claude-sonnet-5"})
	if containsArg(args, "-p") || containsArg(args, "--print") {
		t.Fatalf("args = %v, planning must not include -p/--print", args)
	}
	if !containsArgPair(args, "--model", "claude-sonnet-5") {
		t.Fatalf("args = %v, missing --model claude-sonnet-5", args)
	}
	if args[len(args)-1] != "planning prompt" {
		t.Fatalf("args = %v, want prompt as final argv", args)
	}
}

// TestClaudeSessionIDFromInitEvent verifies the init stream-json event yields the session id.
// Contract evidence: {"type":"system","subtype":"init","session_id":"claude-sess-1"}.
func TestClaudeSessionIDFromInitEvent(t *testing.T) {
	line := []byte(`{"type":"system","subtype":"init","session_id":"claude-sess-1"}`)
	if got := claudeSessionIDFromLine(line, nil); got != "claude-sess-1" {
		t.Fatalf("session id = %q, want claude-sess-1", got)
	}
}

// TestClaudeSessionIDFromResultEventFallback verifies the result event is a fallback session source.
// Contract evidence: {"type":"result","session_id":"claude-sess-1"}.
func TestClaudeSessionIDFromResultEventFallback(t *testing.T) {
	line := []byte(`{"type":"result","session_id":"claude-sess-1"}`)
	if got := claudeSessionIDFromLine(line, nil); got != "claude-sess-1" {
		t.Fatalf("session id = %q, want claude-sess-1", got)
	}
}

// TestClaudeCLIDrainsSessionIDFromFixture verifies Run drains stream-json stdout to recover the session id.
// Contract evidence: cmd.Dir = repo, stdout pipe parsed line-by-line for init/result events.
func TestClaudeCLIDrainsSessionIDFromFixture(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script fixture is Unix-only")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "claude-fixture")
	body := "#!/bin/sh\n" +
		`printf '%s\n' '{"type":"system","subtype":"init","session_id":"claude-sess-1"}'` + "\n" +
		`printf '%s\n' '{"type":"result","session_id":"claude-sess-1"}'` + "\n" +
		"exit 0\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	sessionID, err := (ClaudeCLI{Path: script}).Run(context.Background(), dir, "prompt", "", StageOptions{})
	if err != nil {
		t.Fatalf("Run error = %v", err)
	}
	if sessionID != "claude-sess-1" {
		t.Fatalf("session id = %q, want claude-sess-1", sessionID)
	}
}

// TestAgentRegistryIncludesClaudeCandidate verifies Claude is registered as a real backend.
func TestAgentRegistryIncludesClaudeCandidate(t *testing.T) {
	registry := NewAgentRegistry()
	if tool, err := registry.Tool("claude"); err != nil || tool.Name() != "claude" {
		t.Fatalf("claude tool = %#v, err = %v", tool, err)
	}
	if !validAgentTool("claude") || validAgentTool("claude-code") {
		t.Fatal("validAgentTool should accept claude while rejecting unknown aliases")
	}
}

// TestClaudeRoleSessionIDIsVisible verifies status rows can read claude:<role> sessions.
func TestClaudeRoleSessionIDIsVisible(t *testing.T) {
	state := State{Sessions: map[string]string{sessionStateKey("claude", "executor"): "sess-7"}}
	if got := statusRoleSessionID(state, "executor"); got != "sess-7" {
		t.Fatalf("statusRoleSessionID = %q, want sess-7", got)
	}
}
