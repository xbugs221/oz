#!/usr/bin/env bash
# 文件功能目的：验证 prompt/stage 配置不再接受 writing，sealed run 不再读取 legacy prompt 备份。
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
RESULT_DIR="$ROOT/test-results/5-clean-wo-legacy/prompt-legacy-removed"
TEST_FILE="$ROOT/internal/app/prompt_legacy_removed_contract_test.go"

rm -rf "$RESULT_DIR"
mkdir -p "$RESULT_DIR"
trap 'rm -f "$TEST_FILE"' EXIT

cat > "$TEST_FILE" <<'GO'
// Package app receives an injected contract test for removing prompt legacy compatibility.
package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestPromptConfigRejectsWritingPrompt verifies old prompt aliases no longer configure execution or fix.
func TestPromptConfigRejectsWritingPrompt(t *testing.T) {
	repo := t.TempDir()
	mustWritePrompt(t, filepath.Join(repo, "wo.yaml"), "wo:\n  prompts:\n    writing: \"old {{.Stage}}\\n\"\n")

	_, err := LoadWorkflowConfig(repo)
	if err == nil {
		t.Fatal("expected prompts.writing to be rejected")
	}
	if !strings.Contains(err.Error(), "writing") {
		t.Fatalf("error should name writing, got %v", err)
	}
}

// TestStageConfigRejectsWritingStage verifies old stage aliases are no longer part of the config surface.
func TestStageConfigRejectsWritingStage(t *testing.T) {
	repo := t.TempDir()
	mustWritePrompt(t, filepath.Join(repo, "wo.yaml"), "wo:\n  workflow:\n    stages:\n      writing:\n        cli: codex\n")

	_, err := LoadWorkflowConfig(repo)
	if err == nil {
		t.Fatal("expected workflow.stages.writing to be rejected")
	}
	if !strings.Contains(err.Error(), "writing") {
		t.Fatalf("error should name writing, got %v", err)
	}
}

// TestPromptSnapshotDoesNotReadLegacyPromptDir verifies sealed runs fail closed without YAML snapshots.
func TestPromptSnapshotDoesNotReadLegacyPromptDir(t *testing.T) {
	repo := t.TempDir()
	runID := "legacy-dir-run"
	mustWritePrompt(t, filepath.Join(runDir(repo, runID), "prompts", "wo-start.md"), "legacy {{.Stage}}\n")

	got, err := promptForStage(repo, State{RunID: runID, Stage: "execution", Sealed: true})
	if err == nil {
		t.Fatalf("expected missing prompt-snapshot.yaml to fail, got prompt %q", got)
	}
	if strings.Contains(got, "legacy") {
		t.Fatalf("legacy prompt directory should not be rendered: %q", got)
	}
	if !strings.Contains(err.Error(), "prompt-snapshot.yaml") {
		t.Fatalf("error should point to missing YAML snapshot, got %v", err)
	}
}

// TestPromptSnapshotDoesNotMapWritingKey verifies frozen writing-only snapshots no longer resume current prompt roles.
func TestPromptSnapshotDoesNotMapWritingKey(t *testing.T) {
	repo := t.TempDir()
	runID := "writing-only-run"
	mustWritePrompt(t, filepath.Join(runDir(repo, runID), "prompt-snapshot.yaml"), "prompts:\n  writing: \"old {{.Stage}}\\n\"\n")

	for _, stage := range []string{"execution", "qa_1", "fix_1"} {
		got, err := promptForStage(repo, State{RunID: runID, Stage: stage, Sealed: true})
		if err == nil {
			t.Fatalf("%s: expected writing-only snapshot to fail, got prompt %q", stage, got)
		}
		if !strings.Contains(err.Error(), "prompts.") {
			t.Fatalf("%s: error should name missing prompt key, got %v", stage, err)
		}
	}
}

// TestPromptSnapshotIncludesAllCurrentRoleKeys verifies snapshots cover the full current prompt role set.
func TestPromptSnapshotIncludesAllCurrentRoleKeys(t *testing.T) {
	repo := t.TempDir()
	runID := "current-roles-run"
	mustWritePrompt(t, filepath.Join(repo, "wo.yaml"), `wo:
  prompts:
    planning: planning
    execution: execution
    review: review
    qa: qa
    fix: fix
    archive: archive
`)

	if err := snapshotRunPrompts(repo, runID); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(runDir(repo, runID), "prompt-snapshot.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	for _, key := range []string{"planning:", "execution:", "review:", "qa:", "fix:", "archive:"} {
		if !strings.Contains(body, key) {
			t.Fatalf("snapshot missing %s:\n%s", key, body)
		}
	}
	if strings.Contains(body, "writing:") {
		t.Fatalf("snapshot should not include writing:\n%s", body)
	}
}

// TestStageKindsExcludeWriting verifies the runtime role table exposes only current workflow stages.
func TestStageKindsExcludeWriting(t *testing.T) {
	kinds := map[string]bool{}
	for _, kind := range roleStageKinds() {
		if kind == "writing" {
			t.Fatal("roleStageKinds should not include legacy writing")
		}
		kinds[kind] = true
	}
	for _, kind := range []string{"planning", "execution", "review", "qa", "fix", "archive"} {
		if !kinds[kind] {
			t.Fatalf("roleStageKinds should include %s, got %#v", kind, kinds)
		}
	}
	config := DefaultWorkflowConfig()
	if _, ok := config.Prompts["writing"]; ok {
		t.Fatalf("default prompts should not include writing: %#v", config.Prompts)
	}
	if _, ok := config.Stages["writing"]; ok {
		t.Fatalf("default stages should not include writing: %#v", config.Stages)
	}
	for _, key := range []string{"planning", "execution", "review", "qa", "fix", "archive"} {
		if _, ok := config.Prompts[key]; !ok {
			t.Fatalf("default prompts should include %s: %#v", key, config.Prompts)
		}
	}
	for _, stage := range []string{"planning", "execution", "review_1", "qa_1", "fix_1", "archive"} {
		if _, ok := config.Stages[stage]; !ok {
			t.Fatalf("default stages should include %s: %#v", stage, config.Stages)
		}
	}
}
GO

(
  cd "$ROOT"
  go test ./internal/app -run 'TestPromptConfigRejectsWritingPrompt|TestStageConfigRejectsWritingStage|TestPromptSnapshotDoesNot(ReadLegacyPromptDir|MapWritingKey)|TestPromptSnapshotIncludesAllCurrentRoleKeys|TestStageKindsExcludeWriting' -count=1
) | tee "$RESULT_DIR/contract.log"
