// 文件功能目的：验证主阶段 prompt 在重试场景中的构建策略。
package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

// TestPromptForStageReturnsFailureBodyAfterValidationFail ensures retry 提示不再重复发送原始 prompt。
func TestPromptForStageReturnsFailureBodyAfterValidationFail(t *testing.T) {
	repo := t.TempDir()
	state := State{
		ChangeName: "demo",
		Stage:      "execution",
		Workflow:   DefaultWorkflowConfig(),
		Validation: map[string]StageValidationState{
			"execution": {
				Status:       validationStatusFailed,
				Kind:         validationKindCommands,
				Attempts:     1,
				LastArtifact: "validation-execution-1.json",
				LastError:    "go test ./... exited 1",
			},
		},
	}
	state.Workflow.Prompts["execution"] = "original execution prompt should not be repeated"
	attemptPath := filepath.Join(repo, state.Validation["execution"].LastArtifact)
	if err := os.WriteFile(attemptPath, mustMarshalValidationAttemptJSON(t, ValidationAttempt{
		Stage:      "execution",
		Attempt:    1,
		Status:     validationStatusFailed,
		StartedAt:  "2026-01-01T00:00:00Z",
		FinishedAt: "2026-01-01T00:00:01Z",
		Commands: []ValidationCommandResult{{
			Command:  "go test ./...",
			ExitCode: 1,
			Output:   "validation failed",
		}},
	}), 0o644); err != nil {
		t.Fatal(err)
	}

	prompt, err := promptForStage(repo, state)
	if err != nil {
		t.Fatal(err)
	}
	if strings.HasPrefix(prompt, "original execution prompt") {
		t.Fatalf("prompt starts with original body: %s", prompt)
	}
	if !strings.HasPrefix(prompt, "# Validation gate failed") {
		t.Fatalf("prompt should start from failure body, got: %s", prompt)
	}
}

// TestPromptForStageValidationFailurePromptPreservesUTF8 verifies retry prompts survive multibyte truncation.
func TestPromptForStageValidationFailurePromptPreservesUTF8(t *testing.T) {
	repo := t.TempDir()
	state := State{
		ChangeName: "demo",
		Stage:      "execution",
		Workflow:   DefaultWorkflowConfig(),
		Validation: map[string]StageValidationState{
			"execution": {
				Status:       validationStatusFailed,
				Kind:         validationKindCommands,
				Attempts:     1,
				LastArtifact: "validation-execution-1.json",
				LastError:    "validation artifact includes multibyte output",
			},
		},
	}
	state.Workflow.Prompts["execution"] = "original execution prompt should not be repeated"
	artifactPath := filepath.Join(repo, state.Validation["execution"].LastArtifact)
	artifactBody := strings.Repeat("a", 11997) + "🙂tail"
	if err := os.WriteFile(artifactPath, []byte(artifactBody), 0o644); err != nil {
		t.Fatal(err)
	}

	prompt, err := promptForStage(repo, state)
	if err != nil {
		t.Fatal(err)
	}
	if !utf8.ValidString(prompt) {
		t.Fatalf("retry prompt is not valid UTF-8 near boundary: %q", prompt[len(prompt)-80:])
	}
	if !strings.Contains(prompt, "[validation artifact excerpt truncated]") {
		t.Fatalf("retry prompt should include truncation marker, got suffix: %q", prompt[len(prompt)-120:])
	}
}

// TestPromptForStageReturnsArtifactFailureBodyAfterFail validates artifact gate 重试提示也不重复发送原始 prompt。
func TestPromptForStageReturnsArtifactFailureBodyAfterFail(t *testing.T) {
	repo := t.TempDir()
	state := State{
		ChangeName: "demo",
		Stage:      "execution",
		Workflow:   DefaultWorkflowConfig(),
		ArtifactGates: map[string]StageValidationState{
			"execution": {
				Status:       validationStatusFailed,
				Kind:         validationKindArtifact,
				Attempts:     1,
				LastArtifact: "validation-execution-1.json",
				LastError:    "artifact schema error",
			},
		},
	}
	state.Workflow.Prompts["execution"] = "original execution prompt should not be repeated"

	prompt, err := promptForStage(repo, state)
	if err != nil {
		t.Fatal(err)
	}
	if strings.HasPrefix(prompt, "original execution prompt") {
		t.Fatalf("prompt starts with original body: %s", prompt)
	}
	if !strings.HasPrefix(prompt, "# Stage artifact gate failed") {
		t.Fatalf("prompt should start from artifact failure body, got: %s", prompt)
	}
}

// TestPromptForStageReturnsBasePromptWithoutFailure validates base prompt remains unchanged on first turn.
func TestPromptForStageReturnsBasePromptWithoutFailure(t *testing.T) {
	state := State{
		ChangeName: "demo",
		Stage:      "execution",
		Workflow:   DefaultWorkflowConfig(),
	}
	state.Workflow.Prompts["execution"] = "base execution prompt"

	prompt, err := promptForStage(t.TempDir(), state)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, "base execution prompt") {
		t.Fatalf("prompt should keep base template when no failure: %s", prompt)
	}
}

func mustMarshalValidationAttemptJSON(t *testing.T, attempt ValidationAttempt) []byte {
	t.Helper()
	data, err := json.MarshalIndent(attempt, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return append(data, '\n')
}
