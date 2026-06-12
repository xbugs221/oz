#!/usr/bin/env bash
# Sources: 13-修正-wo-status-多轮并行状态展示
# 文件功能目的：验证 wo status/watch 在多轮 review/fix 与 parallel subagent 并存时不泄漏内部 fan-in，并显示当前轮次状态。
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
RESULT_DIR="$ROOT/test-results/status-multiround-parallel-display"
TEST_FILE="$ROOT/tests/app/status_multiround_parallel_display_contract_test.gotest"
LOG="$RESULT_DIR/status-multiround-parallel-display.log"

mkdir -p "$RESULT_DIR"
: >"$LOG"

cleanup() {
  # cleanup 删除动态 Go 测试文件，避免创建阶段合同污染生产源码。
  rm -f "$TEST_FILE"
}
trap cleanup EXIT

note() {
  # note 同步记录测试步骤到终端和 runtime log，便于执行阶段复查失败点。
  printf '%s\n' "$*" | tee -a "$LOG"
}

cd "$ROOT"

note "写入 internal/app 包级合同测试，构造三轮 review 中第三轮失败的真实 durable state"
cat >"$TEST_FILE" <<'GO'
package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestHumanStatusMultiRoundParallelDisplayContract 验证多轮状态行和 subagent 明细不会混入内部 fan-in 或旧轮次。
func TestHumanStatusMultiRoundParallelDisplayContract(t *testing.T) {
	repo, state := multiroundStatusFixture(t)
	text := strings.Join(compactStatusLines(buildHumanStatusView(repo, state, "-w1", "")), "\n")
	saveMultiroundStatusResult(t, text)

	for _, banned := range []string{
		"- 并行",
		"implementation_context",
		"parallel-review",
		"LGTM_WITH_MINOR_CONCERNS",
		"completed - -",
		"需求分析",
		"review1-target",
	} {
		if strings.Contains(text, banned) {
			t.Fatalf("human status leaked %q:\n%s", banned, text)
		}
	}

	for _, want := range []string{
		"执行   executor-session ✓   9.00",
		"审核   reviewer-session ✓2x 3.00",
		"修正   fixer-session    ✓2  4.00",
		"代码 impl-code        ✓   1.00",
		"外部 impl-docs        ✓   1.00",
		"目标 review3-target   ✓   1.00",
		"代码 review3-quality  ✓   1.00",
		"测试 review3-test     ✓   1.00",
		"风险 review3-risk     ✓   1.00",
		"上下 review3-context  ✓   1.00",
	} {
		if !hasExactCompactLine(text, want) {
			t.Fatalf("human status missing exact line %q:\n%s", want, text)
		}
	}
}

// multiroundStatusFixture 创建真实 run state 和 artifact，复现 execution 后第三轮 review 失败的业务状态。
func multiroundStatusFixture(t *testing.T) (string, State) {
	t.Helper()
	repo := gitRepo(t)
	changeName := "status-display"
	changeDir := filepath.Join(repo, "docs", "changes", changeName)
	if err := os.MkdirAll(filepath.Join(changeDir, "tests"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"proposal.md", "design.md", "spec.md", "task.md", "acceptance.json"} {
		if err := os.WriteFile(filepath.Join(changeDir, name), []byte(name+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	workflow := DefaultWorkflowConfig()
	workflow.MaxReviewIterations = 3
	runID := "20260610T130000.000000000Z"
	runPath := runDir(repo, runID)
	state := State{
		RunID:      runID,
		ChangeName: changeName,
		Sealed:     true,
		Status:     statusFailed,
		Stage:      "review_3",
		Engine:     "go-dag",
		Error:      "exit status 1",
		Sessions: map[string]string{
			sessionStateKey("codex", "executor"): "executor-session",
			sessionStateKey("codex", "reviewer"): "reviewer-session",
			sessionStateKey("codex", "fixer"):    "fixer-session",
			sessionStateKey("pi", "subagent:implementation_context:代码库侦察员:0"): "impl-code",
			sessionStateKey("pi", "subagent:implementation_context:外部资料研究员:0"): "impl-docs",
			sessionStateKey("pi", "subagent:review:目标核对审核员:1"):   "review1-target",
			sessionStateKey("pi", "subagent:review:目标核对审核员:3"):   "review3-target",
			sessionStateKey("pi", "subagent:review:代码质量审核员:3"):   "review3-quality",
			sessionStateKey("pi", "subagent:review:测试有效性审核员:3"):  "review3-test",
			sessionStateKey("pi", "subagent:review:安全风险审核员:3"):   "review3-risk",
			sessionStateKey("pi", "subagent:review:上下文一致性审核员:3"): "review3-context",
		},
		Stages: map[string]string{
			"execution": "completed",
			"review_1":  "completed",
			"fix_1":     "completed",
			"review_2":  "completed",
			"fix_2":     "completed",
			"review_3":  "running",
		},
		StageTimings: map[string]StageTiming{
			"execution": {StartedAt: "2026-06-10T00:00:00Z", FinishedAt: "2026-06-10T00:09:00Z"},
			"review_1":  {StartedAt: "2026-06-10T00:10:00Z", FinishedAt: "2026-06-10T00:11:00Z"},
			"fix_1":     {StartedAt: "2026-06-10T00:12:00Z", FinishedAt: "2026-06-10T00:14:00Z"},
			"review_2":  {StartedAt: "2026-06-10T00:15:00Z", FinishedAt: "2026-06-10T00:16:00Z"},
			"fix_2":     {StartedAt: "2026-06-10T00:17:00Z", FinishedAt: "2026-06-10T00:19:00Z"},
			"review_3":  {StartedAt: "2026-06-10T00:20:00Z", FinishedAt: "2026-06-10T00:21:00Z"},
		},
		DAGNodes: map[string]DAGNodeState{},
		Paths:    map[string]string{},
		Workflow: workflow,
	}

	addSuccessNode(state.DAGNodes, "planning_context_1", filepath.Join(runPath, "parallel-members", "planning_context", "requirement.json"), "2026-06-10T00:00:00Z", "2026-06-10T00:01:00Z")
	addSuccessNode(state.DAGNodes, "planning_context_2", filepath.Join(runPath, "parallel-members", "planning_context", "code.json"), "2026-06-10T00:00:00Z", "2026-06-10T00:01:00Z")
	addSuccessNode(state.DAGNodes, "planning_context_3", filepath.Join(runPath, "parallel-members", "planning_context", "docs.json"), "2026-06-10T00:00:00Z", "2026-06-10T00:01:00Z")
	addSuccessNode(state.DAGNodes, "planning_context_fanin", filepath.Join(runPath, "parallel-planning-context.json"), "2026-06-10T00:01:00Z", "2026-06-10T00:01:01Z")
	addSuccessNode(state.DAGNodes, "before_execution_1", memberArtifactPath(repo, runID, "implementation_context", 0, "代码库侦察员"), "2026-06-10T00:02:00Z", "2026-06-10T00:03:00Z")
	addSuccessNode(state.DAGNodes, "before_execution_2", memberArtifactPath(repo, runID, "implementation_context", 0, "外部资料研究员"), "2026-06-10T00:02:00Z", "2026-06-10T00:03:00Z")
	addSuccessNode(state.DAGNodes, "before_execution_fanin", filepath.Join(runPath, "parallel-implementation-context.json"), "2026-06-10T00:03:00Z", "2026-06-10T00:03:01Z")
	addSuccessNode(state.DAGNodes, "execution", "", "", "2026-06-10T00:09:00Z")
	addSuccessNode(state.DAGNodes, "review_1", "", "", "2026-06-10T00:11:00Z")
	addSuccessNode(state.DAGNodes, "fix_1", "", "", "2026-06-10T00:14:00Z")
	addSuccessNode(state.DAGNodes, "review_2", "", "", "2026-06-10T00:16:00Z")
	addSuccessNode(state.DAGNodes, "fix_2", "", "", "2026-06-10T00:19:00Z")
	state.DAGNodes["review_3"] = DAGNodeState{Status: "failed", FinishedAt: "2026-06-10T00:21:00Z", Error: "exit status 1"}

	for iteration := 1; iteration <= 3; iteration++ {
		for index, member := range workflow.Parallel.Groups["review"].Members {
			start := "2026-06-10T00:20:00Z"
			finish := "2026-06-10T00:21:00Z"
			nodeID := "before_review_" + string(rune('0'+iteration)) + "_" + string(rune('0'+index+1))
			addSuccessNode(state.DAGNodes, nodeID, memberArtifactPath(repo, runID, "review", iteration, member.Name), start, finish)
		}
		addSuccessNode(state.DAGNodes, "before_review_"+string(rune('0'+iteration))+"_fanin", filepath.Join(runPath, "parallel-review-"+string(rune('0'+iteration))+".json"), "2026-06-10T00:21:00Z", "2026-06-10T00:21:01Z")
	}

	if err := saveState(repo, state); err != nil {
		t.Fatal(err)
	}
	if err := writeJSONFile(filepath.Join(runPath, "parallel-planning-context.json"), minimalParallelArtifact("planning_context", workflow.Parallel.Groups["planning_context"], "completed")); err != nil {
		t.Fatal(err)
	}
	if err := writeJSONFile(filepath.Join(runPath, "parallel-implementation-context.json"), ParallelArtifact{
		Group: "implementation_context",
		Mode:  workflow.Parallel.Groups["implementation_context"].Mode,
		Members: []ParallelMemberResult{
			{Name: "代码库侦察员", Purpose: "汇总 execution 需要读取的文件和测试模式", Status: "completed", Summary: "ok"},
			{Name: "外部资料研究员", Purpose: "查询 execution 依赖的外部库文档和开源实现", Status: "已完成外部依赖调研", Summary: "ok"},
		},
		Summary: "implementation context fanin completed",
	}); err != nil {
		t.Fatal(err)
	}
	for _, member := range workflow.Parallel.Groups["implementation_context"].Members {
		if err := writeJSONFile(memberArtifactPath(repo, runID, "implementation_context", 0, member.Name), ParallelMemberResult{Name: member.Name, Purpose: member.Purpose, Status: "completed", Summary: "ok"}); err != nil {
			t.Fatal(err)
		}
	}
	for iteration := 1; iteration <= 3; iteration++ {
		status := "completed"
		if iteration == 3 {
			status = "LGTM_WITH_MINOR_CONCERNS"
		}
		if err := writeJSONFile(filepath.Join(runPath, "parallel-review-"+string(rune('0'+iteration))+".json"), minimalParallelArtifact("review", workflow.Parallel.Groups["review"], status)); err != nil {
			t.Fatal(err)
		}
		for _, member := range workflow.Parallel.Groups["review"].Members {
			if err := writeJSONFile(memberArtifactPath(repo, runID, "review", iteration, member.Name), ParallelMemberResult{Name: member.Name, Purpose: member.Purpose, Status: "completed", Summary: "ok"}); err != nil {
				t.Fatal(err)
			}
		}
	}
	return repo, state
}

// addSuccessNode 记录一个成功 DAG node，用于让状态视图基于真实运行证据渲染。
func addSuccessNode(nodes map[string]DAGNodeState, id string, artifact string, startedAt string, finishedAt string) {
	nodes[id] = DAGNodeState{Status: "success", Artifact: artifact, StartedAt: startedAt, FinishedAt: finishedAt}
}

// minimalParallelArtifact 构造 fan-in artifact，覆盖 raw member status 被误渲染的风险。
func minimalParallelArtifact(group string, config ParallelGroupConfig, status string) ParallelArtifact {
	artifact := ParallelArtifact{Group: group, Mode: config.Mode, Summary: group + " completed"}
	for _, member := range config.Members {
		artifact.Members = append(artifact.Members, ParallelMemberResult{Name: member.Name, Purpose: member.Purpose, Status: status, Summary: "ok"})
	}
	return artifact
}

// hasExactCompactLine 使用完整行匹配，避免子串误判 status 输出。
func hasExactCompactLine(output string, want string) bool {
	for _, line := range strings.Split(output, "\n") {
		if strings.TrimSpace(line) == want {
			return true
		}
	}
	return false
}

// saveMultiroundStatusResult 保存 compact lines，作为 acceptance runtime log 的可复查证据。
func saveMultiroundStatusResult(t *testing.T, text string) {
	t.Helper()
	resultDir := os.Getenv("WO_STATUS_MULTIRUN_RESULT_DIR")
	if resultDir == "" {
		return
	}
	if err := os.MkdirAll(resultDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(resultDir, "compact-lines.txt"), []byte(text+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}
GO

note "运行 go test，期望执行前失败于 status 多轮并行展示合同"
if WO_STATUS_MULTIRUN_RESULT_DIR="$RESULT_DIR" \
  OZ_MIGRATED_APP_RUN=TestHumanStatusMultiRoundParallelDisplayContract \
  go test ./tests/app -run TestMigratedAppTestsRunWithGoToolchain -count=1 -v 2>&1 | tee -a "$LOG"; then
  note "合同测试已通过"
else
  note "合同测试失败；若失败点是目标展示行为缺失，则符合创建阶段预期"
  exit 1
fi
