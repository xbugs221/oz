// Package app tests oz flow gate-state separation and acceptance preflight repair.
package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRootArtifactGateFailureUsesArtifactGateState proves artifact gates do not pollute command validation state.
func TestRootArtifactGateFailureUsesArtifactGateState(t *testing.T) {
	repo := t.TempDir()
	t.Setenv("XDG_STATE_HOME", filepath.Join(repo, "state"))
	state := State{
		RunID:      "run-artifact-gate",
		ChangeName: "19-gate-state-preflight",
		Status:     statusRunning,
		Stage:      "execution",
		Workflow:   DefaultWorkflowConfig(),
	}

	if err := recordStageArtifactGateFailure(repo, &state, fmt.Errorf("execution 阶段 artifact 未完成")); err != nil {
		t.Fatalf("recordStageArtifactGateFailure returned error: %v", err)
	}
	if gate := state.ArtifactGates["execution"]; gate.Kind != validationKindArtifact || gate.Status != validationStatusFailed || gate.LastError == "" {
		t.Fatalf("artifact gate state = %#v, want failed artifact gate with last_error", gate)
	}
	if _, contaminated := state.Validation["execution"]; contaminated {
		t.Fatalf("artifact gate failure must not write state.Validation[execution], got %#v", state.Validation["execution"])
	}
}

// TestRootExecutionRunsOzValidateAsRetryableGate proves oz validate failures return to executor repair.
func TestRootExecutionRunsOzValidateAsRetryableGate(t *testing.T) {
	repo := t.TempDir()
	t.Setenv("XDG_STATE_HOME", filepath.Join(repo, "state"))
	installGateStateFakeOz(t, false)
	state := State{
		RunID:      "run-change-validate",
		ChangeName: "19-gate-state-preflight",
		Status:     statusRunning,
		Stage:      "execution",
		Workflow:   DefaultWorkflowConfig(),
		Validation: map[string]StageValidationState{},
		Stages:     map[string]string{},
	}
	engine := NewEngine(repo, nil)

	passed, err := engine.validateStage(context.Background(), &state)
	if err != nil {
		t.Fatalf("validateStage returned unexpected error: %v", err)
	}
	if passed {
		t.Fatal("oz validate failure should fail the stage validation gate")
	}
	if state.Status != statusRunning || state.Stage != "execution" {
		t.Fatalf("state = %s/%s, want retryable running execution", state.Status, state.Stage)
	}
	current := state.Validation["execution"]
	if current.Kind != validationKindChange || current.Status != validationStatusFailed {
		t.Fatalf("validation state = %#v, want failed change validation", current)
	}
	if state.Stages["execution"] != "validation_failed" {
		t.Fatalf("stage marker = %q, want validation_failed", state.Stages["execution"])
	}
	prompt := validationFailurePrompt(repo, state)
	if !strings.Contains(prompt, "oz validate 19-gate-state-preflight --json") || !strings.Contains(prompt, "producer missing") {
		t.Fatalf("retry prompt should include oz validate diagnostics, got: %s", prompt)
	}
}

// TestRootAcceptancePreflightReturnsEvidenceWithoutProducerToExecutor proves unproducible evidence is repairable.
func TestRootAcceptancePreflightReturnsEvidenceWithoutProducerToExecutor(t *testing.T) {
	repo := t.TempDir()
	state := rootPreflightState(t, repo, "run-preflight-fail", rootAcceptanceWithoutEvidenceProducer())
	engine := NewEngine(repo, nil)

	passed, err := engine.runAcceptancePreflight(&state)
	if err != nil {
		t.Fatalf("runAcceptancePreflight returned unexpected error: %v", err)
	}
	if passed {
		t.Fatal("preflight should fail when required_evidence has no producer")
	}
	if state.Status != statusRunning || state.Stage != "execution" {
		t.Fatalf("state = %s/%s, want retryable running execution", state.Status, state.Stage)
	}
	if !strings.Contains(state.AcceptancePreflight.LastError, "console-without-producer") {
		t.Fatalf("preflight error should name missing evidence producer, got %q", state.AcceptancePreflight.LastError)
	}
	if state.AcceptancePreflight.LastArtifact == "" || state.Stages["execution"] != "validation_failed" {
		t.Fatalf("preflight should persist retry artifact and marker: %#v stages=%#v", state.AcceptancePreflight, state.Stages)
	}
	if !shouldForceStageRerun(state) {
		t.Fatal("preflight failure should force the same execution stage to rerun")
	}
	prompt := validationFailurePrompt(repo, state)
	if !strings.HasPrefix(prompt, "# Acceptance preflight gate failed") || !strings.Contains(prompt, "console-without-producer") {
		t.Fatalf("preflight retry prompt missing producer diagnostics: %s", prompt)
	}
	if _, contaminated := state.Validation["execution"]; contaminated {
		t.Fatalf("acceptance preflight failure must not write validation.execution, got %#v", state.Validation["execution"])
	}
}

func installGateStateFakeOz(t *testing.T, valid bool) {
	t.Helper()
	previous := ozCommand
	previousPrefix := ozCommandPrefix
	ozCommand = os.Args[0]
	ozCommandPrefix = []string{"-test.run=TestGateStateFakeOzCommand", "--"}
	if valid {
		t.Setenv("OZ_GATE_STATE_FAKE_VALIDATE", "valid")
	} else {
		t.Setenv("OZ_GATE_STATE_FAKE_VALIDATE", "invalid")
	}
	t.Cleanup(func() {
		ozCommand = previous
		ozCommandPrefix = previousPrefix
	})
}

// TestGateStateFakeOzCommand serves oz validate JSON to validation gate subprocesses.
func TestGateStateFakeOzCommand(t *testing.T) {
	mode := os.Getenv("OZ_GATE_STATE_FAKE_VALIDATE")
	if mode == "" {
		return
	}
	args := os.Args
	for _, arg := range args {
		if arg == "--" {
			if mode == "valid" {
				_, _ = os.Stdout.WriteString(`{"valid":true,"errors":[]}` + "\n")
				os.Exit(0)
			}
			_, _ = os.Stdout.WriteString(`{"valid":false,"errors":["producer missing"]}` + "\n")
			os.Exit(1)
		}
	}
	os.Exit(1)
}

// TestRootAcceptancePreflightPassesWhenEvidenceHasRequiredTestProducer proves traceable evidence may advance.
func TestRootAcceptancePreflightPassesWhenEvidenceHasRequiredTestProducer(t *testing.T) {
	repo := t.TempDir()
	state := rootPreflightState(t, repo, "run-preflight-pass", rootAcceptanceWithEvidenceProducer())
	engine := NewEngine(repo, nil)

	passed, err := engine.runAcceptancePreflight(&state)
	if err != nil {
		t.Fatalf("runAcceptancePreflight returned unexpected error: %v", err)
	}
	if !passed {
		t.Fatalf("preflight should pass when required_evidence is produced by a required_test: %#v", state.AcceptancePreflight)
	}
	if state.Status != statusRunning || state.Stage != "execution" {
		t.Fatalf("state = %s/%s, want still running execution before normal advance", state.Status, state.Stage)
	}
	if state.AcceptancePreflight.Status != validationStatusPassed {
		t.Fatalf("acceptance preflight state = %#v, want passed", state.AcceptancePreflight)
	}
}

// TestRootAcceptancePreflightPassesWhenSiblingWrapperProducesEvidence proves real test wrappers can produce logs.
func TestRootAcceptancePreflightPassesWhenSiblingWrapperProducesEvidence(t *testing.T) {
	repo := t.TempDir()
	writeRootAcceptanceScriptProducer(t, repo)
	state := rootPreflightState(t, repo, "run-preflight-script-pass", rootAcceptanceWithSiblingScriptProducer())
	engine := NewEngine(repo, nil)

	passed, err := engine.runAcceptancePreflight(&state)
	if err != nil {
		t.Fatalf("runAcceptancePreflight returned unexpected error: %v", err)
	}
	if !passed {
		t.Fatalf("preflight should pass when a sibling test wrapper produces runtime evidence: %#v", state.AcceptancePreflight)
	}
}

// rootPreflightState writes a sealed-run acceptance snapshot so preflight uses real run lookup rules.
func rootPreflightState(t *testing.T, repo, runID, acceptanceBody string) State {
	t.Helper()
	t.Setenv("XDG_STATE_HOME", filepath.Join(repo, "state"))
	runPath := runDir(repo, runID)
	if err := os.MkdirAll(runPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runPath, "acceptance.json"), []byte(acceptanceBody), 0o644); err != nil {
		t.Fatal(err)
	}
	return State{
		RunID:      runID,
		ChangeName: "19-gate-state-preflight",
		Status:     statusRunning,
		Stage:      "execution",
		Workflow:   DefaultWorkflowConfig(),
		Validation: map[string]StageValidationState{},
	}
}

// writeRootAcceptanceScriptProducer creates the shell wrapper pattern used by UI contract tests.
func writeRootAcceptanceScriptProducer(t *testing.T, repo string) {
	t.Helper()
	dir := filepath.Join(repo, "docs/changes/212-professional-workspace/tests")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := `#!/usr/bin/env bash
set -euo pipefail
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../.." && pwd)"
RESULT_DIR="$ROOT_DIR/test-results/212-professional-workspace"
mkdir -p "$RESULT_DIR"
pnpm exec vitest run --config tests/config/vitest.config.ts \
  docs/changes/212-professional-workspace/tests/professional-workspace-nav.acceptance.test.ts \
  | tee "$RESULT_DIR/contract.log"
`
	if err := os.WriteFile(filepath.Join(dir, "test_professional_workspace_nav_contract.sh"), []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
}

// rootAcceptanceWithoutEvidenceProducer requires evidence that no required test can produce.
func rootAcceptanceWithoutEvidenceProducer() string {
	return `{
  "summary": "preflight should block evidence without producer",
  "coverage": [
    {
      "spec": "需求：execution 后执行 acceptance preflight / 场景：evidence 无 producer 时回到 executor 修复",
      "tests": ["contract-only"],
      "evidence": ["console-without-producer"],
      "risk": "preflight should reject this contract before review"
    }
  ],
  "required_tests": [
    {
      "id": "contract-only",
      "source": "change_contract",
      "path": "docs/changes/19-拆分wo门禁状态并新增验收预检阻断/tests/test_gate_state_and_acceptance_preflight_contract.sh",
      "command": "bash docs/changes/19-拆分wo门禁状态并新增验收预检阻断/tests/test_gate_state_and_acceptance_preflight_contract.sh",
      "purpose": "prove gate state behavior only",
      "assertions": ["artifact gate failure writes artifact_gates instead of command validation"]
    }
  ],
  "required_evidence": [
    {
      "id": "console-without-producer",
      "kind": "console",
      "path": "test-results/19-gate-state-preflight/console.log",
      "purpose": "console evidence is required but no test says how to produce it"
    }
  ]
}`
}

// rootAcceptanceWithSiblingScriptProducer requires evidence produced by a shell wrapper near the test file.
func rootAcceptanceWithSiblingScriptProducer() string {
	return `{
  "summary": "preflight should pass evidence produced by a sibling shell wrapper",
  "coverage": [
    {
      "spec": "需求：专业导航提供工作区 tab / 场景：当前专业会话可切到工作区并看到文件树",
      "tests": ["professional-workspace-nav-contract"],
      "evidence": ["professional-workspace-nav-log"],
      "risk": "shell wrapper writes the local runtime log"
    }
  ],
  "required_tests": [
    {
      "id": "professional-workspace-nav-contract",
      "source": "change_contract",
      "path": "docs/changes/212-professional-workspace/tests/professional-workspace-nav.acceptance.test.ts",
      "command": "pnpm exec vitest run --config tests/config/vitest.config.ts docs/changes/212-professional-workspace/tests/professional-workspace-nav.acceptance.test.ts",
      "purpose": "prove professional workspace nav renders",
      "assertions": ["workspace tab renders current pro workspace files"]
    }
  ],
  "required_evidence": [
    {
      "id": "professional-workspace-nav-log",
      "kind": "runtime_log",
      "path": "test-results/212-professional-workspace/contract.log",
      "purpose": "runtime log generated by the local contract wrapper"
    }
  ]
}`
}

// rootAcceptanceWithEvidenceProducer requires evidence that a required test names by id and path.
func rootAcceptanceWithEvidenceProducer() string {
	return `{
  "summary": "preflight should pass evidence with producer",
  "coverage": [
    {
      "spec": "需求：execution 后执行 acceptance preflight / 场景：evidence 无 producer 时回到 executor 修复",
      "tests": ["contract-produces-log"],
      "evidence": ["gate-state-preflight-log"],
      "risk": "preflight only checks producer traceability, not full command execution"
    }
  ],
  "required_tests": [
    {
      "id": "contract-produces-log",
      "source": "change_contract",
      "path": "docs/changes/19-拆分wo门禁状态并新增验收预检阻断/tests/test_gate_state_and_acceptance_preflight_contract.sh",
      "command": "bash docs/changes/19-拆分wo门禁状态并新增验收预检阻断/tests/test_gate_state_and_acceptance_preflight_contract.sh",
      "purpose": "generate gate-state-preflight-log at test-results/19-gate-state-preflight/contract.log",
      "assertions": [
        "command writes runtime evidence gate-state-preflight-log to test-results/19-gate-state-preflight/contract.log"
      ]
    }
  ],
  "required_evidence": [
    {
      "id": "gate-state-preflight-log",
      "kind": "runtime_log",
      "path": "test-results/19-gate-state-preflight/contract.log",
      "purpose": "runtime log generated by contract-produces-log"
    }
  ]
}`
}
