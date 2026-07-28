// Package app tests the durable, unbounded quality-delivery loop and its recoverable blocks.
package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// qualityDeliveryLoopRunner writes one repaired audit and one QA-targeted repair cycle.
type qualityDeliveryLoopRunner struct {
	repo       string
	runID      string
	changeName string
	calls      int
}

// Run produces each durable artifact while the production engine owns transitions and gates.
func (r *qualityDeliveryLoopRunner) Run(_ context.Context, _ string, prompt string, _ string, _ StageOptions) (string, error) {
	r.calls++
	base := runDir(r.repo, r.runID)
	switch {
	case strings.Contains(prompt, "写入：`audit-1.json`（相对运行目录）"):
		if err := os.WriteFile(filepath.Join(r.repo, "README.md"), []byte("pre-QA audit repair progress\n"), 0o644); err != nil {
			return "", err
		}
		audit := cleanReviewForStageDecision()
		audit.Decision = "needs_more"
		audit.Findings = []Finding{blockingFindingForStageDecision()}
		audit.Evidence = []string{"go test ./internal/app；quality-loop runtime audit repair verified"}
		if err := writeJSONFile(filepath.Join(base, "audit-1.json"), audit); err != nil {
			return "", err
		}
		return "quality-repair-session", nil
	case strings.Contains(prompt, "写入：`audit-2.json`（相对运行目录）"):
		audit := cleanReviewForStageDecision()
		audit.Evidence = []string{"go test ./internal/app；quality-loop runtime audit clean verified"}
		if err := writeJSONFile(filepath.Join(base, "audit-2.json"), audit); err != nil {
			return "", err
		}
		return "quality-repair-session", nil
	case strings.Contains(prompt, "写入（相对运行目录）：`qa-1.json`"):
		qa := cleanRepairDAGQA()
		qa.Decision = "needs_fix"
		qa.Findings = []Finding{blockingFindingForStageDecision()}
		qa.AcceptanceMatrix[0].Status = "failed"
		if err := writeJSONFile(filepath.Join(base, "qa-1.json"), qa); err != nil {
			return "", err
		}
		return "quality-qa-session", nil
	case strings.Contains(prompt, "写入：`targeted-repair-1.json`（相对运行目录）"):
		if err := os.WriteFile(filepath.Join(r.repo, "README.md"), []byte("targeted repair progress\n"), 0o644); err != nil {
			return "", err
		}
		repair := cleanReviewForStageDecision()
		repair.Evidence = []string{"go test ./internal/app；quality-loop runtime targeted repair verified"}
		if err := writeJSONFile(filepath.Join(base, "targeted-repair-1.json"), repair); err != nil {
			return "", err
		}
		return "quality-repair-session", nil
	case strings.Contains(prompt, "写入（相对运行目录）：`qa-2.json`"):
		if err := writeJSONFile(filepath.Join(base, "qa-2.json"), cleanRepairDAGQA()); err != nil {
			return "", err
		}
		return "quality-qa-session-2", nil
	case strings.Contains(prompt, "delivery-summary"):
		if err := archiveRepairEvidence(r.repo, r.runID, r.changeName); err != nil {
			return "", err
		}
		return "quality-archive-session", nil
	default:
		return "quality-execution-session", nil
	}
}

// qualityDeliveryLoopResult exposes persisted engine state and its artifact repository.
type qualityDeliveryLoopResult struct {
	Repo  string
	State State
	Calls int
}

// qualityLoopEvidenceEnvelope preserves real states and artifacts for independent QA review.
type qualityLoopEvidenceEnvelope struct {
	States             map[string]State               `json:"states,omitempty"`
	Repairs            map[string]Review              `json:"repairs,omitempty"`
	QAs                map[string]QA                  `json:"qas,omitempty"`
	ValidationAttempts map[string]ValidationAttempt   `json:"validation_attempts,omitempty"`
	AcceptanceResults  map[string]AcceptanceRunResult `json:"acceptance_results,omitempty"`
}

// successfulQualityValidationCommand returns an observable configured gate that always passes.
func successfulQualityValidationCommand() ValidationCommand {
	return ValidationCommand{
		Executable: "sh",
		Args:       []string{"-c", "printf 'quality-loop-validation-ok\\n'"},
	}
}

// failingTargetedQualityValidationCommand passes early stages but rejects the targeted repair diff.
func failingTargetedQualityValidationCommand() ValidationCommand {
	return ValidationCommand{
		Executable: "sh",
		Args: []string{"-c", `
first_line=
IFS= read -r first_line < README.md || true
if [ "$first_line" = "targeted repair progress" ]; then
  counter_path=test-results/quality-loop-validation-attempt
  mkdir -p test-results
  counter=0
  if [ -f "$counter_path" ]; then
    IFS= read -r counter < "$counter_path" || true
  fi
  counter=$((counter + 1))
  printf '%s\n' "$counter" > "$counter_path"
  printf 'quality-loop-targeted-validation-failed attempt=%s\n' "$counter"
  exit 7
fi
printf 'quality-loop-validation-ok\n'
`},
	}
}

// runQualityDeliveryLoopWithValidation executes the production loop with one configured validation command.
func runQualityDeliveryLoopWithValidation(t *testing.T, validationCommand ValidationCommand) qualityDeliveryLoopResult {
	t.Helper()
	repo, changeName, acceptanceSource, head, diff := newRepairEvidenceFixture(t)
	runID := newRunID()
	workflow := DefaultWorkflowConfig()
	workflow.Validation.Commands = []ValidationCommand{validationCommand}
	state := State{
		RunID: runID, ChangeName: changeName, Sealed: true, Status: statusRunning,
		Stage: workflowStageExecution, BaselineHead: head, BaselineDiff: diff,
		Workflow: workflow, Sessions: map[string]string{}, Stages: map[string]string{},
		Paths: map[string]string{}, DAGNodes: map[string]DAGNodeState{},
	}
	if err := saveState(repo, state); err != nil {
		t.Fatal(err)
	}
	if err := snapshotRunPrompts(repo, runID); err != nil {
		t.Fatal(err)
	}
	if err := snapshotQualityLoopAcceptance(repo, &state, acceptanceSource); err != nil {
		t.Fatal(err)
	}
	runner := &qualityDeliveryLoopRunner{repo: repo, runID: runID, changeName: changeName}
	registry := NewAgentRegistry()
	registry.Register(fakeWorkflowTool{runner: runner})
	engine := NewEngine(repo, registry)
	if err := engine.runGoDAGLocked(context.Background(), state); err != nil {
		t.Fatal(err)
	}
	completed, err := loadState(repo, runID)
	if err != nil {
		t.Fatal(err)
	}
	return qualityDeliveryLoopResult{Repo: repo, State: completed, Calls: runner.calls}
}

// runRealQualityDeliveryLoop executes the production loop with an observable successful validation gate.
func runRealQualityDeliveryLoop(t *testing.T) qualityDeliveryLoopResult {
	t.Helper()
	return runQualityDeliveryLoopWithValidation(t, successfulQualityValidationCommand())
}

// readQualityValidationAttempt loads one engine-produced deterministic gate artifact.
func readQualityValidationAttempt(t *testing.T, path string) ValidationAttempt {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var attempt ValidationAttempt
	if err := json.Unmarshal(data, &attempt); err != nil {
		t.Fatal(err)
	}
	return attempt
}

// readQualityAcceptanceResult loads one engine-produced acceptance result from its durable path.
func readQualityAcceptanceResult(t *testing.T, repo, path string) AcceptanceRunResult {
	t.Helper()
	if !filepath.IsAbs(path) {
		path = filepath.Join(repo, filepath.FromSlash(path))
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var result AcceptanceRunResult
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

// writeQualityLoopEvidence writes real persisted states and artifacts consumed by change 45.
func writeQualityLoopEvidence(t *testing.T, envName string, evidence qualityLoopEvidenceEnvelope) {
	t.Helper()
	path := os.Getenv(envName)
	if path == "" {
		return
	}
	data, err := json.MarshalIndent(evidence, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}

// applyQualityDecision applies the pure transition fields used by engine.advance.
func applyQualityDecision(state *State, decision StageDecision) {
	if decision.QualityLoop != nil {
		state.QualityLoop = *decision.QualityLoop
	}
	state.Stage = decision.NextStage
	state.Status = decision.NextStatus
	state.Error = decision.BlockedReason
}

// qualityLoopState returns a minimal sealed state using the new workflow generation.
func qualityLoopState(stage string) State {
	return State{
		RunID:         "quality-loop-test",
		ChangeName:    "45-quality-loop",
		Sealed:        true,
		Status:        statusRunning,
		Stage:         stage,
		Workflow:      DefaultWorkflowConfig(),
		Stages:        map[string]string{},
		Validation:    map[string]StageValidationState{},
		AcceptanceRun: map[string]StageValidationState{},
	}
}

// TestPreQAFullAuditRequiresCleanConfirmation verifies repaired audits repeat before independent QA.
func TestPreQAFullAuditRequiresCleanConfirmation(t *testing.T) {
	state := qualityLoopState(workflowStageExecution)
	decision, err := DecideNextStage(state, Review{}, QA{})
	if err != nil {
		t.Fatal(err)
	}
	applyQualityDecision(&state, decision)
	if state.Stage != "audit_1" || state.QualityLoop.Mode != "pre_qa_audit" {
		t.Fatalf("execution transition = %s/%s", state.Stage, state.QualityLoop.Mode)
	}

	needsMore := cleanReviewForStageDecision()
	needsMore.Decision = "needs_more"
	needsMore.Findings = []Finding{blockingFindingForStageDecision()}
	state.AcceptanceRun[state.Stage] = StageValidationState{Status: validationStatusPassed}
	decision, err = DecideNextStage(state, needsMore, QA{})
	if err != nil {
		t.Fatal(err)
	}
	applyQualityDecision(&state, decision)
	if state.Stage != "audit_2" {
		t.Fatalf("repaired audit advanced to %s, want audit_2", state.Stage)
	}

	state.AcceptanceRun[state.Stage] = StageValidationState{Status: validationStatusPassed}
	decision, err = DecideNextStage(state, cleanReviewForStageDecision(), QA{})
	if err != nil {
		t.Fatal(err)
	}
	applyQualityDecision(&state, decision)
	if state.Stage != "qa_1" || !state.QualityLoop.RequiredTestsPassed {
		t.Fatalf("clean audit transition = %s tests=%v", state.Stage, state.QualityLoop.RequiredTestsPassed)
	}
	result := runRealQualityDeliveryLoop(t)
	completed := result.State
	if completed.Status != statusDone || completed.Stage != workflowStageDone ||
		completed.Stages["audit_1"] != "completed" || completed.Stages["audit_2"] != "completed" ||
		completed.Stages["qa_1"] != "completed" {
		t.Fatalf("real engine quality loop = %s/%s calls=%d error=%q stages=%#v gates=%#v",
			completed.Status, completed.Stage, result.Calls, completed.Error, completed.Stages, completed.ArtifactGates)
	}
	if completed.QualityLoop.DiffHash == "" ||
		completed.QualityLoop.DiffHash != completed.Validation["targeted_repair_1"].DiffHash {
		t.Fatalf("archive overwrote trusted QA diff hash: quality=%s repair=%s",
			completed.QualityLoop.DiffHash, completed.Validation["targeted_repair_1"].DiffHash)
	}
	if completed.Validation["execution"].DiffHash == completed.Validation["audit_1"].DiffHash {
		t.Fatal("pre-QA audit needs_more did not produce tracked source progress")
	}
	audit1, err := ReadRepair(filepath.Join(runDir(result.Repo, completed.RunID), "audit-1.json"))
	if err != nil {
		t.Fatal(err)
	}
	audit2, err := ReadRepair(filepath.Join(runDir(result.Repo, completed.RunID), "audit-2.json"))
	if err != nil {
		t.Fatal(err)
	}
	qa1, err := ReadQA(filepath.Join(runDir(result.Repo, completed.RunID), "qa-1.json"))
	if err != nil {
		t.Fatal(err)
	}
	audit2Acceptance := readQualityAcceptanceResult(
		t,
		result.Repo,
		completed.AcceptanceRun["audit_2"].LastArtifact,
	)
	writeQualityLoopEvidence(t, "QUALITY_PRE_QA_STATE", qualityLoopEvidenceEnvelope{
		States:            map[string]State{"final": completed},
		Repairs:           map[string]Review{"audit_1": audit1, "audit_2": audit2},
		QAs:               map[string]QA{"qa_1": qa1},
		AcceptanceResults: map[string]AcceptanceRunResult{"audit_2": audit2Acceptance},
	})
}

// TestQAFailureTargetsFindingsAndSelfValidates verifies QA findings drive a gated targeted repair.
func TestQAFailureTargetsFindingsAndSelfValidates(t *testing.T) {
	state := qualityLoopState("qa_1")
	qa := cleanQAForStageDecision()
	qa.Decision = "needs_fix"
	qa.Findings = []Finding{blockingFindingForStageDecision()}
	qa.AcceptanceMatrix = []AcceptanceResult{{ID: "quality-delivery-loop", Status: "failed"}}
	decision, err := DecideNextStage(state, Review{}, qa)
	if err != nil {
		t.Fatal(err)
	}
	applyQualityDecision(&state, decision)
	if state.Stage != "targeted_repair_1" || state.QualityLoop.SourceQAArtifact != "qa-1.json" {
		t.Fatalf("QA failure transition = %s source=%s", state.Stage, state.QualityLoop.SourceQAArtifact)
	}

	state.AcceptanceRun[state.Stage] = StageValidationState{Status: validationStatusPassed}
	decision, err = DecideNextStage(state, cleanReviewForStageDecision(), QA{})
	if err != nil {
		t.Fatal(err)
	}
	applyQualityDecision(&state, decision)
	if state.Stage != "qa_2" || !state.QualityLoop.RequiredTestsPassed || !state.QualityLoop.FailedTestsReplayed {
		t.Fatalf("targeted repair gate = %s tests=%v replay=%v", state.Stage, state.QualityLoop.RequiredTestsPassed, state.QualityLoop.FailedTestsReplayed)
	}
	result := runRealQualityDeliveryLoop(t)
	completed := result.State
	sourceGate := completed.ArtifactGates["qa_1"]
	if sourceGate.Kind != validationKindQAReadOnly ||
		sourceGate.Status != validationStatusPassed ||
		sourceGate.CheckpointHash == "" {
		t.Fatalf("completed source QA gate lost durable trust: %#v", sourceGate)
	}
	qa1, err := ReadQA(filepath.Join(runDir(result.Repo, completed.RunID), "qa-1.json"))
	if err != nil {
		t.Fatal(err)
	}
	repair1, err := ReadRepair(filepath.Join(runDir(result.Repo, completed.RunID), "targeted-repair-1.json"))
	if err != nil {
		t.Fatal(err)
	}
	targetedValidation := completed.Validation["targeted_repair_1"]
	targetedAcceptance := completed.AcceptanceRun["targeted_repair_1"]
	validationAttempt := readQualityValidationAttempt(t, targetedValidation.LastArtifact)
	configuredValidationPassed := false
	for _, command := range validationAttempt.Commands {
		if command.ExitCode == 0 && strings.Contains(command.Output, "quality-loop-validation-ok") {
			configuredValidationPassed = true
		}
	}
	if targetedValidation.Status != validationStatusPassed || !configuredValidationPassed ||
		targetedValidation.DiffHash == "" || targetedValidation.DiffHash != targetedAcceptance.DiffHash ||
		validationAttempt.DiffHash != targetedValidation.DiffHash {
		t.Fatalf("targeted validation state=%#v acceptance=%#v attempt=%#v",
			targetedValidation, targetedAcceptance, validationAttempt)
	}

	failedResult := runQualityDeliveryLoopWithValidation(t, failingTargetedQualityValidationCommand())
	failed := failedResult.State
	failedValidation := failed.Validation["targeted_repair_1"]
	_, qa2Created := failed.Stages["qa_2"]
	validationFailureBlockedQA2 := failed.Status == statusBlockedStalled &&
		failed.Stage == statusBlockedStalled &&
		failedValidation.Status == validationStatusFailed &&
		!qa2Created
	if !validationFailureBlockedQA2 {
		t.Fatalf("failed targeted validation advanced unexpectedly: state=%s/%s stages=%#v validation=%#v",
			failed.Status, failed.Stage, failed.Stages, failedValidation)
	}
	qa2, err := ReadQA(filepath.Join(runDir(result.Repo, completed.RunID), "qa-2.json"))
	if err != nil {
		t.Fatal(err)
	}
	targetedAcceptanceResult := readQualityAcceptanceResult(
		t,
		result.Repo,
		targetedAcceptance.LastArtifact,
	)
	writeQualityLoopEvidence(t, "QUALITY_TARGETED_STATE", qualityLoopEvidenceEnvelope{
		States:  map[string]State{"final": completed, "failed_validation": failed},
		Repairs: map[string]Review{"targeted_repair_1": repair1},
		QAs:     map[string]QA{"qa_1": qa1, "qa_2": qa2},
		ValidationAttempts: map[string]ValidationAttempt{
			"targeted_repair_1": validationAttempt,
		},
		AcceptanceResults: map[string]AcceptanceRunResult{
			"targeted_repair_1": targetedAcceptanceResult,
		},
	})
}

// TestRepeatedQAFindingWithRepairProgressContinues preserves the previous QA progress baseline.
func TestRepeatedQAFindingWithRepairProgressContinues(t *testing.T) {
	state := qualityLoopState("qa_1")
	state.BaselineDiff = "before repair"
	qa := cleanQAForStageDecision()
	qa.Decision = "needs_fix"
	qa.Findings = []Finding{blockingFindingForStageDecision()}

	decision, err := DecideNextStage(state, Review{}, qa)
	if err != nil {
		t.Fatal(err)
	}
	applyQualityDecision(&state, decision)
	firstFailureProgress := state.QualityLoop.ProgressHash

	state.BaselineDiff = "after real repair"
	state.AcceptanceRun[state.Stage] = StageValidationState{Status: validationStatusPassed}
	decision, err = DecideNextStage(state, cleanReviewForStageDecision(), QA{})
	if err != nil {
		t.Fatal(err)
	}
	applyQualityDecision(&state, decision)
	if state.Stage != "qa_2" || state.QualityLoop.ProgressHash != firstFailureProgress {
		t.Fatalf("repair transition = %s progress=%s, want qa_2 with prior QA baseline", state.Stage, state.QualityLoop.ProgressHash)
	}

	decision, err = DecideNextStage(state, Review{}, qa)
	if err != nil {
		t.Fatal(err)
	}
	applyQualityDecision(&state, decision)
	if state.Stage != "targeted_repair_2" || state.Status != statusRunning {
		t.Fatalf("same finding with progress = %s/%s, want targeted_repair_2/running", state.Stage, state.Status)
	}
}

// TestQualityLoopQASessionsAreIsolated keeps QA rounds fresh while repair stages share one session.
func TestQualityLoopQASessionsAreIsolated(t *testing.T) {
	state := qualityLoopState("qa_1")
	qaOption := state.Workflow.Stages["qa_1"]
	qaOption.Tool = "pi"
	state.Workflow.Stages["qa_1"] = qaOption
	state.Sessions = map[string]string{
		sessionStateKey("pi", "qa_1"):        "qa-one",
		sessionStateKey("codex", "repairer"): "shared-repairer",
	}
	key, id := promptRoleSession(state)
	if key != "pi:qa_1" || id != "qa-one" {
		t.Fatalf("qa_1 session = %s/%s", key, id)
	}
	state.Stage = "qa_2"
	key, id = promptRoleSession(state)
	if key != "pi:qa_2" || id != "" {
		t.Fatalf("qa_2 session = %s/%s, want isolated empty session", key, id)
	}
	state.Sessions[sessionStateKey("pi", "qa_2")] = "qa-two"
	if id := statusRoleSessionID(state, "qa"); id != "qa-two" {
		t.Fatalf("dynamic QA status session = %q", id)
	}
	for _, stage := range []string{"audit_2", "targeted_repair_2"} {
		state.Stage = stage
		key, id = promptRoleSession(state)
		if key != "codex:repairer" || id != "shared-repairer" {
			t.Fatalf("%s session = %s/%s, want shared repairer", stage, key, id)
		}
	}
}

// TestQualityLoopArchiveContextIncludesDynamicArtifacts gives archive the full durable QA chain.
func TestQualityLoopArchiveContextIncludesDynamicArtifacts(t *testing.T) {
	repo, changeName, acceptanceSource, _, _ := newRepairEvidenceFixture(t)
	state := qualityLoopState(workflowStageArchive)
	state.RunID = newRunID()
	state.ChangeName = changeName
	state.Stages = map[string]string{
		"audit_1":           "completed",
		"qa_1":              "completed",
		"targeted_repair_1": "completed",
		"qa_2":              "completed",
	}
	if err := snapshotQualityLoopAcceptance(repo, &state, acceptanceSource); err != nil {
		t.Fatal(err)
	}
	context, err := promptContext(repo, state)
	if err != nil {
		t.Fatal(err)
	}
	if len(context.PreviousRepairPaths) != 2 ||
		!strings.HasSuffix(context.PreviousRepairPaths[0], "audit-1.json") ||
		!strings.HasSuffix(context.PreviousRepairPaths[1], "targeted-repair-1.json") {
		t.Fatalf("archive repair paths = %#v", context.PreviousRepairPaths)
	}
	if len(context.PreviousQAPaths) != 2 ||
		!strings.HasSuffix(context.PreviousQAPaths[0], "qa-1.json") ||
		!strings.HasSuffix(context.PreviousQAPaths[1], "qa-2.json") ||
		context.LatestPreviousQAPath != context.PreviousQAPaths[1] {
		t.Fatalf("archive QA paths = %#v latest=%q", context.PreviousQAPaths, context.LatestPreviousQAPath)
	}
}

// TestRepairLoopHasNoIterationCeiling verifies twelve QA failures do not create a review-limit block.
func TestRepairLoopHasNoIterationCeiling(t *testing.T) {
	state := qualityLoopState("qa_1")
	for iteration := 1; iteration <= 12; iteration++ {
		state.BaselineDiff = fmt.Sprintf("progress-%d", iteration)
		qa := cleanQAForStageDecision()
		qa.Decision = "needs_fix"
		finding := blockingFindingForStageDecision()
		finding.Title = fmt.Sprintf("finding-%d", iteration)
		qa.Findings = []Finding{finding}
		decision, err := DecideNextStage(state, Review{}, qa)
		if err != nil {
			t.Fatal(err)
		}
		applyQualityDecision(&state, decision)
		wantRepair := fmt.Sprintf("targeted_repair_%d", iteration)
		if state.Stage != wantRepair || state.Status != statusRunning {
			t.Fatalf("iteration %d = %s/%s, want %s/running", iteration, state.Stage, state.Status, wantRepair)
		}
		state.AcceptanceRun[state.Stage] = StageValidationState{Status: validationStatusPassed}
		decision, err = DecideNextStage(state, cleanReviewForStageDecision(), QA{})
		if err != nil {
			t.Fatal(err)
		}
		applyQualityDecision(&state, decision)
	}
	decision, err := DecideNextStage(state, Review{}, cleanQAForStageDecision())
	if err != nil {
		t.Fatal(err)
	}
	applyQualityDecision(&state, decision)
	decision, err = DecideNextStage(state, Review{}, QA{})
	if err != nil {
		t.Fatal(err)
	}
	applyQualityDecision(&state, decision)
	if state.Status != statusDone || state.Stage != workflowStageDone {
		t.Fatalf("final state = %s/%s", state.Status, state.Stage)
	}
	result := runRealUnboundedQualityLoop(t)
	completed := result.State
	qaArtifacts := map[string]QA{}
	for iteration := 1; iteration <= 13; iteration++ {
		qa, err := ReadQA(filepath.Join(runDir(result.Repo, completed.RunID), fmt.Sprintf("qa-%d.json", iteration)))
		if err != nil {
			t.Fatal(err)
		}
		qaArtifacts[fmt.Sprintf("qa_%d", iteration)] = qa
		if iteration <= 12 && !QANeedsFix(qa) {
			t.Fatalf("qa_%d unexpectedly clean", iteration)
		}
		if iteration == 13 && QANeedsFix(qa) {
			t.Fatal("qa_13 must be the final clean QA")
		}
	}
	for stage, status := range completed.Stages {
		if stage == statusBlocked || status == statusBlocked {
			t.Fatalf("unbounded loop contains review-limit block: %s=%s", stage, status)
		}
	}
	writeQualityLoopEvidence(t, "QUALITY_UNBOUNDED_STATE", qualityLoopEvidenceEnvelope{
		States: map[string]State{"final": completed},
		QAs:    qaArtifacts,
	})
}

// TestRepairEnvironmentBlockResumesWithoutBudget verifies safe diagnostics and original-stage recovery.
func TestRepairEnvironmentBlockResumesWithoutBudget(t *testing.T) {
	repo, changeName, acceptanceSource, head, diff := newRepairEvidenceFixture(t)
	missingName := "environment/available-after-resume"
	missingPath := filepath.Join(repo, filepath.FromSlash(missingName))
	names := qualityEnvironmentNamesFromError(fmt.Errorf("blocked_environment: %s=do-not-record", missingName))
	if len(names) != 1 || strings.Contains(strings.Join(names, ","), "do-not-record") {
		t.Fatalf("unsafe environment diagnostics: %#v", names)
	}
	state := qualityLoopState("audit_3")
	state.RunID = newRunID()
	state.ChangeName = changeName
	state.BaselineHead = head
	state.BaselineDiff = diff
	if err := blockQualityEnvironment(repo, &state, names); err != nil {
		t.Fatal(err)
	}
	if state.Status != statusBlockedEnvironment {
		t.Fatalf("blocked status = %s", state.Status)
	}
	if err := saveState(repo, state); err != nil {
		t.Fatal(err)
	}
	if err := snapshotRunPrompts(repo, state.RunID); err != nil {
		t.Fatal(err)
	}
	if err := snapshotQualityLoopAcceptance(repo, &state, acceptanceSource); err != nil {
		t.Fatal(err)
	}
	if err := saveState(repo, state); err != nil {
		t.Fatal(err)
	}
	blocked, err := loadState(repo, state.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(missingPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(missingPath, []byte("ready"), 0o600); err != nil {
		t.Fatal(err)
	}
	registry := NewAgentRegistry()
	registry.Register(fakeWorkflowTool{runner: &fakeWorkflowRunner{}})
	engine := NewEngine(repo, registry)
	resumed, err := engine.prepareRestartRun(state.RunID, false)
	if err != nil {
		t.Fatal(err)
	}
	if resumed.Status != statusRunning || resumed.Stage != "audit_3" {
		t.Fatalf("resumed state = %s/%s", resumed.Status, resumed.Stage)
	}
	if !shouldForceStageRerun(resumed) {
		t.Fatal("resumed environment block must rerun its original stage")
	}
	if resumed.Stages["audit_3"] == "needs_more" {
		t.Fatal("environment resume must not be rewritten as a quality needs_more result")
	}
	resumedPersisted, err := loadState(repo, state.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if err := engine.runStage(context.Background(), &resumed); err != nil {
		t.Fatal(err)
	}
	if shouldForceStageRerun(resumed) {
		t.Fatal("successful original-stage rerun must clear its resume marker")
	}
	rerunPersisted, err := loadState(repo, state.RunID)
	if err != nil {
		t.Fatal(err)
	}
	writeQualityLoopEvidence(t, "QUALITY_ENVIRONMENT_STATE", qualityLoopEvidenceEnvelope{
		States: map[string]State{
			"blocked": blocked,
			"resumed": resumedPersisted,
			"rerun":   rerunPersisted,
		},
	})
}

// TestRepairStalledBlockResumesWithProgress verifies unchanged failures pause and new code resumes.
func TestRepairStalledBlockResumesWithProgress(t *testing.T) {
	repo, changeName, acceptanceSource, _, _ := newRepairEvidenceFixture(t)
	progressPath := filepath.Join(repo, "README.md")
	if err := os.WriteFile(progressPath, []byte("first repair\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	head, diff, err := gitSnapshot(repo)
	if err != nil {
		t.Fatal(err)
	}
	content, err := gitChangeContentSnapshot(repo)
	if err != nil {
		t.Fatal(err)
	}
	state := qualityLoopState("qa_1")
	state.RunID = newRunID()
	state.ChangeName = changeName
	state.BaselineHead = head
	state.BaselineDiff = diff
	state.QualityLoop.DiffHash = qualityHashStrings(content)
	if err := snapshotQualityLoopAcceptance(repo, &state, acceptanceSource); err != nil {
		t.Fatal(err)
	}
	state.QualityLoop.EvidenceHash, err = qualityCurrentEvidenceHash(repo, state)
	if err != nil {
		t.Fatal(err)
	}
	qa := cleanQAForStageDecision()
	qa.Decision = "needs_fix"
	qa.Findings = []Finding{blockingFindingForStageDecision()}
	state.QualityLoop.FindingFingerprint = qaFindingFingerprint(qa)
	state.QualityLoop.ProgressHash = qualityProgressHash(state)
	decision, err := DecideNextStage(state, Review{}, qa)
	if err != nil {
		t.Fatal(err)
	}
	applyQualityDecision(&state, decision)
	if state.Status != statusBlockedStalled {
		t.Fatalf("stalled decision = %s/%s", state.Status, state.Stage)
	}
	if err := saveState(repo, state); err != nil {
		t.Fatal(err)
	}
	blocked, err := loadState(repo, state.RunID)
	if err != nil {
		t.Fatal(err)
	}
	engine := NewEngine(repo, NewAgentRegistry())
	if err := engine.prepareQualityLoopResume(&state, false); err == nil {
		t.Fatal("unchanged stalled run unexpectedly resumed")
	}
	if err := os.WriteFile(progressPath, []byte("second repair\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := engine.prepareQualityLoopResume(&state, false); err != nil {
		t.Fatal(err)
	}
	if state.Status != statusRunning || state.Stage != "audit_1" {
		t.Fatalf("stalled resume = %s/%s", state.Status, state.Stage)
	}
	if !shouldForceStageRerun(state) {
		t.Fatal("resumed stalled QA source progress must run a fresh audit")
	}
	if state.Stages["qa_1"] == "needs_more" {
		t.Fatal("stalled resume must use the dedicated rerun marker")
	}
	resumed, err := loadState(repo, state.RunID)
	if err != nil {
		t.Fatal(err)
	}
	writeQualityLoopEvidence(t, "QUALITY_STALLED_STATE", qualityLoopEvidenceEnvelope{
		States: map[string]State{"blocked": blocked, "resumed": resumed},
		QAs:    map[string]QA{"source": qa},
	})
}

// TestRepairStalledBlockResumesWithEvidenceProgress accepts new runtime evidence without source churn.
func TestRepairStalledBlockResumesWithEvidenceProgress(t *testing.T) {
	repo, changeName, acceptanceSource, head, diff := newRepairEvidenceFixture(t)
	content, err := gitChangeContentSnapshot(repo)
	if err != nil {
		t.Fatal(err)
	}
	state := qualityLoopState("qa_1")
	state.RunID = newRunID()
	state.ChangeName = changeName
	state.BaselineHead = head
	state.BaselineDiff = diff
	state.QualityLoop.DiffHash = qualityHashStrings(content)
	if err := snapshotQualityLoopAcceptance(repo, &state, acceptanceSource); err != nil {
		t.Fatal(err)
	}
	evidencePath := filepath.Join(repo, "test-results", "repair-dag", "runtime.log")
	if err := os.MkdirAll(filepath.Dir(evidencePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(evidencePath, []byte("--- PASS: TestEvidence (0.01s)\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	state.QualityLoop.EvidenceHash, state.QualityLoop.EvidenceProgressHash, err =
		qualityCurrentEvidenceHashes(repo, state)
	if err != nil {
		t.Fatal(err)
	}
	qa := cleanQAForStageDecision()
	qa.Decision = "needs_fix"
	qa.Findings = []Finding{blockingFindingForStageDecision()}
	state.QualityLoop.FindingFingerprint = qaFindingFingerprint(qa)
	state.QualityLoop.ProgressHash = qualityProgressHash(state)
	decision, err := DecideNextStage(state, Review{}, qa)
	if err != nil {
		t.Fatal(err)
	}
	applyQualityDecision(&state, decision)
	if err := saveState(repo, state); err != nil {
		t.Fatal(err)
	}
	engine := NewEngine(repo, NewAgentRegistry())
	if err := engine.prepareQualityLoopResume(&state, false); err == nil {
		t.Fatal("unchanged stalled run unexpectedly resumed")
	}
	if err := os.WriteFile(evidencePath, []byte("--- PASS: TestEvidence (0.02s)\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := engine.prepareQualityLoopResume(&state, false); err == nil {
		t.Fatal("volatile-only evidence unexpectedly resumed stalled run")
	}
	if err := os.Remove(evidencePath); err != nil {
		t.Fatal(err)
	}
	if err := engine.prepareQualityLoopResume(&state, false); err == nil {
		t.Fatal("missing required evidence unexpectedly resumed stalled run")
	}
	if err := os.WriteFile(evidencePath, []byte("new runtime evidence\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := engine.prepareQualityLoopResume(&state, false); err != nil {
		t.Fatal(err)
	}
	if state.Status != statusRunning || state.Stage != "audit_1" {
		t.Fatalf("evidence resume = %s/%s", state.Status, state.Stage)
	}
	if !shouldForceStageRerun(state) {
		t.Fatal("evidence progress after QA must rerun deterministic gates through a fresh audit")
	}
}

// TestQualityEvidenceContentHashIncludesDirectoryFiles tracks trace directories as evidence progress.
func TestQualityEvidenceContentHashIncludesDirectoryFiles(t *testing.T) {
	repo := t.TempDir()
	evidenceDir := filepath.Join(repo, "test-results", "trace")
	if err := os.MkdirAll(evidenceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(evidenceDir, "trace.json")
	if err := os.WriteFile(path, []byte("first\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	first := qualityEvidenceContentHash(repo, "test-results/trace")
	if err := os.WriteFile(path, []byte("second\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	second := qualityEvidenceContentHash(repo, "test-results/trace")
	if first == "unavailable" || first == second {
		t.Fatalf("directory evidence hashes = %q/%q", first, second)
	}
}
