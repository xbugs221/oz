// Package app tests Codex CLI process wrapping and diagnostic capture.
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

// TestCodexCLICapturesStderr verifies Codex diagnostics do not need terminal passthrough.
func TestCodexCLICapturesStderr(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script fixture is Unix-only")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "codex-fixture")
	body := "#!/bin/sh\nprintf '%s\\n' '{\"type\":\"thread.started\",\"thread_id\":\"t-1\"}'\nprintf '%s\\n' 'codex internal error line' >&2\nexit 7\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	_, err := (CodexCLI{Path: script}).Run(context.Background(), dir, "prompt", "", StageOptions{})
	if err == nil {
		t.Fatal("expected codex failure")
	}
	if !strings.Contains(err.Error(), "stderr：codex internal error line") {
		t.Fatalf("error does not include bounded stderr: %v", err)
	}
}

// TestDrainCodexJSONLMirrorsSessionProgress verifies only session-level events reach the terminal.
func TestDrainCodexJSONLMirrorsSessionProgress(t *testing.T) {
	input := strings.Join([]string{
		`{"type":"thread.started","thread_id":"t-1"}`,
		`{"type":"turn.started"}`,
		`{"type":"item.started","item":{"type":"command_execution","command":"go test ./..."}}`,
		`{"type":"item.completed","item":{"type":"command_execution","command":"go test ./...","exit_code":0}}`,
		`{"type":"item.completed","item":{"type":"file_change","changes":[{"path":"cmd/main.go","kind":"modify"}]}}`,
		`{"type":"item.completed","item":{"type":"agent_message","text":"internal model narration"}}`,
		`{"type":"turn.completed"}`,
	}, "\n")
	var progress bytes.Buffer
	threadID, err := drainCodexJSONL(strings.NewReader(input), &progress)
	if err != nil {
		t.Fatal(err)
	}
	if threadID != "t-1" {
		t.Fatalf("thread id = %q, want t-1", threadID)
	}
	for _, want := range []string{
		"agent session started: tool=codex session=t-1",
	} {
		if !strings.Contains(progress.String(), want) {
			t.Fatalf("progress missing %q in:\n%s", want, progress.String())
		}
	}
	for _, forbidden := range []string{
		"codex command",
		"codex file change",
		"codex message",
		"codex turn",
		"internal model narration",
	} {
		if strings.Contains(progress.String(), forbidden) {
			t.Fatalf("progress leaked %q in:\n%s", forbidden, progress.String())
		}
	}
}

// TestDrainCodexJSONLAcceptsLargeEventLine verifies command output payloads do not stall stage completion.
func TestDrainCodexJSONLAcceptsLargeEventLine(t *testing.T) {
	largeOutput := strings.Repeat("x", 128*1024)
	input := strings.Join([]string{
		`{"type":"thread.started","thread_id":"t-1"}`,
		`{"type":"item.completed","item":{"type":"command_execution","aggregated_output":"` + largeOutput + `","exit_code":0}}`,
		`{"type":"turn.completed"}`,
	}, "\n")
	threadID, err := drainCodexJSONL(strings.NewReader(input), nil)
	if err != nil {
		t.Fatal(err)
	}
	if threadID != "t-1" {
		t.Fatalf("thread id = %q, want t-1", threadID)
	}
}

// TestCodexCLIDoesNotRequireTurnCompleted verifies process exit is the control signal.
func TestCodexCLIDoesNotRequireTurnCompleted(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script fixture is Unix-only")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "codex-fixture")
	body := "#!/bin/sh\nprintf '%s\\n' '{\"type\":\"thread.started\",\"thread_id\":\"t-1\"}'\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	threadID, err := (CodexCLI{Path: script}).Run(context.Background(), dir, "prompt", "", StageOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if threadID != "t-1" {
		t.Fatalf("thread id = %q, want t-1", threadID)
	}
}

// TestCodexCLIReportsProcessProgress verifies terminal output includes process boundaries.
func TestCodexCLIReportsProcessProgress(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script fixture is Unix-only")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "codex-fixture")
	body := "#!/bin/sh\nprintf '%s\\n' '{\"type\":\"thread.started\",\"thread_id\":\"t-1\"}'\nprintf '%s\\n' '{\"type\":\"turn.completed\"}'\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	var progress bytes.Buffer
	threadID, err := (CodexCLI{Path: script, Progress: &progress}).Run(context.Background(), dir, "prompt", "", StageOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if threadID != "t-1" {
		t.Fatalf("thread id = %q, want t-1", threadID)
	}
	for _, want := range []string{
		"agent process started: tool=codex pid=",
		"agent session started: tool=codex session=t-1",
		"agent process exited: tool=codex pid=",
		"exit=0",
	} {
		if !strings.Contains(progress.String(), want) {
			t.Fatalf("progress missing %q in:\n%s", want, progress.String())
		}
	}
}
