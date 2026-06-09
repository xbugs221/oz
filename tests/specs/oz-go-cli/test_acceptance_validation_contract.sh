#!/usr/bin/env bash
# Sources: 2-合并-wo-执行器到-oz-仓库
# 文件目的：验证 oz validate 正式校验当前 wo 允许的 acceptance.json 格式。
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

cd "$repo_root"
oz="$tmp/oz"
go build -o "$oz" ./cmd/oz

project="$tmp/project"
change="1-统一验收合同"
change_dir="$project/docs/changes/$change"
mkdir -p "$change_dir/tests"

cat >"$change_dir/proposal.md" <<'MD'
# 统一验收合同

## 背景

执行器在 sealed run 前需要验收合同。
MD

cat >"$change_dir/design.md" <<'MD'
# 设计

使用当前 wo 已允许的 acceptance.json 字段作为 oz validate 的正式校验对象。
MD

cat >"$change_dir/spec.md" <<'MD'
# 规格

### 需求：统一验收合同

系统必须在提案进入执行前校验验收合同。

#### 场景：合法合同通过

- **当** 用户运行 oz validate
- **则** 当前 wo 允许的 acceptance.json 格式通过校验
MD

cat >"$change_dir/task.md" <<'MD'
# 任务

- [x] 1.1 写入验收合同
MD

cat >"$change_dir/tests/merge_contract_test.sh" <<'SH'
#!/usr/bin/env bash
# 这个测试代表提案自带的真实契约测试入口，断言验收合同随 change 落盘。
set -euo pipefail
test -s docs/changes/1-统一验收合同/acceptance.json
SH

cat >"$change_dir/acceptance.json" <<'JSON'
{
  "summary": "验证 oz validate 接受当前 wo 允许的验收合同格式",
  "required_tests": [
    {
      "id": "sample-contract",
      "source": "change_contract",
      "path": "docs/changes/1-统一验收合同/tests/merge_contract_test.sh",
      "command": "bash docs/changes/1-统一验收合同/tests/merge_contract_test.sh",
      "purpose": "证明 change 自带契约测试入口被验收合同引用"
    }
  ],
  "required_evidence": [
    {
      "id": "validate-json",
      "kind": "runtime_log",
      "path": "test-results/oz-validate-valid.json",
      "purpose": "记录 oz validate 对当前 wo 格式 acceptance.json 的校验结果"
    }
  ]
}
JSON

(cd "$project" && "$oz" validate "$change" --json) >"$tmp/valid.json"
grep -q '"valid": true' "$tmp/valid.json"
grep -q '"acceptance.json"' "$tmp/valid.json"

mv "$change_dir/acceptance.json" "$change_dir/acceptance.json.bak"
if (cd "$project" && "$oz" validate "$change" --json) >"$tmp/missing.json" 2>"$tmp/missing.err"; then
  echo "oz validate 不应接受缺失 acceptance.json 的 change" >&2
  exit 1
fi
grep -qi 'acceptance' "$tmp/missing.err" "$tmp/missing.json"
mv "$change_dir/acceptance.json.bak" "$change_dir/acceptance.json"

cat >"$change_dir/acceptance.json" <<'JSON'
{
  "summary": "包含当前 wo 不允许的扩展字段",
  "coverage": [],
  "required_tests": [
    {
      "id": "sample-contract",
      "source": "change_contract",
      "path": "docs/changes/1-统一验收合同/tests/merge_contract_test.sh",
      "command": "bash docs/changes/1-统一验收合同/tests/merge_contract_test.sh",
      "purpose": "证明 change 自带契约测试入口被验收合同引用"
    }
  ],
  "required_evidence": []
}
JSON
if (cd "$project" && "$oz" validate "$change" --json) >"$tmp/unknown.json" 2>"$tmp/unknown.err"; then
  echo "oz validate 不应接受当前 wo schema 不允许的 coverage 字段" >&2
  exit 1
fi
grep -qi 'coverage\|unknown\|acceptance' "$tmp/unknown.err" "$tmp/unknown.json"

echo "PASS"
