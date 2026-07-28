// Package app tests security and progress guards for recoverable quality-loop blocks.
package app

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

var errRecoveryGuardObserved = errors.New("recovery guard reached stage runner")
var errRecoveryBackendUnavailable = errors.New("recovery backend unavailable")
var errRecoveryStartupWrite = errors.New("recovery startup writer failed")
var errRecoveryDetachedStart = errors.New("recovery detached worker failed to start")
var errRecoveryDetachedHandoff = errors.New("recovery detached worker handoff failed")

type recoveryGuardRunner struct {
	called bool
}

// runRecoveryGit executes one Git setup command for a temporary recovery fixture.
func runRecoveryGit(t *testing.T, repo string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = repo
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
	}
}

// Run records that the run-loop intervention guard allowed the resumed stage to execute.
func (r *recoveryGuardRunner) Run(context.Context, string, string, string, StageOptions) (string, error) {
	r.called = true
	return "", errRecoveryGuardObserved
}

type recoveryEnvironmentBlockRunner struct{}

// Run returns an explicit environment marker without writing a stage artifact.
func (recoveryEnvironmentBlockRunner) Run(context.Context, string, string, string, StageOptions) (string, error) {
	return "", errors.New("blocked_environment: LEADING_SECRET_FRAGMENT,TEST_RECOVERY_ACCOUNT=secret")
}

type recoveryFailingWriter struct{}

// Write fails before a worker can own the resumed run.
func (recoveryFailingWriter) Write([]byte) (int, error) {
	return 0, errRecoveryStartupWrite
}

// TestQualityEnvironmentMarkerRedactionCoversValidationArtifact verifies secrets never reach durable gate JSON.
func TestQualityEnvironmentMarkerRedactionCoversValidationArtifact(t *testing.T) {
	repo := t.TempDir()
	t.Setenv("XDG_STATE_HOME", filepath.Join(repo, "state"))
	attempt := ValidationAttempt{
		Stage:   "audit_1",
		Attempt: 1,
		Status:  validationStatusFailed,
		Commands: []ValidationCommandResult{{
			Command:  "API_TOKEN=prefix-command-secret echo blocked_environment: API_TOKEN=command-secret",
			ExitCode: 1,
			Output: "before\nAPI_TOKEN=prefix-output-secret blocked_environment: LEADING_FRAGMENT,credentials.json=output-secret," +
				"MIDDLE_FRAGMENT,SECOND_TOKEN=second-secret,TRAILING_FRAGMENT\nafter",
		}},
	}
	path, err := writeValidationAttempt(repo, "redaction-run", attempt)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	for _, secret := range []string{
		"prefix-command-secret",
		"command-secret",
		"prefix-output-secret",
		"LEADING_FRAGMENT",
		"output-secret",
		"MIDDLE_FRAGMENT",
		"second-secret",
		"TRAILING_FRAGMENT",
	} {
		if strings.Contains(body, secret) {
			t.Fatalf("validation artifact leaked %q: %s", secret, body)
		}
	}
	var persisted ValidationAttempt
	if err := json.Unmarshal(data, &persisted); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(persisted.Commands[0].Command, "API_TOKEN") ||
		!strings.Contains(persisted.Commands[0].Output, "credentials.json") {
		t.Fatalf("redacted diagnostics lost prerequisite names: %#v", persisted.Commands[0])
	}
}

// TestQualityEnvironmentMarkerParserDropsCommaSecretFragments verifies values cannot become state diagnostics.
func TestQualityEnvironmentMarkerParserDropsCommaSecretFragments(t *testing.T) {
	names := qualityEnvironmentNamesFromText(
		"blocked_environment: LEADING_SECRET_FRAGMENT,API_TOKEN=first-secret," +
			"MIDDLE_SECRET_FRAGMENT,credentials.json=second-secret,TRAILING_SECRET_FRAGMENT",
	)
	if got := strings.Join(names, ","); got != "API_TOKEN,credentials.json" {
		t.Fatalf("environment names = %q, want only declared identifiers", got)
	}
	state := qualityLoopState("audit_1")
	setQualityEnvironmentBlock(&state, names, "")
	stateJSON, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{
		"LEADING_SECRET_FRAGMENT",
		"first-secret",
		"MIDDLE_SECRET_FRAGMENT",
		"second-secret",
		"TRAILING_SECRET_FRAGMENT",
	} {
		if strings.Contains(string(stateJSON), secret) {
			t.Fatalf("environment state leaked %q: %s", secret, stateJSON)
		}
	}
}

// TestQualityEnvironmentAvailableDistinguishesBasenamePathsAndENV verifies untyped markers remain deterministic.
func TestQualityEnvironmentAvailableDistinguishesBasenamePathsAndENV(t *testing.T) {
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "credentials.json"), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !qualityEnvironmentAvailable(repo, "credentials.json") {
		t.Fatal("repository-relative basename prerequisite was not detected")
	}

	const envName = "OZ_RECOVERY_HARDENING_TOKEN"
	t.Setenv(envName, "")
	if err := os.WriteFile(filepath.Join(repo, envName), []byte("not-an-env-value\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if qualityEnvironmentAvailable(repo, envName) {
		t.Fatal("conventional ENV marker was incorrectly satisfied by a same-named repository file")
	}
	t.Setenv(envName, "ready")
	if !qualityEnvironmentAvailable(repo, envName) {
		t.Fatal("non-empty conventional ENV marker was not detected")
	}
}

// TestQualityProgressHashIgnoresBareHeadChanges verifies an empty commit cannot count as repair progress.
func TestQualityProgressHashIgnoresBareHeadChanges(t *testing.T) {
	state := qualityLoopState("audit_2")
	state.BaselineHead = "before-empty-commit"
	state.BaselineDiff = " M internal/app/example.go"
	state.QualityLoop.TestsHash = "tests"
	state.QualityLoop.ValidationHash = "validation"
	state.QualityLoop.EvidenceHash = "evidence"
	before := qualityProgressHash(state)
	state.BaselineHead = "after-empty-commit"
	if after := qualityProgressHash(state); after != before {
		t.Fatalf("bare HEAD change altered progress hash: before=%s after=%s", before, after)
	}
}

// TestStalledResumeRejectsEmptyCommit verifies bare repository history cannot unblock a paused failure.
func TestStalledResumeRejectsEmptyCommit(t *testing.T) {
	repo, changeName, acceptanceSource, head, diff := newRepairEvidenceFixture(t)
	state := qualityLoopState(statusBlockedStalled)
	state.RunID = newRunID()
	state.ChangeName = changeName
	state.Status = statusBlockedStalled
	state.BaselineHead = head
	state.BaselineDiff = diff
	state.QualityLoop.BlockedFromStage = "qa_1"
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

	cmd := exec.Command("git", "commit", "--allow-empty", "-q", "-m", "empty progress")
	cmd.Dir = repo
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("create empty commit: %v\n%s", err, output)
	}
	engine := NewEngine(repo, NewAgentRegistry())
	if err := engine.prepareQualityLoopResume(&state, false); err == nil {
		t.Fatal("empty commit unexpectedly unblocked stalled run")
	}
	if state.Status != statusBlockedStalled || state.Stage != statusBlockedStalled {
		t.Fatalf("empty commit mutated stalled state: %s/%s", state.Status, state.Stage)
	}
}

// TestGitChangeContentSnapshotTracksEffectiveContent verifies commits do not change snapshot semantics.
func TestGitChangeContentSnapshotTracksEffectiveContent(t *testing.T) {
	repo, _, _, _, _ := newRepairEvidenceFixture(t)
	before, err := gitChangeContentSnapshot(repo)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("new source content\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	changed, err := gitChangeContentSnapshot(repo)
	if err != nil {
		t.Fatal(err)
	}
	if changed == before {
		t.Fatal("effective source content change did not alter snapshot")
	}
	cmd := exec.Command("git", "add", "README.md")
	cmd.Dir = repo
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("stage source: %v\n%s", err, output)
	}
	cmd = exec.Command("git", "commit", "-q", "-m", "commit existing source")
	cmd.Dir = repo
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("commit source: %v\n%s", err, output)
	}
	committed, err := gitChangeContentSnapshot(repo)
	if err != nil {
		t.Fatal(err)
	}
	if committed != changed {
		t.Fatal("committing unchanged worktree content altered the effective snapshot")
	}
	cmd = exec.Command("git", "commit", "--allow-empty", "-q", "-m", "empty metadata")
	cmd.Dir = repo
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("empty commit: %v\n%s", err, output)
	}
	emptyCommitted, err := gitChangeContentSnapshot(repo)
	if err != nil {
		t.Fatal(err)
	}
	if emptyCommitted != committed {
		t.Fatal("empty commit altered the effective snapshot")
	}
}

// TestGitChangeContentSnapshotPreservesWhitespacePaths hashes exact NUL-delimited Git paths.
func TestGitChangeContentSnapshotPreservesWhitespacePaths(t *testing.T) {
	repo, changeName, _, _, _ := newRepairEvidenceFixture(t)
	path := filepath.Join(repo, " tracked source.go ")
	if err := os.WriteFile(path, []byte("package tracked\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runRecoveryGit(t, repo, "add", " tracked source.go ")
	runRecoveryGit(t, repo, "commit", "-q", "-m", "add whitespace source")
	before, err := gitChangeContentSnapshotForChange(repo, changeName)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("package changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	after, err := gitChangeContentSnapshotForChange(repo, changeName)
	if err != nil {
		t.Fatal(err)
	}
	if before == after {
		t.Fatal("source change at a whitespace-bearing Git path escaped the snapshot")
	}
}

// TestGitChangeContentSnapshotDetectsDirtySubmodule hashes recursive submodule worktree content.
func TestGitChangeContentSnapshotDetectsDirtySubmodule(t *testing.T) {
	repo, changeName, _, _, _ := newRepairEvidenceFixture(t)
	child := filepath.Join(t.TempDir(), "child")
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatal(err)
	}
	runRecoveryGit(t, child, "init", "-q")
	runRecoveryGit(t, child, "config", "user.name", "fixture")
	runRecoveryGit(t, child, "config", "user.email", "fixture@example.com")
	childSource := filepath.Join(child, "source.go")
	if err := os.WriteFile(childSource, []byte("package child\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runRecoveryGit(t, child, "add", "source.go")
	runRecoveryGit(t, child, "commit", "-q", "-m", "child source")
	runRecoveryGit(t, repo, "-c", "protocol.file.allow=always", "submodule", "add", "-q", child, "vendor/child")
	runRecoveryGit(t, repo, "commit", "-qam", "add child module")
	before, err := gitChangeContentSnapshotForChange(repo, changeName)
	if err != nil {
		t.Fatal(err)
	}
	checkedOutSource := filepath.Join(repo, "vendor", "child", "source.go")
	if err := os.WriteFile(checkedOutSource, []byte("package dirtychild\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	after, err := gitChangeContentSnapshotForChange(repo, changeName)
	if err != nil {
		t.Fatal(err)
	}
	if before == after {
		t.Fatal("dirty submodule source escaped the effective snapshot")
	}
}

// TestGitChangeContentSnapshotIgnoresSiblingProposal keeps concurrent proposal work out of QA identity.
func TestGitChangeContentSnapshotIgnoresSiblingProposal(t *testing.T) {
	repo, changeName, _, _, _ := newRepairEvidenceFixture(t)
	before, err := gitChangeContentSnapshotForChange(repo, changeName)
	if err != nil {
		t.Fatal(err)
	}
	sibling := filepath.Join(repo, "docs", "changes", "2-并行提案", "brief.md")
	if err := os.MkdirAll(filepath.Dir(sibling), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sibling, []byte("# unrelated proposal\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	afterSibling, err := gitChangeContentSnapshotForChange(repo, changeName)
	if err != nil {
		t.Fatal(err)
	}
	if afterSibling != before {
		t.Fatal("sibling active proposal changed the current run source identity")
	}
	current := filepath.Join(repo, "docs", "changes", changeName, "implementation-note.md")
	if err := os.WriteFile(current, []byte("# current proposal\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	afterCurrent, err := gitChangeContentSnapshotForChange(repo, changeName)
	if err != nil {
		t.Fatal(err)
	}
	if afterCurrent == before {
		t.Fatal("current proposal content was incorrectly excluded from source identity")
	}
}

// TestStalledResumeRejectsCommitOfExistingDiff prevents metadata-only representation progress.
func TestStalledResumeRejectsCommitOfExistingDiff(t *testing.T) {
	repo, changeName, acceptanceSource, head, diff := newRepairEvidenceFixture(t)
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("pending repair content\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	state := qualityLoopState(statusBlockedStalled)
	state.RunID = newRunID()
	state.ChangeName = changeName
	state.Status = statusBlockedStalled
	state.BaselineHead = head
	state.BaselineDiff = diff
	state.QualityLoop.BlockedFromStage = "qa_1"
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

	cmd := exec.Command("git", "add", "README.md")
	cmd.Dir = repo
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("stage existing diff: %v\n%s", err, output)
	}
	cmd = exec.Command("git", "commit", "-q", "-m", "commit existing diff")
	cmd.Dir = repo
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("commit existing diff: %v\n%s", err, output)
	}
	engine := NewEngine(repo, NewAgentRegistry())
	if err := engine.prepareQualityLoopResume(&state, false); err == nil {
		t.Fatal("committing unchanged worktree content unexpectedly unblocked stalled run")
	}
	if state.Status != statusBlockedStalled || state.Stage != statusBlockedStalled {
		t.Fatalf("commit-existing-diff mutated stalled state: %s/%s", state.Status, state.Stage)
	}
}

// TestQABlockedResumeWithSourceProgressRoutesToTargetedRepair reruns deterministic gates.
func TestQABlockedResumeWithSourceProgressRoutesToTargetedRepair(t *testing.T) {
	repo, changeName, acceptanceSource, head, diff := newRepairEvidenceFixture(t)
	state := qualityLoopState(statusBlockedStalled)
	state.RunID = newRunID()
	state.ChangeName = changeName
	state.Status = statusBlockedStalled
	state.BaselineHead = head
	state.BaselineDiff = diff
	state.QualityLoop.BlockedFromStage = "qa_2"
	content, err := gitChangeContentSnapshot(repo)
	if err != nil {
		t.Fatal(err)
	}
	state.QualityLoop.DiffHash = qualityHashStrings(content)
	if err := snapshotQualityLoopAcceptance(repo, &state, acceptanceSource); err != nil {
		t.Fatal(err)
	}
	state.Stages["audit_1"] = "completed"
	recordPassedQualityLoopCheckpoint(t, repo, &state, "audit_1")
	state.QualityLoop.EvidenceHash, err = qualityCurrentEvidenceHash(repo, state)
	if err != nil {
		t.Fatal(err)
	}
	qa := cleanRepairDAGQA()
	qa.Decision = "needs_fix"
	qa.Findings = []Finding{blockingFindingForStageDecision()}
	qa.AcceptanceMatrix[0].Status = "failed"
	if err := writeJSONFile(filepath.Join(runDir(repo, state.RunID), "qa-2.json"), qa); err != nil {
		t.Fatal(err)
	}
	state.ArtifactGates = map[string]StageValidationState{
		"qa_2": {
			Kind: validationKindQAReadOnly, Status: validationStatusPassed,
			DiffHash:       state.QualityLoop.DiffHash,
			CheckpointHash: trustedQualityLoopCheckpointHash(t, repo, state, "audit_1"),
		},
	}
	state.QualityLoop.SourceQAArtifact = "qa-2.json"
	state.QualityLoop.SourceQAHash = qaArtifactContentHash(qa)
	state.QualityLoop.FindingFingerprint = qualityHashStrings("stale-prior-qa-finding")
	state.QualityLoop.ProgressHash = qualityProgressHash(state)
	probe := state
	probe.Stage = "qa_2"
	checkpoint, expectedHash, checkpointErr := qualityLoopQACheckpointDiffHash(probe)
	if checkpointErr != nil {
		t.Fatal(checkpointErr)
	}
	if state.ArtifactGates["qa_2"].DiffHash != expectedHash {
		t.Fatalf("trusted QA gate diff = %s, checkpoint %s = %s",
			state.ArtifactGates["qa_2"].DiffHash, checkpoint, expectedHash)
	}
	if err := verifyQualityLoopDurableCheckpoint(repo, probe, checkpoint); err != nil {
		t.Fatalf("trusted QA durable checkpoint rejected: %v", err)
	}
	if !qualityLoopTrustedSourceQA(repo, state, "qa_2", qa) {
		t.Fatal("production-shaped source QA was not trusted")
	}
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("new repair progress\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	engine := NewEngine(repo, NewAgentRegistry())
	if err := engine.prepareQualityLoopResume(&state, false); err != nil {
		t.Fatal(err)
	}
	if state.Status != statusRunning || state.Stage != "targeted_repair_2" ||
		state.QualityLoop.Mode != "qa_targeted_repair" ||
		state.QualityLoop.SourceQAArtifact != "qa-2.json" ||
		state.QualityLoop.SourceQAHash != qaArtifactContentHash(qa) ||
		state.QualityLoop.FindingFingerprint != qaFindingFingerprint(qa) ||
		!state.QualityLoop.ResumeRerunPending {
		t.Fatalf("QA progress resume = %s/%s quality=%#v", state.Status, state.Stage, state.QualityLoop)
	}
	if _, err := promptContext(repo, state); err != nil {
		t.Fatalf("resumed targeted repair prompt rejected refreshed QA baseline: %v", err)
	}
	checkpointPath := acceptanceTestArtifactPath(repo, state.AcceptanceRun[checkpoint].LastArtifact)
	checkpointData, err := os.ReadFile(checkpointPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(checkpointPath, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := promptContext(repo, state); err == nil {
		t.Fatal("targeted repair accepted a tampered durable QA checkpoint")
	}
	if err := os.WriteFile(checkpointPath, checkpointData, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := promptContext(repo, state); err != nil {
		t.Fatalf("targeted repair rejected restored durable QA checkpoint: %v", err)
	}
	var metadataDrift AcceptanceRunResult
	if err := decodeStrictArtifactJSON(checkpointData, &metadataDrift); err != nil {
		t.Fatal(err)
	}
	metadataDrift.StartedAt = "2099-01-01T00:00:00Z"
	if err := writeJSONFile(checkpointPath, metadataDrift); err != nil {
		t.Fatal(err)
	}
	if _, err := promptContext(repo, state); err == nil {
		t.Fatal("targeted repair accepted acceptance metadata outside the former partial checkpoint hash")
	}
	if err := os.WriteFile(checkpointPath, checkpointData, 0o644); err != nil {
		t.Fatal(err)
	}
	qa.Findings[0].Evidence += " tampered after QA"
	if err := writeJSONFile(filepath.Join(runDir(repo, state.RunID), "qa-2.json"), qa); err != nil {
		t.Fatal(err)
	}
	if _, err := promptContext(repo, state); err == nil {
		t.Fatal("targeted repair accepted source QA evidence drift")
	}
	legacyQA := cleanRepairDAGQA()
	legacyQA.Decision = "needs_fix"
	legacyQA.Findings = []Finding{blockingFindingForStageDecision()}
	legacyQA.AcceptanceMatrix[0].Status = "failed"
	if err := writeJSONFile(filepath.Join(runDir(repo, state.RunID), "qa-2.json"), legacyQA); err != nil {
		t.Fatal(err)
	}
	state.Stage = "targeted_repair_2"
	state.QualityLoop.SourceQAHash = qaArtifactContentHash(legacyQA)
	state.ArtifactGates["qa_2"] = StageValidationState{
		Kind: "artifact", Status: validationStatusPassed, DiffHash: state.QualityLoop.DiffHash,
	}
	if state.Sessions == nil {
		state.Sessions = map[string]string{}
	}
	state.Sessions["codex:repairer"] = "existing-repairer-session"
	state.Stages["targeted_repair_2"] = "running"
	if err := writeJSONFile(filepath.Join(runDir(repo, state.RunID), "targeted-repair-2.json"), cleanReviewForStageDecision()); err != nil {
		t.Fatal(err)
	}
	audit := cleanReviewForStageDecision()
	audit.Evidence = []string{"go test ./internal/app；runtime checkpoint and QA evidence verified"}
	if err := writeJSONFile(filepath.Join(runDir(repo, state.RunID), "audit-1.json"), audit); err != nil {
		t.Fatal(err)
	}
	if err := snapshotRunPrompts(repo, state.RunID); err != nil {
		t.Fatal(err)
	}
	if err := saveState(repo, state); err != nil {
		t.Fatal(err)
	}
	guard := &recoveryGuardRunner{}
	registry := NewAgentRegistry()
	registry.Register(fakeWorkflowTool{runner: guard})
	runErr := NewEngine(repo, registry).runLoop(context.Background(), state)
	if !errors.Is(runErr, errRecoveryGuardObserved) || !guard.called {
		t.Fatalf("legacy targeted repair rerun guard = called:%v err:%v", guard.called, runErr)
	}
	rerouted, err := loadState(repo, state.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if rerouted.Stage != "audit_2" || rerouted.QualityLoop.Mode != "pre_qa_audit" ||
		rerouted.Stages["targeted_repair_2"] != "" ||
		rerouted.Sessions["codex:repairer"] != state.Sessions["codex:repairer"] {
		t.Fatalf("legacy targeted repair trust route = %s quality=%#v stages=%#v sessions=%#v",
			rerouted.Stage, rerouted.QualityLoop, rerouted.Stages, rerouted.Sessions)
	}
}

// TestQABlockedResumeWithUntrustedArtifactRoutesToFreshAudit rejects QA output that never passed its read-only gate.
func TestQABlockedResumeWithUntrustedArtifactRoutesToFreshAudit(t *testing.T) {
	repo, changeName, acceptanceSource, head, diff := newRepairEvidenceFixture(t)
	state := qualityLoopState(statusBlockedStalled)
	state.RunID = newRunID()
	state.ChangeName = changeName
	state.Status = statusBlockedStalled
	state.BaselineHead = head
	state.BaselineDiff = diff
	state.Stages["audit_1"] = "completed"
	state.Stages["qa_2"] = statusBlockedStalled
	state.QualityLoop.BlockedFromStage = "qa_2"
	content, err := gitChangeContentSnapshotForChange(repo, changeName)
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
	qa := cleanRepairDAGQA()
	qa.Decision = "needs_fix"
	qa.Findings = []Finding{blockingFindingForStageDecision()}
	qa.AcceptanceMatrix[0].Status = "failed"
	if err := writeJSONFile(filepath.Join(runDir(repo, state.RunID), "qa-2.json"), qa); err != nil {
		t.Fatal(err)
	}
	state.QualityLoop.SourceQAArtifact = "qa-2.json"
	state.QualityLoop.SourceQAHash = qaArtifactContentHash(qa)
	state.QualityLoop.ProgressHash = qualityProgressHash(state)
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("untrusted QA source progress\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	engine := NewEngine(repo, NewAgentRegistry())
	if err := engine.prepareQualityLoopResume(&state, false); err != nil {
		t.Fatal(err)
	}
	if state.Status != statusRunning || state.Stage != "audit_2" ||
		state.QualityLoop.Mode != "pre_qa_audit" ||
		state.QualityLoop.SourceQAArtifact != "" ||
		state.QualityLoop.SourceQAHash != "" {
		t.Fatalf("untrusted QA resume = %s/%s quality=%#v", state.Status, state.Stage, state.QualityLoop)
	}
}

// TestQABlockedResumeWithCleanArtifactRoutesToFreshAudit prevents a stale clean QA retry loop.
func TestQABlockedResumeWithCleanArtifactRoutesToFreshAudit(t *testing.T) {
	repo, changeName, acceptanceSource, head, diff := newRepairEvidenceFixture(t)
	state := qualityLoopState(statusBlockedStalled)
	state.RunID = newRunID()
	state.ChangeName = changeName
	state.Status = statusBlockedStalled
	state.BaselineHead = head
	state.BaselineDiff = diff
	state.Stages["audit_1"] = "completed"
	state.Stages["qa_1"] = "completed"
	state.Stages["qa_2"] = statusBlockedStalled
	state.QualityLoop.BlockedFromStage = "qa_2"
	evidencePath := filepath.Join(repo, "test-results", "repair-dag", "runtime.log")
	if err := os.MkdirAll(filepath.Dir(evidencePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(evidencePath, []byte("fresh audit evidence\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	content, err := gitChangeContentSnapshotForChange(repo, changeName)
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
	if err := writeJSONFile(filepath.Join(runDir(repo, state.RunID), "qa-2.json"), cleanRepairDAGQA()); err != nil {
		t.Fatal(err)
	}
	state.QualityLoop.ProgressHash = qualityProgressHash(state)
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("retain clean QA source progress\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	engine := NewEngine(repo, NewAgentRegistry())
	if err := engine.prepareQualityLoopResume(&state, false); err != nil {
		t.Fatal(err)
	}
	if state.Status != statusRunning || state.Stage != "audit_2" ||
		state.QualityLoop.Mode != "pre_qa_audit" || state.QualityLoop.SourceQAArtifact != "" {
		t.Fatalf("clean QA progress resume = %s/%s quality=%#v", state.Status, state.Stage, state.QualityLoop)
	}
	state.Stages["audit_2"] = "completed"
	recordPassedQualityLoopCheckpoint(t, repo, &state, "audit_2")
	decision, err := DecideNextStage(state, cleanReviewForStageDecision(), QA{})
	if err != nil {
		t.Fatal(err)
	}
	if decision.NextStage != "qa_3" {
		t.Fatalf("fresh audit next stage = %q, want qa_3", decision.NextStage)
	}
	state.Stage = decision.NextStage
	state.Status = decision.NextStatus
	state.QualityLoop = *decision.QualityLoop
	input, blocked, err := engine.prepareQualityLoopQAReadOnlyGate(&state)
	if err != nil || blocked || input.DiffHash == "" || input.CheckpointHash == "" {
		t.Fatalf("fresh audit QA checkpoint = input:%#v blocked:%v err:%v", input, blocked, err)
	}
}

// TestEnvironmentResumeAbsorbsDeclaredPathBeforeRunLoopGuard exercises recovery through the real intervention guard.
func TestEnvironmentResumeAbsorbsDeclaredPathBeforeRunLoopGuard(t *testing.T) {
	repo, changeName, acceptanceSource, head, diff := newRepairEvidenceFixture(t)
	state := qualityLoopState("audit_2")
	state.RunID = newRunID()
	state.ChangeName = changeName
	state.BaselineHead = head
	state.BaselineDiff = diff
	state.Sessions = map[string]string{}
	state.Paths = map[string]string{}
	if err := snapshotRunPrompts(repo, state.RunID); err != nil {
		t.Fatal(err)
	}
	if err := snapshotQualityLoopAcceptance(repo, &state, acceptanceSource); err != nil {
		t.Fatal(err)
	}
	if err := blockQualityEnvironment(repo, &state, []string{"credentials.json"}); err != nil {
		t.Fatal(err)
	}
	if err := saveState(repo, state); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "credentials.json"), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	runner := &recoveryGuardRunner{}
	registry := NewAgentRegistry()
	registry.Register(fakeWorkflowTool{runner: runner})
	engine := NewEngine(repo, registry)
	if err := engine.prepareQualityLoopResume(&state, false); err != nil {
		t.Fatal(err)
	}
	if state.BaselineDiff == diff || state.QualityLoop.DiffHash == "" {
		t.Fatalf("environment path was not absorbed into baseline: %#v", state.QualityLoop)
	}
	err := engine.runLoop(context.Background(), state)
	if !errors.Is(err, errRecoveryGuardObserved) {
		t.Fatalf("run-loop did not reach stage runner after recovery: %v", err)
	}
	if !runner.called {
		t.Fatal("run-loop intervention guard rejected the declared environment path")
	}
	persisted, err := loadState(repo, state.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Status == statusAborted {
		t.Fatalf("declared environment path triggered manual-intervention abort: %#v", persisted)
	}
}

// TestEnvironmentResumeRejectsUndeclaredRepositoryChanges verifies recovery cannot absorb unrelated edits.
func TestEnvironmentResumeRejectsUndeclaredRepositoryChanges(t *testing.T) {
	repo, changeName, acceptanceSource, head, diff := newRepairEvidenceFixture(t)
	state := qualityLoopState("audit_2")
	state.RunID = newRunID()
	state.ChangeName = changeName
	state.BaselineHead = head
	state.BaselineDiff = diff
	if err := snapshotQualityLoopAcceptance(repo, &state, acceptanceSource); err != nil {
		t.Fatal(err)
	}
	if err := blockQualityEnvironment(repo, &state, []string{"credentials.json"}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "credentials.json"), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("unrelated source edit\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	engine := NewEngine(repo, NewAgentRegistry())
	if err := engine.prepareQualityLoopResume(&state, false); err == nil {
		t.Fatal("environment recovery absorbed an undeclared repository change")
	}
	if state.Status != statusBlockedEnvironment || state.Stage != statusBlockedEnvironment {
		t.Fatalf("rejected recovery mutated block state: %s/%s", state.Status, state.Stage)
	}
}

// TestEnvironmentResumeExpandsPreexistingUntrackedDirectory prevents hidden sibling changes from entering the baseline.
func TestEnvironmentResumeExpandsPreexistingUntrackedDirectory(t *testing.T) {
	repo, changeName, acceptanceSource, head, _ := newRepairEvidenceFixture(t)
	scratch := filepath.Join(repo, "scratch")
	if err := os.MkdirAll(scratch, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(scratch, "existing.txt"), []byte("existing\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, baselineDiff, err := gitSnapshot(repo)
	if err != nil {
		t.Fatal(err)
	}
	state := qualityLoopState("audit_2")
	state.RunID = newRunID()
	state.ChangeName = changeName
	state.BaselineHead = head
	state.BaselineDiff = baselineDiff
	if err := snapshotQualityLoopAcceptance(repo, &state, acceptanceSource); err != nil {
		t.Fatal(err)
	}
	if err := blockQualityEnvironment(repo, &state, []string{"scratch/credential.json"}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(scratch, "credential.json"), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(scratch, "unrelated.go"), []byte("package scratch\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	engine := NewEngine(repo, NewAgentRegistry())
	err = engine.prepareQualityLoopResume(&state, false)
	if err == nil || !strings.Contains(err.Error(), "scratch/unrelated.go") {
		t.Fatalf("preexisting untracked directory sibling was not rejected: %v", err)
	}
	if state.Status != statusBlockedEnvironment || state.Stage != statusBlockedEnvironment {
		t.Fatalf("rejected untracked sibling mutated block state: %s/%s", state.Status, state.Stage)
	}
}

// TestEnvironmentResumeRejectsPreexistingDirtyContentChanges binds recovery to content, not only status lines.
func TestEnvironmentResumeRejectsPreexistingDirtyContentChanges(t *testing.T) {
	for _, test := range []struct {
		name string
		path string
	}{
		{name: "tracked", path: "README.md"},
		{name: "untracked", path: "scratch/unrelated.go"},
	} {
		t.Run(test.name, func(t *testing.T) {
			repo, changeName, acceptanceSource, _, _ := newRepairEvidenceFixture(t)
			dirtyPath := filepath.Join(repo, filepath.FromSlash(test.path))
			if err := os.MkdirAll(filepath.Dir(dirtyPath), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(dirtyPath, []byte("dirty before block\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			head, diff, err := gitSnapshot(repo)
			if err != nil {
				t.Fatal(err)
			}
			state := qualityLoopState("audit_2")
			state.RunID = newRunID()
			state.ChangeName = changeName
			state.BaselineHead = head
			state.BaselineDiff = diff
			if err := snapshotQualityLoopAcceptance(repo, &state, acceptanceSource); err != nil {
				t.Fatal(err)
			}
			if err := blockQualityEnvironment(repo, &state, []string{"credentials.json"}); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(repo, "credentials.json"), []byte("{}\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(dirtyPath, []byte("dirty after block\n"), 0o644); err != nil {
				t.Fatal(err)
			}

			engine := NewEngine(repo, NewAgentRegistry())
			err = engine.prepareQualityLoopResume(&state, false)
			if err == nil || !strings.Contains(err.Error(), test.path) {
				t.Fatalf("dirty content change %q was not rejected: %v", test.path, err)
			}
			if state.Status != statusBlockedEnvironment || state.Stage != statusBlockedEnvironment {
				t.Fatalf("rejected dirty content mutated block state: %s/%s", state.Status, state.Stage)
			}
		})
	}
}

// recoveryUnavailableRegistry returns a sealed-workflow backend that deterministically fails resolution.
func recoveryUnavailableRegistry() *AgentRegistry {
	resolved := []string{}
	registry := &AgentRegistry{}
	registry.Register(dependencyProbeTool{
		name: "codex", resolved: &resolved, err: errRecoveryBackendUnavailable,
	})
	return registry
}

// newBlockedRecoveryBackendFixture creates a ready environment block whose backend cannot resume.
func newBlockedRecoveryBackendFixture(t *testing.T) (string, State) {
	t.Helper()
	repo, changeName, _, head, diff := newRepairEvidenceFixture(t)
	const environmentName = "TEST_RECOVERY_BACKEND_READY"
	t.Setenv(environmentName, "ready")
	state := qualityLoopState("audit_2")
	state.RunID = newRunID()
	state.ChangeName = changeName
	state.BaselineHead = head
	state.BaselineDiff = diff
	state.Sessions = map[string]string{}
	state.Paths = map[string]string{}
	if err := blockQualityEnvironment(repo, &state, []string{environmentName}); err != nil {
		t.Fatal(err)
	}
	if err := saveState(repo, state); err != nil {
		t.Fatal(err)
	}
	return repo, state
}

// assertPersistedRecoveryBlock verifies a failed command did not durably clear the recoverable pause.
func assertPersistedRecoveryBlock(t *testing.T, repo string, state State) {
	t.Helper()
	persisted, err := loadState(repo, state.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Status != statusBlockedEnvironment ||
		persisted.Stage != statusBlockedEnvironment ||
		persisted.QualityLoop.BlockedFromStage != "audit_2" ||
		strings.Join(persisted.QualityLoop.MissingEnvironmentNames, ",") != "TEST_RECOVERY_BACKEND_READY" {
		t.Fatalf("backend failure cleared durable block: %#v", persisted)
	}
}

// TestRecoveryBackendFailurePreservesDurableBlock covers inline, detached, and explicit restart entrypoints.
func TestRecoveryBackendFailurePreservesDurableBlock(t *testing.T) {
	for _, mode := range []string{"inline", "detached", "restart"} {
		t.Run(mode, func(t *testing.T) {
			repo, state := newBlockedRecoveryBackendFixture(t)
			engine := NewEngine(repo, recoveryUnavailableRegistry())
			var err error
			switch mode {
			case "inline":
				err = engine.ResumeRunJSON(context.Background(), state.RunID, io.Discard)
			case "detached":
				originalStart := startDetachedCommand
				startCalled := false
				startDetachedCommand = func(string, string) error {
					startCalled = true
					return nil
				}
				t.Cleanup(func() {
					startDetachedCommand = originalStart
				})
				err = engine.ResumeDetachedAfterUserChoice(context.Background(), state.RunID)
				if startCalled {
					t.Fatal("detached worker started before backend resolution passed")
				}
			case "restart":
				_, err = engine.prepareRestartRun(state.RunID, false)
			}
			if !errors.Is(err, errRecoveryBackendUnavailable) {
				t.Fatalf("%s recovery error = %v, want backend resolution failure", mode, err)
			}
			assertPersistedRecoveryBlock(t, repo, state)
		})
	}
}

// TestRecoveryDetachedStartFailureRestoresRunAndBatchState covers every prepared detached recovery entrypoint.
func TestRecoveryDetachedStartFailureRestoresRunAndBatchState(t *testing.T) {
	for _, mode := range []string{"resume", "restart", "batch"} {
		t.Run(mode, func(t *testing.T) {
			repo, state := newBlockedRecoveryBackendFixture(t)
			runner := &recoveryGuardRunner{}
			registry := NewAgentRegistry()
			registry.Register(fakeWorkflowTool{runner: runner})
			engine := NewEngine(repo, registry)
			before, err := loadState(repo, state.RunID)
			if err != nil {
				t.Fatal(err)
			}
			var beforeBatch *BatchState
			if mode == "batch" {
				batch := BatchState{
					BatchID: newRunID(), Status: batchStatusRunning,
					Changes: []string{state.ChangeName}, RunIDs: map[string]string{state.ChangeName: state.RunID},
				}
				if err := saveBatchState(repo, batch); err != nil {
					t.Fatal(err)
				}
				beforeBatch = &batch
			}
			originalRunStart := startDetachedCommand
			originalBatchStart := startDetachedBatchCommand
			startDetachedCommand = func(string, string) error { return errRecoveryDetachedStart }
			startDetachedBatchCommand = func(string, string) error { return errRecoveryDetachedStart }
			t.Cleanup(func() {
				startDetachedCommand = originalRunStart
				startDetachedBatchCommand = originalBatchStart
			})

			switch mode {
			case "resume":
				err = engine.ResumeDetachedAfterUserChoice(context.Background(), state.RunID)
			case "restart":
				err = engine.RestartRunDetached(state.RunID, false)
			case "batch":
				err = engine.RestartBatchDetached(beforeBatch.BatchID)
			}
			if !errors.Is(err, errRecoveryDetachedStart) {
				t.Fatalf("%s detached error = %v", mode, err)
			}
			persisted, loadErr := loadState(repo, state.RunID)
			if loadErr != nil {
				t.Fatal(loadErr)
			}
			if !reflect.DeepEqual(persisted, before) {
				t.Fatalf("%s did not restore run state:\nbefore=%#v\nafter=%#v", mode, before, persisted)
			}
			if beforeBatch != nil {
				persistedBatch, loadErr := loadBatchState(repo, beforeBatch.BatchID)
				if loadErr != nil {
					t.Fatal(loadErr)
				}
				if !reflect.DeepEqual(persistedBatch, *beforeBatch) {
					t.Fatalf("batch did not restore queue state:\nbefore=%#v\nafter=%#v", *beforeBatch, persistedBatch)
				}
			}
		})
	}
}

// TestRecoveryDetachedRollbackRejectsConcurrentStateProgress prevents stale whole-state compensation.
func TestRecoveryDetachedRollbackRejectsConcurrentStateProgress(t *testing.T) {
	for _, mode := range []string{"resume", "restart", "batch"} {
		t.Run(mode, func(t *testing.T) {
			repo, state := newBlockedRecoveryBackendFixture(t)
			registry := NewAgentRegistry()
			registry.Register(fakeWorkflowTool{runner: &recoveryGuardRunner{}})
			engine := NewEngine(repo, registry)
			var batchID string
			if mode == "batch" {
				batchID = newRunID()
				if err := saveBatchState(repo, BatchState{
					BatchID: batchID, Status: batchStatusRunning,
					Changes: []string{state.ChangeName}, RunIDs: map[string]string{state.ChangeName: state.RunID},
				}); err != nil {
					t.Fatal(err)
				}
			}
			concurrentProgress := func() error {
				current, err := loadState(repo, state.RunID)
				if err != nil {
					return err
				}
				current.Stage = "qa_99"
				current.Status = statusRunning
				current.Stages["qa_99"] = statusRunning
				return saveState(repo, current)
			}
			originalRunStart := startDetachedCommand
			originalBatchStart := startDetachedBatchCommand
			startDetachedCommand = func(string, string) error {
				if err := concurrentProgress(); err != nil {
					return err
				}
				return errRecoveryDetachedStart
			}
			startDetachedBatchCommand = func(string, string) error {
				if err := concurrentProgress(); err != nil {
					return err
				}
				return errRecoveryDetachedStart
			}
			t.Cleanup(func() {
				startDetachedCommand = originalRunStart
				startDetachedBatchCommand = originalBatchStart
			})

			var err error
			switch mode {
			case "resume":
				err = engine.ResumeDetachedAfterUserChoice(context.Background(), state.RunID)
			case "restart":
				err = engine.RestartRunDetached(state.RunID, false)
			case "batch":
				err = engine.RestartBatchDetached(batchID)
			}
			if !errors.Is(err, errRecoveryDetachedStart) || !strings.Contains(err.Error(), "代次已变化") {
				t.Fatalf("%s concurrent rollback error = %v", mode, err)
			}
			persisted, loadErr := loadState(repo, state.RunID)
			if loadErr != nil {
				t.Fatal(loadErr)
			}
			if persisted.Stage != "qa_99" || persisted.Stages["qa_99"] != statusRunning {
				t.Fatalf("%s stale rollback overwrote concurrent progress: %#v", mode, persisted)
			}
		})
	}
}

// TestRecoveryDetachedHandoffFailurePreservesPreparedState avoids racing a process that already started.
func TestRecoveryDetachedHandoffFailurePreservesPreparedState(t *testing.T) {
	for _, mode := range []string{"resume", "restart", "batch"} {
		t.Run(mode, func(t *testing.T) {
			repo, state := newBlockedRecoveryBackendFixture(t)
			runner := &recoveryGuardRunner{}
			registry := NewAgentRegistry()
			registry.Register(fakeWorkflowTool{runner: runner})
			engine := NewEngine(repo, registry)
			var batchID string
			if mode == "batch" {
				batchID = newRunID()
				if err := saveBatchState(repo, BatchState{
					BatchID: batchID, Status: batchStatusRunning,
					Changes: []string{state.ChangeName}, RunIDs: map[string]string{state.ChangeName: state.RunID},
				}); err != nil {
					t.Fatal(err)
				}
			}
			handoffErr := &detachedWorkerStartError{
				Cause: errRecoveryDetachedHandoff, ProcessStarted: true,
			}
			originalRunStart := startDetachedCommand
			originalBatchStart := startDetachedBatchCommand
			startDetachedCommand = func(string, string) error { return handoffErr }
			startDetachedBatchCommand = func(string, string) error { return handoffErr }
			t.Cleanup(func() {
				startDetachedCommand = originalRunStart
				startDetachedBatchCommand = originalBatchStart
			})

			var err error
			switch mode {
			case "resume":
				err = engine.ResumeDetachedAfterUserChoice(context.Background(), state.RunID)
			case "restart":
				err = engine.RestartRunDetached(state.RunID, false)
			case "batch":
				err = engine.RestartBatchDetached(batchID)
			}
			if !errors.Is(err, errRecoveryDetachedHandoff) || !detachedWorkerProcessStarted(err) {
				t.Fatalf("%s handoff error = %v", mode, err)
			}
			persisted, loadErr := loadState(repo, state.RunID)
			if loadErr != nil {
				t.Fatal(loadErr)
			}
			if persisted.Status != statusRunning || persisted.Stage != "audit_2" ||
				persisted.QualityLoop.BlockedFromStage != "" ||
				len(persisted.QualityLoop.MissingEnvironmentNames) != 0 {
				t.Fatalf("%s rolled back state owned by a launched worker: %#v", mode, persisted)
			}
		})
	}
}

// TestDetachedWorkerReleaseFailureReportsStartedProcess verifies the production handoff boundary.
func TestDetachedWorkerReleaseFailureReportsStartedProcess(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	originalRelease := releaseDetachedProcess
	releaseDetachedProcess = func(process *os.Process) error {
		if err := process.Release(); err != nil {
			t.Fatalf("release test process: %v", err)
		}
		return errRecoveryDetachedHandoff
	}
	t.Cleanup(func() { releaseDetachedProcess = originalRelease })

	cmd := exec.Command(executable, "-test.run=^$")
	err = startDetachedWorkerCommand(cmd, filepath.Join(t.TempDir(), "worker.log"))
	if !errors.Is(err, errRecoveryDetachedHandoff) || !detachedWorkerProcessStarted(err) {
		t.Fatalf("release failure lost process-started state: %v", err)
	}
}

// TestRecoveryStartupWriterFailureRestoresDurableBlock covers resume and restart pre-worker transactions.
func TestRecoveryStartupWriterFailureRestoresDurableBlock(t *testing.T) {
	for _, mode := range []string{"resume", "restart"} {
		t.Run(mode, func(t *testing.T) {
			repo, state := newBlockedRecoveryBackendFixture(t)
			before, err := loadState(repo, state.RunID)
			if err != nil {
				t.Fatal(err)
			}
			runner := &recoveryGuardRunner{}
			registry := NewAgentRegistry()
			registry.Register(fakeWorkflowTool{runner: runner})
			engine := NewEngine(repo, registry)
			switch mode {
			case "resume":
				err = engine.ResumeRunJSON(context.Background(), state.RunID, recoveryFailingWriter{})
			case "restart":
				err = engine.RestartRunJSON(context.Background(), state.RunID, recoveryFailingWriter{})
			}
			if !errors.Is(err, errRecoveryStartupWrite) || !isRunnerStartupWriteError(err) {
				t.Fatalf("%s startup error = %v, want typed writer failure", mode, err)
			}
			if runner.called {
				t.Fatalf("%s started a worker after startup output failed", mode)
			}
			persisted, err := loadState(repo, state.RunID)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(persisted, before) {
				t.Fatalf("%s did not restore pre-call state:\nbefore=%#v\nafter=%#v", mode, before, persisted)
			}
		})
	}
}

// TestGoDAGEnvironmentBlockSkipsArtifactGate verifies a paused node cannot create a fictional gate failure.
func TestGoDAGEnvironmentBlockSkipsArtifactGate(t *testing.T) {
	repo, changeName, acceptanceSource, head, diff := newRepairEvidenceFixture(t)
	state := qualityLoopState("audit_1")
	state.RunID = newRunID()
	state.ChangeName = changeName
	state.BaselineHead = head
	state.BaselineDiff = diff
	state.Sessions = map[string]string{}
	state.Paths = map[string]string{}
	state.ArtifactGates = map[string]StageValidationState{}
	if err := snapshotRunPrompts(repo, state.RunID); err != nil {
		t.Fatal(err)
	}
	if err := snapshotQualityLoopAcceptance(repo, &state, acceptanceSource); err != nil {
		t.Fatal(err)
	}
	if err := saveState(repo, state); err != nil {
		t.Fatal(err)
	}
	registry := NewAgentRegistry()
	registry.Register(fakeWorkflowTool{runner: recoveryEnvironmentBlockRunner{}})
	engine := NewEngine(repo, registry)
	node := WorkflowNode{ID: "audit_1", Name: "audit_1", Type: "main_stage", Stage: "audit_1"}
	if err := engine.runGoDAGNode(context.Background(), state.RunID, node); err != nil {
		t.Fatal(err)
	}
	persisted, err := loadState(repo, state.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Status != statusBlockedEnvironment ||
		persisted.QualityLoop.BlockedFromStage != "audit_1" ||
		strings.Join(persisted.QualityLoop.MissingEnvironmentNames, ",") != "TEST_RECOVERY_ACCOUNT" {
		t.Fatalf("Go-DAG environment block = %#v", persisted)
	}
	if _, exists := persisted.ArtifactGates[statusBlockedEnvironment]; exists {
		t.Fatalf("Go-DAG wrote a fictional blocked-stage artifact gate: %#v", persisted.ArtifactGates)
	}
	if nodeState := persisted.DAGNodes[node.ID]; nodeState.Status != statusBlockedEnvironment {
		t.Fatalf("Go-DAG node status = %#v", nodeState)
	}
}
