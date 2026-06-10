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

// renderBundledPromptForRoleContract renders one bundled prompt with realistic run state.
func renderBundledPromptForRoleContract(t *testing.T, templateFile, templateName, stage string, sessions map[string]string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "prompts-template", templateFile))
	if err != nil {
		t.Fatal(err)
	}
	state := State{
		RunID:      "role-contract-run",
		ChangeName: "demo",
		Stage:      stage,
		Workflow:   DefaultWorkflowConfig(),
		Sessions:   sessions,
		Stages:     map[string]string{},
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

// TestBundledReviewPromptKeepsArtifactRules verifies reviewer prompt keeps compact JSON rules.
func TestBundledReviewPromptKeepsExamplesOnlyForFirstReviewerTurn(t *testing.T) {
	first := renderBundledPromptForRoleContract(t, "wo-review.md", "wo-review", "review_1", nil)
	requirePromptContains(t, first, "严格 JSON", "decision", "scope", "non_blocking_findings", "review-1.json")

	resumed := renderBundledPromptForRoleContract(t, "wo-review.md", "wo-review", "review_2", map[string]string{"codex:reviewer": "review-session"})
	requirePromptContains(t, resumed, "review-2.json", "review-1.json", "fix-1-summary.md", "JSON object")
	requirePromptOmits(t, resumed, "JSON schema：", "如需修复，使用：", "如需提前终止无效循环，使用：", "\"summary\": \"一句话总结审核结果\"", "\"decision\": \"needs_fix\"")
}

// TestBundledQAPromptKeepsArtifactRules verifies QA prompt keeps compact JSON rules.
func TestBundledQAPromptKeepsExamplesOnlyForFirstQATurn(t *testing.T) {
	first := renderBundledPromptForRoleContract(t, "wo-qa.md", "wo-qa", "qa_1", nil)
	requirePromptContains(t, first, "decision", "scope", "acceptance_matrix")

	resumed := renderBundledPromptForRoleContract(t, "wo-qa.md", "wo-qa", "qa_2", map[string]string{"codex:qa": "qa-session"})
	requirePromptContains(t, resumed, "qa-2.json", "schema")
	requirePromptOmits(t, resumed, "clean 示例：", "needs_fix 示例：", "\"summary\": \"核心业务路径已通过 QA\"", "\"decision\": \"needs_fix\"")
}

// TestBundledFixPromptKeepsMethodologyOnlyForFirstFixerTurn verifies fixes stop replaying startup methodology.
func TestBundledFixPromptKeepsMethodologyOnlyForFirstFixerTurn(t *testing.T) {
	first := renderBundledPromptForRoleContract(t, "wo-fix.md", "wo-fix", "fix_1", nil)
	requirePromptContains(t, first, "review-1.json", "qa-1.json", "必须做根因分析", "禁止只按错误文本打补丁", "fix-1-summary.md")

	resumed := renderBundledPromptForRoleContract(t, "wo-fix.md", "wo-fix", "fix_2", map[string]string{"codex:fixer": "fix-session"})
	requirePromptContains(t, resumed, "review-2.json", "qa-2.json", "fix-2-summary.md", "只修复当前 review/QA artifact 中列出的 findings")
	requirePromptOmits(t, resumed, "充分理解评审意见", "从根源入手，不能治标不治本", "禁止只按错误文本打补丁")
}

// TestBundledExecutionPromptDelegatesToOzExec verifies execution prompt stays as a skill entry point.
func TestBundledExecutionPromptDelegatesToOzExec(t *testing.T) {
	prompt := renderBundledPromptForRoleContract(t, "wo-start.md", "wo-start", "execution", nil)
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
	prompt := renderBundledPromptForRoleContract(t, "wo-done.md", "wo-done", "archive", map[string]string{
		"codex:reviewer": "review-session",
		"codex:qa":       "qa-session",
		"codex:fixer":    "fix-session",
	})
	requirePromptContains(t, prompt,
		"delivery-summary.md",
		"最终审核",
		"oz-archive",
	)
}
GO

(
  cd "$ROOT"
  go test ./internal/app -run 'TestBundled(Review|QA|Fix)PromptKeeps.*First.*Turn|TestBundledExecutionPromptDelegatesToOzExec|TestBundledDonePromptRequiresAuditableDeliverySummary' -count=1
) | tee "$RESULT_DIR/contract.log"
