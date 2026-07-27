// Package app contains workflow engine state and execution boundaries.
package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var (
	qualityGoTestCaseDuration = regexp.MustCompile(`(?m)(--- (?:PASS|FAIL|SKIP): .+?) \(\d+(?:\.\d+)?s\)$`)
	qualityGoPackageDuration  = regexp.MustCompile(`(?m)^((?:ok|FAIL|\?)\s+\S+)\s+(?:\d+(?:\.\d+)?s|\(cached\))$`)
	qualityRFC3339Timestamp   = regexp.MustCompile(`\b\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})\b`)
	qualityRunTimestamp       = regexp.MustCompile(`\b\d{8}T\d{6}(?:\.\d+)?Z\b`)
	qualityAttemptCounter     = regexp.MustCompile(`\battempt=\d+\b`)
)

// runStage builds the stage prompt and invokes the proper agent session.
func (e *Engine) runStage(ctx context.Context, state *State) error {
	e.routeUntrustedQualityLoopTargetedRepair(state)
	if state.Sessions == nil {
		state.Sessions = map[string]string{}
	}
	role := stageSessionRoleForState(*state, state.Stage)
	if state.Paths == nil {
		state.Paths = map[string]string{}
	}
	if state.Stages == nil {
		state.Stages = map[string]string{}
	}
	if state.StageTimings == nil {
		state.StageTimings = map[string]StageTiming{}
	}
	qaInput, qaBlocked, err := e.prepareQualityLoopQAReadOnlyGate(state)
	if err != nil {
		return err
	}
	if qaBlocked {
		return saveState(e.Repo, *state)
	}
	archiveBlocked, err := e.prepareQualityLoopArchiveReadOnlyGate(state)
	if err != nil {
		return err
	}
	if archiveBlocked {
		return saveState(e.Repo, *state)
	}
	prompt, err := promptForStage(e.Repo, *state)
	if err != nil {
		return err
	}
	if qaInput.DiffHash != "" {
		armQualityLoopQAReadOnlyGate(state, qaInput)
	}
	options, err := e.stageOptionsForRun(state)
	if err != nil {
		return err
	}
	tool, err := e.Registry.Tool(options.Tool)
	if err != nil {
		return err
	}
	runner := tool.NewRunner()
	if e.stageRuntime == nil {
		e.stageRuntime = map[string]stageRuntime{}
	}
	e.stageRuntime[state.Stage] = stageRuntime{}
	timing := state.StageTimings[state.Stage]
	timing.StartedAt = time.Now().UTC().Format(time.RFC3339Nano)
	timing.FinishedAt = ""
	state.StageTimings[state.Stage] = timing
	state.Stages[state.Stage] = "running"
	if err := saveState(e.Repo, *state); err != nil {
		return err
	}
	e.printProgress(*state, "running")
	sessionKey := sessionStateKey(options.Tool, role)
	if runner, ok := runner.(progressSetter); ok {
		runner.SetProgress(&stageProgressWriter{engine: e, state: state, sessionKey: sessionKey})
	}
	sessionID, err := runner.Run(ctx, e.Repo, prompt, state.Sessions[sessionKey], options)
	if err != nil {
		timing := state.StageTimings[state.Stage]
		timing.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
		state.StageTimings[state.Stage] = timing
		if names := qualityEnvironmentNamesFromError(err); usesQualityLoop(state.Workflow) && len(names) > 0 {
			blockErr := blockQualityEnvironment(e.Repo, state, names)
			saveErr := saveState(e.Repo, *state)
			warnWorkflowWrite("save environment blocked stage state", saveErr)
			return errors.Join(blockErr, saveErr)
		} else if ctx.Err() != nil {
			state.Stages[state.Stage] = statusInterrupted
			saveErr := saveState(e.Repo, *state)
			warnWorkflowWrite("save interrupted stage state", saveErr)
			return errors.Join(err, saveErr)
		} else {
			state.Stages[state.Stage] = statusFailed
			saveErr := saveState(e.Repo, *state)
			warnWorkflowWrite("save failed stage state", saveErr)
			return errors.Join(err, saveErr)
		}
	}
	if sessionID != "" {
		state.Sessions[sessionKey] = sessionID
		meta := e.stageRuntime[state.Stage]
		if meta.Thread == "" {
			meta.Thread = sessionID
			e.stageRuntime[state.Stage] = meta
		}
	}
	if usesQualityLoop(state.Workflow) {
		state.QualityLoop.ResumeRerunPending = false
	}
	timing = state.StageTimings[state.Stage]
	timing.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
	state.StageTimings[state.Stage] = timing
	if qaInput.DiffHash != "" {
		qaReadOnlyPassed, gateErr := e.verifyQualityLoopQAReadOnlyGate(state)
		if gateErr != nil {
			return gateErr
		}
		if !qaReadOnlyPassed {
			return saveState(e.Repo, *state)
		}
	}
	archiveReadOnlyPassed, gateErr := e.verifyQualityLoopArchiveReadOnlyGate(state)
	if gateErr != nil {
		return gateErr
	}
	if !archiveReadOnlyPassed {
		return saveState(e.Repo, *state)
	}
	head, diff, snapshotErr := gitSnapshot(e.Repo)
	if snapshotErr != nil {
		return snapshotErr
	}
	state.BaselineHead = head
	state.BaselineDiff = diff
	if usesQualityLoop(state.Workflow) && qaInput.DiffHash == "" && !isQualityLoopArchiveStage(*state) {
		content, contentErr := gitChangeContentSnapshotForChange(e.Repo, state.ChangeName)
		if contentErr != nil {
			return contentErr
		}
		state.QualityLoop.DiffHash = qualityHashStrings(content)
	}
	return saveState(e.Repo, *state)
}

// routeUntrustedQualityLoopTargetedRepair sends legacy or tampered repair inputs through a fresh audit.
func (e *Engine) routeUntrustedQualityLoopTargetedRepair(state *State) bool {
	if state == nil || !usesQualityLoop(state.Workflow) {
		return false
	}
	stage, err := parseWorkflowStage(state.Stage)
	if err != nil || !stage.isKind(workflowStageTargetedRepair) {
		return false
	}
	qaStage := fmt.Sprintf("qa_%d", stage.Iteration)
	qaPath := filepath.Join(runDir(e.Repo, state.RunID), fmt.Sprintf("qa-%d.json", stage.Iteration))
	qa, qaErr := ReadQA(qaPath)
	contract, contractErr := readAcceptanceForState(e.Repo, *state)
	if qaErr == nil && contractErr == nil && QANeedsFix(qa) &&
		ValidateQAAgainstAcceptance(qa, contract) == nil &&
		qualityLoopTrustedSourceQA(e.Repo, *state, qaStage, qa) {
		return false
	}
	previousStage := state.Stage
	state.Stage = qualityLoopResumeAuditStage(state)
	state.QualityLoop.ResumeRerunPending = true
	delete(state.Stages, previousStage)
	delete(state.StageTimings, previousStage)
	delete(state.DAGNodes, previousStage)
	delete(state.ArtifactGates, previousStage)
	delete(state.ArtifactGates, qaStage)
	return true
}

func warnWorkflowWrite(action string, err error) {
	if err == nil {
		return
	}
	fmt.Fprintf(os.Stderr, "oz flow warning: %s: %v\n", action, err)
}

// stageOptionsForRun resolves dynamic stage options and persists automatic escalations.
func (e *Engine) stageOptionsForRun(state *State) (StageOptions, error) {
	options, err := state.Workflow.StageOption(state.Stage)
	if err != nil && usesQualityLoop(state.Workflow) {
		stage, parseErr := parseWorkflowStage(state.Stage)
		if parseErr == nil {
			switch stage.Kind {
			case workflowStageAudit, workflowStageTargetedRepair:
				options, err = qualityLoopBaseStageOption(state.Workflow, workflowStageRepair)
			case workflowStageQA:
				options, err = qualityLoopBaseStageOption(state.Workflow, workflowStageQA)
			}
		}
	}
	if err != nil {
		return StageOptions{}, err
	}
	escalation, err := fixEscalation(e.Repo, *state)
	if err != nil {
		return StageOptions{}, err
	}
	if !escalation.Enabled {
		return options, nil
	}
	options.Reasoning = higherReasoning(options.Reasoning, escalation.Reasoning)
	options.Fast = false
	state.Workflow.Stages[state.Stage] = options
	if err := saveState(e.Repo, *state); err != nil {
		return StageOptions{}, err
	}
	return options, nil
}

// qualityLoopBaseStageOption reuses the sealed first-round option for a dynamic stage.
func qualityLoopBaseStageOption(workflow WorkflowConfig, kind string) (StageOptions, error) {
	for _, key := range []string{kind + "_1", kind} {
		if option, ok := workflow.Stages[key]; ok {
			return option, nil
		}
	}
	return StageOptions{}, fmt.Errorf("workflow config 缺少动态阶段类型 %q", kind)
}

// validateStage runs configured deterministic checks before a stage may advance.
func (e *Engine) validateStage(ctx context.Context, state *State) (bool, error) {
	ensureWorkflowConfig(state)
	if !shouldValidateStage(*state) {
		return true, nil
	}
	if state.Validation == nil {
		state.Validation = map[string]StageValidationState{}
	}
	current, err := reserveValidationAttempt(
		e.Repo,
		state,
		state.Validation[state.Stage],
		validationKindCommands,
		func(reserved StageValidationState) {
			state.Validation[state.Stage] = reserved
		},
	)
	if err != nil {
		return false, err
	}
	attempt := runStageValidation(ctx, e.Repo, state.ChangeName, state.Stage, current.Attempts, state.Workflow.Validation)
	if usesQualityLoop(state.Workflow) {
		attempt.Kind = validationAttemptKind(attempt)
		// DiffHash records the exact source snapshot supplied to this validation attempt.
		attempt.DiffHash = state.QualityLoop.DiffHash
		current.DiffHash = attempt.DiffHash
	}
	artifactPath, err := writeValidationAttempt(e.Repo, state.RunID, attempt)
	if err != nil {
		return false, err
	}
	current.Kind = validationAttemptKind(attempt)
	current.LastArtifact = artifactPath
	current.Status = attempt.Status
	current.LastError = firstValidationError(attempt)
	state.Validation[state.Stage] = current
	if usesQualityLoop(state.Workflow) {
		state.QualityLoop.ValidationHash = qualityValidationProgressHash(attempt)
		state.QualityLoop.ValidationProgressHash = qualityValidationOutcomeHash(attempt)
	}
	if attempt.Status == validationStatusPassed {
		clearStageValidationFailure(state)
		return true, nil
	}
	if usesQualityLoop(state.Workflow) {
		var output strings.Builder
		for _, command := range attempt.Commands {
			output.WriteString(command.Output)
			output.WriteByte('\n')
		}
		if names := qualityEnvironmentNamesFromText(output.String()); len(names) > 0 {
			if err := blockQualityEnvironment(e.Repo, state, names); err != nil {
				return false, err
			}
			return false, nil
		}
	}
	failureKey := current.LastError
	if usesQualityLoop(state.Workflow) {
		failureKey = qualityValidationFailureKey(attempt, current.LastError)
	}
	if recordQualityGateFailure(state, current.Kind, failureKey) {
		return false, nil
	}
	if isQualityLoopRepairStage(*state) {
		state.Stages[state.Stage] = "validation_failed"
		return false, nil
	}
	if current.Attempts >= state.Workflow.Validation.MaxAttemptsPerStage {
		state.Status = statusValidationBlocked
		state.Stage = statusValidationBlocked
		state.Error = current.LastError
		return false, nil
	}
	state.Stages[state.Stage] = "validation_failed"
	return false, nil
}

// qualityValidationProgressHash binds progress to redacted command outcomes and output content.
func qualityValidationProgressHash(attempt ValidationAttempt) string {
	safeAttempt := redactValidationAttempt(attempt)
	parts := make([]string, 0, len(safeAttempt.Commands))
	for _, command := range safeAttempt.Commands {
		outputHash := qualityHashStrings(command.Output)
		parts = append(parts, fmt.Sprintf("%s\x00%d\x00%s", command.Command, command.ExitCode, outputHash))
	}
	return qualityHashStrings(parts...)
}

// qualityValidationOutcomeHash records command outcomes with volatile diagnostics normalized.
func qualityValidationOutcomeHash(attempt ValidationAttempt) string {
	safeAttempt := redactValidationAttempt(attempt)
	parts := make([]string, 0, len(safeAttempt.Commands))
	for _, command := range safeAttempt.Commands {
		parts = append(parts, fmt.Sprintf(
			"%s\x00%d\x00%s",
			command.Command,
			command.ExitCode,
			qualityHashStrings(qualityStableDiagnosticText(command.Output)),
		))
	}
	return qualityHashStrings(parts...)
}

// qualityValidationFailureKey fingerprints semantic failures without volatile diagnostics.
func qualityValidationFailureKey(attempt ValidationAttempt, failure string) string {
	return qualityHashStrings(strings.TrimSpace(failure), qualityValidationOutcomeHash(attempt))
}

// qualityStableDiagnosticText removes known runtime noise while preserving substantive failures.
func qualityStableDiagnosticText(output string) string {
	stable := qualityGoTestCaseDuration.ReplaceAllString(output, `$1 (<duration>)`)
	stable = qualityGoPackageDuration.ReplaceAllString(stable, `$1 <duration>`)
	stable = qualityRFC3339Timestamp.ReplaceAllString(stable, "<timestamp>")
	stable = qualityRunTimestamp.ReplaceAllString(stable, "<run-id>")
	return qualityAttemptCounter.ReplaceAllString(stable, "attempt=<n>")
}
