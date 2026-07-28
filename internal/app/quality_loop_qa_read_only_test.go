// Package app verifies that independent quality-loop QA cannot mutate tested source content.
package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
)

// qualityLoopQAReadOnlyRunner writes a clean QA artifact and optionally mutates tracked source.
type qualityLoopQAReadOnlyRunner struct {
	repo           string
	runID          string
	sourcePath     string
	mutate         bool
	commitMutation bool
	calls          int
	artifact       *QA
	duringRun      func() error
}

// Run simulates one independent QA turn without invoking an external agent CLI.
func (r *qualityLoopQAReadOnlyRunner) Run(context.Context, string, string, string, StageOptions) (string, error) {
	r.calls++
	if r.mutate {
		body := "// 文件功能目的：模拟 QA 越权写入实现源码。\npackage mutation\n"
		if err := os.WriteFile(r.sourcePath, []byte(body), 0o644); err != nil {
			return "", err
		}
		if r.commitMutation {
			cmd := exec.Command("git", "add", filepath.Base(r.sourcePath))
			cmd.Dir = r.repo
			if output, err := cmd.CombinedOutput(); err != nil {
				return "", fmt.Errorf("stage QA mutation: %w: %s", err, output)
			}
			cmd = exec.Command("git", "commit", "-q", "-m", "qa mutation")
			cmd.Dir = r.repo
			if output, err := cmd.CombinedOutput(); err != nil {
				return "", fmt.Errorf("commit QA mutation: %w: %s", err, output)
			}
		}
	}
	if r.duringRun != nil {
		if err := r.duringRun(); err != nil {
			return "", err
		}
	}
	artifact := cleanRepairDAGQA()
	if r.artifact != nil {
		artifact = *r.artifact
	}
	if err := writeJSONFile(filepath.Join(runDir(r.repo, r.runID), "qa-1.json"), artifact); err != nil {
		return "", err
	}
	return "isolated-qa-session", nil
}

// recordPassedQualityLoopCheckpoint writes production-shaped validation and acceptance artifacts.
func recordPassedQualityLoopCheckpoint(t *testing.T, repo string, state *State, stage string) {
	t.Helper()
	if state.Validation == nil {
		state.Validation = map[string]StageValidationState{}
	}
	if state.AcceptanceRun == nil {
		state.AcceptanceRun = map[string]StageValidationState{}
	}
	originalStage := state.Stage
	state.Stage = stage
	attempt := ValidationAttempt{
		Stage: stage, Kind: validationKindCommands, Attempt: 1,
		Status: validationStatusPassed, DiffHash: state.QualityLoop.DiffHash,
		Commands: []ValidationCommandResult{{
			Command: "quality-loop checkpoint validation", ExitCode: 0, Output: "passed\n",
		}},
	}
	validationPath, err := writeValidationAttempt(repo, state.RunID, attempt)
	if err != nil {
		t.Fatal(err)
	}
	state.Validation[stage] = StageValidationState{
		Attempts: 1, Kind: validationKindCommands, Status: validationStatusPassed,
		LastArtifact: validationPath, DiffHash: state.QualityLoop.DiffHash,
	}
	state.QualityLoop.ValidationHash = qualityValidationProgressHash(attempt)
	state.QualityLoop.ValidationProgressHash = qualityValidationOutcomeHash(attempt)
	result, err := runAcceptanceRequiredTestsForState(context.Background(), repo, *state, 1)
	if err != nil {
		t.Fatal(err)
	}
	state.AcceptanceRun[stage] = StageValidationState{
		Attempts: 1, Kind: acceptanceRunKind, Status: validationStatusPassed,
		LastArtifact: result.ResultPath, DiffHash: state.QualityLoop.DiffHash,
	}
	state.QualityLoop.TestsHash = result.TestsHash
	state.QualityLoop.EvidenceHash = result.EvidenceHash
	state.QualityLoop.TestsProgressHash = result.TestsProgressHash
	state.QualityLoop.EvidenceProgressHash = result.EvidenceProgressHash
	state.Stage = originalStage
}

// trustedQualityLoopCheckpointHash returns the exact durable artifact binding used by QA gates.
func trustedQualityLoopCheckpointHash(t *testing.T, repo string, state State, checkpoint string) string {
	t.Helper()
	hash, err := qualityLoopCheckpointTrustHash(repo, state, checkpoint)
	if err != nil {
		t.Fatal(err)
	}
	return hash
}

// newQualityLoopQAReadOnlyFixture creates a QA stage backed by one completed, tested audit.
func newQualityLoopQAReadOnlyFixture(t *testing.T, mutate bool) (string, State, *Engine, *qualityLoopQAReadOnlyRunner) {
	t.Helper()
	repo, changeName, acceptanceSource, head, diff := newRepairEvidenceFixture(t)
	runID := newRunID()
	state := State{
		RunID: runID, ChangeName: changeName, Sealed: true, Status: statusRunning, Stage: "qa_1",
		BaselineHead: head, BaselineDiff: diff, Workflow: DefaultWorkflowConfig(),
		Sessions: map[string]string{}, Stages: map[string]string{
			"execution": "completed",
			"audit_1":   "completed",
		},
		Paths:         map[string]string{},
		Validation:    map[string]StageValidationState{},
		ArtifactGates: map[string]StageValidationState{},
		AcceptanceRun: map[string]StageValidationState{},
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
	audit := cleanReviewForStageDecision()
	audit.Evidence = []string{"go test ./internal/app；QA input audit verified"}
	if err := writeJSONFile(filepath.Join(runDir(repo, runID), "audit-1.json"), audit); err != nil {
		t.Fatal(err)
	}
	content, err := gitChangeContentSnapshotForChange(repo, changeName)
	if err != nil {
		t.Fatal(err)
	}
	diffHash := qualityHashStrings(content)
	state.QualityLoop = QualityLoopState{
		Mode: "pre_qa_audit", DiffHash: diffHash,
	}
	recordPassedQualityLoopCheckpoint(t, repo, &state, "audit_1")
	if err := saveState(repo, state); err != nil {
		t.Fatal(err)
	}
	runner := &qualityLoopQAReadOnlyRunner{
		repo: repo, runID: runID, sourcePath: filepath.Join(repo, "qa-mutated.go"), mutate: mutate,
	}
	registry := NewAgentRegistry()
	registry.Register(fakeWorkflowTool{runner: runner})
	return repo, state, NewEngine(repo, registry), runner
}

// TestQualityLoopQAReadOnlyGateBlocksSourceMutation verifies a clean QA cannot bless untested code.
func TestQualityLoopQAReadOnlyGateBlocksSourceMutation(t *testing.T) {
	repo, state, engine, runner := newQualityLoopQAReadOnlyFixture(t, true)
	trustedHash := state.QualityLoop.DiffHash
	if err := engine.runStage(context.Background(), &state); err != nil {
		t.Fatal(err)
	}
	if runner.calls != 1 {
		t.Fatalf("QA calls = %d, want 1", runner.calls)
	}
	if state.Status != statusBlockedStalled || state.Stage != statusBlockedStalled ||
		state.QualityLoop.BlockedFromStage != "qa_1" {
		t.Fatalf("QA mutation state = %s/%s from=%q", state.Status, state.Stage, state.QualityLoop.BlockedFromStage)
	}
	gate := state.ArtifactGates["qa_1"]
	if gate.Kind != validationKindQAReadOnly || gate.Status != validationStatusFailed ||
		gate.LastArtifact == "" || !fileExists(gate.LastArtifact) || gate.LastError == "" || gate.DiffHash == trustedHash {
		t.Fatalf("QA mutation gate = %#v trusted=%s", gate, trustedHash)
	}
	if state.QualityLoop.DiffHash != trustedHash || state.Stages["qa_1"] == "completed" {
		t.Fatalf("QA mutation trusted=%s stage=%q", state.QualityLoop.DiffHash, state.Stages["qa_1"])
	}
	if err := os.Remove(filepath.Join(repo, "qa-mutated.go")); err != nil {
		t.Fatal(err)
	}
	if err := engine.prepareQualityLoopResume(&state, false); err != nil {
		t.Fatal(err)
	}
	if state.Status != statusRunning || state.Stage != "qa_1" || !state.QualityLoop.ResumeRerunPending {
		t.Fatalf("restored QA state = %s/%s rerun=%v", state.Status, state.Stage, state.QualityLoop.ResumeRerunPending)
	}
}

// TestQualityLoopQAGateRejectsCheckpointDriftDuringRun freezes durable input before QA starts.
func TestQualityLoopQAGateRejectsCheckpointDriftDuringRun(t *testing.T) {
	repo, state, engine, runner := newQualityLoopQAReadOnlyFixture(t, false)
	resultPath := filepath.Join(repo, filepath.FromSlash(state.AcceptanceRun["audit_1"].LastArtifact))
	runner.duringRun = func() error {
		data, err := os.ReadFile(resultPath)
		if err != nil {
			return err
		}
		var result AcceptanceRunResult
		if err := decodeStrictArtifactJSON(data, &result); err != nil {
			return err
		}
		result.StartedAt = "2099-01-01T00:00:00Z"
		return writeJSONFile(resultPath, result)
	}
	if err := engine.runStage(context.Background(), &state); err != nil {
		t.Fatal(err)
	}
	gate := state.ArtifactGates["qa_1"]
	if runner.calls != 1 || state.Status != statusBlockedStalled ||
		state.QualityLoop.BlockedFromStage != "qa_1" ||
		gate.Status != validationStatusFailed ||
		!strings.Contains(gate.LastError, "QA 执行期间变化") {
		t.Fatalf("checkpoint drift reached trusted QA: calls=%d state=%s/%s gate=%#v",
			runner.calls, state.Status, state.Stage, gate)
	}
}

// TestQualityLoopQAReadOnlyGateRequiresLatestTestedDiff rejects a stale trusted hash before QA runs.
func TestQualityLoopQAReadOnlyGateRequiresLatestTestedDiff(t *testing.T) {
	_, state, engine, runner := newQualityLoopQAReadOnlyFixture(t, false)
	state.QualityLoop.DiffHash = qualityHashStrings("stale-unverified-diff")
	if err := engine.runStage(context.Background(), &state); err != nil {
		t.Fatal(err)
	}
	if runner.calls != 0 {
		t.Fatalf("QA ran %d times with stale tested diff", runner.calls)
	}
	if state.Status != statusBlockedStalled || state.Stage != statusBlockedStalled ||
		state.ArtifactGates["qa_1"].Status != validationStatusFailed {
		t.Fatalf("stale QA binding state = %s/%s gate=%#v", state.Status, state.Stage, state.ArtifactGates["qa_1"])
	}
}

// TestQualityLoopQAGateRejectsTamperedCheckpoint proves QA replays durable artifact trust checks.
func TestQualityLoopQAGateRejectsTamperedCheckpoint(t *testing.T) {
	repo, state, engine, runner := newQualityLoopQAReadOnlyFixture(t, false)
	resultPath := filepath.Join(repo, filepath.FromSlash(state.AcceptanceRun["audit_1"].LastArtifact))
	if err := os.WriteFile(resultPath, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := engine.runStage(context.Background(), &state); err != nil {
		t.Fatal(err)
	}
	if runner.calls != 0 || state.Status != statusBlockedStalled ||
		state.QualityLoop.BlockedFromStage != "qa_1" {
		t.Fatalf("tampered checkpoint reached QA: calls=%d state=%s/%s from=%q",
			runner.calls, state.Status, state.Stage, state.QualityLoop.BlockedFromStage)
	}
}

// TestQualityLoopQAReadOnlyGateIgnoresSiblingProposal keeps concurrent demand docs out of QA identity.
func TestQualityLoopQAReadOnlyGateIgnoresSiblingProposal(t *testing.T) {
	repo, state, engine, runner := newQualityLoopQAReadOnlyFixture(t, false)
	sibling := filepath.Join(repo, "docs", "changes", "2-并行提案", "brief.md")
	if err := os.MkdirAll(filepath.Dir(sibling), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sibling, []byte("# independent demand\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := engine.runStage(context.Background(), &state); err != nil {
		t.Fatal(err)
	}
	if runner.calls != 1 || state.Status != statusRunning || state.Stage != "qa_1" {
		t.Fatalf("sibling proposal blocked current QA: calls=%d state=%s/%s", runner.calls, state.Status, state.Stage)
	}
}

// TestQualityLoopAuditFinalBindingIgnoresSiblingProposal keeps all repair gates change-scoped.
func TestQualityLoopAuditFinalBindingIgnoresSiblingProposal(t *testing.T) {
	repo, changeName, acceptanceSource, _, _ := newRepairEvidenceFixture(t)
	state := qualityLoopState("audit_1")
	state.RunID = newRunID()
	state.ChangeName = changeName
	if err := snapshotQualityLoopAcceptance(repo, &state, acceptanceSource); err != nil {
		t.Fatal(err)
	}
	content, err := gitChangeContentSnapshotForChange(repo, changeName)
	if err != nil {
		t.Fatal(err)
	}
	state.QualityLoop.DiffHash = qualityHashStrings(content)
	audit := cleanReviewForStageDecision()
	audit.Evidence = []string{"go test ./internal/app；runtime change-scoped final binding verified"}
	if err := writeJSONFile(filepath.Join(runDir(repo, state.RunID), "audit-1.json"), audit); err != nil {
		t.Fatal(err)
	}
	sibling := filepath.Join(repo, "docs", "changes", "2-并行提案", "brief.md")
	if err := os.MkdirAll(filepath.Dir(sibling), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sibling, []byte("# concurrent demand\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	engine := NewEngine(repo, NewAgentRegistry())
	result, err := engine.completeMainStage(context.Background(), &state, stageGatePipelineLoop)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Done || result.Blocked || state.Status != statusRunning || state.Stage != "qa_1" {
		t.Fatalf("sibling proposal blocked audit binding: result=%#v state=%s/%s", result, state.Status, state.Stage)
	}
}

// TestQualityLoopQAReadOnlyGateReroutesChangedEvidenceButBlocksUnsafeEvidence protects QA continuity.
func TestQualityLoopQAReadOnlyGateReroutesChangedEvidenceButBlocksUnsafeEvidence(t *testing.T) {
	for _, action := range []string{"modify", "directory", "fifo", "device"} {
		t.Run(action, func(t *testing.T) {
			repo, state, engine, runner := newQualityLoopQAReadOnlyFixture(t, false)
			path := filepath.Join(repo, "test-results", "repair-dag", "runtime.log")
			switch action {
			case "modify":
				if err := os.WriteFile(path, []byte("changed evidence\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			case "directory":
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(path, 0o755); err != nil {
					t.Fatal(err)
				}
			case "fifo":
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if err := syscall.Mkfifo(path, 0o600); err != nil {
					t.Fatal(err)
				}
			case "device":
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink("/dev/null", path); err != nil {
					t.Fatal(err)
				}
			}
			if err := engine.runStage(context.Background(), &state); err != nil {
				t.Fatal(err)
			}
			if action == "modify" {
				if runner.calls != 0 || state.Status != statusRunning || state.Stage != "audit_2" ||
					!state.QualityLoop.ResumeRerunPending ||
					state.QualityLoop.BlockedFromStage != "" {
					t.Fatalf("QA evidence %s did not reroute: calls=%d state=%s/%s from=%q",
						action, runner.calls, state.Status, state.Stage, state.QualityLoop.BlockedFromStage)
				}
				return
			}
			if runner.calls != 0 || state.Status != statusBlockedStalled ||
				state.QualityLoop.BlockedFromStage != "qa_1" {
				t.Fatalf("QA evidence %s calls=%d state=%s/%s from=%q",
					action, runner.calls, state.Status, state.Stage, state.QualityLoop.BlockedFromStage)
			}
		})
	}
}

// TestQualityLoopQAReroutesEvidenceChangedDuringRun ensures a completed QA cannot bless stale evidence.
func TestQualityLoopQAReroutesEvidenceChangedDuringRun(t *testing.T) {
	repo, state, engine, runner := newQualityLoopQAReadOnlyFixture(t, false)
	evidencePath := filepath.Join(repo, "test-results", "repair-dag", "runtime.log")
	runner.duringRun = func() error {
		return os.WriteFile(evidencePath, []byte("new evidence after QA started\n"), 0o644)
	}

	if err := engine.runStage(context.Background(), &state); err != nil {
		t.Fatal(err)
	}
	if runner.calls != 1 || state.Status != statusRunning || state.Stage != "audit_2" ||
		!state.QualityLoop.ResumeRerunPending ||
		state.QualityLoop.BlockedFromStage != "" {
		t.Fatalf("mid-QA evidence drift did not reroute: calls=%d state=%s/%s from=%q",
			runner.calls, state.Status, state.Stage, state.QualityLoop.BlockedFromStage)
	}
}

// TestQualityLoopQAReadOnlyGateBlocksCommittedSourceMutation binds QA to effective repository content.
func TestQualityLoopQAReadOnlyGateBlocksCommittedSourceMutation(t *testing.T) {
	_, state, engine, runner := newQualityLoopQAReadOnlyFixture(t, true)
	runner.commitMutation = true
	if err := engine.runStage(context.Background(), &state); err != nil {
		t.Fatal(err)
	}
	if runner.calls != 1 || state.Status != statusBlockedStalled ||
		state.QualityLoop.BlockedFromStage != "qa_1" {
		t.Fatalf("committed QA mutation escaped gate: calls=%d state=%s/%s from=%q",
			runner.calls, state.Status, state.Stage, state.QualityLoop.BlockedFromStage)
	}
}

// TestQualityLoopQAArtifactEnvironmentMarkerBlocks redacts QA secrets and resumes the exact QA stage.
func TestQualityLoopQAArtifactEnvironmentMarkerBlocks(t *testing.T) {
	repo, state, engine, runner := newQualityLoopQAReadOnlyFixture(t, false)
	missingName := "environment/qa-account"
	sentinels := []string{
		"qa-prefix-secret",
		"summary-secret",
		"evidence-secret",
		"finding-secret",
		"non-blocking-secret",
		"matrix-artifact-secret",
		"matrix-evidence-secret",
	}
	qa := cleanRepairDAGQA()
	qa.Decision = "needs_fix"
	qa.Summary = "API_TOKEN=" + sentinels[0] + " blocked_environment: " + missingName + "=" + sentinels[1]
	qa.Evidence = []string{"blocked_environment: " + missingName + "=" + sentinels[2]}
	qa.Findings = []Finding{{
		Title:          "QA environment is unavailable",
		Severity:       "blocker",
		Scope:          "current_change",
		Evidence:       "blocked_environment: " + missingName + "=" + sentinels[3],
		Recommendation: "restore the declared QA account",
	}}
	qa.NonBlockingFindings = []Finding{{
		Title:          "Historical environment note",
		Severity:       "minor",
		Scope:          "out_of_scope_existing",
		Evidence:       "blocked_environment: " + missingName + "=" + sentinels[4],
		Recommendation: "track separately",
	}}
	qa.AcceptanceMatrix[0].Artifact = "blocked_environment: " + missingName + "=" + sentinels[5]
	qa.AcceptanceMatrix[0].Evidence = "blocked_environment: " + missingName + "=" + sentinels[6]
	runner.artifact = &qa

	if err := engine.runStage(context.Background(), &state); err != nil {
		t.Fatal(err)
	}
	result, err := engine.completeMainStage(context.Background(), &state, stageGatePipelineLoop)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Blocked || state.Status != statusBlockedEnvironment || state.Stage != statusBlockedEnvironment ||
		state.QualityLoop.BlockedFromStage != "qa_1" ||
		strings.Join(state.QualityLoop.MissingEnvironmentNames, ",") != missingName {
		t.Fatalf("QA environment state = result:%#v status:%s stage:%s quality:%#v",
			result, state.Status, state.Stage, state.QualityLoop)
	}
	if state.Stages["qa_1"] != statusBlockedEnvironment || state.Stages["targeted_repair_1"] != "" {
		t.Fatalf("QA environment marker advanced as quality failure: %#v", state.Stages)
	}
	if err := saveState(repo, state); err != nil {
		t.Fatal(err)
	}
	artifactPath := filepath.Join(runDir(repo, state.RunID), "qa-1.json")
	for _, path := range []string{artifactPath, filepath.Join(runDir(repo, state.RunID), "state.json")} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, sentinel := range sentinels {
			if strings.Contains(string(data), sentinel) {
				t.Fatalf("%s persisted QA environment secret %q", path, sentinel)
			}
		}
	}

	missingPath := filepath.Join(repo, filepath.FromSlash(missingName))
	if err := os.MkdirAll(filepath.Dir(missingPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(missingPath, []byte("ready\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := engine.prepareQualityLoopResume(&state, false); err != nil {
		t.Fatal(err)
	}
	if state.Status != statusRunning || state.Stage != "qa_1" || !state.QualityLoop.ResumeRerunPending {
		t.Fatalf("resumed QA environment state = %s/%s rerun=%v",
			state.Status, state.Stage, state.QualityLoop.ResumeRerunPending)
	}
}

// newQualityLoopArchiveGateFixture creates a sealed archive gate with durable required evidence.
func newQualityLoopArchiveGateFixture(t *testing.T) (string, string, State, *Engine, string) {
	t.Helper()
	repo, changeName, acceptanceSource, _, _ := newRepairEvidenceFixture(t)
	state := qualityLoopState(workflowStageArchive)
	state.RunID = newRunID()
	state.ChangeName = changeName
	state.ArtifactGates = map[string]StageValidationState{}
	if err := snapshotQualityLoopAcceptance(repo, &state, acceptanceSource); err != nil {
		t.Fatal(err)
	}
	evidencePath := filepath.Join(repo, "test-results", "repair-dag", "runtime.log")
	if err := os.MkdirAll(filepath.Dir(evidencePath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(evidencePath, []byte("archive evidence\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	content, err := gitChangeContentSnapshotForChange(repo, changeName)
	if err != nil {
		t.Fatal(err)
	}
	state.QualityLoop.DiffHash = qualityHashStrings(content)
	state.Stages["audit_1"] = "completed"
	state.Stages["qa_1"] = "completed"
	recordPassedQualityLoopCheckpoint(t, repo, &state, "audit_1")
	qa := cleanRepairDAGQA()
	if err := writeJSONFile(filepath.Join(runDir(repo, state.RunID), "qa-1.json"), qa); err != nil {
		t.Fatal(err)
	}
	state.QualityLoop.SourceQAArtifact = "qa-1.json"
	state.QualityLoop.SourceQAHash = qaArtifactContentHash(qa)
	state.ArtifactGates["qa_1"] = StageValidationState{
		Kind: validationKindQAReadOnly, Status: validationStatusPassed,
		DiffHash:       state.QualityLoop.DiffHash,
		CheckpointHash: trustedQualityLoopCheckpointHash(t, repo, state, "audit_1"),
	}
	return repo, changeName, state, NewEngine(repo, NewAgentRegistry()), evidencePath
}

// TestQualityLoopArchiveGateAllowsMoveButRejectsSourceMutation protects the final clean QA snapshot.
func TestQualityLoopArchiveGateAllowsMoveButRejectsSourceMutation(t *testing.T) {
	for _, mutate := range []bool{false, true} {
		t.Run(fmt.Sprintf("mutate=%v", mutate), func(t *testing.T) {
			repo, changeName, state, engine, _ := newQualityLoopArchiveGateFixture(t)
			blocked, err := engine.prepareQualityLoopArchiveReadOnlyGate(&state)
			if err != nil || blocked {
				t.Fatalf("prepare archive gate = blocked:%v err:%v", blocked, err)
			}
			if err := archiveRepairEvidence(repo, state.RunID, changeName); err != nil {
				t.Fatal(err)
			}
			if mutate {
				if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("archive source mutation\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			passed, err := engine.verifyQualityLoopArchiveReadOnlyGate(&state)
			if err != nil {
				t.Fatal(err)
			}
			if mutate {
				if passed || state.Status != statusBlockedStalled || state.QualityLoop.BlockedFromStage != workflowStageArchive {
					t.Fatalf("archive mutation passed=%v state=%s/%s from=%q", passed, state.Status, state.Stage, state.QualityLoop.BlockedFromStage)
				}
				return
			}
			if !passed {
				t.Fatalf("content-preserving archive move was blocked: %#v", state.ArtifactGates[workflowStageArchive])
			}
			gate := state.ArtifactGates[workflowStageArchive]
			if gate.Kind != validationKindArchiveReadOnly || gate.Status != validationStatusPassed || gate.LastError != "" {
				t.Fatalf("successful archive gate evidence = %#v", gate)
			}
		})
	}
}

// TestQualityLoopArchiveGateRejectsRequiredEvidenceMutation keeps final QA evidence immutable.
func TestQualityLoopArchiveGateRejectsRequiredEvidenceMutation(t *testing.T) {
	for _, action := range []string{"modify", "delete", "directory", "fifo", "device"} {
		t.Run(action, func(t *testing.T) {
			repo, changeName, state, engine, evidencePath := newQualityLoopArchiveGateFixture(t)
			blocked, err := engine.prepareQualityLoopArchiveReadOnlyGate(&state)
			if err != nil || blocked {
				t.Fatalf("prepare archive gate = blocked:%v err:%v", blocked, err)
			}
			if err := archiveRepairEvidence(repo, state.RunID, changeName); err != nil {
				t.Fatal(err)
			}
			switch action {
			case "modify":
				if err := os.WriteFile(evidencePath, []byte("tampered evidence\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			case "delete":
				if err := os.Remove(evidencePath); err != nil {
					t.Fatal(err)
				}
			case "directory":
				if err := os.Remove(evidencePath); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(evidencePath, 0o755); err != nil {
					t.Fatal(err)
				}
			case "fifo":
				if err := os.Remove(evidencePath); err != nil {
					t.Fatal(err)
				}
				if err := syscall.Mkfifo(evidencePath, 0o600); err != nil {
					t.Fatal(err)
				}
			case "device":
				if err := os.Remove(evidencePath); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink("/dev/null", evidencePath); err != nil {
					t.Fatal(err)
				}
			}
			passed, err := engine.verifyQualityLoopArchiveReadOnlyGate(&state)
			if err != nil {
				t.Fatal(err)
			}
			if passed || state.Status != statusBlockedStalled ||
				state.QualityLoop.BlockedFromStage != workflowStageArchive {
				t.Fatalf("archive evidence %s passed=%v state=%s/%s from=%q",
					action, passed, state.Status, state.Stage, state.QualityLoop.BlockedFromStage)
			}
		})
	}
}

// TestQualityLoopArchiveGateRejectsTamperedCheckpoint keeps accepted artifacts immutable through archive.
func TestQualityLoopArchiveGateRejectsTamperedCheckpoint(t *testing.T) {
	_, _, state, engine, _ := newQualityLoopArchiveGateFixture(t)
	blocked, err := engine.prepareQualityLoopArchiveReadOnlyGate(&state)
	if err != nil || blocked {
		t.Fatalf("prepare archive gate = blocked:%v err:%v", blocked, err)
	}
	if err := os.WriteFile(state.Validation["audit_1"].LastArtifact, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	passed, err := engine.verifyQualityLoopArchiveReadOnlyGate(&state)
	if err != nil {
		t.Fatal(err)
	}
	if passed || state.Status != statusBlockedStalled ||
		state.QualityLoop.BlockedFromStage != workflowStageArchive {
		t.Fatalf("tampered archive checkpoint passed=%v state=%s/%s from=%q",
			passed, state.Status, state.Stage, state.QualityLoop.BlockedFromStage)
	}
}

// TestQualityLoopArchiveGateRejectsFinalQAContentDrift keeps the final QA decision immutable.
func TestQualityLoopArchiveGateRejectsFinalQAContentDrift(t *testing.T) {
	repo, _, state, engine, _ := newQualityLoopArchiveGateFixture(t)
	path := filepath.Join(runDir(repo, state.RunID), "qa-1.json")
	qa, err := ReadQA(path)
	if err != nil {
		t.Fatal(err)
	}
	qa.Summary += " drifted after completion"
	if err := writeJSONFile(path, qa); err != nil {
		t.Fatal(err)
	}
	blocked, err := engine.prepareQualityLoopArchiveReadOnlyGate(&state)
	if err != nil {
		t.Fatal(err)
	}
	if !blocked || state.Status != statusBlockedStalled ||
		state.QualityLoop.BlockedFromStage != workflowStageArchive {
		t.Fatalf("drifted final QA blocked=%v state=%s/%s from=%q",
			blocked, state.Status, state.Stage, state.QualityLoop.BlockedFromStage)
	}
}

// TestQualityLoopArchiveBlockedResumeRestoresProposalAndFreshGates exercises the full recovery chain.
func TestQualityLoopArchiveBlockedResumeRestoresProposalAndFreshGates(t *testing.T) {
	repo, changeName, state, engine, _ := newQualityLoopArchiveGateFixture(t)
	state.Stages["audit_1"] = "completed"
	state.Stages["qa_1"] = "completed"
	blocked, err := engine.prepareQualityLoopArchiveReadOnlyGate(&state)
	if err != nil || blocked {
		t.Fatalf("prepare archive gate = blocked:%v err:%v", blocked, err)
	}
	firstArchiveHash := state.ArtifactGates[workflowStageArchive].DiffHash
	if err := archiveRepairEvidence(repo, state.RunID, changeName); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("retain archive source progress\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	passed, err := engine.verifyQualityLoopArchiveReadOnlyGate(&state)
	if err != nil {
		t.Fatal(err)
	}
	if passed || state.Status != statusBlockedStalled {
		t.Fatalf("archive source progress passed=%v state=%s/%s", passed, state.Status, state.Stage)
	}
	if err := engine.prepareQualityLoopResume(&state, true); err != nil {
		t.Fatal(err)
	}
	active := filepath.Join(repo, "docs", "changes", changeName)
	activeInfo, activeErr := os.Stat(active)
	activeReady := activeErr == nil && activeInfo.IsDir()
	if state.Status != statusRunning || state.Stage != "audit_2" || !activeReady {
		t.Fatalf("archive resume = %s/%s active=%v err=%v", state.Status, state.Stage, activeReady, activeErr)
	}
	if _, exists := state.ArtifactGates[workflowStageArchive]; exists {
		t.Fatalf("fresh audit retained stale archive gate: %#v", state.ArtifactGates[workflowStageArchive])
	}
	if err := engine.verifyQualityLoopActiveAcceptance(state); err != nil {
		t.Fatalf("restored active acceptance = %v", err)
	}
	audit := cleanReviewForStageDecision()
	audit.Evidence = []string{"go test ./internal/app；runtime restored archive audit verified"}
	if err := writeJSONFile(filepath.Join(runDir(repo, state.RunID), "audit-2.json"), audit); err != nil {
		t.Fatal(err)
	}
	result, err := engine.completeMainStage(context.Background(), &state, stageGatePipelineLoop)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Done || result.Blocked || state.Stage != "qa_2" {
		t.Fatalf("restored audit gates = result:%#v state=%s/%s", result, state.Status, state.Stage)
	}
	input, qaBlocked, err := engine.prepareQualityLoopQAReadOnlyGate(&state)
	if err != nil || qaBlocked || input.DiffHash == "" || input.CheckpointHash == "" {
		t.Fatalf("restored audit QA checkpoint = input:%#v blocked:%v err:%v", input, qaBlocked, err)
	}
	armQualityLoopQAReadOnlyGate(&state, input)
	qa := cleanRepairDAGQA()
	if err := writeJSONFile(filepath.Join(runDir(repo, state.RunID), "qa-2.json"), qa); err != nil {
		t.Fatal(err)
	}
	qaPassed, err := engine.verifyQualityLoopQAReadOnlyGate(&state)
	if err != nil || !qaPassed {
		t.Fatalf("restored QA gate = passed:%v err:%v", qaPassed, err)
	}
	state.Stages["qa_2"] = "completed"
	decision, err := DecideNextStage(state, Review{}, qa)
	if err != nil {
		t.Fatal(err)
	}
	applyQualityDecision(&state, decision)
	if state.Stage != workflowStageArchive {
		t.Fatalf("restored clean QA next stage = %s", state.Stage)
	}
	blocked, err = engine.prepareQualityLoopArchiveReadOnlyGate(&state)
	if err != nil || blocked {
		t.Fatalf("second archive prepare = blocked:%v err:%v", blocked, err)
	}
	secondGate := state.ArtifactGates[workflowStageArchive]
	if secondGate.DiffHash == "" {
		t.Fatal("second archive gate was not re-armed")
	}
	if secondGate.DiffHash == firstArchiveHash {
		t.Fatal("second archive gate reused the stale pre-recovery invariant")
	}
	if err := archiveRepairEvidence(repo, state.RunID, changeName); err != nil {
		t.Fatal(err)
	}
	passed, err = engine.verifyQualityLoopArchiveReadOnlyGate(&state)
	if err != nil || !passed || state.ArtifactGates[workflowStageArchive].Status != validationStatusPassed {
		t.Fatalf("second archive verify = passed:%v gate:%#v err:%v",
			passed, state.ArtifactGates[workflowStageArchive], err)
	}
}

// TestArchiveBlockedDetachedStartFailureRestoresProposalLocation covers resume, restart, and batch rollback.
func TestArchiveBlockedDetachedStartFailureRestoresProposalLocation(t *testing.T) {
	for _, mode := range []string{"resume", "restart", "batch"} {
		t.Run(mode, func(t *testing.T) {
			repo, changeName, state, gateEngine, _ := newQualityLoopArchiveGateFixture(t)
			state.Stages["audit_1"] = "completed"
			state.Stages["qa_1"] = "completed"
			blocked, err := gateEngine.prepareQualityLoopArchiveReadOnlyGate(&state)
			if err != nil || blocked {
				t.Fatalf("prepare archive gate = blocked:%v err:%v", blocked, err)
			}
			if err := archiveRepairEvidence(repo, state.RunID, changeName); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("archive gate drift\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			passed, err := gateEngine.verifyQualityLoopArchiveReadOnlyGate(&state)
			if err != nil || passed || state.Status != statusBlockedStalled {
				t.Fatalf("archive block = passed:%v state:%s/%s err:%v", passed, state.Status, state.Stage, err)
			}
			if err := saveState(repo, state); err != nil {
				t.Fatal(err)
			}
			before, err := loadState(repo, state.RunID)
			if err != nil {
				t.Fatal(err)
			}
			archivedMatches, err := filepath.Glob(filepath.Join(repo, "docs", "changes", "archive", "*-"+changeName))
			if err != nil || len(archivedMatches) != 1 {
				t.Fatalf("archived proposal matches = %v err=%v", archivedMatches, err)
			}
			if mode == "resume" {
				if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("new resume progress\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			var batchID string
			if mode == "batch" {
				batchID = newRunID()
				if err := saveBatchState(repo, BatchState{
					BatchID: batchID, Status: batchStatusRunning,
					Changes: []string{changeName}, RunIDs: map[string]string{changeName: state.RunID},
				}); err != nil {
					t.Fatal(err)
				}
			}
			registry := NewAgentRegistry()
			registry.Register(fakeWorkflowTool{runner: &recoveryGuardRunner{}})
			engine := NewEngine(repo, registry)
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
				err = engine.RestartBatchDetached(batchID)
			}
			if !errors.Is(err, errRecoveryDetachedStart) {
				t.Fatalf("%s detached error = %v", mode, err)
			}
			persisted, err := loadState(repo, state.RunID)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(persisted, before) {
				t.Fatalf("%s did not restore archive-blocked state:\nbefore=%#v\nafter=%#v", mode, before, persisted)
			}
			if _, err := os.Stat(changePath(repo, changeName)); !os.IsNotExist(err) {
				t.Fatalf("%s left proposal active after failed start: %v", mode, err)
			}
			if info, err := os.Stat(archivedMatches[0]); err != nil || !info.IsDir() {
				t.Fatalf("%s did not restore archived proposal: info=%v err=%v", mode, info, err)
			}
		})
	}
}

// TestArchiveBlockedDetachedPreparationFailureRestoresProposalLocation covers compensation before worker start.
func TestArchiveBlockedDetachedPreparationFailureRestoresProposalLocation(t *testing.T) {
	for _, mode := range []string{"resume", "restart", "batch"} {
		t.Run(mode, func(t *testing.T) {
			repo, changeName, state, gateEngine, _ := newQualityLoopArchiveGateFixture(t)
			state.Stages["audit_1"] = "completed"
			state.Stages["qa_1"] = "completed"
			blocked, err := gateEngine.prepareQualityLoopArchiveReadOnlyGate(&state)
			if err != nil || blocked {
				t.Fatalf("prepare archive gate = blocked:%v err:%v", blocked, err)
			}
			if err := archiveRepairEvidence(repo, state.RunID, changeName); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("archive gate drift\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			passed, err := gateEngine.verifyQualityLoopArchiveReadOnlyGate(&state)
			if err != nil || passed || state.Status != statusBlockedStalled {
				t.Fatalf("archive block = passed:%v state:%s/%s err:%v", passed, state.Status, state.Stage, err)
			}
			if err := saveState(repo, state); err != nil {
				t.Fatal(err)
			}
			before, err := loadState(repo, state.RunID)
			if err != nil {
				t.Fatal(err)
			}
			archivedMatches, err := filepath.Glob(filepath.Join(repo, "docs", "changes", "archive", "*-"+changeName))
			if err != nil || len(archivedMatches) != 1 {
				t.Fatalf("archived proposal matches = %v err=%v", archivedMatches, err)
			}
			if mode == "resume" {
				if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("new resume progress\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			var batchID string
			if mode == "batch" {
				batchID = newRunID()
				if err := saveBatchState(repo, BatchState{
					BatchID: batchID, Status: batchStatusRunning,
					Changes: []string{changeName}, RunIDs: map[string]string{changeName: state.RunID},
				}); err != nil {
					t.Fatal(err)
				}
			}
			registry := NewAgentRegistry()
			registry.Register(fakeWorkflowTool{runner: &recoveryGuardRunner{}})
			engine := NewEngine(repo, registry)
			originalAfterRestore := afterQualityLoopProposalRestore
			afterQualityLoopProposalRestore = func(string) error { return errRecoveryDetachedStart }
			t.Cleanup(func() { afterQualityLoopProposalRestore = originalAfterRestore })

			switch mode {
			case "resume":
				err = engine.ResumeDetachedAfterUserChoice(context.Background(), state.RunID)
			case "restart":
				err = engine.RestartRunDetached(state.RunID, false)
			case "batch":
				err = engine.RestartBatchDetached(batchID)
			}
			if !errors.Is(err, errRecoveryDetachedStart) {
				t.Fatalf("%s detached preparation error = %v", mode, err)
			}
			persisted, err := loadState(repo, state.RunID)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(persisted, before) {
				t.Fatalf("%s did not restore preparation state:\nbefore=%#v\nafter=%#v", mode, before, persisted)
			}
			if _, err := os.Stat(changePath(repo, changeName)); !os.IsNotExist(err) {
				t.Fatalf("%s left proposal active after preparation failure: %v", mode, err)
			}
			if info, err := os.Stat(archivedMatches[0]); err != nil || !info.IsDir() {
				t.Fatalf("%s did not restore archived proposal: info=%v err=%v", mode, info, err)
			}
		})
	}
}
