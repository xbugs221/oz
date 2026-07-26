#!/usr/bin/env bash
# 文件功能目的：长期验证 oz flow 的同会话优化、独立 QA 放行和旧运行兼容合同。

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd)"
cd "$REPO_ROOT"

# verify_source_contract 校验长期规格、角色和阶段入口仍表达优化业务合同。
verify_source_contract() {
  rg -q 'Sources: 44-简化审核修正为自审自修循环' docs/specs/codex-workflow-cli/spec.md
  rg -q 'Session: "repairer"' internal/app/stage_role.go
  rg -q 'max_repair_iterations|MaxRepairIterations' internal/app
  rg -q 'repair_' internal/app/workflow_stage.go internal/app/stage_decision.go
  rg -q 'RepairConfirmationPending' internal/app/state_model.go internal/app/stage_decision.go
  rg -q '强制重审确认' prompts-template/oz-flow-repair.md
}

# verify_runtime_contract 运行真实状态机回归，覆盖会话复用、QA、上限与旧快照恢复。
verify_runtime_contract() {
  go test ./internal/app -run 'Test.*(Repair|Legacy).*' -count=1
}

verify_source_contract
verify_runtime_contract
