// Package app tests immutable acceptance snapshots and tracked proposal evidence promotion.
package app

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/xbugs221/oz/internal/acceptance"
	"github.com/xbugs221/oz/internal/testsupport"
)

// TestPromoteQualityAcceptanceEvidenceUsesSealedMapping proves archive promotion never rereads test-results.
func TestPromoteQualityAcceptanceEvidenceUsesSealedMapping(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", filepath.Join(t.TempDir(), "state"))
	const changeName = "1-demo"
	const sourcePath = "test-results/demo/final-demo.webm"
	const archivePath = "tests/evidence/proposals/1-demo/final-demo.webm"
	repo := acceptanceRunRepo(
		t,
		changeName,
		"pass-test",
		reviewableDemoCommand(sourcePath),
		sourcePath,
	)
	runRecoveryGit(t, repo, "init", "-q")
	contract, err := ReadAcceptance(acceptancePath(repo, changeName))
	if err != nil {
		t.Fatal(err)
	}
	contract.RequiredEvidence[0].Kind = "demo_video"
	contract.RequiredEvidence[0].Purpose = "展示审核人员按用户路径完成操作并看到最终结果。"
	contract.SubmissionEvidence = []acceptance.SubmissionEvidence{{
		EvidenceID:  contract.RequiredEvidence[0].ID,
		SourcePath:  sourcePath,
		ArchivePath: archivePath,
	}}
	contract.DeliveryReport = proposalDeliveryReport(contract.RequiredEvidence[0].ID)
	if err := writeJSONFile(acceptancePath(repo, changeName), contract); err != nil {
		t.Fatal(err)
	}
	state := State{
		RunID:         newRunID(),
		ChangeName:    changeName,
		Sealed:        true,
		Status:        statusRunning,
		Stage:         "audit_1",
		Workflow:      DefaultWorkflowConfig(),
		Stages:        map[string]string{"qa_1": "completed"},
		Validation:    map[string]StageValidationState{},
		AcceptanceRun: map[string]StageValidationState{},
		QualityLoop:   QualityLoopState{DiffHash: qualityHashStrings("tracked source snapshot")},
	}
	if err := snapshotQualityLoopAcceptance(repo, &state, acceptancePath(repo, changeName)); err != nil {
		t.Fatal(err)
	}
	engine := NewEngine(repo, NewAgentRegistry())
	passed, err := engine.runAcceptanceGate(context.Background(), &state)
	if err != nil || !passed {
		t.Fatalf("acceptance gate = passed:%v err:%v", passed, err)
	}
	checkpoint := state.AcceptanceRun["audit_1"]
	checkpoint.DiffHash = state.QualityLoop.DiffHash
	state.AcceptanceRun["audit_1"] = checkpoint
	writeFinalDeliveryQA(t, repo, state, contract.RequiredTests[0].ID, contract.RequiredEvidence[0].ID)
	if err := os.WriteFile(filepath.Join(repo, filepath.FromSlash(sourcePath)), []byte("overwritten temporary demo"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := promoteQualityAcceptanceEvidence(repo, state, "audit_1"); err != nil {
		t.Fatal(err)
	}
	archived, err := os.ReadFile(filepath.Join(repo, filepath.FromSlash(archivePath)))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(archived, testsupport.ReviewableWebM()) {
		t.Fatalf("archived evidence did not preserve the sealed video snapshot")
	}
	for _, relative := range []string{
		"tests/evidence/proposals/1-demo/README.md",
		"tests/evidence/proposals/1-demo/DELIVERY.md",
		"tests/evidence/proposals/1-demo/manifest.json",
		"tests/evidence/proposals/1-demo/result.json",
	} {
		if !fileExists(filepath.Join(repo, filepath.FromSlash(relative))) {
			t.Fatalf("archive bundle missing %s", relative)
		}
	}
}

// TestPromoteQualityAcceptanceEvidenceRestoresArchiveFromSeal refreshes an uncommitted package from trusted state.
func TestPromoteQualityAcceptanceEvidenceRestoresArchiveFromSeal(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", filepath.Join(t.TempDir(), "state"))
	const changeName = "1-demo"
	const sourcePath = "test-results/demo/final-demo.webm"
	const archivePath = "tests/evidence/proposals/1-demo/final-demo.webm"
	repo := acceptanceRunRepo(
		t,
		changeName,
		"pass-test",
		reviewableDemoCommand(sourcePath),
		sourcePath,
	)
	runRecoveryGit(t, repo, "init", "-q")
	contract, err := ReadAcceptance(acceptancePath(repo, changeName))
	if err != nil {
		t.Fatal(err)
	}
	contract.RequiredEvidence[0].Kind = "demo_video"
	contract.RequiredEvidence[0].Purpose = "展示审核人员按用户路径完成操作并看到最终结果。"
	contract.SubmissionEvidence = []acceptance.SubmissionEvidence{{
		EvidenceID: contract.RequiredEvidence[0].ID, SourcePath: sourcePath, ArchivePath: archivePath,
	}}
	contract.DeliveryReport = proposalDeliveryReport(contract.RequiredEvidence[0].ID)
	if err := writeJSONFile(acceptancePath(repo, changeName), contract); err != nil {
		t.Fatal(err)
	}
	state := State{
		RunID: newRunID(), ChangeName: changeName, Sealed: true, Status: statusRunning, Stage: "audit_1",
		Workflow: DefaultWorkflowConfig(), Stages: map[string]string{"qa_1": "completed"}, Validation: map[string]StageValidationState{},
		AcceptanceRun: map[string]StageValidationState{},
		QualityLoop:   QualityLoopState{DiffHash: qualityHashStrings("tracked source snapshot")},
	}
	if err := snapshotQualityLoopAcceptance(repo, &state, acceptancePath(repo, changeName)); err != nil {
		t.Fatal(err)
	}
	engine := NewEngine(repo, NewAgentRegistry())
	if passed, runErr := engine.runAcceptanceGate(context.Background(), &state); runErr != nil || !passed {
		t.Fatalf("acceptance gate = passed:%v err:%v", passed, runErr)
	}
	checkpoint := state.AcceptanceRun["audit_1"]
	checkpoint.DiffHash = state.QualityLoop.DiffHash
	state.AcceptanceRun["audit_1"] = checkpoint
	writeFinalDeliveryQA(t, repo, state, contract.RequiredTests[0].ID, contract.RequiredEvidence[0].ID)
	if err := promoteQualityAcceptanceEvidence(repo, state, "audit_1"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, filepath.FromSlash(archivePath)), []byte("tampered archive"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := promoteQualityAcceptanceEvidence(repo, state, "audit_1"); err != nil {
		t.Fatal(err)
	}
	restored, err := os.ReadFile(filepath.Join(repo, filepath.FromSlash(archivePath)))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(restored, testsupport.ReviewableWebM()) {
		t.Fatalf("restored archive did not match the sealed video snapshot")
	}
}

// reviewableDemoCommand writes the embedded real WebM fixture to the requested evidence path.
func reviewableDemoCommand(path string) string {
	return "mkdir -p test-results/demo && printf '%s' '" + testsupport.ReviewableWebMBase64() + "' | base64 -d > " + path
}

// proposalDeliveryReport describes the user-facing scenario exercised by promotion tests.
func proposalDeliveryReport(evidenceID string) *acceptance.DeliveryReport {
	return &acceptance.DeliveryReport{
		UserBenefits: []string{"审核人员能够沿用户路径查看最终演示，确认交付结果已经可用。"},
		Scenarios: []acceptance.DeliveryScenario{{
			ID:        "final-demo",
			Title:     "查看最终能力演示",
			UserValue: "用户能够完成一次完整操作，并看到清晰的最终结果。",
			Steps: []acceptance.DeliveryStep{{
				Action:   "打开演示视频并按其中路径完成一次用户操作。",
				Expected: "视频展示完整操作过程以及用户最终看到的结果。",
			}},
			EvidenceIDs: []string{evidenceID},
		}},
	}
}

// writeFinalDeliveryQA records the independent user-visible observation consumed by report generation.
func writeFinalDeliveryQA(t *testing.T, repo string, state State, testID, evidenceID string) {
	t.Helper()
	qa := QA{
		Summary:  "最终用户路径已经完成独立验收。",
		Decision: "clean",
		Evidence: []string{"runtime video shows the complete user workflow"},
		AcceptanceMatrix: []AcceptanceResult{
			{ID: testID, Status: "passed", Artifact: "final demo", Evidence: "真实演示完成用户路径"},
			{ID: evidenceID, Status: "passed", Artifact: "final demo", Evidence: "真实视频展示最终结果"},
		},
		UserAcceptance: []UserAcceptance{{
			ScenarioID:  "final-demo",
			Status:      "passed",
			Observed:    "演示中普通用户完成了完整操作，并看到了可直接使用的最终结果。",
			EvidenceIDs: []string{evidenceID},
		}},
	}
	if err := writeJSONFile(filepath.Join(runDir(repo, state.RunID), "qa-1.json"), qa); err != nil {
		t.Fatal(err)
	}
}
