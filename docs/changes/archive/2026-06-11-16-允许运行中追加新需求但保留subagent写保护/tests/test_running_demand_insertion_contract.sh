#!/usr/bin/env bash
# 文件功能目的：用临时 Go 契约测试验证运行中新增非当前需求不应中止当前 run，同时源码和当前 change 修改仍受保护。
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../../../.." && pwd)"
LOG="$ROOT/test-results/16-running-demand-insertion.log"
TMP="$(mktemp -d)"
TEMP_TEST="$TMP/internal/app/running_demand_insertion_contract_test.go"

note() {
  # 函数目的：记录契约测试步骤和 go test 输出。
  printf '[running-demand] %s\n' "$*" | tee -a "$LOG"
}

cleanup() {
  # 函数目的：删除临时 Go 测试包，避免契约测试污染工作区。
  rm -rf "$TMP"
}

mkdir -p "$(dirname "$LOG")"
: >"$LOG"
trap cleanup EXIT

note "构造临时 Go 测试包，复用真实 internal/app 源码和 docs/changes/archive/2026-06-11-16-允许运行中追加新需求但保留subagent写保护/tests/app 迁移测试"
cp "$ROOT/go.mod" "$TMP/go.mod"
if [[ -f "$ROOT/go.sum" ]]; then
  cp "$ROOT/go.sum" "$TMP/go.sum"
fi
mkdir -p "$TMP/internal" "$TMP/cmd" "$TMP/docs" "$TMP/profiles-template" "$TMP/prompts-template" "$TMP/skills"
cp -R "$ROOT/internal"/* "$TMP/internal/"
rm -f "$TMP/internal/app/agy_test.go"
cp -R "$ROOT/cmd"/* "$TMP/cmd/" 2>/dev/null || true
cp -R "$ROOT/docs"/* "$TMP/docs/" 2>/dev/null || true
cp -R "$ROOT/profiles-template"/* "$TMP/profiles-template/" 2>/dev/null || true
cp -R "$ROOT/prompts-template"/* "$TMP/prompts-template/" 2>/dev/null || true
cp -R "$ROOT/skills"/* "$TMP/skills/" 2>/dev/null || true
for gotest in "$ROOT"/tests/app/*.gotest; do
  base="$(basename "$gotest" .gotest)"
  if [[ "$base" == "agy_test" || "$base" == "pi_test" ]]; then
    note "跳过 $base，避免与 platform_test 的命令行 helper 重名影响本契约"
    continue
  fi
  cp "$gotest" "$TMP/internal/app/${base}_test.go"
done

cat >"$TEMP_TEST" <<'GO'
// 文件功能目的：验证运行中新增非当前 change 不会触发当前 run 的人工干预中止。
package app

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestRunningDemandInsertionDoesNotAbortCurrentRun proves a new unrelated change is user-owned context, not subagent damage.
func TestRunningDemandInsertionDoesNotAbortCurrentRun(t *testing.T) {
	repo := gitRepo(t)
	mustChange(t, repo, "10-当前需求")
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-m", "current change baseline")
	head, diff, err := gitSnapshot(repo)
	if err != nil {
		t.Fatal(err)
	}
	state := State{
		RunID:        "run-demand-insertion",
		ChangeName:   "10-当前需求",
		Sealed:       true,
		Status:       statusRunning,
		Stage:        "execution",
		BaselineHead: head,
		BaselineDiff: diff,
		Sessions:     map[string]string{},
		Stages:       map[string]string{},
		Workflow:     DefaultWorkflowConfig(),
	}

	mustWritePrompt(t, filepath.Join(repo, "docs", "changes", "11-运行中新需求", "brief.md"), "# 新需求\n")
	engine := NewEngine(repo, testRegistry(fakeRunner{}))
	if err := engine.detectManualIntervention(&state); err != nil {
		t.Fatalf("新增非当前 change 不应中止当前 run: %v", err)
	}
	if state.Status == statusAborted {
		t.Fatal("新增非当前 change 被错误写成 aborted_manual_intervention")
	}
	if !strings.Contains(state.BaselineDiff, "11-运行中新需求") {
		t.Fatalf("baseline 未吸收新增需求 diff:\n%s", state.BaselineDiff)
	}
}

// TestExistingProtectedBaselineDiffDoesNotAbortUnrelatedDemand proves only post-baseline path deltas are guarded.
func TestExistingProtectedBaselineDiffDoesNotAbortUnrelatedDemand(t *testing.T) {
	repo := gitRepo(t)
	mustChange(t, repo, "10-当前需求")
	mustWritePrompt(t, filepath.Join(repo, "internal", "app", "existing.go"), "package app\n")
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-m", "current change baseline")
	mustWritePrompt(t, filepath.Join(repo, "docs", "changes", "10-当前需求", "task.md"), "- [x] execution edit\n")
	mustWritePrompt(t, filepath.Join(repo, "internal", "app", "existing.go"), "package app\n\nconst executionEdit = true\n")
	head, diff, err := gitSnapshot(repo)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(diff, "10-当前需求") || !strings.Contains(diff, "existing.go") {
		t.Fatalf("baseline should include protected paths:\n%s", diff)
	}
	state := State{
		RunID:        "run-existing-protected-baseline",
		ChangeName:   "10-当前需求",
		Sealed:       true,
		Status:       statusRunning,
		Stage:        "review_1",
		BaselineHead: head,
		BaselineDiff: diff,
		Sessions:     map[string]string{},
		Stages:       map[string]string{"execution": "completed"},
		Workflow:     DefaultWorkflowConfig(),
	}

	mustWritePrompt(t, filepath.Join(repo, "docs", "changes", "11-运行中新需求", "brief.md"), "# 新需求\n")
	engine := NewEngine(repo, testRegistry(fakeRunner{}))
	if err := engine.detectManualIntervention(&state); err != nil {
		t.Fatalf("新增非当前 change 应只按 baseline delta 判断: %v", err)
	}
	if !strings.Contains(state.BaselineDiff, "11-运行中新需求") {
		t.Fatalf("baseline 未吸收新增需求 diff:\n%s", state.BaselineDiff)
	}
}

// TestCurrentRunAndSourceChangesStillAbort proves the narrowed guard still catches subagent-style repository writes.
func TestCurrentRunAndSourceChangesStillAbort(t *testing.T) {
	cases := []struct {
		name  string
		write func(t *testing.T, repo string)
	}{
		{
			name: "source",
			write: func(t *testing.T, repo string) {
				mustWritePrompt(t, filepath.Join(repo, "internal", "app", "rogue_write.go"), "package app\n")
			},
		},
		{
			name: "current-change",
			write: func(t *testing.T, repo string) {
				mustWritePrompt(t, filepath.Join(repo, "docs", "changes", "10-当前需求", "spec.md"), "# 被错误改写\n")
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := gitRepo(t)
			mustChange(t, repo, "10-当前需求")
			runGit(t, repo, "add", ".")
			runGit(t, repo, "commit", "-m", "current change baseline")
			head, diff, err := gitSnapshot(repo)
			if err != nil {
				t.Fatal(err)
			}
			state := State{
				RunID:        "run-guard-" + tc.name,
				ChangeName:   "10-当前需求",
				Sealed:       true,
				Status:       statusRunning,
				Stage:        "execution",
				BaselineHead: head,
				BaselineDiff: diff,
				Sessions:     map[string]string{},
				Stages:       map[string]string{},
				Workflow:     DefaultWorkflowConfig(),
			}
			if err := saveState(repo, state); err != nil {
				t.Fatal(err)
			}
			tc.write(t, repo)
			engine := NewEngine(repo, testRegistry(fakeRunner{}))
			if err := engine.detectManualIntervention(&state); err == nil {
				t.Fatal("当前 run 相关路径或源码变化必须继续被阻断")
			}
			final, err := loadState(repo, state.RunID)
			if err != nil {
				t.Fatal(err)
			}
			if final.Status != statusAborted {
				t.Fatalf("status = %q, want %q", final.Status, statusAborted)
			}
		})
	}
}

// TestCurrentChangeRenameIntoUnrelatedDemandStillAborts proves rename parsing includes the protected old path.
func TestCurrentChangeRenameIntoUnrelatedDemandStillAborts(t *testing.T) {
	cases := []struct {
		name  string
		write func(t *testing.T, repo string)
	}{
		{
			name: "staged",
			write: func(t *testing.T, repo string) {
				runGit(t, repo, "mv", "docs/changes/10-当前需求/spec.md", "docs/changes/11-运行中新需求/stolen.md")
			},
		},
		{
			name: "committed",
			write: func(t *testing.T, repo string) {
				runGit(t, repo, "mv", "docs/changes/10-当前需求/spec.md", "docs/changes/11-运行中新需求/stolen.md")
				runGit(t, repo, "commit", "-m", "move current change into unrelated demand")
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := gitRepo(t)
			mustChange(t, repo, "10-当前需求")
			mustWritePrompt(t, filepath.Join(repo, "docs", "changes", "10-当前需求", "spec.md"), "# 当前规格\n")
			mustChange(t, repo, "11-运行中新需求")
			runGit(t, repo, "add", ".")
			runGit(t, repo, "commit", "-m", "current change baseline")
			head, diff, err := gitSnapshot(repo)
			if err != nil {
				t.Fatal(err)
			}
			state := State{
				RunID:        "run-rename-" + tc.name,
				ChangeName:   "10-当前需求",
				Sealed:       true,
				Status:       statusRunning,
				Stage:        "execution",
				BaselineHead: head,
				BaselineDiff: diff,
				Sessions:     map[string]string{},
				Stages:       map[string]string{},
				Workflow:     DefaultWorkflowConfig(),
			}
			if err := saveState(repo, state); err != nil {
				t.Fatal(err)
			}
			tc.write(t, repo)
			engine := NewEngine(repo, testRegistry(fakeRunner{}))
			if err := engine.detectManualIntervention(&state); err == nil {
				t.Fatal("当前 change 文件移动到非当前 change 仍必须阻断")
			}
			final, err := loadState(repo, state.RunID)
			if err != nil {
				t.Fatal(err)
			}
			if final.Status != statusAborted {
				t.Fatalf("status = %q, want %q", final.Status, statusAborted)
			}
		})
	}
}
GO

note "运行临时 Go 契约测试"
set +e
(cd "$TMP" && go test ./internal/app -run 'TestRunningDemandInsertionDoesNotAbortCurrentRun|TestExistingProtectedBaselineDiffDoesNotAbortUnrelatedDemand|TestCurrentRunAndSourceChangesStillAbort|TestCurrentChangeRenameIntoUnrelatedDemandStillAborts' -count=1) >"$LOG.tmp" 2>&1
code=$?
set -e
cat "$LOG.tmp" | tee -a "$LOG"
rm -f "$LOG.tmp"
exit "$code"
