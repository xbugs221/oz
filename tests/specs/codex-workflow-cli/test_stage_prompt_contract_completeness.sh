#!/usr/bin/env bash
# 文件功能目的：验证 oz flow 内置阶段提示词首轮保留完整阶段合同，续轮只省略示例和方法论。
# Sources: 6-统一-oz-flow-阶段产物门禁重试并修复-parallel-artifact-合同, 18-修复GitHub-CI并更新仓库文档
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
RESULT_DIR="$ROOT/test-results/6-stage-artifact-gate/stage-prompt-contract"
TEST_FILE="$ROOT/internal/app/change_six_stage_prompt_contract_test.go"

mkdir -p "$RESULT_DIR"
trap 'rm -f "$TEST_FILE"' EXIT

cat > "$TEST_FILE" <<'GO'
// Package app receives an injected contract test for change 6 prompt completeness.
package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// renderChangeSixPrompt renders one bundled prompt with realistic state paths.
func renderChangeSixPrompt(t *testing.T, templateFile, templateName, stage string, sessions map[string]string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "prompts-template", templateFile))
	if err != nil {
		t.Fatal(err)
	}
	stages := map[string]string{}
	if stage == "archive" {
		stages = map[string]string{"repair_1": "completed", "qa_1": "completed"}
	}
	workflow := DefaultWorkflowConfig()
	workflow.MaxRepairIterations = 2
	workflow.MaxReviewIterations = 0
	state := State{
		RunID:      "change-six-prompt-contract",
		ChangeName: "6-统一-oz-flow-阶段产物门禁重试并修复-parallel-artifact-合同",
		Stage:      stage,
		Workflow:   workflow,
		Sessions:   sessions,
		Stages:     stages,
	}
	context, err := promptContext(t.TempDir(), state)
	if err != nil {
		t.Fatal(err)
	}
	got, err := renderPromptTemplate(templateName, string(data), context)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

// requireChangeSixPromptContains reports missing fragments with the rendered prompt.
func requireChangeSixPromptContains(t *testing.T, prompt string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
}

// requireChangeSixPromptOmits reports over-repeated fragments with the rendered prompt.
func requireChangeSixPromptOmits(t *testing.T, prompt string, rejects ...string) {
	t.Helper()
	for _, reject := range rejects {
		if strings.Contains(prompt, reject) {
			t.Fatalf("prompt unexpectedly contains %q:\n%s", reject, prompt)
		}
	}
}

// TestChangeSixDiscussPromptKeepsPlanningEntry verifies planning still enters oz-plan.
func TestChangeSixDiscussPromptKeepsPlanningEntry(t *testing.T) {
	prompt := renderChangeSixPrompt(t, "oz-flow-discuss.md", "oz-flow-discuss", "planning", nil)
	requireChangeSixPromptContains(t, prompt, "讨论规划阶段", "oz-plan")
}

// TestChangeSixExecutionPromptDelegatesToOzExec prevents start prompts from duplicating oz-exec.
func TestChangeSixExecutionPromptDelegatesToOzExec(t *testing.T) {
	prompt := renderChangeSixPrompt(t, "oz-flow-start.md", "oz-flow-start", "execution", nil)
	requireChangeSixPromptContains(t, prompt,
		"state.json.change_name",
		"oz-exec",
		"acceptance.json",
		"不要超出当前提案范围",
	)
	requireChangeSixPromptOmits(t, prompt, "proposal.md", "design.md", "spec.md", "required_tests", "tasks.done", "review-1.json", "fix-1-summary.md", "只修复当前 review/QA artifact 中列出的 findings")
}

// TestChangeSixRepairPromptKeepsSelfReviewContract verifies repair has inputs, output, schema, evidence, and session rules.
func TestChangeSixRepairPromptKeepsSelfReviewContract(t *testing.T) {
	first := renderChangeSixPrompt(t, "oz-flow-repair.md", "oz-flow-repair", "repair_1", nil)
	requireChangeSixPromptContains(t, first,
		"state.json",
		"acceptance.json",
		"完整变更",
		"repair-1.json",
		"严格 JSON",
		"decision",
		"scope",
		"non_blocking_findings",
		"needs_more",
		"repairer 不能自行归档",
	)

	resumed := renderChangeSixPrompt(t, "oz-flow-repair.md", "oz-flow-repair", "repair_2", map[string]string{"codex:repairer": "repair-session"})
	requireChangeSixPromptContains(t, resumed, "复用 backend-scoped 会话", "codex:repairer", "repair-2.json", "repair-1.json", "严格 JSON")
	requireChangeSixPromptOmits(t, resumed, "review-2.json", "fix-2-summary.md")
}

// TestChangeSixQAPromptKeepsFirstTurnAcceptanceContract verifies QA keeps required tests, evidence, and matrix rules.
func TestChangeSixQAPromptKeepsFirstTurnAcceptanceContract(t *testing.T) {
	first := renderChangeSixPrompt(t, "oz-flow-qa.md", "oz-flow-qa", "qa_1", nil)
	requireChangeSixPromptContains(t, first,
		"state.json",
		"acceptance.json",
		"repair-1.json",
		"required_tests",
		"required_evidence",
		"acceptance_matrix",
		"qa-1.json",
		"不修改源码",
		"decision",
		"scope",
	)

	resumed := renderChangeSixPrompt(t, "oz-flow-qa.md", "oz-flow-qa", "qa_2", map[string]string{"codex:qa": "qa-session"})
	requireChangeSixPromptContains(t, resumed, "复用当前角色会话", "qa-2.json", "schema")
	requireChangeSixPromptOmits(t, resumed, "clean 示例：", "needs_fix 示例：")
}

// TestChangeSixArchivePromptDelegatesToOzArchive verifies archive prompt stays as a skill entry point.
func TestChangeSixArchivePromptKeepsDeliveryContract(t *testing.T) {
	prompt := renderChangeSixPrompt(t, "oz-flow-done.md", "oz-flow-done", "archive", nil)
	requireChangeSixPromptContains(t, prompt,
		"state.json.change_name",
		"oz-archive",
		"delivery-summary.md",
		"最终审核",
		"repair-1.json",
		"qa-1.json",
	)
}
GO

(
  cd "$ROOT"
  go test ./internal/app -run 'TestChangeSix.*Prompt' -count=1
) | tee "$RESULT_DIR/contract.log"

(
  cd "$ROOT"
  go test ./internal/app -run 'TestParallelEnabledPromptsCarryFanoutArtifacts|TestBundledOzSkillPromptsDelegateToSkills' -count=1
) | tee -a "$RESULT_DIR/contract.log"
