#!/usr/bin/env bash
# 文件功能目的：验证 wo 将 artifact gate 与 validation gate 状态拆分，并在 execution 后阻断不可执行的验收合同。
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
RESULT_DIR="$ROOT/test-results/19-gate-state-preflight"
TEST_FILE="$ROOT/internal/app/gate_state_preflight_contract_test.go"
LOG="$RESULT_DIR/contract.log"

mkdir -p "$RESULT_DIR"
: >"$LOG"

cleanup() {
  rm -f "$TEST_FILE"
}
trap cleanup EXIT

note() {
  # note 同时写入 stdout 和 runtime log，便于 QA 复核失败点。
  printf '%s\n' "$*" | tee -a "$LOG"
}

cd "$ROOT"

note "写入 internal/app 包级合同测试，锁定 artifact_gates 与 acceptance_preflight 新语义"
cat >"$TEST_FILE" <<'GO'
// Package app validates wo gate-state separation and acceptance preflight blocking.
package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestArtifactGateFailureUsesArtifactGateState 证明阶段产物门禁不再污染 command validation 状态。
func TestArtifactGateFailureUsesArtifactGateState(t *testing.T) {
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

	gate, ok := state.ArtifactGates["execution"]
	if !ok {
		t.Fatalf("artifact gate failure must be recorded in state.ArtifactGates, got %#v", state.ArtifactGates)
	}
	if gate.Kind != validationKindArtifact || gate.Status != validationStatusFailed || gate.LastError == "" {
		t.Fatalf("artifact gate state = %#v, want failed artifact gate with last_error", gate)
	}
	if _, contaminated := state.Validation["execution"]; contaminated {
		t.Fatalf("artifact gate failure must not write state.Validation[execution], got %#v", state.Validation["execution"])
	}

	data, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	if !strings.Contains(body, `"artifact_gates"`) {
		t.Fatalf("state JSON must expose artifact_gates, got %s", body)
	}
	if strings.Contains(body, `"validation":{"execution"`) {
		t.Fatalf("state JSON must not expose artifact failure as validation.execution: %s", body)
	}
}

// TestAcceptancePreflightBlocksEvidenceWithoutProducer 证明验收证据没有生产者时阻断用户检查合同。
func TestAcceptancePreflightBlocksEvidenceWithoutProducer(t *testing.T) {
	repo := t.TempDir()
	t.Setenv("XDG_STATE_HOME", filepath.Join(repo, "state"))
	state := preflightState(t, repo, "run-preflight-fail", acceptanceWithoutEvidenceProducer())
	engine := NewEngine(repo, nil)

	passed, err := engine.runAcceptancePreflight(&state)
	if err != nil {
		t.Fatalf("runAcceptancePreflight returned unexpected error: %v", err)
	}
	if passed {
		t.Fatal("preflight should fail when required_evidence has no producer")
	}
	if state.Status != statusAcceptanceContractBlocked || state.Stage != statusAcceptanceContractBlocked {
		t.Fatalf("state = %s/%s, want blocked_acceptance_contract", state.Status, state.Stage)
	}
	if state.AcceptancePreflight.Status != validationStatusFailed {
		t.Fatalf("acceptance preflight state = %#v, want failed", state.AcceptancePreflight)
	}
	if !strings.Contains(state.AcceptancePreflight.LastError, "console-without-producer") {
		t.Fatalf("preflight error should name missing evidence producer, got %q", state.AcceptancePreflight.LastError)
	}
	if _, contaminated := state.Validation["execution"]; contaminated {
		t.Fatalf("acceptance preflight failure must not write validation.execution, got %#v", state.Validation["execution"])
	}
}

// TestAcceptancePreflightPassesWhenEvidenceHasRequiredTestProducer 证明 evidence 可追溯生产者时允许进入后续阶段。
func TestAcceptancePreflightPassesWhenEvidenceHasRequiredTestProducer(t *testing.T) {
	repo := t.TempDir()
	t.Setenv("XDG_STATE_HOME", filepath.Join(repo, "state"))
	state := preflightState(t, repo, "run-preflight-pass", acceptanceWithEvidenceProducer())
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

// preflightState writes a sealed-run acceptance snapshot so preflight reads the same source as real wo runs.
func preflightState(t *testing.T, repo, runID, acceptanceBody string) State {
	t.Helper()
	change := "19-gate-state-preflight"
	runPath := runDir(repo, runID)
	if err := os.MkdirAll(runPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runPath, "acceptance.json"), []byte(acceptanceBody), 0o644); err != nil {
		t.Fatal(err)
	}
	return State{
		RunID:      runID,
		ChangeName: change,
		Status:     statusRunning,
		Stage:      "execution",
		Workflow:   DefaultWorkflowConfig(),
		Validation: map[string]StageValidationState{},
	}
}

// acceptanceWithoutEvidenceProducer 表示证据只被 coverage 要求，但没有测试命令或断言说明如何生成。
func acceptanceWithoutEvidenceProducer() string {
	return `{
  "summary": "preflight should block evidence without producer",
  "coverage": [
    {
      "spec": "需求：execution 后执行 acceptance preflight / 场景：evidence 无 producer 时阻断为验收合同问题",
      "tests": ["contract-only"],
      "evidence": ["console-without-producer"],
      "risk": "preflight should reject this contract before review"
    }
  ],
  "required_tests": [
    {
      "id": "contract-only",
      "source": "change_contract",
      "path": "docs/changes/archive/2026-06-12-19-拆分wo门禁状态并新增验收预检阻断/tests/test_gate_state_and_acceptance_preflight_contract.sh",
      "command": "bash docs/changes/archive/2026-06-12-19-拆分wo门禁状态并新增验收预检阻断/tests/test_gate_state_and_acceptance_preflight_contract.sh",
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

// acceptanceWithEvidenceProducer 表示测试命令和断言都明确生产 runtime log evidence。
func acceptanceWithEvidenceProducer() string {
	return `{
  "summary": "preflight should pass evidence with producer",
  "coverage": [
    {
      "spec": "需求：execution 后执行 acceptance preflight / 场景：evidence 无 producer 时阻断为验收合同问题",
      "tests": ["contract-produces-log"],
      "evidence": ["gate-state-preflight-log"],
      "risk": "preflight only checks producer traceability, not full command execution"
    }
  ],
  "required_tests": [
    {
      "id": "contract-produces-log",
      "source": "change_contract",
      "path": "docs/changes/archive/2026-06-12-19-拆分wo门禁状态并新增验收预检阻断/tests/test_gate_state_and_acceptance_preflight_contract.sh",
      "command": "bash docs/changes/archive/2026-06-12-19-拆分wo门禁状态并新增验收预检阻断/tests/test_gate_state_and_acceptance_preflight_contract.sh",
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
GO

note "运行 Go 合同测试；当前实现预期失败于新增状态字段或 preflight 方法缺失"
go test ./internal/app -run 'TestArtifactGateFailureUsesArtifactGateState|TestAcceptancePreflight' -count=1 2>&1 | tee -a "$LOG"

note "PASS"
