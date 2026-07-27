#!/usr/bin/env bash
# 文件功能目的：验证内置角色模板首轮/续轮差异，以及 done 模板对最终交付摘要的审核价值约束。
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
RESULT_DIR="$ROOT/test-results/30-role-prompt-contract"
TEST_FILE="$ROOT/internal/app/prompt_first_turn_contract_test.go"

mkdir -p "$RESULT_DIR"
trap 'rm -f "$TEST_FILE"' EXIT

cat > "$TEST_FILE" <<'GO'
// Package app receives an injected contract test for role prompt first-turn behavior.
package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// roleContractRepairWorkflow preserves the finite repair-v1 role contract under test.
func roleContractRepairWorkflow() WorkflowConfig {
	base := DefaultWorkflowConfig()
	repair := base.Stages["audit_1"]
	qa := base.Stages["qa_1"]
	return WorkflowConfig{
		Engine:              base.Engine,
		Generation:          repairWorkflowGeneration,
		MaxRepairIterations: 2,
		Stages: map[string]StageOptions{
			"planning":  base.Stages["planning"],
			"execution": base.Stages["execution"],
			"repair_1":  repair,
			"qa_1":      qa,
			"repair_2":  repair,
			"qa_2":      qa,
			"archive":   base.Stages["archive"],
		},
		Validation: base.Validation,
		Prompts:    base.Prompts,
	}
}

// sealRoleContractAcceptance writes a valid run-local acceptance snapshot for role prompts.
func sealRoleContractAcceptance(t *testing.T, repo string, state *State) {
	t.Helper()
	t.Setenv("XDG_STATE_HOME", filepath.Join(repo, "state"))
	source := filepath.Join(repo, "acceptance.json")
	body := `{
  "summary": "role prompt contract",
  "coverage": [{
    "spec": "需求：角色提示词合同 / 场景：渲染封存运行提示词",
    "tests": ["prompt-contract"],
    "evidence": [],
    "risk": "只验证提示词结构"
  }],
  "required_tests": [{
    "id": "prompt-contract",
    "source": "change_contract",
    "path": "tests/prompt-contract.sh",
    "command": "bash tests/prompt-contract.sh",
    "purpose": "验证角色提示词保持首轮和续轮合同",
    "assertions": ["角色提示词使用封存验收并保持会话与产物边界"]
  }],
  "required_evidence": []
}`
	if err := os.WriteFile(source, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := snapshotQualityLoopAcceptance(repo, state, source); err != nil {
		t.Fatal(err)
	}
}

// renderBundledPromptForRoleContract renders one bundled prompt with realistic run state.
func renderBundledPromptForRoleContract(t *testing.T, templateFile, templateName, stage string, sessions map[string]string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "prompts-template", templateFile))
	if err != nil {
		t.Fatal(err)
	}
	stages := map[string]string{}
	if stage == "archive" {
		stages = map[string]string{"repair_1": "completed", "qa_1": "completed"}
	}
	state := State{
		RunID:      "role-contract-run",
		ChangeName: "demo",
		Sealed:     true,
		Stage:      stage,
		Workflow:   roleContractRepairWorkflow(),
		Sessions:   sessions,
		Stages:     stages,
	}
	repo := t.TempDir()
	sealRoleContractAcceptance(t, repo, &state)
	context, err := promptContext(repo, state)
	if err != nil {
		t.Fatal(err)
	}
	got, err := renderPromptTemplate(templateName, string(data), context)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

// requirePromptContains explains missing prompt fragments with the rendered prompt body.
func requirePromptContains(t *testing.T, prompt string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
}

// requirePromptOmits explains repeated prompt fragments with the rendered prompt body.
func requirePromptOmits(t *testing.T, prompt string, rejects ...string) {
	t.Helper()
	for _, reject := range rejects {
		if strings.Contains(prompt, reject) {
			t.Fatalf("prompt repeated %q:\n%s", reject, prompt)
		}
	}
}

// TestBundledRepairPromptKeepsOneSessionContract verifies every repair round keeps the durable schema and one session.
func TestBundledRepairPromptKeepsOneSessionContract(t *testing.T) {
	first := renderBundledPromptForRoleContract(t, "oz-flow-repair.md", "oz-flow-repair", "repair_1", nil)
	requirePromptContains(t, first, "严格 JSON", "decision", "scope", "non_blocking_findings", "repair-1.json", "needs_more", "repairer 不能自行归档")

	resumed := renderBundledPromptForRoleContract(t, "oz-flow-repair.md", "oz-flow-repair", "repair_2", map[string]string{"codex:repairer": "repair-session"})
	requirePromptContains(t, resumed, "repair-2.json", "repair-1.json", "codex:repairer", "复用 backend-scoped 会话")
	requirePromptOmits(t, resumed, "review-2.json", "fix-2-summary.md")
}

// TestBundledQAPromptKeepsArtifactRules verifies QA prompt keeps compact JSON rules.
func TestBundledQAPromptKeepsExamplesOnlyForFirstQATurn(t *testing.T) {
	first := renderBundledPromptForRoleContract(t, "oz-flow-qa.md", "oz-flow-qa", "qa_1", nil)
	requirePromptContains(t, first, "decision", "scope", "acceptance_matrix", "repair-1.json", "不修改源码")

	resumed := renderBundledPromptForRoleContract(t, "oz-flow-qa.md", "oz-flow-qa", "qa_2", map[string]string{"codex:qa": "qa-session"})
	requirePromptContains(t, resumed, "qa-2.json", "schema")
	requirePromptOmits(t, resumed, "clean 示例：", "needs_fix 示例：", "\"summary\": \"核心业务路径已通过 QA\"", "\"decision\": \"needs_fix\"")
}

// TestBundledExecutionPromptDelegatesToOzExec verifies execution prompt stays as a skill entry point.
func TestBundledExecutionPromptDelegatesToOzExec(t *testing.T) {
	prompt := renderBundledPromptForRoleContract(t, "oz-flow-start.md", "oz-flow-start", "execution", nil)
	requirePromptContains(t, prompt,
		"oz-exec",
		"state.json.change_name",
		"acceptance.json",
		"不要超出当前提案范围",
	)
	requirePromptOmits(t, prompt, "proposal.md", "design.md", "spec.md", "required_tests", "tasks.done", "review-1.json", "fix-1-summary.md", "只修复当前 review/QA artifact 中列出的 findings")
}

// TestBundledDonePromptRequiresAuditableDeliverySummary verifies the final summary is useful to human reviewers.
func TestBundledDonePromptRequiresAuditableDeliverySummary(t *testing.T) {
	prompt := renderBundledPromptForRoleContract(t, "oz-flow-done.md", "oz-flow-done", "archive", map[string]string{
		"codex:repairer": "repair-session",
		"codex:qa":       "qa-session",
	})
	requirePromptContains(t, prompt,
		"delivery-summary.md",
		"最终审核",
		"oz-archive",
		"repair-1.json",
		"qa-1.json",
	)
}
GO

(
  cd "$ROOT"
  go test ./internal/app -run 'TestBundled(RepairPromptKeepsOneSessionContract|QAPromptKeepsExamplesOnlyForFirstQATurn|ExecutionPromptDelegatesToOzExec|DonePromptRequiresAuditableDeliverySummary)' -count=1
) | tee "$RESULT_DIR/contract.log"
