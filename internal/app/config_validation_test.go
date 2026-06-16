// Package app tests validation command parsing and execution behavior.
package app

import (
	"context"
	"os/exec"
	"strings"
	"testing"
)

// TestValidationCommandsAcceptShellShortcutAndArgv verifies shell shortcuts coexist with exact argv commands.
func TestValidationCommandsAcceptShellShortcutAndArgv(t *testing.T) {
	workflow, err := workflowConfigFromYAML([]byte(`
validation:
  limit: 2
  commands:
    - ./git-hooks/pre-commit
    - pnpm build
    - run: go test ./...
    - executable: go
      args:
        - test
        - ./internal/app
`), "validation.yaml", nil)
	if err != nil {
		t.Fatal(err)
	}

	commands := workflow.Validation.Commands
	if workflow.Validation.MaxAttemptsPerStage != 2 {
		t.Fatalf("limit = %d, want 2", workflow.Validation.MaxAttemptsPerStage)
	}
	if len(commands) != 4 {
		t.Fatalf("commands = %d, want 4", len(commands))
	}
	if commands[0].Run != "./git-hooks/pre-commit" || commands[1].Run != "pnpm build" || commands[2].Run != "go test ./..." {
		t.Fatalf("shell shortcut commands = %#v", commands[:3])
	}
	if commands[3].Executable != "go" || strings.Join(commands[3].Args, " ") != "test ./internal/app" {
		t.Fatalf("argv command = %#v", commands[3])
	}
}

// TestValidationShellShortcutRunsThroughBash proves shortcut commands use bash semantics.
func TestValidationShellShortcutRunsThroughBash(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash is not available")
	}

	attempt := runValidationCommands(context.Background(), t.TempDir(), "execution", 1, ValidationConfig{
		Commands: []ValidationCommand{{Run: `[[ -n "$BASH_VERSION" ]] && printf shell-ok`}},
	})
	if attempt.Status != validationStatusPassed {
		t.Fatalf("status = %q, want passed: %#v", attempt.Status, attempt.Commands)
	}
	if len(attempt.Commands) != 1 {
		t.Fatalf("commands = %d, want 1", len(attempt.Commands))
	}
	result := attempt.Commands[0]
	if result.Command != `[[ -n "$BASH_VERSION" ]] && printf shell-ok` {
		t.Fatalf("command label = %q", result.Command)
	}
	if result.Output != "shell-ok" {
		t.Fatalf("output = %q, want shell-ok", result.Output)
	}
}

// TestValidationCommandRejectsEmptyShellShortcut keeps empty shorthand commands from becoming no-op gates.
func TestValidationCommandRejectsEmptyShellShortcut(t *testing.T) {
	_, err := workflowConfigFromYAML([]byte(`
validation:
  commands:
    - " "
`), "bad-validation.yaml", nil)
	if err == nil {
		t.Fatal("empty validation command should fail")
	}
	if !strings.Contains(err.Error(), "validation command 字符串不能为空") {
		t.Fatalf("error = %q", err)
	}
}
