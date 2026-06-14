#!/usr/bin/env bash
# 文件功能目的：验证工作流配置 schema、profile、parallel 和 validation 解析已经拆成稳定边界。
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
log="$repo_root/test-results/refactor-workflow-config/workflow-config-boundary-contract.log"
mkdir -p "$(dirname "$log")"
: >"$log"

note() {
  # note 记录关键步骤，同时产出 workflow-config-boundary-log 证据。
  printf '%s\n' "$*" | tee -a "$log"
}

fail() {
  # fail 用配置业务语义说明失败原因。
  note "FAIL: $*"
  exit 1
}

cd "$repo_root"

note "evidence id: workflow-config-boundary-log"
note "evidence path: $log"
note "test id: workflow-config-boundary-contract"

for file in \
  internal/app/config_schema.go \
  internal/app/config_profiles.go \
  internal/app/config_parallel.go \
  internal/app/config_validation.go
do
  [[ -f "$file" ]] || fail "缺少配置解析边界文件：$file"
  note "已发现边界文件：$file"
done

for symbol in \
  'type workflowConfigInput' \
  'type stageOptionsInput' \
  'func WorkflowProfileYAML' \
  'func BuiltInWorkflowProfiles' \
  'func parallelGroupConfigFromInput' \
  'func parallelConfigFromStages' \
  'func validateParallelConfig' \
  'func validationConfigFromInput' \
  'func cloneValidationCommands'
do
  if rg -n "$symbol" internal/app/config.go | tee -a "$log" | grep -q .; then
    fail "config.go 仍直接定义已拆分配置职责：$symbol"
  fi
done

note "运行默认配置、legacy 拒绝、profile 和 parallel 业务合同"
for test_script in \
  tests/specs/codex-workflow-cli/test_tree_config_contract.sh \
  tests/specs/codex-workflow-cli/test_legacy_config_rejection_contract.sh \
  tests/specs/codex-workflow-cli/test_mada_profile_discovery_contract.sh \
  tests/specs/codex-workflow-cli/test_mada_profiles_config_contract.sh \
  tests/specs/codex-workflow-cli/test_parallel_config_contract.sh
do
  note "运行 $test_script"
  bash "$test_script" 2>&1 | tee -a "$log"
done

note "PASS: workflow-config-boundary-contract"
