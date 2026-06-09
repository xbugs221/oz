#!/usr/bin/env bash
# 文件功能目的：验证 review/fix 首轮保留完整要求，续轮只提供增量上下文和必要输出边界。
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
RESULT_DIR="$ROOT/test-results/5-clean-wo-legacy/review-fix-resumed-prompt-compact"
TEST_FILE="$ROOT/internal/app/review_fix_resumed_prompt_compact_contract_test.go"

rm -rf "$RESULT_DIR"
mkdir -p "$RESULT_DIR"
trap 'rm -f "$TEST_FILE"' EXIT

cat > "$TEST_FILE" <<'GO'
// Package app receives an injected contract test for compact resumed role prompts.
package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// renderRolePromptForCompactContract renders bundled prompts with realistic run state and sessions.
func renderRolePromptForCompactContract(t *testing.T, templateFile, templateName, stage string, sessions map[string]string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "prompts-template", templateFile))
	if err != nil {
		t.Fatal(err)
	}
	state := State{
		RunID:      "compact-contract-run",
		ChangeName: "demo",
		Stage:      stage,
		Workflow:   DefaultWorkflowConfig(),
		Sessions:   sessions,
		Stages:     map[string]string{},
	}
	prompt, err := renderPromptTemplate(templateName, string(data), promptContext(t.TempDir(), state))
	if err != nil {
		t.Fatal(err)
	}
	return prompt
}

// requireContains reports prompt fragments that should remain in the target turn.
func requireContains(t *testing.T, prompt string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
}

// requireOmits reports prompt fragments that would repeat startup-only instructions.
func requireOmits(t *testing.T, prompt string, rejects ...string) {
	t.Helper()
	for _, reject := range rejects {
		if strings.Contains(prompt, reject) {
			t.Fatalf("resumed prompt repeated %q:\n%s", reject, prompt)
		}
	}
}

// TestReviewPromptResumedTurnIsIncremental verifies reviewer startup instructions are not replayed.
func TestReviewPromptResumedTurnIsIncremental(t *testing.T) {
	first := renderRolePromptForCompactContract(t, "wo-review.md", "wo-review", "review_1", nil)
	requireContains(t, first,
		"严格 JSON 要求：",
		"作为一个评审专家",
		"JSON schema：",
		"如需修复，使用：",
		"如需提前终止无效循环，使用：",
	)

	resumed := renderRolePromptForCompactContract(t, "wo-review.md", "wo-review", "review_2", map[string]string{"codex:reviewer": "review-session"})
	requireContains(t, resumed,
		"复用当前角色会话：`codex:reviewer`",
		"review-2.json",
		"review-1.json",
		"fix-1-summary.md",
		"只输出一个 JSON 对象",
	)
	requireOmits(t, resumed,
		"严格 JSON 要求：",
		"作为一个评审专家",
		"JSON schema：",
		"如需修复，使用：",
		"如需提前终止无效循环，使用：",
		"clean 的 evidence 必须引用验证命令 artifact",
		"第 2 轮及之后，如果连续两轮没有实质变化",
		"severity 仅允许",
		"编写前可先本地复测",
	)
	if len(resumed) >= len(first)/2 {
		t.Fatalf("review resumed prompt should be less than half of first turn: first=%d resumed=%d", len(first), len(resumed))
	}
}

// TestFixPromptResumedTurnIsIncremental verifies fixer startup instructions are not replayed.
func TestFixPromptResumedTurnIsIncremental(t *testing.T) {
	first := renderRolePromptForCompactContract(t, "wo-fix.md", "wo-fix", "fix_1", nil)
	requireContains(t, first,
		"充分理解评审意见",
		"根据评审意见列出的问题根因逐项验证并修复",
		"必须做根因分析",
		"禁止只按错误文本打补丁",
		"fix-1-summary.md",
	)

	resumed := renderRolePromptForCompactContract(t, "wo-fix.md", "wo-fix", "fix_2", map[string]string{"codex:fixer": "fix-session"})
	requireContains(t, resumed,
		"复用当前角色会话：`codex:fixer`",
		"review-2.json",
		"qa-2.json",
		"fix-2-summary.md",
		"只修复当前 review/QA artifact 中列出的 findings",
	)
	requireOmits(t, resumed,
		"充分理解评审意见",
		"根据评审意见列出的问题根因逐项验证并修复",
		"从根源入手，不能治标不治本",
		"必须做根因分析",
		"禁止只按错误文本打补丁",
		"不得删除、弱化或绕过",
		"普通修复轮次不需要读取所有旧 review/fix artifact",
	)
	if len(resumed) >= len(first)*2/3 {
		t.Fatalf("fix resumed prompt should be clearly shorter than first turn: first=%d resumed=%d", len(first), len(resumed))
	}
}
GO

(
  cd "$ROOT"
  go test ./internal/app -run 'TestReviewPromptResumedTurnIsIncremental|TestFixPromptResumedTurnIsIncremental' -count=1
) | tee "$RESULT_DIR/contract.log"
