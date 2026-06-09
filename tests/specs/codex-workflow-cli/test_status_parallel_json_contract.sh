#!/usr/bin/env bash
# 文件功能目的：验证 wo status --run-id --json 在存在 parallel artifacts 时仍保持 runner 机器接口，不输出人类并行摘要。
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
RESULT_DIR="$ROOT/test-results/32-status-parallel/json"
TEST_FILE="$ROOT/internal/app/status_parallel_json_contract_test.go"

rm -rf "$RESULT_DIR"
mkdir -p "$RESULT_DIR"
cleanup() {
  rm -f "$TEST_FILE"
}
trap cleanup EXIT

cd "$ROOT"

cat > "$TEST_FILE" <<'GO'
package app

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestStatusJSONDoesNotExposeParallelHumanSummary 验证 runner JSON status 不因人类 status 增强而改变字段合同。
func TestStatusJSONDoesNotExposeParallelHumanSummary(t *testing.T) {
	repo := gitRepo(t)
	chdir(t, repo)
	workflow := DefaultWorkflowConfig()
	workflow.Parallel.Enabled = true
	workflow.Parallel.Groups["review"] = ParallelGroupConfig{
		Mode: "gate_input",
		Members: []ParallelMemberConfig{
			{Name: "目标核对审核员", Purpose: "核对目标"},
			{Name: "安全风险审核员", Purpose: "检查风险"},
		},
	}
	state := State{
		RunID:      "parallel-json-run",
		ChangeName: "demo",
		Sealed:     true,
		Status:     statusRunning,
		Stage:      "review_1",
		Stages:     map[string]string{"execution": "completed"},
		Paths:      map[string]string{"review": "review-1.json"},
		Sessions:   map[string]string{"codex:reviewer": "reviewer-session"},
		Workflow:   workflow,
	}
	if err := saveState(repo, state); err != nil {
		t.Fatal(err)
	}
	if err := writeJSONFile(filepath.Join(runDir(repo, state.RunID), "parallel-review-1.json"), ParallelArtifact{
		Group:   "review",
		Mode:    "gate_input",
		Summary: "review helpers completed",
		Members: []ParallelMemberResult{
			{Name: "目标核对审核员", Purpose: "核对目标", Status: "success", Summary: "target matches"},
			{Name: "安全风险审核员", Purpose: "检查风险", Status: "success", Summary: "no risk"},
		},
	}); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	if err := Run([]string{"status", "--run-id", state.RunID, "--json"}, strings.NewReader(""), &stdout, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	statusSaveJSONResult(t, stdout.String())

	var raw map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &raw); err != nil {
		t.Fatalf("status JSON is not parseable: %v\n%s", err, stdout.String())
	}
	for _, want := range []string{"run_id", "change_name", "status", "stage", "stages", "paths", "sessions", "error"} {
		if _, ok := raw[want]; !ok {
			t.Fatalf("status JSON missing %q: %s", want, stdout.String())
		}
	}
	for _, banned := range []string{"parallel", "parallel_status", "parallel_summary", "members", "stage_timings"} {
		if _, ok := raw[banned]; ok {
			t.Fatalf("status JSON leaked %q: %s", banned, stdout.String())
		}
	}
	for _, bannedText := range []string{"并行", "代码库侦察员", "安全风险审核员"} {
		if strings.Contains(stdout.String(), bannedText) {
			t.Fatalf("status JSON leaked human parallel text %q: %s", bannedText, stdout.String())
		}
	}
}

func statusSaveJSONResult(t *testing.T, text string) {
	t.Helper()
	resultDir := os.Getenv("WO_STATUS_PARALLEL_RESULT_DIR")
	if resultDir == "" {
		return
	}
	if err := os.MkdirAll(resultDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(resultDir, "status.json"), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}
GO

WO_STATUS_PARALLEL_RESULT_DIR="$RESULT_DIR" go test ./internal/app -run TestStatusJSONDoesNotExposeParallelHumanSummary -count=1 -v | tee "$RESULT_DIR/contract.log"
