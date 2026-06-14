#!/usr/bin/env bash
# Sources: 12-收窄验收gate到提案范围
# 文件功能目的：验证 review/QA prompt 明确要求 finding scope 分类和非阻断历史债务字段。
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
result_dir="$repo_root/test-results/12-scope-gate"
log="$result_dir/prompt-scope-contract.log"

mkdir -p "$result_dir"
: >"$log"

note() {
  # note 记录合同执行步骤，便于执行阶段判断失败是否来自目标行为缺失。
  printf '%s\n' "$*" | tee -a "$log"
}

require_text() {
  # require_text 确认指定文件包含关键合同文本。
  local path="$1"
  local text="$2"
  if ! grep -qF "$text" "$path"; then
    printf 'missing %q in %s\n' "$text" "$path" | tee -a "$log" >&2
    return 1
  fi
}

cd "$repo_root"

note "检查 review prompt 包含 scope 和非阻断历史债务合同"
require_text prompts-template/oz-flow-review.md "non_blocking_findings"
require_text prompts-template/oz-flow-review.md "out_of_scope_existing"
require_text prompts-template/oz-flow-review.md "current_change"
require_text prompts-template/oz-flow-review.md "acceptance_contract"
require_text prompts-template/oz-flow-review.md "introduced_regression"

note "检查 QA prompt 包含 scope 和 acceptance_matrix 边界合同"
require_text prompts-template/oz-flow-qa.md "non_blocking_findings"
require_text prompts-template/oz-flow-qa.md "out_of_scope_existing"
require_text prompts-template/oz-flow-qa.md "current_change"
require_text prompts-template/oz-flow-qa.md "acceptance_contract"
require_text prompts-template/oz-flow-qa.md "introduced_regression"
require_text prompts-template/oz-flow-qa.md "acceptance_matrix"

note "检查规格文档记录 gate scope 合同"
require_text docs/specs/codex-workflow-cli/spec.md "out_of_scope_existing"
require_text docs/specs/codex-workflow-cli/spec.md "non_blocking_findings"

note "PASS"
