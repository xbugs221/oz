// Package app tests repair findings discovered during the dynamic quality-loop audit.
package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// trustQualityLoopSourceQAForPrompt records the durable tested input consumed by one QA stage.
func trustQualityLoopSourceQAForPrompt(t *testing.T, repo string, state *State, qaStage, checkpoint string) {
	t.Helper()
	content, err := gitChangeContentSnapshotForChange(repo, state.ChangeName)
	if err != nil {
		t.Fatal(err)
	}
	state.QualityLoop.DiffHash = qualityHashStrings(content)
	state.Stages[checkpoint] = "completed"
	recordPassedQualityLoopCheckpoint(t, repo, state, checkpoint)
	if state.ArtifactGates == nil {
		state.ArtifactGates = map[string]StageValidationState{}
	}
	state.ArtifactGates[qaStage] = StageValidationState{
		Kind: validationKindQAReadOnly, Status: validationStatusPassed,
		DiffHash:       state.QualityLoop.DiffHash,
		CheckpointHash: trustedQualityLoopCheckpointHash(t, repo, *state, checkpoint),
	}
}

// TestQualityLoopPromptUsesRealDynamicArtifacts verifies every dynamic mode references existing artifacts.
func TestQualityLoopPromptUsesRealDynamicArtifacts(t *testing.T) {
	repo, changeName, acceptanceSource, _, _ := newRepairEvidenceFixture(t)
	state := qualityLoopState("audit_2")
	state.RunID = newRunID()
	state.ChangeName = changeName
	state.Stages = map[string]string{"audit_1": "completed"}
	if err := snapshotQualityLoopAcceptance(repo, &state, acceptanceSource); err != nil {
		t.Fatal(err)
	}
	validRepair := cleanReviewForStageDecision()
	validRepair.Evidence = []string{"go test ./internal/app; runtime checkpoint verified"}
	if err := writeJSONFile(filepath.Join(runDir(repo, state.RunID), "audit-1.json"), validRepair); err != nil {
		t.Fatal(err)
	}

	contextValue, err := promptContext(repo, state)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(contextValue.LatestPreviousRepairPath, "audit-1.json") ||
		strings.Contains(contextValue.LatestPreviousRepairPath, "repair-1.json") {
		t.Fatalf("audit checkpoint = %q", contextValue.LatestPreviousRepairPath)
	}
	if contextValue.AcceptancePath != filepath.Join(runDir(repo, state.RunID), "acceptance.json") {
		t.Fatalf("acceptance path = %q, want sealed snapshot", contextValue.AcceptancePath)
	}

	state.Stage = "targeted_repair_2"
	state.Stages["targeted_repair_1"] = "completed"
	if err := writeJSONFile(filepath.Join(runDir(repo, state.RunID), "targeted-repair-1.json"), validRepair); err != nil {
		t.Fatal(err)
	}
	sourceQA := cleanRepairDAGQA()
	sourceQA.Decision = "needs_fix"
	sourceQA.Findings = []Finding{blockingFindingForStageDecision()}
	sourceQA.AcceptanceMatrix[0].Status = "failed"
	if err := writeJSONFile(filepath.Join(runDir(repo, state.RunID), "qa-2.json"), sourceQA); err != nil {
		t.Fatal(err)
	}
	state.QualityLoop.SourceQAArtifact = "qa-2.json"
	state.QualityLoop.SourceQAHash = qaArtifactContentHash(sourceQA)
	state.QualityLoop.FindingFingerprint = qaFindingFingerprint(sourceQA)
	trustQualityLoopSourceQAForPrompt(t, repo, &state, "qa_2", "targeted_repair_1")
	contextValue, err = promptContext(repo, state)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(contextValue.LatestPreviousRepairPath, "targeted-repair-1.json") ||
		!strings.HasSuffix(contextValue.LatestPreviousQAPath, "qa-2.json") {
		t.Fatalf("targeted context repair=%q qa=%q", contextValue.LatestPreviousRepairPath, contextValue.LatestPreviousQAPath)
	}

	state.Stage = "qa_2"
	contextValue, err = promptContext(repo, state)
	if err != nil {
		t.Fatal(err)
	}
	if !contextValue.HasRepairCheckpoint || !strings.HasSuffix(contextValue.RepairPath, "targeted-repair-1.json") ||
		!strings.HasSuffix(contextValue.QAPath, "qa-2.json") {
		t.Fatalf("qa context checkpoint=%q output=%q", contextValue.RepairPath, contextValue.QAPath)
	}
}

// TestQualityLoopValidationUsesProgressInsteadOfAttemptLimit verifies repeated failures stall semantically.
func TestQualityLoopValidationUsesProgressInsteadOfAttemptLimit(t *testing.T) {
	repo, changeName, _, _, _ := newRepairEvidenceFixture(t)
	state := qualityLoopState("targeted_repair_1")
	state.ChangeName = changeName
	state.Workflow.Validation.MaxAttemptsPerStage = 1
	state.Workflow.Validation.Commands = []ValidationCommand{{Executable: "sh", Args: []string{"-c", "exit 7"}}}
	content, err := gitChangeContentSnapshot(repo)
	if err != nil {
		t.Fatal(err)
	}
	state.QualityLoop.DiffHash = qualityHashStrings(content)
	engine := NewEngine(repo, NewAgentRegistry())

	passed, err := engine.validateStage(context.Background(), &state)
	if err != nil || passed {
		t.Fatalf("first validation = passed:%v err:%v", passed, err)
	}
	if state.Status != statusRunning || state.Stage != "targeted_repair_1" {
		t.Fatalf("first failure hit fixed limit: %s/%s", state.Status, state.Stage)
	}
	passed, err = engine.validateStage(context.Background(), &state)
	if err != nil || passed {
		t.Fatalf("second validation = passed:%v err:%v", passed, err)
	}
	if state.Status != statusBlockedStalled || state.Stage != statusBlockedStalled {
		t.Fatalf("unchanged failure = %s/%s, want blocked_stalled", state.Status, state.Stage)
	}
}

// TestQualityLoopAcceptanceUsesSealedContract verifies active acceptance edits cannot weaken a run.
func TestQualityLoopAcceptanceUsesSealedContract(t *testing.T) {
	repo, changeName, acceptanceSource, _, _ := newRepairEvidenceFixture(t)
	state := qualityLoopState("audit_1")
	state.RunID = newRunID()
	state.ChangeName = changeName
	if err := snapshotQualityLoopAcceptance(repo, &state, acceptanceSource); err != nil {
		t.Fatal(err)
	}
	weakened := strings.Replace(repairDAGAcceptanceJSON(),
		"bash docs/changes/1-演示/tests/test_repair_dag.sh", "exit 41", 1)
	if err := os.WriteFile(acceptanceSource, []byte(weakened), 0o644); err != nil {
		t.Fatal(err)
	}
	engine := NewEngine(repo, NewAgentRegistry())
	passed, err := engine.runAcceptanceGate(context.Background(), &state)
	if err != nil || !passed {
		t.Fatalf("sealed acceptance gate = passed:%v err:%v state=%s/%s", passed, err, state.Status, state.Stage)
	}
}

// TestQualityLoopMissingSealedAcceptanceFailsClosed verifies quality runs never use an active fallback.
func TestQualityLoopMissingSealedAcceptanceFailsClosed(t *testing.T) {
	repo, changeName, _, _, _ := newRepairEvidenceFixture(t)
	state := qualityLoopState("audit_1")
	state.RunID = newRunID()
	state.ChangeName = changeName
	if _, err := readAcceptanceForState(repo, state); err == nil {
		t.Fatal("quality loop accepted the active contract without a sealed snapshot")
	}
	if _, err := promptContext(repo, state); err == nil {
		t.Fatal("quality loop prompt accepted a missing sealed contract")
	}
}

// TestQualityLoopTargetedRepairRequiresSourceQA verifies an empty scope cannot enter repair.
func TestQualityLoopTargetedRepairRequiresSourceQA(t *testing.T) {
	repo, changeName, acceptanceSource, _, _ := newRepairEvidenceFixture(t)
	state := qualityLoopState("targeted_repair_2")
	state.RunID = newRunID()
	state.ChangeName = changeName
	if err := snapshotQualityLoopAcceptance(repo, &state, acceptanceSource); err != nil {
		t.Fatal(err)
	}
	if _, err := promptContext(repo, state); err == nil {
		t.Fatal("targeted repair accepted a missing source QA artifact")
	}
}

// TestQualityLoopAcceptanceEnvironmentMarkerBlocks verifies required-test diagnostics reach the environment state.
func TestQualityLoopAcceptanceEnvironmentMarkerBlocks(t *testing.T) {
	repo, changeName, acceptanceSource, _, _ := newRepairEvidenceFixture(t)
	testPath := filepath.Join(repo, "docs", "changes", changeName, "tests", "test_repair_dag.sh")
	body := "#!/usr/bin/env bash\n# 文件功能目的：模拟缺少运行环境。\nset -euo pipefail\necho 'blocked_environment: environment/qa-account=secret-value'\nexit 1\n"
	if err := os.WriteFile(testPath, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	state := qualityLoopState("audit_1")
	state.RunID = newRunID()
	state.ChangeName = changeName
	if err := snapshotQualityLoopAcceptance(repo, &state, acceptanceSource); err != nil {
		t.Fatal(err)
	}
	engine := NewEngine(repo, NewAgentRegistry())
	passed, err := engine.runAcceptanceGate(context.Background(), &state)
	if err != nil || passed {
		t.Fatalf("environment acceptance = passed:%v err:%v", passed, err)
	}
	if state.Status != statusBlockedEnvironment ||
		strings.Join(state.QualityLoop.MissingEnvironmentNames, ",") != "environment/qa-account" ||
		strings.Contains(state.Error, "secret-value") {
		t.Fatalf("environment state = %s names=%#v error=%q", state.Status, state.QualityLoop.MissingEnvironmentNames, state.Error)
	}
}

// TestQualityLoopRepairArtifactEnvironmentMarkerBlocks verifies a normal successful agent turn can pause.
func TestQualityLoopRepairArtifactEnvironmentMarkerBlocks(t *testing.T) {
	repo, changeName, acceptanceSource, _, _ := newRepairEvidenceFixture(t)
	state := qualityLoopState("audit_1")
	state.RunID = newRunID()
	state.ChangeName = changeName
	state.Sessions = map[string]string{}
	state.Paths = map[string]string{}
	if err := snapshotQualityLoopAcceptance(repo, &state, acceptanceSource); err != nil {
		t.Fatal(err)
	}
	artifact := cleanReviewForStageDecision()
	artifact.Decision = "needs_more"
	artifact.Findings = []Finding{blockingFindingForStageDecision()}
	artifact.Evidence = []string{"API_TOKEN=repair-prefix-secret blocked_environment: environment/browser-account=secret-value"}
	artifactPath := filepath.Join(runDir(repo, state.RunID), "audit-1.json")
	if err := writeJSONFile(artifactPath, artifact); err != nil {
		t.Fatal(err)
	}
	engine := NewEngine(repo, NewAgentRegistry())
	result, err := engine.completeMainStage(context.Background(), &state, stageGatePipelineLoop)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Blocked || state.Status != statusBlockedEnvironment ||
		strings.Join(state.QualityLoop.MissingEnvironmentNames, ",") != "environment/browser-account" {
		t.Fatalf("artifact environment result=%#v state=%s names=%#v", result, state.Status, state.QualityLoop.MissingEnvironmentNames)
	}
	persistedArtifact, err := os.ReadFile(artifactPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(persistedArtifact), "secret-value") ||
		strings.Contains(string(persistedArtifact), "repair-prefix-secret") ||
		!strings.Contains(string(persistedArtifact), "blocked_environment: environment/browser-account") {
		t.Fatalf("repair artifact was not safely redacted: %s", persistedArtifact)
	}
	if err := saveState(repo, state); err != nil {
		t.Fatal(err)
	}
	persistedState, err := os.ReadFile(filepath.Join(runDir(repo, state.RunID), "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(persistedState), "secret-value") {
		t.Fatalf("environment state leaked repair marker value: %s", persistedState)
	}
}

// TestQualityLoopRepairFindingTitleEnvironmentMarkerBlocks proves every blocking finding field reaches the gate.
func TestQualityLoopRepairFindingTitleEnvironmentMarkerBlocks(t *testing.T) {
	repo, changeName, acceptanceSource, _, _ := newRepairEvidenceFixture(t)
	state := qualityLoopState("audit_1")
	state.RunID = newRunID()
	state.ChangeName = changeName
	if err := snapshotQualityLoopAcceptance(repo, &state, acceptanceSource); err != nil {
		t.Fatal(err)
	}
	artifact := cleanReviewForStageDecision()
	artifact.Decision = "needs_more"
	finding := blockingFindingForStageDecision()
	finding.Title = "blocked_environment: TITLE_ONLY_TOKEN=title-secret"
	artifact.Findings = []Finding{finding}
	artifact.NonBlockingFindings = []Finding{{
		Title:          "blocked_environment: HISTORICAL_TOKEN=historical-secret",
		Severity:       "minor",
		Evidence:       "unrelated historical debt",
		Recommendation: "track separately",
		Scope:          findingScopeOutOfScope,
	}}
	artifactPath := filepath.Join(runDir(repo, state.RunID), "audit-1.json")
	if err := writeJSONFile(artifactPath, artifact); err != nil {
		t.Fatal(err)
	}

	engine := NewEngine(repo, NewAgentRegistry())
	result, err := engine.completeMainStage(context.Background(), &state, stageGatePipelineLoop)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Blocked || state.Status != statusBlockedEnvironment ||
		strings.Join(state.QualityLoop.MissingEnvironmentNames, ",") != "TITLE_ONLY_TOKEN" {
		t.Fatalf("title-only environment result=%#v state=%s names=%#v", result, state.Status, state.QualityLoop.MissingEnvironmentNames)
	}
	persistedArtifact, err := os.ReadFile(artifactPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(persistedArtifact), "title-secret") ||
		strings.Contains(string(persistedArtifact), "historical-secret") {
		t.Fatalf("title-only repair marker leaked values: %s", persistedArtifact)
	}
}

// TestQualityLoopRetryPromptKeepsTargetedScope verifies gate failure context does not erase QA scope.
func TestQualityLoopRetryPromptKeepsTargetedScope(t *testing.T) {
	repo, changeName, acceptanceSource, _, _ := newRepairEvidenceFixture(t)
	state := qualityLoopState("targeted_repair_2")
	state.RunID = newRunID()
	state.ChangeName = changeName
	state.BaselineHead = "baseline-head"
	if err := snapshotRunPrompts(repo, state.RunID); err != nil {
		t.Fatal(err)
	}
	if err := snapshotQualityLoopAcceptance(repo, &state, acceptanceSource); err != nil {
		t.Fatal(err)
	}
	sourceQA := cleanRepairDAGQA()
	sourceQA.Decision = "needs_fix"
	sourceQA.Findings = []Finding{blockingFindingForStageDecision()}
	sourceQA.AcceptanceMatrix[0].Status = "failed"
	if err := writeJSONFile(filepath.Join(runDir(repo, state.RunID), "qa-2.json"), sourceQA); err != nil {
		t.Fatal(err)
	}
	state.QualityLoop.SourceQAArtifact = "qa-2.json"
	state.QualityLoop.SourceQAHash = qaArtifactContentHash(sourceQA)
	state.QualityLoop.FindingFingerprint = qaFindingFingerprint(sourceQA)
	trustQualityLoopSourceQAForPrompt(t, repo, &state, "qa_2", "audit_1")
	state.Validation[state.Stage] = StageValidationState{
		Status: validationStatusFailed, Kind: validationKindCommands,
		LastArtifact: filepath.Join(runDir(repo, state.RunID), "validation-targeted-repair-2-1.json"), LastError: "go test failed",
	}
	if err := os.WriteFile(state.Validation[state.Stage].LastArtifact, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	prompt, err := promptForStage(repo, state)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"qa_targeted_repair", "targeted-repair-2.json", "qa-2.json", "baseline-head"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("retry prompt missing %q:\n%s", want, prompt)
		}
	}
}

// TestNeedsFixQAMatrixMustCoverSealedContract rejects arbitrary or omitted failed IDs.
func TestNeedsFixQAMatrixMustCoverSealedContract(t *testing.T) {
	path := filepath.Join(t.TempDir(), "acceptance.json")
	if err := os.WriteFile(path, []byte(repairDAGAcceptanceJSON()), 0o644); err != nil {
		t.Fatal(err)
	}
	contract, err := ReadAcceptance(path)
	if err != nil {
		t.Fatal(err)
	}
	qa := cleanQAForStageDecision()
	qa.Decision = "needs_fix"
	qa.Findings = []Finding{blockingFindingForStageDecision()}
	qa.AcceptanceMatrix = []AcceptanceResult{{ID: "invented-test", Status: "failed", Evidence: "failed"}}
	if err := ValidateQAAgainstAcceptance(qa, contract); err == nil {
		t.Fatal("needs_fix QA accepted an arbitrary incomplete matrix")
	}
}

// TestQualityLoopGateSnapshotRejectsPostTestDiff verifies gates bind results to their tracked snapshot.
func TestQualityLoopGateSnapshotRejectsPostTestDiff(t *testing.T) {
	repo, changeName, _, _, _ := newRepairEvidenceFixture(t)
	state := qualityLoopState("audit_1")
	state.ChangeName = changeName
	state.AcceptanceRun[state.Stage] = StageValidationState{Status: validationStatusPassed}
	content, err := gitChangeContentSnapshot(repo)
	if err != nil {
		t.Fatal(err)
	}
	state.QualityLoop.DiffHash = qualityHashStrings(content)
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("gate changed tracked source\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	engine := NewEngine(repo, NewAgentRegistry())
	bound, err := engine.bindQualityStageGateSnapshot(&state)
	if err != nil || bound {
		t.Fatalf("changed diff bound=%v err=%v", bound, err)
	}
	if state.AcceptanceRun["audit_1"].Status != validationStatusFailed {
		t.Fatalf("diff mismatch acceptance state = %#v", state.AcceptanceRun["audit_1"])
	}
}

// TestQualityLoopGateStallUsesGateProgress verifies resume checks the gate failure baseline.
func TestQualityLoopGateStallUsesGateProgress(t *testing.T) {
	repo, changeName, acceptanceSource, head, diff := newRepairEvidenceFixture(t)
	state := qualityLoopState(statusBlockedStalled)
	state.RunID = newRunID()
	state.ChangeName = changeName
	state.Status = statusBlockedStalled
	state.BaselineHead = head
	state.BaselineDiff = diff
	state.QualityLoop.BlockedFromStage = "audit_2"
	content, err := gitChangeContentSnapshot(repo)
	if err != nil {
		t.Fatal(err)
	}
	state.QualityLoop.DiffHash = qualityHashStrings(content)
	if err := snapshotQualityLoopAcceptance(repo, &state, acceptanceSource); err != nil {
		t.Fatal(err)
	}
	state.QualityLoop.EvidenceHash, err = qualityCurrentEvidenceHash(repo, state)
	if err != nil {
		t.Fatal(err)
	}
	state.QualityLoop.ProgressHash = "unrelated-finding-progress"
	state.QualityLoop.GateFailureFingerprint = qualityHashStrings(validationKindCommands, "failed")
	state.QualityLoop.GateProgressHash = qualityProgressHash(state)

	engine := NewEngine(repo, NewAgentRegistry())
	if err := engine.prepareQualityLoopResume(&state, false); err == nil {
		t.Fatal("unchanged gate failure must remain blocked_stalled")
	}
}

// TestQualityProgressHashIgnoresVolatileGateArtifacts separates trust hashes from progress.
func TestQualityProgressHashIgnoresVolatileGateArtifacts(t *testing.T) {
	first := qualityLoopState("audit_2")
	first.QualityLoop = QualityLoopState{
		DiffHash:               qualityHashStrings("same-source"),
		TestsHash:              qualityHashStrings("go-test-duration=0.003s"),
		ValidationHash:         qualityHashStrings("validation-attempt=1"),
		EvidenceHash:           qualityHashStrings("run-id=first"),
		TestsProgressHash:      qualityHashStrings("test-a", "failed", "exit=1"),
		ValidationProgressHash: qualityHashStrings("validation-a", "exit=1"),
		EvidenceProgressHash:   qualityHashStrings("evidence-a", "present"),
	}
	second := first
	second.QualityLoop.TestsHash = qualityHashStrings("go-test-duration=0.004s")
	second.QualityLoop.ValidationHash = qualityHashStrings("validation-attempt=2")
	second.QualityLoop.EvidenceHash = qualityHashStrings("run-id=second")
	if qualityProgressHash(first) != qualityProgressHash(second) {
		t.Fatal("volatile gate artifacts incorrectly counted as repair progress")
	}
	second.QualityLoop.TestsProgressHash = qualityHashStrings("test-a", "passed", "exit=0")
	if qualityProgressHash(first) == qualityProgressHash(second) {
		t.Fatal("semantic test outcome change did not count as repair progress")
	}
}

// TestQualityFailureFingerprintsIgnoreVolatileLogs keeps identical failures adjacent.
func TestQualityFailureFingerprintsIgnoreVolatileLogs(t *testing.T) {
	first := ValidationAttempt{Commands: []ValidationCommandResult{{
		Command: "go test ./...", ExitCode: 1, Output: "FAIL package 0.003s\n",
	}}}
	second := first
	second.Commands = []ValidationCommandResult{{
		Command: "go test ./...", ExitCode: 1, Output: "FAIL package 0.004s\n",
	}}
	if qualityValidationFailureKey(first, "go test ./... exited 1") !=
		qualityValidationFailureKey(second, "go test ./... exited 1") {
		t.Fatal("volatile validation output changed the failure fingerprint")
	}
	offsetFirst := ValidationAttempt{Commands: []ValidationCommandResult{{
		Command: "check", ExitCode: 1, Output: "failed at 2026-07-27T16:10:01.123+08:00\n",
	}}}
	offsetSecond := ValidationAttempt{Commands: []ValidationCommandResult{{
		Command: "check", ExitCode: 1, Output: "failed at 2026-07-27T03:10:02.456-05:00\n",
	}}}
	if qualityValidationFailureKey(offsetFirst, "check exited 1") !=
		qualityValidationFailureKey(offsetSecond, "check exited 1") {
		t.Fatal("RFC3339 offset timestamps changed the failure fingerprint")
	}
	resultOne := AcceptanceRunResult{Tests: []AcceptanceRunTestResult{{
		ID: "app", Status: validationStatusFailed, ExitCode: 1,
		LogHash: "duration-0.003s", LogProgressHash: acceptanceLogProgressHash("FAIL package 0.003s\n"),
	}}}
	resultTwo := resultOne
	resultTwo.Tests = []AcceptanceRunTestResult{{
		ID: "app", Status: validationStatusFailed, ExitCode: 1,
		LogHash: "duration-0.004s", LogProgressHash: acceptanceLogProgressHash("FAIL package 0.004s\n"),
	}}
	if acceptanceGateFailureKey(resultOne, errors.New("failed")) !=
		acceptanceGateFailureKey(resultTwo, errors.New("failed")) {
		t.Fatal("volatile required-test log changed the acceptance failure fingerprint")
	}
}

// TestQualityLoopResumeChecksLockBeforeUnblocking preserves recoverable state under an active worker.
func TestQualityLoopResumeChecksLockBeforeUnblocking(t *testing.T) {
	for _, detached := range []bool{false, true} {
		t.Run(fmt.Sprintf("detached=%v", detached), func(t *testing.T) {
			repo, changeName, _, _, _ := newRepairEvidenceFixture(t)
			missingName := "environment/resume-lock-account"
			missingPath := filepath.Join(repo, filepath.FromSlash(missingName))
			if err := os.MkdirAll(filepath.Dir(missingPath), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(missingPath, []byte("ready"), 0o600); err != nil {
				t.Fatal(err)
			}
			state := qualityLoopState("audit_2")
			state.RunID = newRunID()
			state.ChangeName = changeName
			if err := blockQualityEnvironment(repo, &state, []string{missingName}); err != nil {
				t.Fatal(err)
			}
			if err := saveState(repo, state); err != nil {
				t.Fatal(err)
			}
			unlock, err := acquireLock(repo, state.RunID)
			if err != nil {
				t.Fatal(err)
			}
			defer unlock()

			engine := NewEngine(repo, NewAgentRegistry())
			if detached {
				err = engine.ResumeDetachedAfterUserChoice(context.Background(), state.RunID)
			} else {
				err = engine.ResumeRunJSON(context.Background(), state.RunID, io.Discard)
			}
			if !isRunLockedError(err) {
				t.Fatalf("resume error = %v, want active lock", err)
			}
			persisted, err := loadState(repo, state.RunID)
			if err != nil {
				t.Fatal(err)
			}
			if persisted.Status != statusBlockedEnvironment ||
				persisted.Stage != statusBlockedEnvironment ||
				persisted.QualityLoop.BlockedFromStage != "audit_2" ||
				len(persisted.QualityLoop.MissingEnvironmentNames) != 1 {
				t.Fatalf("active lock mutated blocked state: %#v", persisted)
			}
		})
	}
}

// TestQualityLoopBatchPreservesRecoverableRun verifies batch restart keeps the same paused run.
func TestQualityLoopBatchPreservesRecoverableRun(t *testing.T) {
	repo, changeName, acceptanceSource, _, _ := newRepairEvidenceFixture(t)
	missingName := "environment/batch-account"
	state := qualityLoopState("audit_3")
	state.RunID = newRunID()
	state.ChangeName = changeName
	state.BatchID = newRunID()
	if err := blockQualityEnvironment(repo, &state, []string{missingName}); err != nil {
		t.Fatal(err)
	}
	if err := saveState(repo, state); err != nil {
		t.Fatal(err)
	}
	if err := snapshotQualityLoopAcceptance(repo, &state, acceptanceSource); err != nil {
		t.Fatal(err)
	}
	if err := saveState(repo, state); err != nil {
		t.Fatal(err)
	}
	batch := BatchState{
		BatchID: state.BatchID, Status: batchStatusRunning, Changes: []string{changeName},
		RunIDs: map[string]string{changeName: state.RunID},
	}
	if err := saveBatchState(repo, batch); err != nil {
		t.Fatal(err)
	}
	registry := NewAgentRegistry()
	registry.Register(fakeWorkflowTool{runner: &fakeWorkflowRunner{}})
	engine := NewEngine(repo, registry)
	if err := engine.RunBatch(context.Background(), batch.BatchID); err != nil {
		t.Fatal(err)
	}
	pausedBatch, err := loadBatchState(repo, batch.BatchID)
	if err != nil {
		t.Fatal(err)
	}
	if pausedBatch.Status != batchStatusRunning || pausedBatch.RunIDs[changeName] != state.RunID {
		t.Fatalf("paused batch = %#v", pausedBatch)
	}
	missingPath := filepath.Join(repo, filepath.FromSlash(missingName))
	if err := os.MkdirAll(filepath.Dir(missingPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(missingPath, []byte("ready"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := engine.prepareRestartBatch(batch.BatchID, false); err != nil {
		t.Fatal(err)
	}
	resumed, err := loadState(repo, state.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if resumed.Status != statusRunning || resumed.Stage != "audit_3" || !resumed.QualityLoop.ResumeRerunPending {
		t.Fatalf("resumed batch run = %s/%s quality=%#v", resumed.Status, resumed.Stage, resumed.QualityLoop)
	}
}

// TestQualityLoopManualRestartResumesStalledRun verifies explicit restart is accepted as new instruction.
func TestQualityLoopManualRestartResumesStalledRun(t *testing.T) {
	repo, changeName, acceptanceSource, head, diff := newRepairEvidenceFixture(t)
	state := qualityLoopState(statusBlockedStalled)
	state.RunID = newRunID()
	state.ChangeName = changeName
	state.Status = statusBlockedStalled
	state.BaselineHead = head
	state.BaselineDiff = diff
	state.QualityLoop.BlockedFromStage = "audit_2"
	content, err := gitChangeContentSnapshot(repo)
	if err != nil {
		t.Fatal(err)
	}
	state.QualityLoop.DiffHash = qualityHashStrings(content)
	if err := snapshotQualityLoopAcceptance(repo, &state, acceptanceSource); err != nil {
		t.Fatal(err)
	}
	state.QualityLoop.EvidenceHash, err = qualityCurrentEvidenceHash(repo, state)
	if err != nil {
		t.Fatal(err)
	}
	state.QualityLoop.ProgressHash = qualityProgressHash(state)
	if err := saveState(repo, state); err != nil {
		t.Fatal(err)
	}
	registry := NewAgentRegistry()
	registry.Register(fakeWorkflowTool{runner: &fakeWorkflowRunner{}})
	engine := NewEngine(repo, registry)
	resumed, err := engine.prepareRestartRun(state.RunID, false)
	if err != nil {
		t.Fatal(err)
	}
	if resumed.Status != statusRunning || resumed.Stage != "audit_2" || !resumed.QualityLoop.ResumeRerunPending {
		t.Fatalf("manual restart = %s/%s quality=%#v", resumed.Status, resumed.Stage, resumed.QualityLoop)
	}
}

// qualityUnboundedRunner writes twelve failed QA rounds followed by one clean QA and archive.
type qualityUnboundedRunner struct {
	repo       string
	runID      string
	changeName string
}

// Run writes real dynamic artifacts and tracked repair progress for the production engine loop.
func (r *qualityUnboundedRunner) Run(_ context.Context, _ string, prompt string, _ string, _ StageOptions) (string, error) {
	base := runDir(r.repo, r.runID)
	if strings.Contains(prompt, "写入：`audit-1.json`（相对运行目录）") {
		audit := cleanReviewForStageDecision()
		audit.Evidence = []string{"go test ./internal/app; runtime audit passed"}
		return "shared-repair", writeJSONFile(filepath.Join(base, "audit-1.json"), audit)
	}
	for iteration := 1; iteration <= 12; iteration++ {
		repairPath := filepath.Join(base, fmt.Sprintf("targeted-repair-%d.json", iteration))
		if strings.Contains(prompt, fmt.Sprintf("写入：`targeted-repair-%d.json`（相对运行目录）", iteration)) {
			progress := filepath.Join(r.repo, "README.md")
			if err := os.WriteFile(progress, []byte(fmt.Sprintf("repair progress %d\n", iteration)), 0o644); err != nil {
				return "", err
			}
			repair := cleanReviewForStageDecision()
			repair.Evidence = []string{"go test ./internal/app; runtime targeted repair passed"}
			return "shared-repair", writeJSONFile(repairPath, repair)
		}
	}
	for iteration := 1; iteration <= 13; iteration++ {
		qaPath := filepath.Join(base, fmt.Sprintf("qa-%d.json", iteration))
		if !strings.Contains(prompt, fmt.Sprintf("写入（相对运行目录）：`qa-%d.json`", iteration)) {
			continue
		}
		qa := cleanRepairDAGQA()
		if iteration <= 12 {
			qa.Decision = "needs_fix"
			finding := blockingFindingForStageDecision()
			finding.Title = "same QA finding"
			qa.Findings = []Finding{finding}
			qa.AcceptanceMatrix[0].Status = "failed"
		}
		return fmt.Sprintf("qa-session-%d", iteration), writeJSONFile(qaPath, qa)
	}
	if strings.Contains(prompt, "delivery-summary") {
		return "archive-session", archiveRepairEvidence(r.repo, r.runID, r.changeName)
	}
	return "execution-session", nil
}

// runRealUnboundedQualityLoop executes the production engine through twelve persisted QA failures.
func runRealUnboundedQualityLoop(t *testing.T) qualityDeliveryLoopResult {
	t.Helper()
	repo, changeName, acceptanceSource, head, diff := newRepairEvidenceFixture(t)
	runID := newRunID()
	state := State{
		RunID: runID, ChangeName: changeName, Sealed: true, Status: statusRunning,
		Stage: workflowStageExecution, BaselineHead: head, BaselineDiff: diff,
		Workflow: DefaultWorkflowConfig(), Sessions: map[string]string{}, Stages: map[string]string{},
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
	registry := NewAgentRegistry()
	registry.Register(fakeWorkflowTool{runner: &qualityUnboundedRunner{repo: repo, runID: runID, changeName: changeName}})
	engine := NewEngine(repo, registry)
	if err := engine.runGoDAGLocked(context.Background(), state); err != nil {
		t.Fatal(err)
	}
	completed, err := loadState(repo, runID)
	if err != nil {
		t.Fatal(err)
	}
	if completed.Status != statusDone || completed.Stage != workflowStageDone ||
		completed.Stages["targeted_repair_12"] != "completed" || completed.Stages["qa_13"] != "completed" {
		t.Fatalf("unbounded real loop = %s/%s stages=%#v", completed.Status, completed.Stage, completed.Stages)
	}
	return qualityDeliveryLoopResult{Repo: repo, State: completed}
}
