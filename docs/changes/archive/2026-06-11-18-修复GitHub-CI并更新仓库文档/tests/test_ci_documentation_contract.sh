#!/usr/bin/env bash
# 文件功能目的：验证 README、长期规格和 GitHub Actions workflow 对 CI/Release 测试门禁的说明保持一致。
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
RESULT_DIR="$ROOT/test-results/18-github-ci-docs"
LOG="$RESULT_DIR/ci-documentation-contract.log"

mkdir -p "$RESULT_DIR"
: >"$LOG"

note() {
  # 函数目的：记录文档门禁检查过程，供 review/QA 复核。
  printf '[ci-docs] %s\n' "$*" | tee -a "$LOG"
}

fail() {
  # 函数目的：指出缺失的用户可见文档或 workflow 合同。
  printf '[ci-docs] FAIL: %s\n' "$*" | tee -a "$LOG" >&2
  exit 1
}

require_file() {
  # 函数目的：确认本测试依赖的真实仓库文件存在。
  local path="$1"
  [[ -f "$path" ]] || fail "缺少文件：$path"
}

require_text() {
  # 函数目的：在真实文件中查找关键合同文本。
  local path="$1"
  local pattern="$2"
  local message="$3"
  if ! grep -Eq "$pattern" "$path"; then
    fail "$message"
  fi
}

README="$ROOT/README.md"
RELEASE_SPEC="$ROOT/docs/specs/release-automation/spec.md"
CI_WORKFLOW="$ROOT/.github/workflows/ci.yml"
RELEASE_WORKFLOW="$ROOT/.github/workflows/release.yml"

require_file "$README"
require_file "$RELEASE_SPEC"
require_file "$CI_WORKFLOW"
require_file "$RELEASE_WORKFLOW"

note "检查 README 说明 GitHub Actions、CI、Release 和本地复现命令"
require_text "$README" 'GitHub Actions' "README 必须说明 GitHub Actions"
require_text "$README" 'CI' "README 必须说明 CI 门禁"
require_text "$README" 'Release' "README 必须说明 Release 门禁"
require_text "$README" 'go test \./\.\.\.' "README 必须列出 go test ./..."
require_text "$README" 'tests/\*\.sh' "README 必须列出根目录 tests/*.sh 业务测试"
require_text "$README" '本地复现|失败排查|复现 GitHub' "README 必须说明如何本地复现 GitHub CI 失败"

note "检查 release automation 长期规格继续约束同一套门禁"
require_text "$RELEASE_SPEC" 'go test \./\.\.\.' "release automation 规格必须列出 go test ./..."
require_text "$RELEASE_SPEC" 'tests/\*\.sh' "release automation 规格必须列出 tests/*.sh"
require_text "$RELEASE_SPEC" 'CI 和 Release 使用本地 oz|本地 oz' "release automation 规格必须说明 CI/Release 使用本地 oz/wo"

note "检查 CI workflow 运行真实本地门禁"
require_text "$CI_WORKFLOW" 'go test \./\.\.\.' "CI workflow 必须运行 go test ./..."
require_text "$CI_WORKFLOW" 'for script in tests/\*\.sh' "CI workflow 必须遍历根目录 tests/*.sh"
require_text "$CI_WORKFLOW" 'go build -o "\$install_dir/oz" \./cmd/oz' "CI workflow 必须构建本地 oz"
require_text "$CI_WORKFLOW" 'go build -o "\$install_dir/wo" \./cmd/wo' "CI workflow 必须构建本地 wo"

note "检查 Release workflow 使用同一测试门禁"
require_text "$RELEASE_WORKFLOW" 'go test \./\.\.\.' "Release workflow 必须运行 go test ./..."
require_text "$RELEASE_WORKFLOW" 'for script in tests/\*\.sh' "Release workflow 必须遍历根目录 tests/*.sh"
require_text "$RELEASE_WORKFLOW" 'go build -o "\$install_dir/oz" \./cmd/oz' "Release workflow 必须构建本地 oz"
require_text "$RELEASE_WORKFLOW" 'go build -o "\$install_dir/wo" \./cmd/wo' "Release workflow 必须构建本地 wo"

note "contract passed: CI/Release 文档和 workflow 门禁一致"
