// Package app tests OpenCode CLI argument building and JSONL session parsing.
package app

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestOpenCodePlanningArgsPassProjectPromptAndModel verifies TUI planning uses OpenCode syntax.
func TestOpenCodePlanningArgsPassProjectPromptAndModel(t *testing.T) {
	args := opencodePlanningArgs("/repo", "planning prompt", StageOptions{Model: "anthropic/claude"})
	if got := strings.Join(args, "\x00"); !strings.Contains(got, "/repo\x00-m\x00anthropic/claude\x00--prompt\x00planning prompt") {
		t.Fatalf("args = %v, want project, model, and prompt", args)
	}
}

// TestOpenCodeRunArgsSupportNewAndResumeSessions verifies sealed run arguments.
func TestOpenCodeRunArgsSupportNewAndResumeSessions(t *testing.T) {
	args := opencodeRunArgs("/repo", "prompt", "", StageOptions{Model: "anthropic/claude", Reasoning: "high", Fast: true})
	for _, want := range []string{"run", "--format", "json", "--dangerously-skip-permissions", "--dir", "/repo", "-m", "anthropic/claude", "--variant", "high", "prompt"} {
		if !containsArg(args, want) {
			t.Fatalf("args = %v, missing %q", args, want)
		}
	}
	if containsArg(args, "fast_mode") {
		t.Fatalf("args = %v, opencode must ignore fast mode", args)
	}

	resume := opencodeRunArgs("/repo", "prompt", "s-1", StageOptions{})
	if !containsArgPair(resume, "--session", "s-1") {
		t.Fatalf("args = %v, want session resume", resume)
	}
}

// TestDrainOpenCodeJSONLExtractsSessionID verifies raw events expose a resumable session id.
func TestDrainOpenCodeJSONLExtractsSessionID(t *testing.T) {
	input := strings.Join([]string{
		`{"type":"session.updated","sessionID":"s-1"}`,
		`{"type":"message","message":{"content":"done"}}`,
	}, "\n")
	var progress bytes.Buffer
	sessionID, err := drainOpenCodeJSONL(strings.NewReader(input), &progress)
	if err != nil {
		t.Fatal(err)
	}
	if sessionID != "s-1" {
		t.Fatalf("session id = %q, want s-1", sessionID)
	}
	if !strings.Contains(progress.String(), "agent session started: tool=opencode session=s-1") {
		t.Fatalf("progress = %q, want opencode session", progress.String())
	}
}

// TestDrainOpenCodeJSONLIgnoresMessageAndPartIDs verifies only session fields can update resume state.
func TestDrainOpenCodeJSONLIgnoresMessageAndPartIDs(t *testing.T) {
	input := strings.Join([]string{
		`{"type":"session.updated","sessionID":"s-1"}`,
		`{"type":"message","message":{"id":"msg-1","content":"done"}}`,
		`{"type":"part","part":{"id":"part-1"}}`,
	}, "\n")
	sessionID, err := drainOpenCodeJSONL(strings.NewReader(input), nil)
	if err != nil {
		t.Fatal(err)
	}
	if sessionID != "s-1" {
		t.Fatalf("session id = %q, want original session id", sessionID)
	}
}

// TestOpenCodeCLICapturesStderr verifies OpenCode diagnostics are captured separately.
func TestOpenCodeCLICapturesStderr(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script fixture is Unix-only")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "opencode-fixture")
	body := "#!/bin/sh\nprintf '%s\\n' '{\"sessionID\":\"s-1\"}'\nprintf '%s\\n' 'opencode error line' >&2\nexit 7\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := (OpenCodeCLI{Path: script}).Run(context.Background(), dir, "prompt", "", StageOptions{})
	if err == nil {
		t.Fatal("expected opencode failure")
	}
	if !strings.Contains(err.Error(), "stderr：opencode error line") {
		t.Fatalf("error does not include bounded stderr: %v", err)
	}
}
