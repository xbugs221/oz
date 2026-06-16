// Package app runs deterministic validation gates between agent stages.
package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	validationStatusPassed = "passed"
	validationStatusFailed = "failed"
	validationKindCommands = "commands"
	validationKindArtifact = "artifact"
	artifactGateCommand    = "oz flow artifact gate"
)

// StageValidationState is persisted in state.json so failed validation reruns the same stage.
type StageValidationState struct {
	Attempts     int    `json:"attempts"`
	Kind         string `json:"kind,omitempty"`
	Status       string `json:"status"`
	LastArtifact string `json:"last_artifact,omitempty"`
	LastError    string `json:"last_error,omitempty"`
}

// ValidationAttempt stores reproducible command output for one validation gate run.
type ValidationAttempt struct {
	Stage      string                    `json:"stage"`
	Attempt    int                       `json:"attempt"`
	Status     string                    `json:"status"`
	StartedAt  string                    `json:"started_at"`
	FinishedAt string                    `json:"finished_at"`
	Commands   []ValidationCommandResult `json:"commands"`
}

// ValidationCommandResult records one configured validation command.
type ValidationCommandResult struct {
	Command  string `json:"command"`
	ExitCode int    `json:"exit_code"`
	Output   string `json:"output"`
}

// shouldValidateStage limits validation to stages that can change implementation files.
func shouldValidateStage(state State) bool {
	if len(state.Workflow.Validation.Commands) == 0 {
		return false
	}
	return state.Stage == "execution" || strings.HasPrefix(state.Stage, "fix_")
}

// shouldForceStageRerun reports whether a failed validation gate must re-enter the same stage.
func shouldForceStageRerun(state State) bool {
	if state.ArtifactGates != nil && state.ArtifactGates[state.Stage].Status == validationStatusFailed {
		return true
	}
	return state.Validation != nil && state.Validation[state.Stage].Status == validationStatusFailed
}

// clearStageValidationFailure records that the current stage has passed its latest gate.
func clearStageValidationFailure(state *State) {
	if state.Validation == nil {
		return
	}
	current := state.Validation[state.Stage]
	current.Status = validationStatusPassed
	current.LastError = ""
	state.Validation[state.Stage] = current
}

// clearStageArtifactGateFailure records that the current stage artifact now passes its gate.
func clearStageArtifactGateFailure(state *State) {
	if state.ArtifactGates == nil {
		return
	}
	current := state.ArtifactGates[state.Stage]
	if current.Kind == "" && current.Status == "" {
		return
	}
	current.Kind = validationKindArtifact
	current.Status = validationStatusPassed
	current.LastError = ""
	state.ArtifactGates[state.Stage] = current
}

type stageArtifactGateError struct {
	Reason string
	Cause  error
}

// Error returns the deterministic artifact gate failure reason.
func (e stageArtifactGateError) Error() string {
	return e.Reason
}

// Unwrap exposes the original artifact validation error for errors.Is/As callers.
func (e stageArtifactGateError) Unwrap() error {
	return e.Cause
}

// isStageArtifactGateError reports whether a stage may rerun to rewrite its artifact.
func isStageArtifactGateError(err error) bool {
	var gateErr stageArtifactGateError
	return errors.As(err, &gateErr)
}

// recordStageArtifactGateFailure persists a schema/contract failure for a same-stage retry.
func recordStageArtifactGateFailure(repo string, state *State, failure error) error {
	ensureWorkflowConfig(state)
	if state.ArtifactGates == nil {
		state.ArtifactGates = map[string]StageValidationState{}
	}
	if state.Stages == nil {
		state.Stages = map[string]string{}
	}
	current := state.ArtifactGates[state.Stage]
	current.Attempts++
	now := time.Now().UTC().Format(time.RFC3339Nano)
	attempt := ValidationAttempt{
		Stage:      state.Stage,
		Attempt:    current.Attempts,
		Status:     validationStatusFailed,
		StartedAt:  now,
		FinishedAt: now,
		Commands: []ValidationCommandResult{{
			Command:  artifactGateCommand,
			ExitCode: 1,
			Output:   limitValidationOutput(failure.Error()),
		}},
	}
	artifactPath, err := writeValidationAttempt(repo, state.RunID, attempt)
	if err != nil {
		return err
	}
	current.Kind = validationKindArtifact
	current.LastArtifact = artifactPath
	current.Status = validationStatusFailed
	current.LastError = failure.Error()
	state.ArtifactGates[state.Stage] = current
	if current.Attempts >= state.Workflow.Validation.MaxAttemptsPerStage {
		state.Status = statusValidationBlocked
		state.Stage = statusValidationBlocked
		state.Error = current.LastError
		return nil
	}
	state.Stages[state.Stage] = "validation_failed"
	return nil
}

// validationArtifactPath returns directly accessible paths for one validation artifact.
func validationArtifactPath(repo, runID, stage string, attempt int) (string, string, error) {
	name := fmt.Sprintf("validation-%s-%d.json", strings.ReplaceAll(stage, "_", "-"), attempt)
	abs := filepath.Join(runDir(repo, runID), name)
	return abs, abs, nil
}

// runValidationCommands executes configured argv commands and stops at the first failure.
func runValidationCommands(ctx context.Context, repo, stage string, attempt int, config ValidationConfig) ValidationAttempt {
	result := ValidationAttempt{
		Stage:     stage,
		Attempt:   attempt,
		Status:    validationStatusPassed,
		StartedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	for _, command := range config.Commands {
		cmd := validationExecCommand(ctx, command)
		cmd.Dir = repo
		var output bytes.Buffer
		cmd.Stdout = &output
		cmd.Stderr = &output
		err := cmd.Run()
		commandResult := ValidationCommandResult{
			Command:  validationCommandLabel(command),
			ExitCode: commandExitCode(err),
			Output:   limitValidationOutput(output.String()),
		}
		result.Commands = append(result.Commands, commandResult)
		if err != nil {
			result.Status = validationStatusFailed
			break
		}
	}
	result.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
	return result
}

// validationExecCommand builds the OS process for one configured validation command.
func validationExecCommand(ctx context.Context, command ValidationCommand) *exec.Cmd {
	if strings.TrimSpace(command.Run) != "" {
		return exec.CommandContext(ctx, "bash", "-lc", command.Run)
	}
	return exec.CommandContext(ctx, command.Executable, command.Args...)
}

// validationCommandLabel renders the user-facing command used in diagnostics.
func validationCommandLabel(command ValidationCommand) string {
	if strings.TrimSpace(command.Run) != "" {
		return command.Run
	}
	return strings.Join(append([]string{command.Executable}, command.Args...), " ")
}

// writeValidationAttempt persists one gate result and returns its accessible path.
func writeValidationAttempt(repo, runID string, attempt ValidationAttempt) (string, error) {
	abs, rel, err := validationArtifactPath(repo, runID, attempt.Stage, attempt.Attempt)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return "", err
	}
	data, err := json.MarshalIndent(attempt, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(abs, append(data, '\n'), 0o644); err != nil {
		return "", err
	}
	return rel, nil
}

// firstValidationError summarizes the failing command for state.json and progress output.
func firstValidationError(attempt ValidationAttempt) string {
	for _, command := range attempt.Commands {
		if command.ExitCode != 0 {
			return fmt.Sprintf("%s exited %d", command.Command, command.ExitCode)
		}
	}
	return ""
}

// validationFailurePrompt injects the previous failed gate into the next executor turn.
func validationFailurePrompt(repo string, state State) string {
	current := state.Validation[state.Stage]
	if gate, ok := state.ArtifactGates[state.Stage]; ok && gate.Status == validationStatusFailed {
		current = gate
	}
	if current.Status != validationStatusFailed || current.LastArtifact == "" {
		return ""
	}
	body := "\n\n# Validation gate failed\n\n" +
		"The previous attempt for this same stage failed deterministic validation. " +
		"Read the artifact below, fix every failing command, and do not stop at the first Playwright failure if the configured suite still fails.\n\n" +
		"- Artifact: `" + current.LastArtifact + "`\n"
	if current.Kind == validationKindArtifact {
		body = "\n\n# Stage artifact gate failed\n\n" +
			"The previous attempt for this same stage wrote an artifact that failed the deterministic artifact contract gate. " +
			"Read the artifact below and rewrite the required stage artifact at the output path from the original stage prompt.\n\n" +
			"- Artifact: `" + current.LastArtifact + "`\n"
	}
	if current.LastError != "" {
		body += "- Error: " + current.LastError + "\n"
	}
	artifactPath := current.LastArtifact
	if !filepath.IsAbs(artifactPath) {
		artifactPath = repoAbsPath(repo, artifactPath)
	}
	if data, err := os.ReadFile(artifactPath); err == nil {
		excerpt := strings.TrimSpace(limitValidationPromptExcerpt(string(data)))
		if excerpt != "" {
			body += "\n```json\n" + excerpt + "\n```\n"
		}
	}
	return body
}

// commandExitCode normalizes process and launch errors into JSON-friendly exit codes.
func commandExitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}

// limitValidationOutput keeps validation artifacts useful without unbounded log growth.
func limitValidationOutput(output string) string {
	const max = 200_000
	if len(output) <= max {
		return output
	}
	return output[:max] + "\n[validation output truncated]\n"
}

// limitValidationPromptExcerpt keeps retry prompts focused on the actionable failure.
func limitValidationPromptExcerpt(output string) string {
	const max = 12_000
	if len(output) <= max {
		return output
	}
	return output[:max] + "\n[validation artifact excerpt truncated]\n"
}
