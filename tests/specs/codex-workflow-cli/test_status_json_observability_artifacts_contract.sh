#!/usr/bin/env bash
# 文件功能目的：验证 wo status --run-id --json 新增 observability，并给出阶段与子代理固定产物路径。
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
result_dir="$repo_root/test-results/7-status-watch-compact-output"
test_file="$repo_root/tests/app/status_json_observability_artifacts_contract_test.gotest"

mkdir -p "$result_dir"
log="$result_dir/status-json-observability-artifacts.log"
: >"$log"

cleanup() {
  rm -f "$test_file"
}
trap cleanup EXIT

note() {
  # note 记录 JSON 合同测试的关键步骤，方便执行阶段定位失败字段。
  printf '%s\n' "$*" | tee -a "$log"
}

cd "$repo_root"

note "写入 internal/app JSON observability 契约测试"
cat >"$test_file" <<'GO'
package app

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestStatusJSONObservabilityArtifactsContract 验证 JSON status 为下游工具暴露固定产物路径。
func TestStatusJSONObservabilityArtifactsContract(t *testing.T) {
	repo, state, artifacts := jsonObservabilityFixture(t)

	var stdout bytes.Buffer
	inRepoForJSON(t, repo, func() {
		if err := Run([]string{"status", "--run-id", state.RunID, "--json"}, strings.NewReader(""), &stdout, &stdout); err != nil {
			t.Fatal(err)
		}
	})
	got := stdout.String()
	saveJSONResult(t, "status-json-observability.json", got)

	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(got), &payload); err != nil {
		t.Fatalf("status JSON must be valid JSON: %v\n%s", err, got)
	}
	for _, key := range []string{"run_id", "change_name", "status", "stage", "stages", "paths", "sessions", "error"} {
		if _, ok := payload[key]; !ok {
			t.Fatalf("JSON lost existing runner field %q:\n%s", key, got)
		}
	}
	if payload["run_id"] != state.RunID || payload["change_name"] != state.ChangeName || payload["stage"] != state.Stage {
		t.Fatalf("existing runner fields changed:\n%s", got)
	}

	observability, ok := payload["observability"].(map[string]interface{})
	if !ok {
		t.Fatalf("JSON must include observability object:\n%s", got)
	}
	if observability["engine"] != "go-dag" {
		t.Fatalf("observability.engine must be go-dag:\n%s", got)
	}
	rows, ok := observability["rows"].([]interface{})
	if !ok || len(rows) == 0 {
		t.Fatalf("observability.rows must be a non-empty array:\n%s", got)
	}
	byName := map[string]map[string]interface{}{}
	for _, raw := range rows {
		row, ok := raw.(map[string]interface{})
		if !ok {
			t.Fatalf("row must be object: %#v", raw)
		}
		name, _ := row["name"].(string)
		byName[name] = row
	}

	assertRowArtifact(t, byName, "执行阶段", "stage_artifact", filepath.Join(repo, "docs", "changes", state.ChangeName, "task.md"))
	assertRowArtifact(t, byName, "审核阶段", "stage_artifact", filepath.Join(runDir(repo, state.RunID), "review-1.json"))
	assertRowArtifact(t, byName, "测试阶段", "stage_artifact", filepath.Join(runDir(repo, state.RunID), "qa-1.json"))
	assertRowArtifact(t, byName, "归档阶段", "stage_artifact", filepath.Join(runDir(repo, state.RunID), "delivery-summary.md"))

	codeRow := byName["代码侦察"]
	if codeRow == nil {
		t.Fatalf("missing compact subagent row 代码侦察:\n%s", got)
	}
	for key, want := range map[string]interface{}{
		"kind":       "subagent",
		"full_name":  "代码库侦察员",
		"group":      "implementation_context",
		"stage":      "execution",
		"session_id": "subagent-session-1",
		"marker":     "✓",
	} {
		if codeRow[key] != want {
			t.Fatalf("代码侦察 row %s = %#v, want %#v\nrow=%#v\njson=%s", key, codeRow[key], want, codeRow, got)
		}
	}
	assertArtifactMapValue(t, codeRow, "member_artifact", artifacts["code_member"])
	assertArtifactMapValue(t, codeRow, "group_artifact", artifacts["implementation_group"])

	rootArtifacts, ok := observability["artifacts"].(map[string]interface{})
	if !ok {
		t.Fatalf("observability.artifacts must be object:\n%s", got)
	}
	for key, want := range map[string]string{
		"run_state":         filepath.Join(runDir(repo, state.RunID), "state.json"),
		"change_proposal":   filepath.Join(repo, "docs", "changes", state.ChangeName, "proposal.md"),
		"change_design":     filepath.Join(repo, "docs", "changes", state.ChangeName, "design.md"),
		"change_spec":       filepath.Join(repo, "docs", "changes", state.ChangeName, "spec.md"),
		"change_task":       filepath.Join(repo, "docs", "changes", state.ChangeName, "task.md"),
		"change_acceptance": filepath.Join(repo, "docs", "changes", state.ChangeName, "acceptance.json"),
	} {
		if rootArtifacts[key] != want {
			t.Fatalf("observability.artifacts[%s] = %#v, want %q\n%s", key, rootArtifacts[key], want, got)
		}
	}
}

// jsonObservabilityFixture 创建真实 change 文档、状态文件和 subagent artifact，供 JSON 产物路径断言使用。
func jsonObservabilityFixture(t *testing.T) (string, State, map[string]string) {
	t.Helper()
	repo := gitRepo(t)
	changeName := "7-统一输出"
	changeDir := filepath.Join(repo, "docs", "changes", changeName)
	if err := os.MkdirAll(filepath.Join(changeDir, "tests"), 0o755); err != nil {
		t.Fatal(err)
	}
	changeFiles := map[string]string{
		"proposal.md":    "# 提案\n",
		"design.md":      "# 设计\n",
		"spec.md":        "# 规格\n",
		"task.md":        "# 任务\n- [x] 完成\n",
		"acceptance.json": "{\"summary\":\"ok\"}\n",
	}
	for name, body := range changeFiles {
		if err := os.WriteFile(filepath.Join(changeDir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	workflow := DefaultWorkflowConfig()
	workflow.MaxReviewIterations = 1
	workflow.Parallel.Enabled = true
	workflow.Parallel.Groups["implementation_context"] = ParallelGroupConfig{
		Mode: "advisory",
		Members: []ParallelMemberConfig{
			{Name: "代码库侦察员", Purpose: "搜索现有模块", Tool: "pi", Subagent: "explore"},
			{Name: "外部资料研究员", Purpose: "查询外部资料", Tool: "pi", Subagent: "librarian"},
		},
	}

	runID := "20260609T070010.000000000Z"
	runPath := runDir(repo, runID)
	codeArtifact := filepath.Join(runPath, "parallel-members", "implementation_context", "code-scout.json")
	docsArtifact := filepath.Join(runPath, "parallel-members", "implementation_context", "external-docs.json")
	groupArtifact := filepath.Join(runPath, "parallel-implementation-context.json")

	state := State{
		RunID:      runID,
		ChangeName: changeName,
		Sealed:     true,
		Status:     statusRunning,
		Stage:      "execution",
		Engine:     "go-dag",
		Sessions: map[string]string{
			sessionStateKey("codex", "planner"): "planner-session",
			sessionStateKey("codex", "executor"): "writer-session",
			sessionStateKey("pi", "subagent:implementation_context:代码库侦察员:0"): "subagent-session-1",
			sessionStateKey("pi", "subagent:implementation_context:外部资料研究员:0"): "subagent-session-2",
		},
		Stages: map[string]string{"planning": "completed"},
		StageTimings: map[string]StageTiming{
			"planning":  {StartedAt: "2026-06-09T00:00:00Z", FinishedAt: "2026-06-09T00:02:00Z"},
			"execution": {StartedAt: "2026-06-09T00:02:00Z", FinishedAt: "2026-06-09T00:08:30Z"},
		},
		DAGNodes: map[string]DAGNodeState{
			"before_execution_1": {Status: "success", Artifact: codeArtifact, StartedAt: "2026-06-09T00:03:00Z", FinishedAt: "2026-06-09T00:04:06Z"},
			"before_execution_2": {Status: "success", Artifact: docsArtifact, StartedAt: "2026-06-09T00:03:00Z", FinishedAt: "2026-06-09T00:03:48Z"},
		},
		Paths:    map[string]string{},
		Workflow: workflow,
	}
	if err := saveState(repo, state); err != nil {
		t.Fatal(err)
	}
	if err := writeJSONFile(codeArtifact, ParallelMemberResult{Name: "代码库侦察员", Purpose: "搜索现有模块", Status: "success", Summary: "ok"}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSONFile(docsArtifact, ParallelMemberResult{Name: "外部资料研究员", Purpose: "查询外部资料", Status: "success", Summary: "ok"}); err != nil {
		t.Fatal(err)
	}
	if err := writeJSONFile(groupArtifact, ParallelArtifact{
		Group: "implementation_context",
		Mode:  "advisory",
		Members: []ParallelMemberResult{
			{Name: "代码库侦察员", Purpose: "搜索现有模块", Status: "success", Summary: "ok"},
			{Name: "外部资料研究员", Purpose: "查询外部资料", Status: "success", Summary: "ok"},
		},
		Summary: "implementation context completed",
	}); err != nil {
		t.Fatal(err)
	}
	return repo, state, map[string]string{
		"code_member":          codeArtifact,
		"docs_member":          docsArtifact,
		"implementation_group": groupArtifact,
	}
}

// assertRowArtifact 验证阶段 row 中固定 artifact 路径存在且为绝对路径。
func assertRowArtifact(t *testing.T, rows map[string]map[string]interface{}, rowName, key, want string) {
	t.Helper()
	row := rows[rowName]
	if row == nil {
		t.Fatalf("missing row %s", rowName)
	}
	assertArtifactMapValue(t, row, key, want)
}

// assertArtifactMapValue 验证 row.artifacts 中的指定路径字段。
func assertArtifactMapValue(t *testing.T, row map[string]interface{}, key, want string) {
	t.Helper()
	artifacts, ok := row["artifacts"].(map[string]interface{})
	if !ok {
		t.Fatalf("row %s artifacts must be object: %#v", row["name"], row)
	}
	got, ok := artifacts[key].(string)
	if !ok {
		t.Fatalf("row %s missing artifact %s: %#v", row["name"], key, row)
	}
	if got != want {
		t.Fatalf("row %s artifact %s = %q, want %q", row["name"], key, got, want)
	}
	if !filepath.IsAbs(got) {
		t.Fatalf("artifact path must be absolute: %s=%q", key, got)
	}
}

// inRepoForJSON 在临时仓库目录中执行命令解析，覆盖 GitRoot(".") 的真实路径。
func inRepoForJSON(t *testing.T, repo string, fn func()) {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(repo); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Chdir(wd); err != nil {
			t.Fatal(err)
		}
	}()
	fn()
}

// saveJSONResult 保存 JSON 输出，作为 acceptance runtime log 的补充材料。
func saveJSONResult(t *testing.T, name, body string) {
	t.Helper()
	dir := filepath.Join("..", "test-results", "7-status-watch-compact-output")
	_ = os.MkdirAll(dir, 0o755)
	_ = os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644)
}
GO

note "运行 JSON observability 契约测试"
OZ_MIGRATED_APP_RUN=TestStatusJSONObservabilityArtifactsContract \
  go test ./tests/app -run TestMigratedAppTestsRunWithGoToolchain -count=1 2>&1 | tee -a "$log"
