// Package app runs deterministic validation gates between agent stages.
package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	validationStatusPassed             = "passed"
	validationStatusFailed             = "failed"
	validationKindCommands             = "commands"
	validationKindArtifact             = "artifact"
	validationKindQAReadOnly           = "qa_read_only"
	validationKindChange               = "change"
	validationKindAcceptancePreflight  = "acceptance_preflight"
	artifactGateCommand                = "oz flow artifact gate"
	acceptancePreflightGateCommand     = "oz flow acceptance preflight"
	changeValidationCommandDescription = "oz validate"
)

// StageValidationState is persisted in state.json so failed validation reruns the same stage.
type StageValidationState struct {
	Attempts       int    `json:"attempts"`
	Kind           string `json:"kind,omitempty"`
	Status         string `json:"status"`
	LastArtifact   string `json:"last_artifact,omitempty"`
	LastError      string `json:"last_error,omitempty"`
	DiffHash       string `json:"diff_hash,omitempty"`
	CheckpointHash string `json:"checkpoint_hash,omitempty"`
}

// ValidationAttempt stores reproducible command output for one validation gate run.
type ValidationAttempt struct {
	Stage      string                    `json:"stage"`
	Kind       string                    `json:"kind,omitempty"`
	Attempt    int                       `json:"attempt"`
	Status     string                    `json:"status"`
	DiffHash   string                    `json:"diff_hash,omitempty"`
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
	stage, err := parseWorkflowStage(state.Stage)
	if err != nil {
		return false
	}
	if shouldRunChangeValidation(stage) {
		return true
	}
	return len(state.Workflow.Validation.Commands) > 0 && (stage.isKind(workflowStageRepair) || stage.isKind(workflowStageAudit) || stage.isKind(workflowStageTargetedRepair) || stage.isKind(workflowStageFix))
}

// shouldRunChangeValidation forces oz validate after stages that may rewrite the active change.
func shouldRunChangeValidation(stage workflowStage) bool {
	return stage.isKind(workflowStageExecution) || stage.isKind(workflowStageRepair) || stage.isKind(workflowStageAudit) || stage.isKind(workflowStageTargetedRepair) || stage.isKind(workflowStageFix)
}

// shouldForceStageRerun reports whether a failed validation gate must re-enter the same stage.
func shouldForceStageRerun(state State) bool {
	if usesQualityLoop(state.Workflow) && state.QualityLoop.ResumeRerunPending {
		return true
	}
	if usesQualityLoop(state.Workflow) && state.Stages[state.Stage] == "needs_more" {
		return true
	}
	if state.ArtifactGates != nil && state.ArtifactGates[state.Stage].Status == validationStatusFailed {
		return true
	}
	if state.AcceptanceRun != nil && state.AcceptanceRun[state.Stage].Status == validationStatusFailed {
		return true
	}
	if state.AcceptancePreflight.Status == validationStatusFailed {
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
	if current.Kind == validationKindQAReadOnly || current.Kind == validationKindArchiveReadOnly {
		return
	}
	current.Kind = validationKindArtifact
	current.Status = validationStatusPassed
	current.LastArtifact = ""
	current.LastError = ""
	state.ArtifactGates[state.Stage] = current
}

// clearAcceptanceRunFailure records that required_tests passed for the current stage.
func clearAcceptanceRunFailure(state *State) {
	if state.AcceptanceRun == nil {
		return
	}
	current := state.AcceptanceRun[state.Stage]
	if current.Kind == "" && current.Status == "" {
		return
	}
	current.Kind = acceptanceRunKind
	current.Status = validationStatusPassed
	current.LastError = ""
	state.AcceptanceRun[state.Stage] = current
}

// markStageCompleted records stage completion only after deterministic gates pass.
func markStageCompleted(state *State) {
	if state.Stages == nil {
		state.Stages = map[string]string{}
	}
	state.Stages[state.Stage] = "completed"
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
	current, err := reserveValidationAttempt(
		repo,
		state,
		state.ArtifactGates[state.Stage],
		validationKindArtifact,
		func(reserved StageValidationState) {
			state.ArtifactGates[state.Stage] = reserved
		},
	)
	if err != nil {
		return err
	}
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
	if usesQualityLoop(state.Workflow) {
		attempt.Kind = validationKindArtifact
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
	if recordQualityGateFailure(state, validationKindArtifact, current.LastError) {
		return nil
	}
	if isQualityLoopRepairStage(*state) {
		state.Stages[state.Stage] = "validation_failed"
		return nil
	}
	if current.Attempts >= state.Workflow.Validation.MaxAttemptsPerStage {
		state.Status = statusValidationBlocked
		state.Stage = statusValidationBlocked
		state.Error = current.LastError
		return nil
	}
	state.Stages[state.Stage] = "validation_failed"
	return nil
}

// reserveValidationAttempt durably claims a quality-loop namespace before work can emit evidence.
func reserveValidationAttempt(
	repo string,
	state *State,
	current StageValidationState,
	kind string,
	assign func(StageValidationState),
) (StageValidationState, error) {
	current.Attempts++
	if state == nil || !usesQualityLoop(state.Workflow) {
		return current, nil
	}
	current.Kind = kind
	current.Status = statusRunning
	current.LastError = ""
	assign(current)
	if err := saveState(repo, *state); err != nil {
		return StageValidationState{}, err
	}
	return current, nil
}

// validationArtifactPath returns kind-isolated paths while preserving legacy empty-kind names.
func validationArtifactPath(repo, runID, stage, kind string, attempt int) (string, string, error) {
	stagePart := strings.ReplaceAll(stage, "_", "-")
	name := fmt.Sprintf("validation-%s-%d.json", stagePart, attempt)
	if strings.TrimSpace(kind) != "" {
		kindPart := strings.ReplaceAll(strings.TrimSpace(kind), "_", "-")
		name = fmt.Sprintf("validation-%s-%s-%d.json", stagePart, kindPart, attempt)
	}
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

// runStageValidation executes mandatory change validation before user-configured commands.
func runStageValidation(ctx context.Context, repo, changeName, stage string, attempt int, config ValidationConfig) ValidationAttempt {
	// runStageValidation keeps oz validate failures in the normal same-stage retry path.
	result := ValidationAttempt{
		Stage:     stage,
		Attempt:   attempt,
		Status:    validationStatusPassed,
		StartedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	parsed, err := parseWorkflowStage(stage)
	if err == nil && shouldRunChangeValidation(parsed) {
		commandResult := runChangeValidationCommand(ctx, repo, changeName)
		result.Commands = append(result.Commands, commandResult)
		if commandResult.ExitCode != 0 {
			result.Status = validationStatusFailed
			result.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
			return result
		}
	}
	configured := runValidationCommands(ctx, repo, stage, attempt, config)
	result.Commands = append(result.Commands, configured.Commands...)
	if configured.Status == validationStatusFailed {
		result.Status = validationStatusFailed
	}
	result.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
	return result
}

// runChangeValidationCommand runs oz validate for the active change and captures its JSON diagnostics.
func runChangeValidationCommand(ctx context.Context, repo, changeName string) ValidationCommandResult {
	// runChangeValidationCommand uses the same oz command resolution as workflow startup validation.
	path, err := resolveCommand(ozCommand)
	label := changeValidationCommandDescription + " " + changeName + " --json"
	if err != nil {
		return ValidationCommandResult{Command: label, ExitCode: -1, Output: limitValidationOutput(err.Error())}
	}
	args := append([]string{}, ozCommandPrefix...)
	args = append(args, "validate", changeName, "--json")
	cmd := commandContext(ctx, path, args...)
	cmd.Dir = repo
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	runErr := cmd.Run()
	exitCode := commandExitCode(runErr)
	if exitCode == 0 {
		var response ozValidateResponse
		if err := json.Unmarshal(output.Bytes(), &response); err != nil {
			exitCode = -1
			output.WriteString("\n解析 oz validate JSON 失败：" + err.Error())
		} else if !response.Valid {
			exitCode = 1
		}
	}
	return ValidationCommandResult{
		Command:  label,
		ExitCode: exitCode,
		Output:   limitValidationOutput(output.String()),
	}
}

// validationAttemptKind classifies the persisted validation failure for retry prompts.
func validationAttemptKind(attempt ValidationAttempt) string {
	for _, command := range attempt.Commands {
		if command.ExitCode != 0 && strings.HasPrefix(command.Command, changeValidationCommandDescription+" ") {
			return validationKindChange
		}
	}
	return validationKindCommands
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

// writeValidationAttempt persists one redacted gate result and returns its accessible path.
func writeValidationAttempt(repo, runID string, attempt ValidationAttempt) (string, error) {
	abs, rel, err := validationArtifactPath(repo, runID, attempt.Stage, attempt.Kind, attempt.Attempt)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return "", err
	}
	safeAttempt := redactValidationAttempt(attempt)
	data, err := json.MarshalIndent(safeAttempt, "", "  ")
	if err != nil {
		return "", err
	}
	if err := atomicWriteFile(abs, append(data, '\n'), 0o644); err != nil {
		return "", err
	}
	return rel, nil
}

// verifyQualityValidationCheckpoint replays trust checks for one passed validation artifact.
func verifyQualityValidationCheckpoint(repo string, state State, stage string) (ValidationAttempt, error) {
	checkpoint, ok := state.Validation[stage]
	if !ok || checkpoint.Status != validationStatusPassed || checkpoint.Attempts < 1 ||
		strings.TrimSpace(checkpoint.Kind) == "" || strings.TrimSpace(checkpoint.LastArtifact) == "" {
		return ValidationAttempt{}, fmt.Errorf("阶段 %s 缺少最后通过的 validation checkpoint", stage)
	}
	expectedPath, _, err := validationArtifactPath(
		repo,
		state.RunID,
		stage,
		checkpoint.Kind,
		checkpoint.Attempts,
	)
	if err != nil {
		return ValidationAttempt{}, err
	}
	if filepath.Clean(checkpoint.LastArtifact) != filepath.Clean(expectedPath) {
		return ValidationAttempt{}, fmt.Errorf(
			"阶段 %s validation checkpoint 路径不一致：state=%s expected=%s",
			stage,
			checkpoint.LastArtifact,
			expectedPath,
		)
	}
	data, err := readQualityValidationArtifact(expectedPath)
	if err != nil {
		return ValidationAttempt{}, fmt.Errorf("读取 validation checkpoint 失败：%w", err)
	}
	var attempt ValidationAttempt
	if err := decodeStrictArtifactJSON(data, &attempt); err != nil {
		return ValidationAttempt{}, fmt.Errorf("解析 validation checkpoint 失败：%w", err)
	}
	switch {
	case attempt.Stage != stage || attempt.Kind != checkpoint.Kind || attempt.Attempt != checkpoint.Attempts:
		return ValidationAttempt{}, fmt.Errorf("阶段 %s validation stage/kind/attempt 绑定不一致", stage)
	case attempt.Status != validationStatusPassed:
		return ValidationAttempt{}, fmt.Errorf("阶段 %s validation checkpoint 不是通过结果", stage)
	case checkpoint.DiffHash == "" || attempt.DiffHash != checkpoint.DiffHash ||
		attempt.DiffHash != state.QualityLoop.DiffHash:
		return ValidationAttempt{}, fmt.Errorf("阶段 %s validation diff 绑定不一致", stage)
	case state.QualityLoop.ValidationHash == "" ||
		qualityValidationProgressHash(attempt) != state.QualityLoop.ValidationHash:
		return ValidationAttempt{}, fmt.Errorf("阶段 %s validation progress hash 不一致", stage)
	default:
		return attempt, nil
	}
}

// readQualityValidationArtifact reads a checkpoint only through an opened regular file.
func readQualityValidationArtifact(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("validation checkpoint 必须是普通文件：%s", path)
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !opened.Mode().IsRegular() {
		return nil, fmt.Errorf("validation checkpoint 打开后不是普通文件：%s", path)
	}
	if !os.SameFile(info, opened) {
		return nil, fmt.Errorf("validation checkpoint 检查期间被替换：%s", path)
	}
	return io.ReadAll(file)
}

// redactValidationAttempt removes environment marker values from every persisted command field.
func redactValidationAttempt(attempt ValidationAttempt) ValidationAttempt {
	safe := attempt
	safe.Commands = append([]ValidationCommandResult(nil), attempt.Commands...)
	for i := range safe.Commands {
		safe.Commands[i].Command = redactQualityEnvironmentMarkers(safe.Commands[i].Command)
		safe.Commands[i].Output = redactQualityEnvironmentMarkers(safe.Commands[i].Output)
	}
	return safe
}

// firstValidationError summarizes the failing command for state.json and progress output.
func firstValidationError(attempt ValidationAttempt) string {
	for _, command := range attempt.Commands {
		if command.ExitCode != 0 {
			return fmt.Sprintf("%s exited %d", redactQualityEnvironmentMarkers(command.Command), command.ExitCode)
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
	if gate, ok := state.AcceptanceRun[state.Stage]; ok && gate.Status == validationStatusFailed {
		current = gate
	}
	if state.AcceptancePreflight.Status == validationStatusFailed {
		current = StageValidationState{
			Kind:         validationKindAcceptancePreflight,
			Status:       state.AcceptancePreflight.Status,
			LastArtifact: state.AcceptancePreflight.LastArtifact,
			LastError:    state.AcceptancePreflight.LastError,
		}
	}
	if current.Status != validationStatusFailed || current.LastArtifact == "" {
		return ""
	}
	body := "# Validation gate failed\n\n" +
		"The previous attempt for this same stage failed deterministic validation. " +
		"Read the artifact below, fix every failing command, and do not stop at the first Playwright failure if the configured suite still fails.\n\n" +
		"- Artifact: `" + current.LastArtifact + "`\n"
	if current.Kind == validationKindArtifact {
		body = "# Stage artifact gate failed\n\n" +
			"The previous attempt for this same stage wrote an artifact that failed the deterministic artifact contract gate. " +
			"Read the artifact below and rewrite the required stage artifact at the output path from the original stage prompt.\n\n" +
			"- Artifact: `" + current.LastArtifact + "`\n"
	}
	if current.Kind == acceptanceRunKind {
		body = "# Acceptance run gate failed\n\n" +
			"The previous attempt for this same stage failed the active change required_tests contract. " +
			"Read the result below, fix every failing required test and missing evidence, then rerun the same stage.\n\n" +
			"- Artifact: `" + current.LastArtifact + "`\n"
	}
	if current.Kind == validationKindAcceptancePreflight {
		body = "# Acceptance preflight gate failed\n\n" +
			"The previous attempt for this same stage left the active change acceptance contract unable to prove its evidence producer chain. " +
			"Read the artifact below, fix acceptance.json or the bound tests, then rerun the same stage.\n\n" +
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
	return limitUTF8Text(output, max, "\n[validation output truncated]\n")
}

// limitValidationPromptExcerpt keeps retry prompts focused on the actionable failure.
func limitValidationPromptExcerpt(output string) string {
	const max = 12_000
	return limitUTF8Text(output, max, "\n[validation artifact excerpt truncated]\n")
}
