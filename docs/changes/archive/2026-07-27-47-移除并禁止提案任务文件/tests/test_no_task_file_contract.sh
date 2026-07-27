#!/usr/bin/env bash
# 文件功能目的：验证 standard 提案不再使用 task.md，且 active 提案与执行器均禁止重新引入该文件。

set -euo pipefail

REPO_ROOT="$(git -C "$(dirname "${BASH_SOURCE[0]}")" rev-parse --show-toplevel)"
EVIDENCE_ROOT="$REPO_ROOT/test-results/47-no-task-file"
RUNTIME_LOG="$EVIDENCE_ROOT/runtime.log"
TEMP_ROOT="$(mktemp -d)"
PROJECT_ROOT="$TEMP_ROOT/project"
OZ_BIN="$TEMP_ROOT/oz"
CHANGE_NAME="1-标准提案无需任务文件"
CHANGE_ROOT="$PROJECT_ROOT/docs/changes/$CHANGE_NAME"

# cleanup 删除隔离构建与临时提案，避免合同测试污染真实仓库。
cleanup() {
  rm -rf "$TEMP_ROOT"
}
trap cleanup EXIT

# write_standard_fixture 创建不含 task.md 的完整 standard 提案业务样例。
write_standard_fixture() {
  mkdir -p "$CHANGE_ROOT/tests"
  git -C "$PROJECT_ROOT" init -q

  printf '# 简报\n\n验证 standard 提案无需任务文件。\n' >"$CHANGE_ROOT/brief.md"
  printf '# 提案\n\n移除任务文件硬合同。\n' >"$CHANGE_ROOT/proposal.md"
  printf '# 设计\n\n动态计划只保留在运行态。\n' >"$CHANGE_ROOT/design.md"
  cat >"$CHANGE_ROOT/spec.md" <<'MD'
# 规格

### 需求：无任务文件

standard 提案必须在不创建 task.md 的情况下通过严格校验。

#### 场景：standard 校验

- **测试**：`tests/contract_test.sh`
- **真实数据来源**：临时 Git 仓库中的完整 standard 提案
- **入口路径**：`oz validate`
- **关键断言**：不含 task.md 仍通过校验
- **剩余风险**：仅验证提案结构
MD
  printf '#!/usr/bin/env bash\nset -euo pipefail\nprintf "standard-without-task=passed\\n"\n' >"$CHANGE_ROOT/tests/contract_test.sh"
  chmod +x "$CHANGE_ROOT/tests/contract_test.sh"

  cat >"$CHANGE_ROOT/acceptance.json" <<'JSON'
{
  "summary": "验证 standard 提案无需任务文件",
  "coverage": [
    {
      "spec": "需求：无任务文件 / 场景：standard 校验",
      "tests": ["standard-contract"],
      "evidence": [],
      "risk": "仅验证提案结构"
    }
  ],
  "required_tests": [
    {
      "id": "standard-contract",
      "source": "change_contract",
      "path": "docs/changes/1-标准提案无需任务文件/tests/contract_test.sh",
      "command": "bash docs/changes/1-标准提案无需任务文件/tests/contract_test.sh",
      "purpose": "验证完整 standard 结构",
      "assertions": ["standard 提案无需 task.md"],
      "expected_initial_failure": "旧校验器要求 task.md"
    }
  ],
  "required_evidence": []
}
JSON
}

# build_current_oz 从当前源码构建真实 CLI，避免误用 PATH 中旧安装版。
build_current_oz() {
  (cd "$REPO_ROOT" && go build -o "$OZ_BIN" ./cmd/oz)
}

# verify_standard_without_task 核对完整 standard 提案不含 task.md 时仍通过严格校验。
verify_standard_without_task() {
  (cd "$PROJECT_ROOT" && "$OZ_BIN" validate "$CHANGE_NAME" --json) \
    >"$EVIDENCE_ROOT/without-task.json"
  jq -e '.valid == true and (.errors | length) == 0' "$EVIDENCE_ROOT/without-task.json" >/dev/null
}

# verify_active_task_rejected 核对执行器重新创建 task.md 后会被明确拒绝。
verify_active_task_rejected() {
  printf -- '- [x] 不应写入 Git 的执行步骤\n' >"$CHANGE_ROOT/task.md"
  if (cd "$PROJECT_ROOT" && "$OZ_BIN" validate "$CHANGE_NAME" --json) \
    >"$EVIDENCE_ROOT/with-task.json" 2>>"$RUNTIME_LOG"; then
    printf 'active 提案包含 task.md 时仍被接受\n' >&2
    return 1
  fi
  jq -e '.valid == false and (.errors | any(test("task.md.*禁止|禁止.*task.md")))' \
    "$EVIDENCE_ROOT/with-task.json" >/dev/null
}

# verify_skill_prohibition 核对创建、执行和归档技能均明确禁止任务文件。
verify_skill_prohibition() {
  local skill
  for skill in skills/oz-create/SKILL.md skills/oz-exec/SKILL.md skills/oz-archive/SKILL.md; do
    rg -q '禁止(创建|修改|创建或修改).*task\.md|task\.md.*禁止(创建|修改|创建或修改)' "$REPO_ROOT/$skill"
  done
}

# verify_runtime_gate_removed 核对执行门禁不再把任务复选框当作完成信号。
verify_runtime_gate_removed() {
  if rg -n 'task\.md|tasks\.total|tasks\.done|change_task' \
    "$REPO_ROOT/internal/app/stage_artifact_gate.go" >>"$RUNTIME_LOG"; then
    printf '执行 artifact gate 仍依赖 task.md\n' >&2
    return 1
  fi
}

mkdir -p "$EVIDENCE_ROOT" "$PROJECT_ROOT"
: >"$RUNTIME_LOG"
build_current_oz
write_standard_fixture
verify_standard_without_task
verify_active_task_rejected
verify_skill_prohibition
verify_runtime_gate_removed
printf 'no_task_file_contract=passed\n' >>"$RUNTIME_LOG"
