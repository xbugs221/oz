#!/usr/bin/env bash
# Sources: 12-收窄验收gate到提案范围
# 文件功能目的：验证已创建但未运行的旧提案不需要新增 scope 或 non_blocking 字段，也能继续通过 oz validate。
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
result_dir="$repo_root/test-results/12-scope-gate"
log="$result_dir/legacy-active-change-compatibility.log"
oz_bin="$result_dir/oz"
project="$result_dir/legacy-project"
change="1-旧提案兼容"

mkdir -p "$result_dir"
: >"$log"

note() {
  # note 记录合同执行步骤，便于执行阶段判断失败是否来自目标行为缺失。
  printf '%s\n' "$*" | tee -a "$log"
}

cd "$repo_root"

note "构建真实 oz 二进制"
go build -o "$oz_bin" ./cmd/oz 2>&1 | tee -a "$log"

note "创建旧格式 active change，不包含 scope 或 non_blocking 字段"
rm -rf "$project"
mkdir -p "$project/docs/changes/$change/tests"
git -C "$project" init -q

cat >"$project/docs/changes/$change/brief.md" <<'MD'
# 简报

旧提案兼容测试。该提案使用既有 acceptance.json 格式，不包含 scope 或 non_blocking 字段。

验收入口：

- `bash docs/changes/1-旧提案兼容/tests/test_legacy_contract.sh`
MD

cat >"$project/docs/changes/$change/proposal.md" <<'MD'
# 旧提案兼容

验证未运行旧提案无需迁移即可通过 oz validate。
MD

cat >"$project/docs/changes/$change/design.md" <<'MD'
# 设计

保持旧格式 acceptance 合同，不引入任何 review 或 QA 运行期字段。
MD

cat >"$project/docs/changes/$change/spec.md" <<'MD'
# 规格

## 验收矩阵

| 需求 | 场景 | 测试 | 证据 | 风险 |
| --- | --- | --- | --- | --- |
| 旧提案兼容 | 旧格式 acceptance 通过 validate | `legacy-contract` | `legacy-contract-log` | 不启动 sealed run |

### 需求：旧提案兼容

#### 场景：旧格式 acceptance 通过 validate

- **给定** 旧格式 active change
- **当** 用户运行 `oz validate`
- **则** `oz validate` 应当校验成功
MD

cat >"$project/docs/changes/$change/task.md" <<'MD'
# 任务

- [ ] 1.1 运行旧格式合同测试。
MD

cat >"$project/docs/changes/$change/tests/test_legacy_contract.sh" <<'SH'
#!/usr/bin/env bash
# 文件功能目的：作为旧提案兼容测试中的真实合同入口。
set -euo pipefail

test -s docs/changes/1-旧提案兼容/acceptance.json
grep -q "旧格式 active change" docs/changes/1-旧提案兼容/brief.md
SH
chmod +x "$project/docs/changes/$change/tests/test_legacy_contract.sh"

cat >"$project/docs/changes/$change/acceptance.json" <<'JSON'
{
  "summary": "旧格式 active change 不包含 scope 或 non_blocking 字段也应继续通过 validate。",
  "coverage": [
    {
      "spec": "需求：旧提案兼容 / 场景：旧格式 acceptance 通过 validate",
      "tests": ["legacy-contract"],
      "evidence": ["legacy-contract-log"],
      "risk": "只验证 validate 阶段，不启动 sealed run。"
    }
  ],
  "required_tests": [
    {
      "id": "legacy-contract",
      "source": "change_contract",
      "path": "docs/changes/1-旧提案兼容/tests/test_legacy_contract.sh",
      "command": "bash docs/changes/1-旧提案兼容/tests/test_legacy_contract.sh",
      "purpose": "证明旧格式 active change 的测试和 acceptance 合同仍可被 validate 接受。",
      "assertions": [
        "acceptance.json 存在且无需 scope 字段",
        "brief.md 说明这是旧格式 active change"
      ],
      "expected_initial_failure": "该兼容性测试当前应通过；执行阶段必须保持通过。"
    }
  ],
  "required_evidence": [
    {
      "id": "legacy-contract-log",
      "kind": "runtime_log",
      "path": "test-results/legacy-active-change/legacy-contract.log",
      "purpose": "记录旧格式 active change validate 输出。"
    }
  ]
}
JSON

note "运行真实 oz validate，确认未运行旧提案不需要 scope 字段"
(
  cd "$project"
  "$oz_bin" validate "$change" --json
) 2>&1 | tee -a "$log"

note "PASS"
