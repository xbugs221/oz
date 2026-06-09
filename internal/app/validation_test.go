// Package app tests deterministic validation gates that protect workflow stage transitions.
package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestValidationAttemptStopsAtFirstFailureAndTruncatesOutput verifies artifacts preserve the actionable failure.
func TestValidationAttemptStopsAtFirstFailureAndTruncatesOutput(t *testing.T) {
	repo := gitRepo(t)
	attempt := runValidationCommands(context.Background(), repo, "execution", 1, []string{
		"yes x | head -c 200010; exit 7",
		"touch should-not-run",
	})
	if attempt.Status != validationStatusFailed || len(attempt.Commands) != 1 {
		t.Fatalf("attempt = %#v, want first command failure only", attempt)
	}
	if attempt.Commands[0].ExitCode != 7 {
		t.Fatalf("exit = %d, want 7", attempt.Commands[0].ExitCode)
	}
	if !strings.Contains(attempt.Commands[0].Output, "[validation output truncated]") {
		t.Fatal("failed command output should include truncation marker")
	}
	if fileExists(filepath.Join(repo, "should-not-run")) {
		t.Fatal("validation continued after the first failing command")
	}
}

// TestValidationArtifactAndPromptExposeFailure verifies retry prompts include the persisted diagnostic artifact.
func TestValidationArtifactAndPromptExposeFailure(t *testing.T) {
	repo := gitRepo(t)
	mustChange(t, repo, "demo")
	mustPrompts(t, repo)
	state := State{RunID: "validation-run", ChangeName: "demo", Sealed: true, Status: statusRunning, Stage: "execution", Workflow: DefaultWorkflowConfig()}
	if err := saveState(repo, state); err != nil {
		t.Fatal(err)
	}
	attempt := ValidationAttempt{
		Stage:   "execution",
		Attempt: 1,
		Status:  validationStatusFailed,
		Commands: []ValidationCommandResult{{
			Command:  "false",
			ExitCode: 1,
			Output:   "boom\n",
		}},
	}
	artifact, err := writeValidationAttempt(repo, state.RunID, attempt)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(artifact)
	if err != nil {
		t.Fatal(err)
	}
	var decoded ValidationAttempt
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Stage != "execution" || decoded.Attempt != 1 || decoded.Status != validationStatusFailed || decoded.Commands[0].Command != "false" || decoded.Commands[0].ExitCode != 1 {
		t.Fatalf("artifact = %#v", decoded)
	}
	state.Validation = map[string]StageValidationState{"execution": {
		Attempts:     1,
		Status:       validationStatusFailed,
		LastArtifact: artifact,
		LastError:    firstValidationError(attempt),
	}}
	prompt := validationFailurePrompt(repo, state)
	for _, want := range []string{"Validation gate failed", artifact, "false exited 1", `"command": "false"`, `"output": "boom\n"`} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
}
