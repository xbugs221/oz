#!/usr/bin/env bash
# 文件功能目的：验证 oz flow status --run-id --json 新增 observability，并给出阶段与子代理固定产物路径。
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
result_dir="$repo_root/test-results/7-status-watch-compact-output"

mkdir -p "$result_dir"
log="$result_dir/status-json-observability-artifacts.log"
cd "$repo_root"

go test ./internal/app -run TestStatusJSONObservabilityArtifactsContract -count=1 -v 2>&1 | tee "$log"
