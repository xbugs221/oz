#!/usr/bin/env bash
# 验证默认 prompt 历史范围和配置默认值一致性，覆盖本次工作流语义。
set -euo pipefail

cd "$(dirname "$0")/.."

go test ./internal/app \
  -run 'TestBundledReviewPromptUsesLatestHistoryOnly|TestBundledFixPromptFocusesCurrentReview|TestBundledFixPromptRequiresRootCauseAnalysis|TestDefaultWorkflowConfigMatchesUserDocs|TestInitWorkflowConfigWritesDefaultMCYAML' \
  -count=1
