// Package app tests durable, kind-isolated validation evidence for quality-loop runs.
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

// TestQualityValidationKindsDoNotOverwrite proves artifact and command gates keep independent evidence.
func TestQualityValidationKindsDoNotOverwrite(t *testing.T) {
	repo := t.TempDir()
	t.Setenv("XDG_STATE_HOME", filepath.Join(repo, "state"))
	installGateStateFakeOz(t, true)
	state := qualityLoopState("audit_1")
	state.RunID = "quality-validation-kind-isolation"

	if err := recordStageArtifactGateFailure(repo, &state, fmt.Errorf("audit artifact contract failed")); err != nil {
		t.Fatal(err)
	}
	artifactPath := state.ArtifactGates["audit_1"].LastArtifact
	artifactData, err := os.ReadFile(artifactPath)
	if err != nil {
		t.Fatal(err)
	}
	engine := NewEngine(repo, nil)
	passed, err := engine.validateStage(context.Background(), &state)
	if err != nil || !passed {
		t.Fatalf("command validation = passed:%v err:%v", passed, err)
	}
	commandPath := state.Validation["audit_1"].LastArtifact
	if artifactPath == commandPath ||
		!strings.Contains(artifactPath, "audit-1-artifact-1.json") ||
		!strings.Contains(commandPath, "audit-1-commands-1.json") {
		t.Fatalf("validation namespaces = artifact:%q commands:%q", artifactPath, commandPath)
	}
	unchanged, err := os.ReadFile(artifactPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(unchanged) != string(artifactData) {
		t.Fatal("command validation overwrote artifact-gate evidence")
	}
}

// TestQualityValidationReservationSurvivesRestart proves a crash cannot reuse a claimed attempt.
func TestQualityValidationReservationSurvivesRestart(t *testing.T) {
	repo := t.TempDir()
	t.Setenv("XDG_STATE_HOME", filepath.Join(repo, "state"))
	installGateStateFakeOz(t, true)
	state := qualityLoopState("audit_1")
	state.RunID = "quality-validation-reservation"
	if err := saveState(repo, state); err != nil {
		t.Fatal(err)
	}

	reserved, err := reserveValidationAttempt(
		repo,
		&state,
		state.Validation[state.Stage],
		validationKindCommands,
		func(current StageValidationState) {
			state.Validation[state.Stage] = current
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	first := ValidationAttempt{
		Stage:   state.Stage,
		Kind:    validationKindCommands,
		Attempt: reserved.Attempts,
		Status:  validationStatusFailed,
		Commands: []ValidationCommandResult{{
			Command:  "crashed validation",
			ExitCode: 1,
			Output:   "attempt-one-sentinel",
		}},
	}
	firstPath, err := writeValidationAttempt(repo, state.RunID, first)
	if err != nil {
		t.Fatal(err)
	}
	firstData, err := os.ReadFile(firstPath)
	if err != nil {
		t.Fatal(err)
	}

	restarted, err := loadState(repo, state.RunID)
	if err != nil {
		t.Fatal(err)
	}
	engine := NewEngine(repo, nil)
	passed, err := engine.validateStage(context.Background(), &restarted)
	if err != nil || !passed {
		t.Fatalf("restarted validation = passed:%v err:%v", passed, err)
	}
	if restarted.Validation["audit_1"].Attempts != 2 ||
		!strings.Contains(restarted.Validation["audit_1"].LastArtifact, "commands-2.json") {
		t.Fatalf("restarted validation state = %#v", restarted.Validation["audit_1"])
	}
	unchanged, err := os.ReadFile(firstPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(unchanged) != string(firstData) {
		t.Fatal("restart overwrote the reserved first validation attempt")
	}
}

// TestQualityValidationArtifactKindMatchesFailure keeps JSON identity aligned with its namespaced path.
func TestQualityValidationArtifactKindMatchesFailure(t *testing.T) {
	repo := t.TempDir()
	t.Setenv("XDG_STATE_HOME", filepath.Join(repo, "state"))
	installGateStateFakeOz(t, false)
	state := qualityLoopState("audit_1")
	state.RunID = "quality-validation-kind"

	engine := NewEngine(repo, nil)
	passed, err := engine.validateStage(context.Background(), &state)
	if err != nil || passed {
		t.Fatalf("failing change validation = passed:%v err:%v", passed, err)
	}
	path := state.Validation["audit_1"].LastArtifact
	if !strings.Contains(path, "audit-1-change-1.json") {
		t.Fatalf("change validation path = %q", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var attempt ValidationAttempt
	if err := json.Unmarshal(data, &attempt); err != nil {
		t.Fatal(err)
	}
	if attempt.Kind != validationKindChange {
		t.Fatalf("change validation JSON kind = %q", attempt.Kind)
	}
}

// TestQualityValidationOutputChangeCountsAsProgress distinguishes new diagnostics from a repeated failure.
func TestQualityValidationOutputChangeCountsAsProgress(t *testing.T) {
	repo := t.TempDir()
	t.Setenv("XDG_STATE_HOME", filepath.Join(repo, "state"))
	installGateStateFakeOz(t, true)
	outputPath := filepath.Join(repo, "validation-output.txt")
	if err := os.WriteFile(outputPath, []byte("failure-a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	state := qualityLoopState("audit_1")
	state.RunID = "quality-validation-output-progress"
	state.QualityLoop.DiffHash = qualityHashStrings("unchanged source")
	state.Workflow.Validation.Commands = []ValidationCommand{{
		Executable: "sh",
		Args:       []string{"-c", "cat validation-output.txt; exit 7"},
	}}
	engine := NewEngine(repo, nil)

	passed, err := engine.validateStage(context.Background(), &state)
	if err != nil || passed {
		t.Fatalf("first validation failure = passed:%v err:%v", passed, err)
	}
	firstHash := state.QualityLoop.ValidationHash
	firstFingerprint := state.QualityLoop.GateFailureFingerprint
	if err := os.WriteFile(outputPath, []byte("failure-b\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	passed, err = engine.validateStage(context.Background(), &state)
	if err != nil || passed {
		t.Fatalf("changed validation failure = passed:%v err:%v", passed, err)
	}
	if state.Status != statusRunning || state.Stage != "audit_1" {
		t.Fatalf("changed validation output stalled early: %s/%s", state.Status, state.Stage)
	}
	if firstHash == state.QualityLoop.ValidationHash ||
		firstFingerprint == state.QualityLoop.GateFailureFingerprint {
		t.Fatalf("changed validation output did not update progress: quality=%#v", state.QualityLoop)
	}
	secondHash := state.QualityLoop.ValidationHash
	secondFingerprint := state.QualityLoop.GateFailureFingerprint
	passed, err = engine.validateStage(context.Background(), &state)
	if err != nil || passed {
		t.Fatalf("repeated validation failure = passed:%v err:%v", passed, err)
	}
	if state.Status != statusBlockedStalled || state.Stage != statusBlockedStalled {
		t.Fatalf("repeated validation output state = %s/%s", state.Status, state.Stage)
	}
	if state.QualityLoop.ValidationHash != secondHash ||
		state.QualityLoop.GateFailureFingerprint != secondFingerprint {
		t.Fatalf("repeated validation output changed progress: quality=%#v", state.QualityLoop)
	}
}

// TestLegacyValidationArtifactPathRemainsCompatible keeps sealed repair-v1 evidence readable.
func TestLegacyValidationArtifactPathRemainsCompatible(t *testing.T) {
	repo := t.TempDir()
	t.Setenv("XDG_STATE_HOME", filepath.Join(repo, "state"))
	installGateStateFakeOz(t, true)
	state := qualityLoopState("repair_1")
	state.RunID = "legacy-validation-path"
	state.Workflow.Generation = repairWorkflowGeneration
	state.Workflow.MaxRepairIterations = 1

	engine := NewEngine(repo, nil)
	passed, err := engine.validateStage(context.Background(), &state)
	if err != nil || !passed {
		t.Fatalf("legacy validation = passed:%v err:%v", passed, err)
	}
	path := state.Validation["repair_1"].LastArtifact
	if filepath.Base(path) != "validation-repair-1-1.json" {
		t.Fatalf("legacy validation path = %q", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var attempt ValidationAttempt
	if err := json.Unmarshal(data, &attempt); err != nil {
		t.Fatal(err)
	}
	if attempt.Kind != "" {
		t.Fatalf("legacy validation unexpectedly changed JSON kind: %#v", attempt)
	}
}

// TestQualityValidationCheckpointRejectsArtifactTampering keeps QA/archive bound to actual validation output.
func TestQualityValidationCheckpointRejectsArtifactTampering(t *testing.T) {
	testCases := []struct {
		name   string
		tamper func(t *testing.T, path string, state *State)
	}{
		{
			name: "deleted",
			tamper: func(t *testing.T, path string, _ *State) {
				t.Helper()
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "content changed",
			tamper: func(t *testing.T, path string, _ *State) {
				t.Helper()
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				var attempt ValidationAttempt
				if err := json.Unmarshal(data, &attempt); err != nil {
					t.Fatal(err)
				}
				attempt.Commands[0].Output = "forged validation output"
				changed, err := json.MarshalIndent(attempt, "", "  ")
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, append(changed, '\n'), 0o644); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "replaced by symlink",
			tamper: func(t *testing.T, path string, _ *State) {
				t.Helper()
				target := filepath.Join(filepath.Dir(path), "forged-validation.json")
				if err := os.WriteFile(target, []byte("{}\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, path); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "state path replaced",
			tamper: func(t *testing.T, path string, state *State) {
				t.Helper()
				other := filepath.Join(filepath.Dir(path), "copied-validation.json")
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(other, data, 0o644); err != nil {
					t.Fatal(err)
				}
				checkpoint := state.Validation["audit_1"]
				checkpoint.LastArtifact = other
				state.Validation["audit_1"] = checkpoint
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			repo := t.TempDir()
			state := qualityLoopState("qa_1")
			state.RunID = "quality-validation-checkpoint"
			state.QualityLoop.DiffHash = qualityHashStrings("tested source")
			attempt := ValidationAttempt{
				Stage:      "audit_1",
				Kind:       validationKindCommands,
				Attempt:    1,
				Status:     validationStatusPassed,
				DiffHash:   state.QualityLoop.DiffHash,
				StartedAt:  "2026-07-27T00:00:00Z",
				FinishedAt: "2026-07-27T00:00:01Z",
				Commands: []ValidationCommandResult{{
					Command:  "go test ./internal/app",
					ExitCode: 0,
					Output:   "ok",
				}},
			}
			path, err := writeValidationAttempt(repo, state.RunID, attempt)
			if err != nil {
				t.Fatal(err)
			}
			state.QualityLoop.ValidationHash = qualityValidationProgressHash(attempt)
			state.Validation["audit_1"] = StageValidationState{
				Attempts:     attempt.Attempt,
				Kind:         attempt.Kind,
				Status:       validationStatusPassed,
				LastArtifact: path,
				DiffHash:     attempt.DiffHash,
			}
			if _, err := verifyQualityValidationCheckpoint(repo, state, "audit_1"); err != nil {
				t.Fatalf("valid checkpoint rejected: %v", err)
			}

			testCase.tamper(t, path, &state)
			if _, err := verifyQualityValidationCheckpoint(repo, state, "audit_1"); err == nil {
				t.Fatalf("tampered validation checkpoint %q was accepted", testCase.name)
			}
		})
	}
}

// TestWriteValidationAttemptAtomicallyReplacesSymlink keeps checkpoint writes inside their namespace.
func TestWriteValidationAttemptAtomicallyReplacesSymlink(t *testing.T) {
	repo := t.TempDir()
	runID := "quality-validation-atomic-write"
	attempt := ValidationAttempt{
		Stage:      "audit_1",
		Kind:       validationKindCommands,
		Attempt:    1,
		Status:     validationStatusPassed,
		DiffHash:   qualityHashStrings("source"),
		StartedAt:  "2026-07-27T00:00:00Z",
		FinishedAt: "2026-07-27T00:00:01Z",
		Commands: []ValidationCommandResult{{
			Command: "go test ./internal/app",
			Output:  "ok",
		}},
	}
	path, _, err := validationArtifactPath(repo, runID, attempt.Stage, attempt.Kind, attempt.Attempt)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	victim := filepath.Join(repo, "victim.json")
	if err := os.WriteFile(victim, []byte("sentinel\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(victim, path); err != nil {
		t.Fatal(err)
	}

	written, err := writeValidationAttempt(repo, runID, attempt)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(written)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		t.Fatalf("validation artifact mode = %s", info.Mode())
	}
	victimData, err := os.ReadFile(victim)
	if err != nil {
		t.Fatal(err)
	}
	if string(victimData) != "sentinel\n" {
		t.Fatalf("symlink victim changed: %q", victimData)
	}
}
