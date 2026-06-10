#!/usr/bin/env bash
# 文件功能目的：验证 wo status/watch 使用统一极简固定列视图，并且 batch 只是多个 workflow 视图的组合。
set -euo pipefail

repo_root="$(git rev-parse --show-toplevel)"
result_dir="$repo_root/test-results/7-status-watch-compact-output"
test_file="$repo_root/internal/app/status_watch_compact_output_contract_test.go"
tmpdir=""

mkdir -p "$result_dir"
log="$result_dir/status-watch-compact-output.log"
: >"$log"

cleanup() {
  rm -f "$test_file"
  if [[ -n "$tmpdir" ]]; then
    rm -rf "$tmpdir"
  fi
}
trap cleanup EXIT

note() {
  # note 记录测试关键步骤，方便执行阶段复查合同失败点。
  printf '%s\n' "$*" | tee -a "$log"
}

cd "$repo_root"

note "写入 internal/app 包级契约测试，直接覆盖 wo status/watch 渲染路径"
cat >"$test_file" <<'GO'
package app

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestStatusWatchCompactOutputContract 验证 status/watch 使用同一套极简固定列视图。
func TestStatusWatchCompactOutputContract(t *testing.T) {
	repo, state := compactStatusFixture(t)

	var status bytes.Buffer
	inRepo(t, repo, func() {
		if err := Run([]string{"status", "-w1"}, strings.NewReader(""), &status, &status); err != nil {
			t.Fatal(err)
		}
	})
	gotStatus := strings.TrimSpace(status.String())
	saveCompactResult(t, "status-w1.txt", gotStatus)

	wantStatusLines := []string{
		"- 7-统一输出",
		"  规划阶段 planner-session ✓ 2.00",
		"  执行阶段 writer-session → 6.50",
		"    - 并行 implementation_context 2/2 success -",
		"      - 代码库侦察员 success - -",
		"      - 外部资料研究员 success - -",
		"    代码侦察 subagent-session-1 ✓ 1.10",
		"    外部资料 subagent-session-2 ✓ 0.80",
		"  审核阶段 reviewer-session - -",
		"  测试阶段 - - -",
		"  归档阶段 - - -",
	}
	for _, want := range wantStatusLines {
		if !hasExactLine(gotStatus, want) {
			t.Fatalf("status output missing exact line %q:\n%s", want, gotStatus)
		}
	}
	if !strings.HasPrefix(gotStatus, "- 7-统一输出\n") {
		t.Fatalf("status first line must be proposal list item:\n%s", gotStatus)
	}
	for _, banned := range []string{"工作流", "批量任务", "引擎", "耗时"} {
		if strings.Contains(gotStatus, banned) {
			t.Fatalf("status output should not contain %q:\n%s", banned, gotStatus)
		}
	}

	gotWatch := strings.Join(watchStatusLines(repo, "run", StatusRef{Alias: "w1", ID: state.RunID}, "|"), "\n")
	saveCompactResult(t, "watch-w1.txt", gotWatch)
	for _, want := range []string{
		"- 7-统一输出",
		"  规划阶段 planner-session ✓ 2.00",
		"  执行阶段 writer-session | 6.50",
		"    - 并行 implementation_context 2/2 success -",
		"    代码侦察 subagent-session-1 ✓ 1.10",
		"    外部资料 subagent-session-2 ✓ 0.80",
	} {
		if !hasExactLine(gotWatch, want) {
			t.Fatalf("watch output missing exact line %q:\n%s", want, gotWatch)
		}
	}
	if !strings.HasPrefix(gotWatch, "- 7-统一输出\n") {
		t.Fatalf("watch first line must be proposal list item:\n%s", gotWatch)
	}

	batchID := "20260609T070001.000000000Z"
	batch := BatchState{
		BatchID:      batchID,
		Status:       batchStatusRunning,
		Changes:      []string{state.ChangeName, "8-待执行"},
		CurrentIndex: 0,
		RunIDs:       map[string]string{state.ChangeName: state.RunID},
	}
	if err := saveBatchState(repo, batch); err != nil {
		t.Fatal(err)
	}
	gotBatch := strings.Join(watchStatusLines(repo, "batch", StatusRef{Alias: "b1", ID: batchID}, "|"), "\n")
	saveCompactResult(t, "watch-b1.txt", gotBatch)
	for _, want := range []string{
		"- 7-统一输出",
		"  规划阶段 planner-session ✓ 2.00",
		"  执行阶段 writer-session | 6.50",
		"    - 并行 implementation_context 2/2 success -",
		"    代码侦察 subagent-session-1 ✓ 1.10",
		"- 8-待执行",
	} {
		if !hasExactLine(gotBatch, want) {
			t.Fatalf("batch watch output missing exact line %q:\n%s", want, gotBatch)
		}
	}
	if strings.Contains(gotBatch, "批量任务") || strings.Contains(gotBatch, "工作流") {
		t.Fatalf("batch output should only wrap compact workflow views:\n%s", gotBatch)
	}
}

// compactStatusFixture 创建真实仓库、run state、DAG node 和 subagent artifact，模拟正在执行阶段的业务状态。
func compactStatusFixture(t *testing.T) (string, State) {
	t.Helper()
	repo := gitRepo(t)
	changeName := "7-统一输出"
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
	workflow.MaxReviewIterations = 1
	workflow.Parallel.Enabled = true
	workflow.Parallel.Groups["implementation_context"] = ParallelGroupConfig{
		Mode: "advisory",
		Members: []ParallelMemberConfig{
			{Name: "代码库侦察员", Purpose: "搜索现有模块", Tool: "pi", Subagent: "explore"},
			{Name: "外部资料研究员", Purpose: "查询外部资料", Tool: "pi", Subagent: "librarian"},
		},
	}

	runID := "20260609T070000.000000000Z"
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
			sessionStateKey("codex", "reviewer"): "reviewer-session",
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
	return repo, state
}

// inRepo 在临时仓库目录中执行命令解析，覆盖 GitRoot(".") 的真实路径。
func inRepo(t *testing.T, repo string, fn func()) {
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

// hasExactLine 判断输出中是否存在完全相等的一行，避免子串误判。
func hasExactLine(output, want string) bool {
	for _, line := range strings.Split(output, "\n") {
		if line == want {
			return true
		}
	}
	return false
}

// saveCompactResult 保存本测试的中间输出，作为 acceptance runtime log 的补充材料。
func saveCompactResult(t *testing.T, name, body string) {
	t.Helper()
	dir := filepath.Join("..", "test-results", "7-status-watch-compact-output")
	_ = os.MkdirAll(dir, 0o755)
	_ = os.WriteFile(filepath.Join(dir, name), []byte(body+"\n"), 0o644)
}
GO

note "运行 compact human 输出契约测试"
go test ./internal/app -run TestStatusWatchCompactOutputContract -count=1 2>&1 | tee -a "$log"

if ! command -v script >/dev/null 2>&1; then
  echo "缺少 script 命令，无法运行伪 TTY watch 合同测试" | tee -a "$log" >&2
  exit 1
fi

tmpdir="$(mktemp -d)"

export XDG_STATE_HOME="$tmpdir/state"
tty_repo="$tmpdir/repo"
wo_bin="$tmpdir/wo"
raw_capture="$result_dir/watch-tty.raw"
screen_capture="$result_dir/watch-tty-screen.txt"
long_change="9-这是一个非常非常长的中文提案名称用于触发窄终端自动换行并验证watch不会残留旧首行"

note "构建真实 wo 二进制并创建窄 TTY watch 场景"
go build -C "$repo_root" -o "$wo_bin" ./cmd/wo 2>&1 | tee -a "$log"
mkdir -p "$tty_repo"
cd "$tty_repo"
git init >/dev/null
git config user.email test@example.com
git config user.name Test
printf 'test\n' > README.md
git add .
git commit -m init >/dev/null
mkdir -p "docs/changes/$long_change/tests"

python3 - "$tty_repo" "$long_change" <<'PY'
import hashlib
import json
import os
import pathlib
import re
import sys

repo = pathlib.Path(sys.argv[1]).resolve()
change = sys.argv[2]
name = re.sub(r"[^a-z0-9]+", "-", repo.name.lower()).strip("-") or "repo"
digest = hashlib.sha1(str(repo).encode()).hexdigest()[:10]
base = pathlib.Path(os.environ["XDG_STATE_HOME"]) / "wo" / "repos" / f"{name}-{digest}"
run_id = "20260610T020000.000000000Z"
batch_id = "20260610T020001.000000000Z"

def write_json(path, payload):
    # write_json 写入真实 wo runtime state 文件。
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(payload, ensure_ascii=False), encoding="utf-8")

workflow = {
    "engine": "go-dag",
    "max_review_iterations": 1,
    "stages": {},
    "prompts": {},
    "parallel": {"enabled": False, "groups": {}},
    "validation": {"commands": []},
}
write_json(base / "runs" / run_id / "state.json", {
    "run_id": run_id,
    "change_name": change,
    "sealed": True,
    "status": "running",
    "stage": "execution",
    "engine": "go-dag",
    "sessions": {"codex:executor": "writer-session"},
    "stages": {"planning": "completed"},
    "paths": {},
    "workflow_config": workflow,
})
write_json(base / "batches" / batch_id / "state.json", {
    "batch_id": batch_id,
    "status": "running",
    "changes": [change],
    "current_index": 0,
    "run_ids": {change: run_id},
})
PY

note "用 script 分配窄伪 TTY，捕获多个 watch 刷新帧"
COLUMNS=24 timeout -s INT 3s script -q -c "$wo_bin watch" /dev/null >"$raw_capture" 2>/dev/null || true

note "解析终端控制序列，还原最终屏幕"
python3 - "$raw_capture" "$screen_capture" "$long_change" <<'PY' 2>&1 | tee -a "$log"
import re
import sys
import unicodedata

raw_path, screen_path, long_change = sys.argv[1:4]
width = 24
data = open(raw_path, "rb").read().decode("utf-8", errors="ignore")
rows = [[]]
row = 0
col = 0
i = 0

def cell_width(ch):
    # cell_width 近似真实终端宽字符宽度，覆盖中文提案名换行情形。
    if unicodedata.combining(ch):
        return 0
    return 2 if unicodedata.east_asian_width(ch) in {"W", "F"} else 1

def ensure_row(n):
    # ensure_row 扩展屏幕缓冲，便于模拟 ANSI 清屏。
    while len(rows) <= n:
        rows.append([])

def put_char(ch):
    # put_char 按终端列宽写入字符并处理自动换行。
    global row, col
    w = cell_width(ch)
    if col + w > width:
        row += 1
        col = 0
    ensure_row(row)
    line = rows[row]
    while len(line) < col:
        line.append(" ")
    if col == len(line):
        line.append(ch)
    else:
        line[col] = ch
    col += w
    if col >= width:
        row += 1
        col = 0
        ensure_row(row)

while i < len(data):
    ch = data[i]
    if ch == "\x1b":
        match = re.match(r"\x1b\[([0-9;?]*)([A-Za-z])", data[i:])
        if match:
            params, code = match.groups()
            if code == "A":
                row = max(0, row - int(params or "1"))
            elif code == "H":
                row = 0
                col = 0
            elif code == "J":
                mode = params or "0"
                if mode in {"0", "2"}:
                    ensure_row(row)
                    rows[row:] = [[]]
                    if mode == "2":
                        row = 0
                        col = 0
            i += len(match.group(0))
            continue
    if ch == "\r":
        col = 0
    elif ch == "\n":
        row += 1
        col = 0
        ensure_row(row)
    elif ch >= " ":
        put_char(ch)
    i += 1

visible = ["".join(line).rstrip() for line in rows]
business = [
    line.strip()
    for line in visible
    if line.strip()
    and not line.startswith("Script started")
    and not line.startswith("Script done")
]
open(screen_path, "w", encoding="utf-8").write("\n".join(business) + "\n")
print("\n".join(business))
joined = "".join(business).replace(" ", "")
if not joined.startswith("-" + long_change):
    raise SystemExit(f"第一条业务内容应是提案列表，实际屏幕为 {joined!r}")
if re.search(r"([|/\\\\-]?b1|[|/\\\\-]?w1)", joined):
    raise SystemExit(f"最终屏幕仍残留运行 header: {joined!r}")
if not re.search(r"执行阶段writer-session[|/\\\\-]", joined):
    raise SystemExit("最终屏幕缺少执行阶段 spinner marker")
PY
