// Package app tests Agy CLI argument building and process diagnostics.
package app

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestAgyRunArgsMapSealedPromptConversationAndPermissions verifies Agy sealed-run syntax.
// Contract evidence: agy --print maps --conversation conv-123 and keeps prompt as one argv.
func TestAgyRunArgsMapSealedPromptConversationAndPermissions(t *testing.T) {
	args := agyRunArgs("prompt with spaces", "conv-123", StageOptions{Model: "agy-model", Permissions: "danger-full-access"})
	for _, pair := range [][2]string{{"--model", "agy-model"}, {"--conversation", "conv-123"}} {
		if !containsArgPair(args, pair[0], pair[1]) {
			t.Fatalf("args = %v, missing %s %s", args, pair[0], pair[1])
		}
	}
	if !containsArg(args, "--print") || !containsArg(args, "--dangerously-skip-permissions") {
		t.Fatalf("args = %v, missing --print or dangerous permissions flag", args)
	}
	if args[len(args)-1] != "prompt with spaces" {
		t.Fatalf("args = %v, want prompt as one final argv", args)
	}
}

// containsArg reports whether args contains one exact value.
func containsArg(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}

// containsArgPair reports whether args contains a flag immediately followed by its value.
func containsArgPair(args []string, key, value string) bool {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == key && args[i+1] == value {
			return true
		}
	}
	return false
}

// TestAgyPlanningArgsUsePromptInteractive verifies human planning uses Agy interactive mode.
func TestAgyPlanningArgsUsePromptInteractive(t *testing.T) {
	args := agyPlanningArgs("planning prompt", StageOptions{Model: "agy-model", Permissions: "sandbox"})
	if !containsArg(args, "--prompt-interactive") || !containsArgPair(args, "--model", "agy-model") || !containsArg(args, "--sandbox") {
		t.Fatalf("args = %v, missing planning flags", args)
	}
	if args[len(args)-1] != "planning prompt" {
		t.Fatalf("args = %v, want prompt as final arg", args)
	}
}

// TestAgyCLIPreservesConversationAndCapturesStderr verifies Agy has no JSONL session dependency.
func TestAgyCLIPreservesConversationAndCapturesStderr(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script fixture is Unix-only")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "agy-fixture")
	body := "#!/bin/sh\nprintf '%s\\n' 'plain agy output'\nprintf '%s\\n' 'agy error line' >&2\nexit 7\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	sessionID, err := (AgyCLI{Path: script}).Run(context.Background(), dir, "prompt", "conv-123", StageOptions{})
	if err == nil {
		t.Fatal("expected agy failure")
	}
	if sessionID != "conv-123" {
		t.Fatalf("session id = %q, want existing conversation id", sessionID)
	}
	if !strings.Contains(err.Error(), "stderr：agy error line") {
		t.Fatalf("error does not include bounded stderr: %v", err)
	}
}

// TestAgentRegistryIncludesAgyCandidate verifies Agy is registered as a real backend.
func TestAgentRegistryIncludesAgyCandidate(t *testing.T) {
	registry := NewAgentRegistry()
	if tool, err := registry.Tool("agy"); err != nil || tool.Name() != "agy" {
		t.Fatalf("agy tool = %#v, err = %v", tool, err)
	}
	if !validAgentTool("agy") || validAgentTool("pi-ai") {
		t.Fatal("validAgentTool should accept agy while rejecting unknown aliases")
	}
}

// TestAgyRoleSessionIDIsVisible verifies status rows can read agy:<role> sessions.
func TestAgyRoleSessionIDIsVisible(t *testing.T) {
	state := State{Sessions: map[string]string{sessionStateKey("agy", "executor"): "conv-123"}}
	if got := statusRoleSessionID(state, "executor"); got != "conv-123" {
		t.Fatalf("statusRoleSessionID = %q, want agy conversation id", got)
	}
}
