// Package app tests sealed acceptance integrity, durable gate evidence, and safe log persistence.
package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestQualityLoopRejectsTamperedSealedAcceptance verifies every quality-loop reader fails closed.
func TestQualityLoopRejectsTamperedSealedAcceptance(t *testing.T) {
	repo, state := acceptanceIntegrityFixture(t, "audit_1")
	sealedPath := filepath.Join(runDir(repo, state.RunID), "acceptance.json")
	data, err := os.ReadFile(sealedPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sealedPath, append(data, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	tamperedHash := acceptanceContentHash(append(data, '\n'))
	if err := os.WriteFile(
		filepath.Join(runDir(repo, state.RunID), sealedAcceptanceHashFile),
		[]byte(tamperedHash+"\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	if _, err := readAcceptanceForState(repo, state); err == nil || !strings.Contains(err.Error(), "完整性校验失败") {
		t.Fatalf("tampered acceptance read error = %v", err)
	}
	if _, err := promptContext(repo, state); err == nil || !strings.Contains(err.Error(), "完整性校验失败") {
		t.Fatalf("tampered acceptance prompt error = %v", err)
	}
	engine := NewEngine(repo, NewAgentRegistry())
	passed, err := engine.runAcceptanceGate(context.Background(), &state)
	if err != nil || passed || state.AcceptanceRun["audit_1"].Status != validationStatusFailed {
		t.Fatalf("tampered acceptance gate = passed:%v err:%v state=%#v", passed, err, state.AcceptanceRun["audit_1"])
	}
}

// TestQualityLoopRequiresSealedAcceptanceHash verifies missing integrity metadata cannot use an active fallback.
func TestQualityLoopRequiresSealedAcceptanceHash(t *testing.T) {
	repo, state := acceptanceIntegrityFixture(t, "audit_1")
	state.AcceptanceHash = ""
	if _, err := readAcceptanceForState(repo, state); err == nil || !strings.Contains(err.Error(), "完整性哈希") {
		t.Fatalf("missing acceptance hash error = %v", err)
	}
}

// TestQualityLoopAcceptanceResultsAreDurableAndBound verifies dynamic stages keep immutable per-attempt proof.
func TestQualityLoopAcceptanceResultsAreDurableAndBound(t *testing.T) {
	repo, state := acceptanceIntegrityFixture(t, "audit_1")
	engine := NewEngine(repo, NewAgentRegistry())

	passed, err := engine.runAcceptanceGate(context.Background(), &state)
	if err != nil || !passed {
		t.Fatalf("audit acceptance gate = passed:%v err:%v", passed, err)
	}
	firstPath := state.AcceptanceRun["audit_1"].LastArtifact
	firstData, err := os.ReadFile(filepath.Join(repo, filepath.FromSlash(firstPath)))
	if err != nil {
		t.Fatal(err)
	}

	state.Stage = "targeted_repair_1"
	passed, err = engine.runAcceptanceGate(context.Background(), &state)
	if err != nil || !passed {
		t.Fatalf("targeted acceptance gate = passed:%v err:%v", passed, err)
	}
	secondPath := state.AcceptanceRun["targeted_repair_1"].LastArtifact
	if firstPath == secondPath {
		t.Fatalf("dynamic acceptance results share path %q", firstPath)
	}
	unchanged, err := os.ReadFile(filepath.Join(repo, filepath.FromSlash(firstPath)))
	if err != nil {
		t.Fatal(err)
	}
	if string(unchanged) != string(firstData) {
		t.Fatal("later acceptance stage overwrote the first result")
	}

	var result AcceptanceRunResult
	if err := json.Unmarshal(unchanged, &result); err != nil {
		t.Fatal(err)
	}
	if result.RunID != state.RunID || result.Stage != "audit_1" || result.Attempt != 1 {
		t.Fatalf("result identity binding = %#v", result)
	}
	if result.DiffHash != state.QualityLoop.DiffHash || result.ContractHash != state.AcceptanceHash ||
		result.TestsHash == "" || result.EvidenceHash == "" ||
		result.TestsProgressHash == "" || result.EvidenceProgressHash == "" {
		t.Fatalf("result content binding = %#v", result)
	}
	for _, testResult := range result.Tests {
		if !strings.Contains(testResult.LogPath, filepath.ToSlash(filepath.Join(state.RunID, "audit_1", "attempt-1"))) {
			t.Fatalf("log path lacks durable namespace: %q", testResult.LogPath)
		}
	}
}

// TestVerifyQualityAcceptanceCheckpointAcceptsUntamperedResult proves every persisted binding can be replayed.
func TestVerifyQualityAcceptanceCheckpointAcceptsUntamperedResult(t *testing.T) {
	repo, state := passedAcceptanceCheckpointFixture(t)
	result, err := verifyQualityAcceptanceCheckpoint(repo, state, "audit_1")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Valid || result.Status != validationStatusPassed {
		t.Fatalf("verified checkpoint = %#v", result)
	}
}

// TestVerifyQualityAcceptanceCheckpointAcceptsLegacyProgressHash derives the optional semantic hash from a trusted log.
func TestVerifyQualityAcceptanceCheckpointAcceptsLegacyProgressHash(t *testing.T) {
	repo, state := passedAcceptanceCheckpointFixture(t)
	result := readAcceptanceResultForState(t, repo, state, "audit_1")
	result.Tests[0].LogProgressHash = ""
	if err := writeAcceptanceRunResult(repo, result); err != nil {
		t.Fatal(err)
	}
	verified, err := verifyQualityAcceptanceCheckpoint(repo, state, "audit_1")
	if err != nil {
		t.Fatal(err)
	}
	if !verified.Valid || verified.Status != validationStatusPassed {
		t.Fatalf("legacy checkpoint = %#v", verified)
	}
}

// TestVerifyQualityAcceptanceCheckpointRejectsTampering covers metadata, logs, and current evidence.
func TestVerifyQualityAcceptanceCheckpointRejectsTampering(t *testing.T) {
	for _, testCase := range []struct {
		name   string
		tamper func(*testing.T, string, *State)
	}{
		{
			name: "result identity",
			tamper: func(t *testing.T, repo string, state *State) {
				result := readAcceptanceResultForState(t, repo, *state, "audit_1")
				result.Stage = "qa_1"
				if err := writeAcceptanceRunResult(repo, result); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "result status",
			tamper: func(t *testing.T, repo string, state *State) {
				result := readAcceptanceResultForState(t, repo, *state, "audit_1")
				result.Valid = false
				result.Status = validationStatusFailed
				if err := writeAcceptanceRunResult(repo, result); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "test log content",
			tamper: func(t *testing.T, repo string, state *State) {
				result := readAcceptanceResultForState(t, repo, *state, "audit_1")
				path := filepath.Join(repo, filepath.FromSlash(result.Tests[0].LogPath))
				if err := os.WriteFile(path, []byte("tampered log\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "test progress hash",
			tamper: func(t *testing.T, repo string, state *State) {
				result := readAcceptanceResultForState(t, repo, *state, "audit_1")
				result.Tests[0].LogProgressHash = qualityHashStrings("tampered")
				if err := writeAcceptanceRunResult(repo, result); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "test log symlink",
			tamper: func(t *testing.T, repo string, state *State) {
				result := readAcceptanceResultForState(t, repo, *state, "audit_1")
				path := filepath.Join(repo, filepath.FromSlash(result.Tests[0].LogPath))
				data, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				outside := filepath.Join(t.TempDir(), "outside.log")
				if err := os.WriteFile(outside, data, 0o644); err != nil {
					t.Fatal(err)
				}
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(outside, path); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "current evidence",
			tamper: func(t *testing.T, repo string, _ *State) {
				if err := os.WriteFile(
					filepath.Join(repo, "test-results", "demo", "runtime.log"),
					[]byte("tampered evidence\n"),
					0o644,
				); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "state tests hash",
			tamper: func(_ *testing.T, _ string, state *State) {
				state.QualityLoop.TestsHash = qualityHashStrings("tampered")
			},
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			repo, state := passedAcceptanceCheckpointFixture(t)
			testCase.tamper(t, repo, &state)
			if _, err := verifyQualityAcceptanceCheckpoint(repo, state, "audit_1"); err == nil {
				t.Fatalf("tampered checkpoint %q was accepted", testCase.name)
			}
		})
	}
}

// TestAcceptanceLogsAvoidCollisionsAndRedactEnvironmentValues verifies safe persistence before marker scanning.
func TestAcceptanceLogsAvoidCollisionsAndRedactEnvironmentValues(t *testing.T) {
	repo := t.TempDir()
	resultDirRel := "test-results/acceptance-run/collision"
	resultDir := filepath.Join(repo, filepath.FromSlash(resultDirRel))
	if err := os.MkdirAll(resultDir, 0o755); err != nil {
		t.Fatal(err)
	}
	results, diagnostics := runAcceptanceTests(context.Background(), repo, resultDir, resultDirRel, true, []AcceptanceTest{
		{ID: "env/test", Command: "printf 'API_TOKEN=prefix-acceptance-secret blocked_environment: ENV_ONE=top-secret\\n'; exit 7"},
		{ID: "env-test", Command: "printf 'normal output\\n'"},
	})
	if len(diagnostics) != 0 {
		t.Fatalf("unexpected acceptance log diagnostics = %#v", diagnostics)
	}
	if len(results) != 2 || results[0].LogPath == results[1].LogPath {
		t.Fatalf("colliding acceptance logs = %#v", results)
	}
	firstLog, err := os.ReadFile(filepath.Join(repo, filepath.FromSlash(results[0].LogPath)))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(firstLog), "top-secret") ||
		strings.Contains(string(firstLog), "prefix-acceptance-secret") ||
		!strings.Contains(string(firstLog), "blocked_environment: ENV_ONE") {
		t.Fatalf("unsafe persisted environment marker: %q", string(firstLog))
	}
	names := qualityEnvironmentNamesFromAcceptanceResult(repo, AcceptanceRunResult{Tests: results})
	if strings.Join(names, ",") != "ENV_ONE" {
		t.Fatalf("redacted marker names = %#v", names)
	}
}

// TestQualityLoopRepeatedAcceptanceFailureStalls verifies attempt paths do not fake progress.
func TestQualityLoopRepeatedAcceptanceFailureStalls(t *testing.T) {
	repo := acceptanceRunRepo(
		t,
		"1-demo",
		"failing-test",
		`mkdir -p test-results/demo && printf failed > test-results/demo/runtime.log; exit 7`,
		"test-results/demo/runtime.log",
	)
	state := State{
		RunID:         newRunID(),
		ChangeName:    "1-demo",
		Sealed:        true,
		Status:        statusRunning,
		Stage:         "audit_1",
		Workflow:      DefaultWorkflowConfig(),
		Stages:        map[string]string{},
		Validation:    map[string]StageValidationState{},
		AcceptanceRun: map[string]StageValidationState{},
		QualityLoop:   QualityLoopState{DiffHash: qualityHashStrings("unchanged source")},
	}
	if err := snapshotQualityLoopAcceptance(repo, &state, acceptancePath(repo, state.ChangeName)); err != nil {
		t.Fatal(err)
	}
	engine := NewEngine(repo, NewAgentRegistry())
	passed, err := engine.runAcceptanceGate(context.Background(), &state)
	if err != nil || passed {
		t.Fatalf("first acceptance failure = passed:%v err:%v", passed, err)
	}
	firstPath := state.AcceptanceRun[state.Stage].LastArtifact
	firstData, err := os.ReadFile(filepath.Join(repo, filepath.FromSlash(firstPath)))
	if err != nil {
		t.Fatal(err)
	}
	passed, err = engine.runAcceptanceGate(context.Background(), &state)
	if err != nil || passed {
		t.Fatalf("second acceptance failure = passed:%v err:%v", passed, err)
	}
	if state.Status != statusBlockedStalled || state.Stage != statusBlockedStalled {
		t.Fatalf("repeated failure state = %s/%s", state.Status, state.Stage)
	}
	secondPath := state.AcceptanceRun["audit_1"].LastArtifact
	if firstPath == secondPath || !strings.Contains(secondPath, "attempt-2") {
		t.Fatalf("attempt paths = first:%q second:%q", firstPath, secondPath)
	}
	unchanged, err := os.ReadFile(filepath.Join(repo, filepath.FromSlash(firstPath)))
	if err != nil {
		t.Fatal(err)
	}
	if string(unchanged) != string(firstData) {
		t.Fatal("second acceptance failure overwrote first attempt")
	}
}

// TestQualityLoopAcceptanceFailureLogChangeCountsAsProgress verifies new failure output delays a stall.
func TestQualityLoopAcceptanceFailureLogChangeCountsAsProgress(t *testing.T) {
	repo := acceptanceRunRepo(
		t,
		"1-demo",
		"failing-test",
		`mkdir -p test-results/demo; cat failure-output.txt; printf evidence > test-results/demo/runtime.log; exit 7`,
		"test-results/demo/runtime.log",
	)
	outputPath := filepath.Join(repo, "failure-output.txt")
	if err := os.WriteFile(outputPath, []byte("first failure\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	state := State{
		RunID:         newRunID(),
		ChangeName:    "1-demo",
		Sealed:        true,
		Status:        statusRunning,
		Stage:         "audit_1",
		Workflow:      DefaultWorkflowConfig(),
		Stages:        map[string]string{},
		Validation:    map[string]StageValidationState{},
		AcceptanceRun: map[string]StageValidationState{},
		QualityLoop:   QualityLoopState{DiffHash: qualityHashStrings("unchanged source")},
	}
	if err := snapshotQualityLoopAcceptance(repo, &state, acceptancePath(repo, state.ChangeName)); err != nil {
		t.Fatal(err)
	}
	engine := NewEngine(repo, NewAgentRegistry())
	passed, err := engine.runAcceptanceGate(context.Background(), &state)
	if err != nil || passed {
		t.Fatalf("first acceptance failure = passed:%v err:%v", passed, err)
	}
	first := readAcceptanceResultForState(t, repo, state, "audit_1")
	if err := os.WriteFile(outputPath, []byte("second failure\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	passed, err = engine.runAcceptanceGate(context.Background(), &state)
	if err != nil || passed {
		t.Fatalf("changed acceptance failure = passed:%v err:%v", passed, err)
	}
	if state.Status != statusRunning || state.Stage != "audit_1" {
		t.Fatalf("changed failure output stalled early: %s/%s", state.Status, state.Stage)
	}
	second := readAcceptanceResultForState(t, repo, state, "audit_1")
	if len(first.Tests) != 1 || len(second.Tests) != 1 ||
		first.Tests[0].LogHash == "" || first.Tests[0].LogHash == second.Tests[0].LogHash ||
		first.TestsHash == second.TestsHash {
		t.Fatalf("failure output hashes did not change: first=%#v second=%#v", first, second)
	}
	passed, err = engine.runAcceptanceGate(context.Background(), &state)
	if err != nil || passed {
		t.Fatalf("repeated second failure = passed:%v err:%v", passed, err)
	}
	if state.Status != statusBlockedStalled || state.Stage != statusBlockedStalled {
		t.Fatalf("repeated failure output state = %s/%s", state.Status, state.Stage)
	}
}

// TestQualityLoopEvidenceContentChangeCountsAsProgress separates volatile metadata from substantive evidence.
func TestQualityLoopEvidenceContentChangeCountsAsProgress(t *testing.T) {
	repo := t.TempDir()
	path := filepath.Join(repo, "test-results", "evidence.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	result := AcceptanceRunResult{Evidence: []AcceptanceRunEvidenceResult{{
		ID: "runtime", Kind: "state_snapshot", Path: "test-results/evidence.json", Status: "present",
	}}}
	firstBody := `{"change_name":"demo","stage":"audit_1","workflow_config":{},"result":"failed","stage_timings":{"audit_1":{"started_at":"2026-07-27T16:10:01+08:00","finished_at":"2026-07-27T16:10:02+08:00"}},"dag_nodes":{"audit_1":{"status":"success","finished_at":"2026-07-27T16:10:02+08:00"}},"worker":{"pid":101,"hostname":"runner-a","last_heartbeat_at":"2026-07-27T16:10:02+08:00"},"validation":{"audit_1":{"attempts":1,"last_artifact":"/run/validation-audit_1-1.json","commands":[{"command":"go test ./...","exit_code":0,"duration_ms":12}]}},"acceptance_run":{"audit_1":{"attempts":1,"last_artifact":"test-results/stages/audit_1/attempt-1/result.json"}}}`
	if err := os.WriteFile(path, []byte(firstBody), 0o644); err != nil {
		t.Fatal(err)
	}
	_, first := qualityAcceptanceOutcomeHashes(repo, result)
	secondBody := `{"change_name":"demo","stage":"audit_1","workflow_config":{},"result":"failed","stage_timings":{"audit_1":{"started_at":"2026-07-27T03:10:02-05:00","finished_at":"2026-07-27T03:10:03-05:00"}},"dag_nodes":{"audit_1":{"status":"success","finished_at":"2026-07-27T03:10:03-05:00"}},"worker":{"pid":202,"hostname":"runner-b","last_heartbeat_at":"2026-07-27T03:10:03-05:00"},"validation":{"audit_1":{"attempts":2,"last_artifact":"/run/validation-audit_1-2.json","commands":[{"command":"go test ./...","exit_code":0,"duration_ms":19}]}},"acceptance_run":{"audit_1":{"attempts":2,"last_artifact":"test-results/stages/audit_1/attempt-2/result.json"}}}`
	if err := os.WriteFile(path, []byte(secondBody), 0o644); err != nil {
		t.Fatal(err)
	}
	_, volatileOnly := qualityAcceptanceOutcomeHashes(repo, result)
	if first != volatileOnly {
		t.Fatal("volatile evidence metadata incorrectly counted as progress")
	}
	if err := os.WriteFile(path, []byte(`{"change_name":"demo","stage":"audit_1","workflow_config":{},"result":"passed","stage_timings":{"audit_1":{"started_at":"2026-07-27T03:10:03-05:00","finished_at":"2026-07-27T03:10:04-05:00"}},"validation":{"audit_1":{"commands":[{"command":"go test ./...","exit_code":0,"duration_ms":20}]}}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, substantive := qualityAcceptanceOutcomeHashes(repo, result)
	if first == substantive {
		t.Fatal("substantive evidence content change did not count as progress")
	}
	if err := os.WriteFile(path, []byte(`{"result":"failed","expires_at":"2026-07-27T03:10:03-05:00","duration_ms":20}`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, businessFirst := qualityAcceptanceOutcomeHashes(repo, result)
	if err := os.WriteFile(path, []byte(`{"result":"failed","expires_at":"2026-07-28T03:10:03-05:00","duration_ms":25}`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, businessSecond := qualityAcceptanceOutcomeHashes(repo, result)
	if businessFirst == businessSecond {
		t.Fatal("business timestamp or duration change was incorrectly normalized")
	}
	if err := os.WriteFile(path, []byte(`{"order":{"started_at":"2026-07-27T03:10:03-05:00","command":"ship","duration_ms":20}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, nestedBusinessFirst := qualityAcceptanceOutcomeHashes(repo, result)
	if err := os.WriteFile(path, []byte(`{"order":{"started_at":"2026-07-28T03:10:03-05:00","command":"ship","duration_ms":25}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, nestedBusinessSecond := qualityAcceptanceOutcomeHashes(repo, result)
	if nestedBusinessFirst == nestedBusinessSecond {
		t.Fatal("nested business runtime fields were incorrectly normalized")
	}
	firstEngineWithBusiness := `{"change_name":"demo","stage":"audit_1","workflow_config":{},"order":{"change_name":"business","stage":"shipping","workflow_config":{},"started_at":"2026-07-27T03:10:03-05:00","command":"ship","duration_ms":20}}`
	if err := os.WriteFile(path, []byte(firstEngineWithBusiness), 0o644); err != nil {
		t.Fatal(err)
	}
	_, engineBusinessFirst := qualityAcceptanceOutcomeHashes(repo, result)
	secondEngineWithBusiness := `{"change_name":"demo","stage":"audit_1","workflow_config":{},"order":{"change_name":"business","stage":"shipping","workflow_config":{},"started_at":"2026-07-28T03:10:03-05:00","command":"ship","duration_ms":25}}`
	if err := os.WriteFile(path, []byte(secondEngineWithBusiness), 0o644); err != nil {
		t.Fatal(err)
	}
	_, engineBusinessSecond := qualityAcceptanceOutcomeHashes(repo, result)
	if engineBusinessFirst == engineBusinessSecond {
		t.Fatal("business fields nested in engine state were incorrectly normalized")
	}
	validSnapshot := `{"change_name":"demo","stage":"audit_1","workflow_config":{},"stage_timings":{"audit_1":{"started_at":"2026-07-27T03:10:03-05:00"}}}`
	if err := os.WriteFile(path, []byte(validSnapshot), 0o644); err != nil {
		t.Fatal(err)
	}
	_, singleDocument := qualityAcceptanceOutcomeHashes(repo, result)
	if err := os.WriteFile(path, []byte(validSnapshot+"\n"+`{"business":"appended"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, trailingDocument := qualityAcceptanceOutcomeHashes(repo, result)
	if singleDocument == trailingDocument {
		t.Fatal("trailing state snapshot content was silently discarded")
	}
}

// TestQualityEvidenceProgressHashStreamsLargeArtifacts keeps trace and video hashing memory-bounded.
func TestQualityEvidenceProgressHashStreamsLargeArtifacts(t *testing.T) {
	repo := t.TempDir()
	path := filepath.Join(repo, "test-results", "large-trace.bin")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Truncate(qualityEvidenceTextLimit + 1); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	first := qualityEvidenceProgressHash(repo, "test-results/large-trace.bin", "trace")
	file, err = os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteAt([]byte{1}, qualityEvidenceTextLimit); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	second := qualityEvidenceProgressHash(repo, "test-results/large-trace.bin", "trace")
	if first == "unavailable" || first == second {
		t.Fatalf("large evidence hashes = %q/%q", first, second)
	}

	runtimePath := filepath.Join(repo, "test-results", "large-runtime.log")
	filler := strings.Repeat("stable runtime output\n", int(qualityEvidenceTextLimit/20)+1)
	if err := os.WriteFile(runtimePath, []byte(filler+"--- PASS: TestStable (0.01s)\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runtimeFirst := qualityEvidenceProgressHash(repo, "test-results/large-runtime.log", "runtime_log")
	if err := os.WriteFile(runtimePath, []byte(filler+"--- PASS: TestStable (0.09s)\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runtimeDurationOnly := qualityEvidenceProgressHash(repo, "test-results/large-runtime.log", "runtime_log")
	if runtimeFirst == "unavailable" || runtimeFirst != runtimeDurationOnly {
		t.Fatalf("large runtime duration hashes = %q/%q", runtimeFirst, runtimeDurationOnly)
	}
	if err := os.WriteFile(runtimePath, []byte(filler+"--- FAIL: TestStable (0.09s)\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if substantive := qualityEvidenceProgressHash(repo, "test-results/large-runtime.log", "runtime_log"); substantive == runtimeFirst {
		t.Fatal("large runtime substantive change was normalized away")
	}

	longRuntimePrefix := "--- PASS: Test" + strings.Repeat("Long", 20<<10)
	if err := os.WriteFile(runtimePath, []byte(longRuntimePrefix+" (0.01s)\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	longRuntimeFirst := qualityEvidenceProgressHash(repo, "test-results/large-runtime.log", "runtime_log")
	if err := os.WriteFile(runtimePath, []byte(longRuntimePrefix+" (0.09s)\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if longRuntimeDurationOnly := qualityEvidenceProgressHash(repo, "test-results/large-runtime.log", "runtime_log"); longRuntimeDurationOnly != longRuntimeFirst {
		t.Fatalf("long runtime duration hashes = %q/%q", longRuntimeFirst, longRuntimeDurationOnly)
	}

	statePath := filepath.Join(repo, "test-results", "large-state.json")
	padding := strings.Repeat("x", int(qualityEvidenceTextLimit))
	firstState := `{"change_name":"demo","stage":"audit_1","workflow_config":{},"padding":"` +
		padding + `","stage_timings":{"audit_1":{"started_at":"2026-07-27T16:10:01Z"}}}`
	if err := os.WriteFile(statePath, []byte(firstState), 0o644); err != nil {
		t.Fatal(err)
	}
	largeStateFirst := qualityEvidenceProgressHash(repo, "test-results/large-state.json", "state_snapshot")
	secondState := `{"change_name":"demo","stage":"audit_1","workflow_config":{},"padding":"` +
		padding + `","stage_timings":{"audit_1":{"started_at":"2026-07-28T16:10:01Z"}}}`
	if err := os.WriteFile(statePath, []byte(secondState), 0o644); err != nil {
		t.Fatal(err)
	}
	if largeStateDurationOnly := qualityEvidenceProgressHash(repo, "test-results/large-state.json", "state_snapshot"); largeStateDurationOnly != largeStateFirst {
		t.Fatalf("oversized state runtime hashes = %q/%q", largeStateFirst, largeStateDurationOnly)
	}
}

// readAcceptanceResultForState loads the latest durable acceptance result for one stage.
func readAcceptanceResultForState(t *testing.T, repo string, state State, stage string) AcceptanceRunResult {
	t.Helper()
	path := state.AcceptanceRun[stage].LastArtifact
	data, err := os.ReadFile(filepath.Join(repo, filepath.FromSlash(path)))
	if err != nil {
		t.Fatal(err)
	}
	var result AcceptanceRunResult
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

// TestQualityLoopAcceptanceAttemptReservationSurvivesRestart verifies crash recovery advances the namespace.
func TestQualityLoopAcceptanceAttemptReservationSurvivesRestart(t *testing.T) {
	repo, state := acceptanceIntegrityFixture(t, "audit_1")
	engine := NewEngine(repo, NewAgentRegistry())
	reserved, err := engine.reserveAcceptanceRunAttempt(&state)
	if err != nil {
		t.Fatal(err)
	}
	if reserved.Attempts != 1 {
		t.Fatalf("reserved attempt = %d, want 1", reserved.Attempts)
	}
	firstRel, err := acceptanceRunResultDir(state.ChangeName, acceptanceRunBinding{
		RunID: state.RunID, Stage: state.Stage, Attempt: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	firstPath := filepath.Join(repo, filepath.FromSlash(firstRel), "result.json")
	if err := os.MkdirAll(filepath.Dir(firstPath), 0o755); err != nil {
		t.Fatal(err)
	}
	const sentinel = "reserved-attempt-one\n"
	if err := os.WriteFile(firstPath, []byte(sentinel), 0o644); err != nil {
		t.Fatal(err)
	}

	restarted, err := loadState(repo, state.RunID)
	if err != nil {
		t.Fatal(err)
	}
	passed, err := engine.runAcceptanceGate(context.Background(), &restarted)
	if err != nil || !passed {
		t.Fatalf("restarted acceptance gate = passed:%v err:%v", passed, err)
	}
	if restarted.AcceptanceRun["audit_1"].Attempts != 2 ||
		!strings.Contains(restarted.AcceptanceRun["audit_1"].LastArtifact, "attempt-2") {
		t.Fatalf("restarted attempt state = %#v", restarted.AcceptanceRun["audit_1"])
	}
	data, err := os.ReadFile(firstPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != sentinel {
		t.Fatalf("reserved attempt was overwritten: %q", data)
	}
}

// TestQualityLoopDeliveryAcceptanceMustMatchSealed verifies active and archived contracts cannot drift.
func TestQualityLoopDeliveryAcceptanceMustMatchSealed(t *testing.T) {
	repo, state := acceptanceIntegrityFixture(t, "audit_1")
	activePath := acceptancePath(repo, state.ChangeName)
	data, err := os.ReadFile(activePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(activePath, append(data, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	engine := NewEngine(repo, NewAgentRegistry())
	if err := engine.verifyQualityLoopActiveAcceptance(state); err == nil {
		t.Fatal("active acceptance drift was accepted")
	}
	audit := cleanReviewForStageDecision()
	audit.Evidence = []string{"go test ./internal/app；runtime delivery acceptance integrity checked"}
	if err := writeJSONFile(filepath.Join(runDir(repo, state.RunID), "audit-1.json"), audit); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.completeMainStage(context.Background(), &state, stageGatePipelineLoop); err == nil ||
		!strings.Contains(err.Error(), "delivery acceptance") {
		t.Fatalf("repair gate accepted active acceptance drift: %v", err)
	}
	if state.AcceptanceRun["audit_1"].Attempts != 0 {
		t.Fatalf("acceptance commands ran before delivery contract check: %#v", state.AcceptanceRun["audit_1"])
	}

	archiveDir := filepath.Join(repo, "docs", "changes", "archive", "2026-07-27-"+state.ChangeName)
	if err := os.MkdirAll(archiveDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(archiveDir, "acceptance.json"), append(data, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := engine.verifyQualityLoopArchivedAcceptance(state); err == nil {
		t.Fatal("archived acceptance drift was accepted")
	}
	if err := os.WriteFile(filepath.Join(archiveDir, "acceptance.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := engine.verifyQualityLoopArchivedAcceptance(state); err != nil {
		t.Fatalf("matching archived acceptance rejected: %v", err)
	}
}

// TestQualityLoopValidationSourceMutationStopsBeforeAcceptance prevents false result diff bindings.
func TestQualityLoopValidationSourceMutationStopsBeforeAcceptance(t *testing.T) {
	repo, changeName, acceptanceSource, _, _ := newRepairEvidenceFixture(t)
	state := qualityLoopState("audit_1")
	state.RunID = newRunID()
	state.ChangeName = changeName
	state.Validation = map[string]StageValidationState{
		"audit_1": {Kind: validationKindCommands, Status: validationStatusPassed},
	}
	state.AcceptanceRun = map[string]StageValidationState{}
	if err := snapshotQualityLoopAcceptance(repo, &state, acceptanceSource); err != nil {
		t.Fatal(err)
	}
	content, err := gitChangeContentSnapshot(repo)
	if err != nil {
		t.Fatal(err)
	}
	state.QualityLoop.DiffHash = qualityHashStrings(content)
	if err := os.WriteFile(filepath.Join(repo, "validation-mutated.go"), []byte("package mutation\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	engine := NewEngine(repo, NewAgentRegistry())
	unchanged, err := engine.verifyQualityAcceptanceInputDiff(&state)
	if err != nil || unchanged {
		t.Fatalf("validation source mutation = unchanged:%v err:%v", unchanged, err)
	}
	if state.Validation["audit_1"].Status != validationStatusFailed ||
		state.AcceptanceRun["audit_1"].Attempts != 0 {
		t.Fatalf("mutation gate state = validation:%#v acceptance:%#v",
			state.Validation["audit_1"], state.AcceptanceRun["audit_1"])
	}
}

// TestQualityEvidenceContentHashRejectsSymlinkEscape verifies file and root-directory links cannot leave the repository.
func TestQualityEvidenceContentHashRejectsSymlinkEscape(t *testing.T) {
	repo := t.TempDir()
	outside := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, "test-results"), 0o755); err != nil {
		t.Fatal(err)
	}
	outsideFile := filepath.Join(outside, "runtime.log")
	if err := os.WriteFile(outsideFile, []byte("external evidence\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsideFile, filepath.Join(repo, "test-results", "file-link.log")); err != nil {
		t.Fatal(err)
	}
	if got := qualityEvidenceContentHash(repo, "test-results/file-link.log"); got != "unsafe" {
		t.Fatalf("file symlink escape hash = %q, want unsafe", got)
	}

	outsideDir := filepath.Join(outside, "trace")
	if err := os.MkdirAll(outsideDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outsideDir, "trace.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsideDir, filepath.Join(repo, "test-results", "dir-link")); err != nil {
		t.Fatal(err)
	}
	if got := qualityEvidenceContentHash(repo, "test-results/dir-link"); got != "unsafe" {
		t.Fatalf("directory symlink escape hash = %q, want unsafe", got)
	}
}

// acceptanceIntegrityFixture creates one production-shaped sealed quality-loop run.
func acceptanceIntegrityFixture(t *testing.T, stage string) (string, State) {
	t.Helper()
	repo := acceptanceRunRepo(
		t,
		"1-demo",
		"pass-test",
		`mkdir -p test-results/demo && printf ok > test-results/demo/runtime.log`,
		"test-results/demo/runtime.log",
	)
	state := State{
		RunID:         newRunID(),
		ChangeName:    "1-demo",
		Sealed:        true,
		Status:        statusRunning,
		Stage:         stage,
		Workflow:      DefaultWorkflowConfig(),
		Stages:        map[string]string{},
		Validation:    map[string]StageValidationState{},
		AcceptanceRun: map[string]StageValidationState{},
		QualityLoop:   QualityLoopState{DiffHash: qualityHashStrings("tracked source snapshot")},
	}
	source := acceptancePath(repo, state.ChangeName)
	if err := snapshotQualityLoopAcceptance(repo, &state, source); err != nil {
		t.Fatal(err)
	}
	return repo, state
}

// passedAcceptanceCheckpointFixture creates the completed repair-stage state consumed by QA/archive.
func passedAcceptanceCheckpointFixture(t *testing.T) (string, State) {
	t.Helper()
	repo, state := acceptanceIntegrityFixture(t, "audit_1")
	engine := NewEngine(repo, NewAgentRegistry())
	passed, err := engine.runAcceptanceGate(context.Background(), &state)
	if err != nil || !passed {
		t.Fatalf("acceptance checkpoint setup = passed:%v err:%v", passed, err)
	}
	checkpoint := state.AcceptanceRun["audit_1"]
	checkpoint.DiffHash = state.QualityLoop.DiffHash
	state.AcceptanceRun["audit_1"] = checkpoint
	return repo, state
}
