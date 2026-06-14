#!/usr/bin/env bash
# Purpose: verify the public tree-shaped parallel helper config and prompt-only execution contract.
set -euo pipefail

ROOT=$(git rev-parse --show-toplevel)
cd "$ROOT"

go test ./internal/app -run 'TestDefaultWorkflowConfigYAMLIncludesTreeParallelHelpers|TestParallelMemberMetadataStillRequiresCodexPiPreflight|TestParallelPromptExplainsPromptOnlySubagentContract|TestPiRunArgsNeverReceiveSubagentStyleFlags' -count=1 -v
