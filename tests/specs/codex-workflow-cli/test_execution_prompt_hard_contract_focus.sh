#!/usr/bin/env bash
# 文件功能目的：验证 execution prompt 默认聚焦 brief、acceptance 和 tests，而不是要求读取所有长文档。
# Sources: 8-强化验收硬合同并精简执行上下文
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
result_dir="$repo_root/test-results/8-强化验收硬合同"
mkdir -p "$result_dir"
log="$result_dir/execution-hard-contract-prompt.log"
test_file="$repo_root/internal/app/change_eight_execution_prompt_contract_test.go"
trap 'rm -f "$test_file"' EXIT

cat >"$test_file" <<'GO'
// Package app receives an injected contract test for change 8 execution prompt focus.
package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// renderChangeEightPrompt renders a bundled prompt with realistic state fields.
func renderChangeEightPrompt(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "prompts-template", "oz-flow-start.md"))
	if err != nil {
		t.Fatal(err)
	}
	state := State{
		RunID:      "change-eight-hard-contract",
		ChangeName: "8-强化验收硬合同并精简执行上下文",
		Stage:      "execution",
		Workflow:   DefaultWorkflowConfig(),
	}
	context, err := promptContext(t.TempDir(), state)
	if err != nil {
		t.Fatal(err)
	}
	got, err := renderPromptTemplate("oz-flow-start", string(data), context)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

// requireContains reports a missing prompt fragment with the full rendered prompt.
func requireContains(t *testing.T, prompt string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
}

// requireOmits reports an obsolete prompt fragment with the full rendered prompt.
func requireOmits(t *testing.T, prompt string, rejects ...string) {
	t.Helper()
	for _, reject := range rejects {
		if strings.Contains(prompt, reject) {
			t.Fatalf("prompt unexpectedly contains %q:\n%s", reject, prompt)
		}
	}
}

// TestChangeEightExecutionPromptDelegatesToOzExec verifies the execution prompt stays as an entry point.
func TestChangeEightExecutionPromptDelegatesToOzExec(t *testing.T) {
	prompt := renderChangeEightPrompt(t)
	requireContains(t, prompt,
		"oz-exec",
		"state.json.change_name",
		"acceptance.json",
		"不要超出当前提案范围",
	)
	requireOmits(t, prompt,
		"proposal.md",
		"design.md",
		"spec.md",
		"required_tests",
		"tasks.done",
	)
}

// TestChangeEightOzExecSkillMatchesPromptContract verifies the invoked skill does not reintroduce full-document defaults.
func TestChangeEightOzExecSkillMatchesPromptContract(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "skills", "oz-exec", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	skill := string(data)
	requireContains(t, skill,
		"brief.md",
		"acceptance.json",
		"tests/",
		"按需读取",
	)
	requireOmits(t, skill,
		"先读取：\n\n- `proposal.md`",
		"先读取 `proposal.md`、`design.md`、`spec.md`、`task.md`、`acceptance.json`",
	)
}

// TestChangeEightOzCreateSkillCreatesBrief verifies new proposals include the active-change brief required by validate.
func TestChangeEightOzCreateSkillCreatesBrief(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "skills", "oz-create", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	skill := string(data)
	requireContains(t, skill,
		"brief.md",
		"执行阶段默认上下文",
		"oz validate <change> --json",
	)
	requireOmits(t, skill,
		"及 proposal.md、design.md、spec.md、task.md、acceptance.json、tests/",
	)
}
GO

(
  cd "$repo_root"
  go test ./internal/app -run 'TestChangeEight(ExecutionPromptDelegatesToOzExec|OzExecSkillMatchesPromptContract|OzCreateSkillCreatesBrief)' -count=1
) | tee "$log"
