// Package app tests immutable acceptance snapshots and tracked proposal evidence promotion.
package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/xbugs221/oz/internal/acceptance"
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
		`mkdir -p test-results/demo && printf 'sealed demo' > test-results/demo/final-demo.webm`,
		sourcePath,
	)
	runRecoveryGit(t, repo, "init", "-q")
	contract, err := ReadAcceptance(acceptancePath(repo, changeName))
	if err != nil {
		t.Fatal(err)
	}
	contract.RequiredEvidence[0].Kind = "demo_video"
	contract.SubmissionEvidence = []acceptance.SubmissionEvidence{{
		EvidenceID:  contract.RequiredEvidence[0].ID,
		SourcePath:  sourcePath,
		ArchivePath: archivePath,
	}}
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
		Stages:        map[string]string{},
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
	if string(archived) != "sealed demo" {
		t.Fatalf("archived evidence = %q, want sealed snapshot", archived)
	}
	for _, relative := range []string{
		"tests/evidence/proposals/1-demo/README.md",
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
		`mkdir -p test-results/demo && printf 'sealed demo' > test-results/demo/final-demo.webm`,
		sourcePath,
	)
	runRecoveryGit(t, repo, "init", "-q")
	contract, err := ReadAcceptance(acceptancePath(repo, changeName))
	if err != nil {
		t.Fatal(err)
	}
	contract.RequiredEvidence[0].Kind = "demo_video"
	contract.SubmissionEvidence = []acceptance.SubmissionEvidence{{
		EvidenceID: contract.RequiredEvidence[0].ID, SourcePath: sourcePath, ArchivePath: archivePath,
	}}
	if err := writeJSONFile(acceptancePath(repo, changeName), contract); err != nil {
		t.Fatal(err)
	}
	state := State{
		RunID: newRunID(), ChangeName: changeName, Sealed: true, Status: statusRunning, Stage: "audit_1",
		Workflow: DefaultWorkflowConfig(), Stages: map[string]string{}, Validation: map[string]StageValidationState{},
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
	if string(restored) != "sealed demo" {
		t.Fatalf("restored archive = %q, want sealed snapshot", restored)
	}
}
