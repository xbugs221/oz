#!/usr/bin/env bash
# 文件功能目的：验证 oz status --json 暴露 brief.md artifact 和验收合同摘要，避免只靠 task checkbox 判断就绪。
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

result_dir="$repo_root/test-results/8-强化验收硬合同"
mkdir -p "$result_dir"
log="$result_dir/status-hard-contract-summary.log"

cd "$repo_root"
oz="$tmp/oz"
go build -o "$oz" ./cmd/oz

project="$tmp/project"
change="1-状态合同摘要"
change_dir="$project/docs/changes/$change"
mkdir -p "$change_dir/tests"

cat >"$change_dir/brief.md" <<'MD'
# 状态合同摘要

用户从简报理解变更，调度器从 acceptance 摘要理解硬门槛规模。
MD

cat >"$change_dir/proposal.md" <<'MD'
# 状态合同摘要

## 背景

status 需要暴露硬合同规模。
MD

cat >"$change_dir/design.md" <<'MD'
# 设计

status JSON 读取 acceptance.json 并输出摘要。
MD

cat >"$change_dir/spec.md" <<'MD'
# 规格

### 需求：状态合同摘要

系统必须在 status JSON 中暴露验收合同规模。

#### 场景：status 包含合同摘要

- **当** 用户运行 oz status
- **则** JSON 包含 brief artifact 和 acceptance 摘要
MD

cat >"$change_dir/task.md" <<'MD'
# 任务

- [x] 1.1 写入合同摘要示例
MD

cat >"$change_dir/tests/test_contract.sh" <<'SH'
#!/usr/bin/env bash
# 这个测试代表状态合同摘要提案的真实契约测试入口。
set -euo pipefail
test -s docs/changes/1-状态合同摘要/brief.md
test -s docs/changes/1-状态合同摘要/acceptance.json
SH
chmod +x "$change_dir/tests/test_contract.sh"

cat >"$change_dir/acceptance.json" <<'JSON'
{
  "summary": "状态输出包含硬合同摘要",
  "coverage": [
    {
      "spec": "需求：状态合同摘要 / 场景：status 包含合同摘要",
      "tests": ["status-contract"],
      "evidence": ["status-json"],
      "risk": ""
    }
  ],
  "required_tests": [
    {
      "id": "status-contract",
      "source": "change_contract",
      "path": "docs/changes/1-状态合同摘要/tests/test_contract.sh",
      "command": "bash docs/changes/1-状态合同摘要/tests/test_contract.sh",
      "purpose": "证明 status 示例提案拥有真实契约测试入口",
      "assertions": [
        "brief.md 存在并作为用户简报 artifact 暴露",
        "acceptance.json 存在并可被 status 读取为合同摘要"
      ]
    }
  ],
  "required_evidence": [
    {
      "id": "status-json",
      "kind": "runtime_log",
      "path": "test-results/status-contract/status.json",
      "purpose": "记录 oz status --json 输出"
    }
  ]
}
JSON

(cd "$project" && "$oz" status "$change" --json) | tee "$log"

python3 - "$log" <<'PY'
import json
import sys

path = sys.argv[1]
payload = json.load(open(path, encoding="utf-8"))
artifacts = {item["name"]: item["status"] for item in payload.get("artifacts", [])}
if artifacts.get("brief.md") != "present":
    raise SystemExit(f"status artifacts 缺少 present brief.md: {artifacts}")

acceptance = payload.get("acceptance")
if not isinstance(acceptance, dict):
    raise SystemExit(f"status JSON 缺少 acceptance 摘要: {payload}")

checks = {
    "required_tests": 1,
    "required_evidence": 1,
    "coverage": 1,
}
for key, want in checks.items():
    got = acceptance.get(key, {}).get("total")
    if got != want:
        raise SystemExit(f"acceptance.{key}.total = {got}, want {want}")
PY
