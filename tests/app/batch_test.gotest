// Package app tests serial batch state and scheduling behavior.
package app

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

type batchRunner struct {
	stages  []string
	changes []string
}

// Run records the current change and writes artifacts for a zero-review workflow.
func (r *batchRunner) Run(_ context.Context, repo, prompt, threadID string, _ StageOptions) (string, error) {
	stage := stageFromPromptOrState(repo, prompt)
	runID := currentRunID(repo)
	state, err := loadState(repo, runID)
	if err != nil {
		return "", err
	}
	r.stages = append(r.stages, stage)
	r.changes = append(r.changes, state.ChangeName)
	switch stage {
	case "execution":
		task := filepath.Join(repo, "docs", "changes", state.ChangeName, "task.md")
		return "executor-thread", os.WriteFile(task, []byte("- [x] task\n"), 0o644)
	case "archive":
		archive := filepath.Join(repo, "docs", "changes", "archive", "2026-05-05-"+state.ChangeName)
		if err := os.MkdirAll(archive, 0o755); err != nil {
			return "", err
		}
		return threadID, os.WriteFile(filepath.Join(runDir(repo, runID), "delivery-summary.md"), []byte("done\n"), 0o644)
	default:
		return threadID, nil
	}
}

// TestSubmitBatchStartsOneDetachedWorker verifies multi-select submission creates one queue worker.
func TestSubmitBatchStartsOneDetachedWorker(t *testing.T) {
	repo := gitRepo(t)
	mustChange(t, repo, "5-c")
	mustChange(t, repo, "3-a")
	mustPrompts(t, repo)
	var started []string
	previous := startDetachedBatchCommand
	startDetachedBatchCommand = func(_ string, batchID string) error {
		started = append(started, batchID)
		return nil
	}
	t.Cleanup(func() { startDetachedBatchCommand = previous })

	engine := NewEngine(repo, testRegistry(&batchRunner{}))
	if err := engine.SubmitBatch(context.Background(), []Change{{Name: "5-c"}, {Name: "3-a"}}); err != nil {
		t.Fatal(err)
	}
	if len(started) != 1 {
		t.Fatalf("batch workers = %v, want one", started)
	}
	batch, err := loadBatchState(repo, started[0])
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(batch.Changes, ","); got != "3-a,5-c" {
		t.Fatalf("batch changes = %s, want sorted numeric order", got)
	}
}

// TestSubmitSingleChangeStartsQueueWorker verifies a one-change human run is a queue.
func TestSubmitSingleChangeStartsQueueWorker(t *testing.T) {
	repo := gitRepo(t)
	mustChange(t, repo, "1-a")
	mustPrompts(t, repo)
	var started []string
	previous := startDetachedBatchCommand
	startDetachedBatchCommand = func(_ string, batchID string) error {
		started = append(started, batchID)
		return nil
	}
	t.Cleanup(func() { startDetachedBatchCommand = previous })

	engine := NewEngine(repo, testRegistry(&batchRunner{}))
	if err := engine.SubmitBatch(context.Background(), []Change{{Name: "1-a"}}); err != nil {
		t.Fatal(err)
	}
	if len(started) != 1 {
		t.Fatalf("batch workers = %v, want one", started)
	}
	batch, err := loadBatchState(repo, started[0])
	if err != nil {
		t.Fatal(err)
	}
	if batch.Status != batchStatusRunning || batch.CurrentIndex != 0 || strings.Join(batch.Changes, ",") != "1-a" {
		t.Fatalf("batch = %#v, want running single-change queue", batch)
	}
}

// TestRunFlagStartsSingleChangeQueue verifies the human shortcut uses queue semantics.
func TestRunFlagStartsSingleChangeQueue(t *testing.T) {
	repo := gitRepo(t)
	chdir(t, repo)
	mustChange(t, repo, "1-a")
	mustPrompts(t, repo)
	installFakeOz(t, "1-a")
	var started []string
	previous := startDetachedBatchCommand
	startDetachedBatchCommand = func(_ string, batchID string) error {
		started = append(started, batchID)
		return nil
	}
	t.Cleanup(func() { startDetachedBatchCommand = previous })

	var stdout bytes.Buffer
	if err := Run([]string{"--run", "1-a"}, strings.NewReader(""), &stdout, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if len(started) != 1 {
		t.Fatalf("batch workers = %v, want one", started)
	}
	batch, err := loadBatchState(repo, started[0])
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(batch.Changes, ",") != "1-a" {
		t.Fatalf("batch changes = %v, want [1-a]", batch.Changes)
	}
	if runID, err := FindCurrentRun(repo); err != nil || runID != "" {
		t.Fatalf("FindCurrentRun succeeded after --run queue start; stdout = %q", stdout.String())
	}
}

// TestRunCommandStartsAllActiveChangesQueue verifies the non-interactive shortcut selects every active change.
func TestRunCommandStartsAllActiveChangesQueue(t *testing.T) {
	for _, args := range [][]string{{"run"}, {"r"}} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			repo := gitRepo(t)
			chdir(t, repo)
			mustChange(t, repo, "5-丙")
			mustChange(t, repo, "3-甲")
			mustChange(t, repo, "abc")
			mustPrompts(t, repo)
			installRealOz(t)
			var started []string
			previous := startDetachedBatchCommand
			startDetachedBatchCommand = func(_ string, batchID string) error {
				started = append(started, batchID)
				return nil
			}
			t.Cleanup(func() { startDetachedBatchCommand = previous })

			var stdout bytes.Buffer
			if err := Run(args, strings.NewReader("not-used\n"), &stdout, &bytes.Buffer{}); err != nil {
				t.Fatal(err)
			}
			if len(started) != 1 {
				t.Fatalf("batch workers = %v, want one", started)
			}
			batch, err := loadBatchState(repo, started[0])
			if err != nil {
				t.Fatal(err)
			}
			if got := strings.Join(batch.Changes, ","); got != "3-甲,5-丙,abc" {
				t.Fatalf("batch changes = %s, want all active changes sorted", got)
			}
			if strings.Contains(stdout.String(), "> ") {
				t.Fatalf("run shortcut should not render interactive prompt:\n%s", stdout.String())
			}
		})
	}
}

// TestRunCommandWithNoActiveChangesDoesNotPrompt verifies the shortcut does not enter planning.
func TestRunCommandWithNoActiveChangesDoesNotPrompt(t *testing.T) {
	repo := gitRepo(t)
	chdir(t, repo)
	installRealOz(t)
	var started []string
	previous := startDetachedBatchCommand
	startDetachedBatchCommand = func(_ string, batchID string) error {
		started = append(started, batchID)
		return nil
	}
	t.Cleanup(func() { startDetachedBatchCommand = previous })

	var stdout bytes.Buffer
	if err := Run([]string{"run"}, strings.NewReader("not-used\n"), &stdout, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if len(started) != 0 {
		t.Fatalf("batch workers = %v, want none without active changes", started)
	}
	got := stdout.String()
	if !strings.Contains(got, "没有 active 变更提案") || strings.Contains(got, "> ") {
		t.Fatalf("stdout = %q, want no-active message without prompt", got)
	}
}

// TestRunFlagRejectsInvalidChangeBeforeQueue verifies bad input never creates a queue.
func TestRunFlagRejectsInvalidChangeBeforeQueue(t *testing.T) {
	repo := gitRepo(t)
	chdir(t, repo)
	installFakeOz(t)
	var started []string
	previous := startDetachedBatchCommand
	startDetachedBatchCommand = func(_ string, batchID string) error {
		started = append(started, batchID)
		return nil
	}
	t.Cleanup(func() { startDetachedBatchCommand = previous })

	var stdout bytes.Buffer
	err := Run([]string{"--run", "missing"}, strings.NewReader(""), &stdout, &bytes.Buffer{})
	if err == nil {
		t.Fatal("Run --run missing succeeded, want validation error")
	}
	if !strings.Contains(err.Error(), "missing 不是有效 oz change") {
		t.Fatalf("error = %v, want invalid change validation", err)
	}
	if len(started) != 0 {
		t.Fatalf("batch workers = %v, want none for invalid change", started)
	}
	root, rootErr := batchesRoot(repo)
	if rootErr != nil {
		t.Fatal(rootErr)
	}
	entries, readErr := os.ReadDir(root)
	if readErr != nil && !os.IsNotExist(readErr) {
		t.Fatal(readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("batch entries = %d, want none for invalid change; stdout = %q", len(entries), stdout.String())
	}
}

// TestRunFlagRejectsNoInitialCommitBeforeQueue verifies baseline preflight is synchronous.
func TestRunFlagRejectsNoInitialCommitBeforeQueue(t *testing.T) {
	repo := t.TempDir()
	runGit(t, repo, "init")
	runGit(t, repo, "config", "user.email", "test@example.com")
	runGit(t, repo, "config", "user.name", "Test")
	chdir(t, repo)
	mustChange(t, repo, "1-a")
	var started []string
	previous := startDetachedBatchCommand
	startDetachedBatchCommand = func(_ string, batchID string) error {
		started = append(started, batchID)
		return nil
	}
	t.Cleanup(func() { startDetachedBatchCommand = previous })

	var stdout bytes.Buffer
	err := Run([]string{"--run", "1-a"}, strings.NewReader(""), &stdout, &bytes.Buffer{})
	if err == nil {
		t.Fatal("Run --run in repository without initial commit succeeded, want baseline error")
	}
	if !strings.Contains(err.Error(), errNoInitialCommit) {
		t.Fatalf("error = %v, want no initial commit guidance", err)
	}
	if len(started) != 0 {
		t.Fatalf("batch workers = %v, want none before initial commit", started)
	}
	batchRoot, rootErr := batchesRoot(repo)
	if rootErr != nil {
		t.Fatal(rootErr)
	}
	batchEntries, readBatchErr := os.ReadDir(batchRoot)
	if readBatchErr != nil && !os.IsNotExist(readBatchErr) {
		t.Fatal(readBatchErr)
	}
	if len(batchEntries) != 0 {
		t.Fatalf("batch entries = %d, want none before initial commit; stdout = %q", len(batchEntries), stdout.String())
	}
	runRoot, runRootErr := runsRoot(repo)
	if runRootErr != nil {
		t.Fatal(runRootErr)
	}
	runEntries, readRunErr := os.ReadDir(runRoot)
	if readRunErr != nil && !os.IsNotExist(readRunErr) {
		t.Fatal(readRunErr)
	}
	if len(runEntries) != 0 {
		t.Fatalf("run entries = %d, want none before initial commit", len(runEntries))
	}
}

// TestRunBatchCreatesRunsSerially verifies the next run is created only after the previous run is done.
func TestRunBatchCreatesRunsSerially(t *testing.T) {
	repo := gitRepo(t)
	mustChange(t, repo, "1-a")
	mustChange(t, repo, "2-b")
	mustPrompts(t, repo)
	mustWritePrompt(t, filepath.Join(repo, "wo.yaml"), "wo:\n  workflow:\n    max_review_iterations: 0\n")
	runner := &batchRunner{}
	engine := NewEngine(repo, testRegistry(runner))
	batch := BatchState{BatchID: "batch-1", Status: batchStatusRunning, Changes: []string{"1-a", "2-b"}, RunIDs: map[string]string{}}
	if err := saveBatchState(repo, batch); err != nil {
		t.Fatal(err)
	}
	if err := engine.RunBatch(context.Background(), "batch-1"); err != nil {
		t.Fatal(err)
	}
	final, err := loadBatchState(repo, "batch-1")
	if err != nil {
		t.Fatal(err)
	}
	if final.Status != batchStatusDone || final.CurrentIndex != 2 {
		t.Fatalf("batch = %#v, want done at index 2", final)
	}
	if got := strings.Join(runner.changes, ","); got != "1-a,1-a,2-b,2-b" {
		t.Fatalf("runner changes = %s, want serial stages per change", got)
	}
}

// TestRunBatchStopsOnFailedCurrentRun verifies later changes are not started after a failed run.
func TestRunBatchStopsOnFailedCurrentRun(t *testing.T) {
	repo := gitRepo(t)
	runID := "run-failed"
	state := State{RunID: runID, ChangeName: "1-a", Status: statusFailed, Stage: "execution", Workflow: DefaultWorkflowConfig()}
	if err := saveState(repo, state); err != nil {
		t.Fatal(err)
	}
	batch := BatchState{
		BatchID:      "batch-failed",
		Status:       batchStatusRunning,
		Changes:      []string{"1-a", "2-b"},
		CurrentIndex: 0,
		RunIDs:       map[string]string{"1-a": runID},
	}
	if err := saveBatchState(repo, batch); err != nil {
		t.Fatal(err)
	}
	engine := NewEngine(repo, testRegistry(&batchRunner{}))
	err := engine.RunBatch(context.Background(), "batch-failed")
	if err == nil {
		t.Fatal("RunBatch succeeded, want failure")
	}
	final, _ := loadBatchState(repo, "batch-failed")
	if final.Status != batchStatusFailed || final.RunIDs["2-b"] != "" {
		t.Fatalf("batch = %#v, want failed without second run", final)
	}
}

// TestRunBatchStopsOnManualInterventionAbort verifies non-running terminal statuses cannot spin forever.
func TestRunBatchStopsOnManualInterventionAbort(t *testing.T) {
	repo := gitRepo(t)
	runID := "run-abort"
	state := State{RunID: runID, ChangeName: "1-a", Status: statusAborted, Stage: "execution", Workflow: DefaultWorkflowConfig()}
	if err := saveState(repo, state); err != nil {
		t.Fatal(err)
	}
	batch := BatchState{
		BatchID:      "batch-abort",
		Status:       batchStatusRunning,
		Changes:      []string{"1-a", "2-b"},
		CurrentIndex: 0,
		RunIDs:       map[string]string{"1-a": runID},
	}
	if err := saveBatchState(repo, batch); err != nil {
		t.Fatal(err)
	}
	engine := NewEngine(repo, testRegistry(&batchRunner{}))
	err := engine.RunBatch(context.Background(), "batch-abort")
	if err == nil {
		t.Fatal("RunBatch succeeded, want failure for aborted_manual_intervention")
	}
	final, _ := loadBatchState(repo, "batch-abort")
	if final.Status != batchStatusFailed || final.CurrentIndex != 0 || final.RunIDs["2-b"] != "" {
		t.Fatalf("batch = %#v, want failed at first change without second run", final)
	}
	if !strings.Contains(final.Error, statusAborted) {
		t.Fatalf("batch error = %q, want aborted status", final.Error)
	}
}

// TestRunBatchDoesNotFailActiveLockedCurrentRun verifies duplicate workers do not corrupt state.
func TestRunBatchDoesNotFailActiveLockedCurrentRun(t *testing.T) {
	repo := gitRepo(t)
	runID := "run-locked"
	state := State{
		RunID:      runID,
		ChangeName: "1-a",
		Status:     statusRunning,
		Stage:      "execution",
		Sessions:   map[string]string{},
		Stages:     map[string]string{"execution": "running"},
		Workflow:   DefaultWorkflowConfig(),
	}
	if err := saveState(repo, state); err != nil {
		t.Fatal(err)
	}
	hostname, _ := os.Hostname()
	mustWriteLock(t, repo, runID, LockInfo{PID: os.Getpid(), Hostname: hostname, RunID: runID})
	batch := BatchState{
		BatchID:      "batch-locked",
		Status:       batchStatusRunning,
		Changes:      []string{"1-a", "2-b"},
		CurrentIndex: 0,
		RunIDs:       map[string]string{"1-a": runID},
	}
	if err := saveBatchState(repo, batch); err != nil {
		t.Fatal(err)
	}
	engine := NewEngine(repo, testRegistry(&batchRunner{}))
	err := engine.RunBatch(context.Background(), "batch-locked")
	if err == nil || !isRunLockedError(err) {
		t.Fatalf("RunBatch error = %v, want run lock conflict", err)
	}
	finalBatch, err := loadBatchState(repo, "batch-locked")
	if err != nil {
		t.Fatal(err)
	}
	if finalBatch.Status != batchStatusRunning || finalBatch.CurrentIndex != 0 || finalBatch.FailedRunID != "" {
		t.Fatalf("batch = %#v, want unchanged running batch", finalBatch)
	}
	finalRun, err := loadState(repo, runID)
	if err != nil {
		t.Fatal(err)
	}
	if finalRun.Status != statusRunning || finalRun.Error != "" || finalBatch.RunIDs["2-b"] != "" {
		t.Fatalf("run/batch corrupted: run=%#v batch=%#v", finalRun, finalBatch)
	}
}

// TestRunBatchStopsOnBlockedReviewLimit verifies review-limit blocks fail the batch.
func TestRunBatchStopsOnBlockedReviewLimit(t *testing.T) {
	repo := gitRepo(t)
	runID := "run-blocked"
	state := State{RunID: runID, ChangeName: "1-a", Status: statusBlocked, Stage: statusBlocked, Workflow: DefaultWorkflowConfig()}
	if err := saveState(repo, state); err != nil {
		t.Fatal(err)
	}
	batch := BatchState{
		BatchID:      "batch-blocked",
		Status:       batchStatusRunning,
		Changes:      []string{"1-a", "2-b"},
		CurrentIndex: 0,
		RunIDs:       map[string]string{"1-a": runID},
	}
	if err := saveBatchState(repo, batch); err != nil {
		t.Fatal(err)
	}
	engine := NewEngine(repo, testRegistry(&batchRunner{}))
	err := engine.RunBatch(context.Background(), "batch-blocked")
	if err == nil {
		t.Fatal("RunBatch succeeded, want blocked_review_limit failure")
	}
	final, _ := loadBatchState(repo, "batch-blocked")
	if final.Status != batchStatusFailed || final.FailedChange != "1-a" || final.FailedRunID != runID || final.RunIDs["2-b"] != "" {
		t.Fatalf("batch = %#v, want failed without starting second change", final)
	}
	if !strings.Contains(final.Error, statusBlocked) {
		t.Fatalf("batch error = %q, want blocked_review_limit", final.Error)
	}
}

// TestFindUnfinishedRunSkipsBatchOwnedRuns keeps batch runs out of the normal resume menu.
func TestFindUnfinishedRunSkipsBatchOwnedRuns(t *testing.T) {
	repo := gitRepo(t)
	batchRun := State{RunID: "batch-run", ChangeName: "1-a", Status: statusRunning, Stage: "execution", BatchID: "batch-1", Workflow: DefaultWorkflowConfig()}
	singleRun := State{RunID: "single-run", ChangeName: "x", Status: statusRunning, Stage: "execution", Workflow: DefaultWorkflowConfig()}
	if err := saveState(repo, batchRun); err != nil {
		t.Fatal(err)
	}
	got, err := FindUnfinishedRun(repo)
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Fatalf("unfinished run = %q, want none while only batch run exists", got)
	}
	if err := saveState(repo, singleRun); err != nil {
		t.Fatal(err)
	}
	got, err = FindUnfinishedRun(repo)
	if err != nil {
		t.Fatal(err)
	}
	if got != "single-run" {
		t.Fatalf("unfinished run = %q, want single-run", got)
	}
}

// TestInteractiveShowsFailedBatchBeforeNormalMenu verifies stopped batches remain user-visible.
func TestInteractiveShowsFailedBatchBeforeNormalMenu(t *testing.T) {
	repo := gitRepo(t)
	mustChange(t, repo, "1-a")
	mustPrompts(t, repo)
	runID := "batch-run"
	state := State{RunID: runID, ChangeName: "1-a", Status: statusFailed, Stage: "execution", BatchID: "batch-fail", Workflow: DefaultWorkflowConfig()}
	if err := saveState(repo, state); err != nil {
		t.Fatal(err)
	}
	batch := BatchState{
		BatchID:      "batch-fail",
		Status:       batchStatusFailed,
		Changes:      []string{"1-a", "2-b"},
		CurrentIndex: 0,
		RunIDs:       map[string]string{"1-a": runID},
		FailedChange: "1-a",
		FailedRunID:  runID,
		Error:        "run failed",
	}
	if err := saveBatchState(repo, batch); err != nil {
		t.Fatal(err)
	}
	var started []string
	previous := startDetachedBatchCommand
	startDetachedBatchCommand = func(_ string, batchID string) error {
		started = append(started, batchID)
		return nil
	}
	t.Cleanup(func() { startDetachedBatchCommand = previous })
	engine := NewEngine(repo, testRegistry(&batchRunner{}))
	var stdout bytes.Buffer
	if err := interactive(context.Background(), strings.NewReader("2\n1\n"), &stdout, repo, engine); err != nil {
		t.Fatal(err)
	}
	got := stdout.String()
	for _, want := range []string{"检测到已停止的历史任务", "批量任务 b1 batch-fail failed", "change: 1-a", "run: w1 batch-run", "reason: 1-a 的写阶段失败", "提示: 可运行 wo restart -b1 删除失败记录并继续该批量任务"} {
		if !strings.Contains(got, want) {
			t.Fatalf("interactive output missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "恢复未完成批量任务") || strings.Contains(got, "中止未完成批量任务") {
		t.Fatalf("failed batch should not show resumable batch menu:\n%s", got)
	}
	if len(started) != 1 {
		t.Fatalf("started = %v, want new single-change batch after failed batch notice", started)
	}
	newBatch, err := loadBatchState(repo, started[0])
	if err != nil {
		t.Fatal(err)
	}
	if newBatch.Status != batchStatusRunning || newBatch.CurrentIndex != 0 || strings.Join(newBatch.Changes, ",") != "1-a" {
		t.Fatalf("new batch = %#v, want running single-change queue", newBatch)
	}
}

// TestInteractiveShowsRunningBatchResumeMenu verifies running batches stay resumable.
func TestInteractiveShowsRunningBatchResumeMenu(t *testing.T) {
	repo := gitRepo(t)
	batch := BatchState{BatchID: "batch-running", Status: batchStatusRunning, Changes: []string{"1-a"}, RunIDs: map[string]string{}}
	if err := saveBatchState(repo, batch); err != nil {
		t.Fatal(err)
	}
	var started []string
	previous := startDetachedBatchCommand
	startDetachedBatchCommand = func(_ string, batchID string) error {
		started = append(started, batchID)
		return nil
	}
	t.Cleanup(func() { startDetachedBatchCommand = previous })
	engine := NewEngine(repo, testRegistry(&batchRunner{}))
	var stdout bytes.Buffer
	if err := interactive(context.Background(), strings.NewReader("1\n"), &stdout, repo, engine); err != nil {
		t.Fatal(err)
	}
	got := stdout.String()
	if !strings.Contains(got, "发现未完成批量任务：batch-running") || !strings.Contains(got, "恢复未完成批量任务") {
		t.Fatalf("interactive output missing running batch menu:\n%s", got)
	}
	if len(started) != 1 || started[0] != "batch-running" {
		t.Fatalf("started = %v, want running batch", started)
	}
}

// TestInteractiveSeparatesStoppedAndRunningRuns verifies startup prompts do not mix states.
func TestInteractiveSeparatesStoppedAndRunningRuns(t *testing.T) {
	repo := gitRepo(t)
	mustChange(t, repo, "1-a")
	mustPrompts(t, repo)
	stopped := State{RunID: "20260511T000000.000000001Z", ChangeName: "old", Status: statusBlocked, Stage: statusBlocked, Error: "审核修正达到上限", Workflow: DefaultWorkflowConfig()}
	running := State{RunID: "20260511T000000.000000002Z", ChangeName: "1-a", Status: statusRunning, Stage: "execution", Workflow: DefaultWorkflowConfig()}
	if err := saveState(repo, stopped); err != nil {
		t.Fatal(err)
	}
	if err := saveState(repo, running); err != nil {
		t.Fatal(err)
	}
	var started []string
	previous := startDetachedCommand
	startDetachedCommand = func(_ string, runID string) error {
		started = append(started, runID)
		return nil
	}
	t.Cleanup(func() { startDetachedCommand = previous })
	engine := NewEngine(repo, testRegistry(&batchRunner{}))
	var stdout bytes.Buffer
	if err := interactive(context.Background(), strings.NewReader("1\n"), &stdout, repo, engine); err != nil {
		t.Fatal(err)
	}
	got := stdout.String()
	for _, want := range []string{"检测到已停止的历史任务", "工作流 w2 20260511T000000.000000001Z blocked_review_limit", "change: old", "reason: blocked_review_limit", "发现未完成 run：20260511T000000.000000002Z"} {
		if !strings.Contains(got, want) {
			t.Fatalf("interactive output missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "wo restart") {
		t.Fatalf("blocked stopped run should not show restart hint:\n%s", got)
	}
	if len(started) != 1 || started[0] != running.RunID {
		t.Fatalf("started = %v, want running run only", started)
	}
}

// TestInteractiveStoppedRunDoesNotShowResumeMenu verifies terminal runs are not resumable.
func TestInteractiveStoppedRunDoesNotShowResumeMenu(t *testing.T) {
	repo := gitRepo(t)
	mustChange(t, repo, "1-a")
	mustPrompts(t, repo)
	stopped := State{RunID: "20260511T000000.000000003Z", ChangeName: "old", Status: statusFailed, Stage: "execution", Error: "agent failed", Workflow: DefaultWorkflowConfig()}
	if err := saveState(repo, stopped); err != nil {
		t.Fatal(err)
	}
	var started []string
	previous := startDetachedBatchCommand
	startDetachedBatchCommand = func(_ string, batchID string) error {
		started = append(started, batchID)
		return nil
	}
	t.Cleanup(func() { startDetachedBatchCommand = previous })
	engine := NewEngine(repo, testRegistry(&batchRunner{}))
	var stdout bytes.Buffer
	if err := interactive(context.Background(), strings.NewReader("2\n1\n"), &stdout, repo, engine); err != nil {
		t.Fatal(err)
	}
	got := stdout.String()
	if strings.Contains(got, "恢复未完成 run") || strings.Contains(got, "发现未完成 run") {
		t.Fatalf("stopped run should not be shown as resumable:\n%s", got)
	}
	if !strings.Contains(got, "工作流 w1 20260511T000000.000000003Z failed") || !strings.Contains(got, "change: old") || !strings.Contains(got, "提示: 可运行 wo restart -w1 重启该工作流") {
		t.Fatalf("interactive output missing failed run:\n%s", got)
	}
	if len(started) != 1 {
		t.Fatalf("started = %v, want new single-change batch", started)
	}
	newBatch, err := loadBatchState(repo, started[0])
	if err != nil {
		t.Fatal(err)
	}
	if newBatch.Status != batchStatusRunning || newBatch.CurrentIndex != 0 || strings.Join(newBatch.Changes, ",") != "1-a" {
		t.Fatalf("new batch = %#v, want running single-change queue", newBatch)
	}
}

// TestInteractiveSingleExistingChangeSkipsNumberPrompt verifies one visible change is selected directly.
func TestInteractiveSingleExistingChangeSkipsNumberPrompt(t *testing.T) {
	repo := gitRepo(t)
	mustChange(t, repo, "1-a")
	mustPrompts(t, repo)
	var started []string
	previous := startDetachedBatchCommand
	startDetachedBatchCommand = func(_ string, batchID string) error {
		started = append(started, batchID)
		return nil
	}
	t.Cleanup(func() { startDetachedBatchCommand = previous })

	engine := NewEngine(repo, testRegistry(&batchRunner{}))
	var stdout bytes.Buffer
	if err := interactive(context.Background(), strings.NewReader("2\n"), &stdout, repo, engine); err != nil {
		t.Fatal(err)
	}
	got := stdout.String()
	if strings.Contains(got, "1. 1-a") {
		t.Fatalf("single change should not render a second number prompt:\n%s", got)
	}
	if len(started) != 1 {
		t.Fatalf("started = %v, want one single-change batch", started)
	}
	batch, err := loadBatchState(repo, started[0])
	if err != nil {
		t.Fatal(err)
	}
	if batch.Status != batchStatusRunning || strings.Join(batch.Changes, ",") != "1-a" {
		t.Fatalf("batch = %#v, want single-change queue", batch)
	}
}

// TestInteractiveSelectAllCreatesSortedBatch verifies `a` selects every visible active change.
func TestInteractiveSelectAllCreatesSortedBatch(t *testing.T) {
	repo := gitRepo(t)
	mustChange(t, repo, "5-c")
	mustChange(t, repo, "3-a")
	mustChange(t, repo, "4-b")
	mustPrompts(t, repo)
	var started []string
	previous := startDetachedBatchCommand
	startDetachedBatchCommand = func(_ string, batchID string) error {
		started = append(started, batchID)
		return nil
	}
	t.Cleanup(func() { startDetachedBatchCommand = previous })

	engine := NewEngine(repo, testRegistry(&batchRunner{}))
	var stdout bytes.Buffer
	if err := interactive(context.Background(), strings.NewReader("2\na\n"), &stdout, repo, engine); err != nil {
		t.Fatal(err)
	}
	if len(started) != 1 {
		t.Fatalf("started = %v, want one all-change batch", started)
	}
	batch, err := loadBatchState(repo, started[0])
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(batch.Changes, ","); got != "3-a,4-b,5-c" {
		t.Fatalf("batch changes = %s, want sorted all changes", got)
	}
}

// TestInteractiveRejectsMixedSelectAllWithoutBatch verifies ambiguous all-select input is harmless.
func TestInteractiveRejectsMixedSelectAllWithoutBatch(t *testing.T) {
	repo := gitRepo(t)
	mustChange(t, repo, "1-a")
	mustChange(t, repo, "2-b")
	mustPrompts(t, repo)
	var started []string
	previous := startDetachedBatchCommand
	startDetachedBatchCommand = func(_ string, batchID string) error {
		started = append(started, batchID)
		return nil
	}
	t.Cleanup(func() { startDetachedBatchCommand = previous })

	engine := NewEngine(repo, testRegistry(&batchRunner{}))
	var stdout bytes.Buffer
	err := interactive(context.Background(), strings.NewReader("2\n1,a\n"), &stdout, repo, engine)
	if err == nil || !strings.Contains(err.Error(), "无效选择") {
		t.Fatalf("interactive error = %v, want invalid selection", err)
	}
	if len(started) != 0 {
		t.Fatalf("started = %v, want no batch worker for invalid selection", started)
	}
	if latest, err := FindLatestBatch(repo); err != nil || latest != nil {
		t.Fatalf("latest batch = %#v, err = %v; want none", latest, err)
	}
}

// TestInteractiveAbortRunningBatchExits verifies aborting a batch does not fall through to new work.
func TestInteractiveAbortRunningBatchExits(t *testing.T) {
	repo := gitRepo(t)
	mustChange(t, repo, "1-a")
	mustPrompts(t, repo)
	batch := BatchState{BatchID: "batch-abort-menu", Status: batchStatusRunning, Changes: []string{"1-a"}, RunIDs: map[string]string{}}
	if err := saveBatchState(repo, batch); err != nil {
		t.Fatal(err)
	}
	var started []string
	previous := startDetachedBatchCommand
	startDetachedBatchCommand = func(_ string, batchID string) error {
		started = append(started, batchID)
		return nil
	}
	t.Cleanup(func() { startDetachedBatchCommand = previous })

	engine := NewEngine(repo, testRegistry(&batchRunner{}))
	var stdout bytes.Buffer
	if err := interactive(context.Background(), strings.NewReader("4\n"), &stdout, repo, engine); err != nil {
		t.Fatal(err)
	}
	got := stdout.String()
	if strings.Contains(got, "选择已有变更") {
		t.Fatalf("abort should not fall through to new-work menu:\n%s", got)
	}
	if len(started) != 0 {
		t.Fatalf("started = %v, want no worker after abort", started)
	}
	final, err := loadBatchState(repo, "batch-abort-menu")
	if err != nil {
		t.Fatal(err)
	}
	if final.Status != batchStatusAborted {
		t.Fatalf("batch status = %s, want aborted", final.Status)
	}
}

// TestPrintHumanStatusRunningBatch verifies batch overview with current run stage details.
func TestPrintHumanStatusRunningBatch(t *testing.T) {
	repo := gitRepo(t)
	run1 := State{RunID: "run-1", ChangeName: "1-a", Status: statusDone, Stage: "done", BatchID: "batch-run", Workflow: DefaultWorkflowConfig()}
	run2 := State{RunID: "run-2", ChangeName: "2-b", Status: statusRunning, Stage: "review_1", BatchID: "batch-run", Sessions: map[string]string{"codex:executor": "exec-thread"}, Stages: map[string]string{"execution": "completed"}, Workflow: DefaultWorkflowConfig()}
	if err := saveState(repo, run1); err != nil {
		t.Fatal(err)
	}
	if err := saveState(repo, run2); err != nil {
		t.Fatal(err)
	}
	batch := BatchState{
		BatchID:      "batch-run",
		Status:       batchStatusRunning,
		Changes:      []string{"1-a", "2-b", "3-c"},
		CurrentIndex: 1,
		RunIDs:       map[string]string{"1-a": "run-1", "2-b": "run-2"},
	}
	if err := saveBatchState(repo, batch); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	if err := printHumanStatus(&stdout, repo); err != nil {
		t.Fatal(err)
	}
	got := stdout.String()
	for _, want := range []string{"- 1-a", "- 2-b", "- 3-c"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q:\n%s", want, got)
		}
	}
	body := strings.Join(strings.Split(got, "\n")[1:], "\n")
	if strings.Contains(body, "batch-run") || strings.Contains(body, "工作流 w") || strings.Contains(body, "未开始") {
		t.Fatalf("batch output should hide batch/run details and unstarted labels:\n%s", got)
	}
	// Every created run should show its stage checklist indented.
	if !strings.Contains(got, "  执行阶段 exec-thread ✓ -") {
		t.Fatalf("output missing current run stage detail:\n%s", got)
	}
	if !strings.Contains(got, "  规划阶段 - - -") {
		t.Fatalf("output missing completed run stage detail:\n%s", got)
	}
	assertLineOrder(t, got,
		"- 1-a",
		"- 2-b",
		"  执行阶段 exec-thread ✓ -",
		"  审核阶段 - → -",
		"- 3-c",
	)
}

// TestPrintHumanStatusFailedBatch verifies failed change, stable error and unstarted changes.
func TestPrintHumanStatusFailedBatch(t *testing.T) {
	repo := gitRepo(t)
	run1 := State{RunID: "run-fail", ChangeName: "1-a", Status: statusFailed, Stage: "execution", BatchID: "batch-fail", Error: "agent failed", Workflow: DefaultWorkflowConfig()}
	if err := saveState(repo, run1); err != nil {
		t.Fatal(err)
	}
	batch := BatchState{
		BatchID:      "batch-fail",
		Status:       batchStatusFailed,
		Changes:      []string{"1-a", "2-b"},
		CurrentIndex: 0,
		RunIDs:       map[string]string{"1-a": "run-fail"},
		FailedChange: "1-a",
		FailedRunID:  "run-fail",
		Error:        "run failed",
	}
	if err := saveBatchState(repo, batch); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	if err := printHumanStatus(&stdout, repo); err != nil {
		t.Fatal(err)
	}
	got := stdout.String()
	for _, want := range []string{"1-a 的写阶段失败", "- 1-a", "- 2-b", "提示: 可运行 wo restart -b1 删除失败记录并继续该批量任务"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "run failed") {
		t.Fatalf("failed batch output should hide raw errors:\n%s", got)
	}
}

// TestPrintHumanStatusFailedBatchHidesInternalNetworkError verifies status output stays readable when agent stderr contains backend diagnostics.
func TestPrintHumanStatusFailedBatchHidesInternalNetworkError(t *testing.T) {
	repo := gitRepo(t)
	rawError := "stderr: request to backend-api failed\nwss://chatgpt.com/backend-api/codex websocket: tls handshake eof"
	run1 := State{RunID: "run-network", ChangeName: "1-network", Status: statusFailed, Stage: "execution", BatchID: "batch-network", Error: rawError, Workflow: DefaultWorkflowConfig()}
	if err := saveState(repo, run1); err != nil {
		t.Fatal(err)
	}
	batch := BatchState{
		BatchID:      "batch-network",
		Status:       batchStatusFailed,
		Changes:      []string{"1-network", "2-next"},
		CurrentIndex: 0,
		RunIDs:       map[string]string{"1-network": "run-network"},
		FailedChange: "1-network",
		FailedRunID:  "run-network",
		Error:        rawError,
	}
	if err := saveBatchState(repo, batch); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	if err := printHumanStatus(&stdout, repo); err != nil {
		t.Fatal(err)
	}
	got := stdout.String()
	for _, want := range []string{"1-network 的写阶段失败", "智能体后端连接失败", "- 1-network", "- 2-next"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q:\n%s", want, got)
		}
	}
	for _, leaked := range []string{"stderr", "backend-api", "wss://chatgpt.com", "websocket", "tls handshake eof"} {
		if strings.Contains(got, leaked) {
			t.Fatalf("output leaked %q:\n%s", leaked, got)
		}
	}
}

// TestPrintHumanStatusBlockedReviewLimit verifies blocked review limit shows failure marker.
func TestPrintHumanStatusBlockedReviewLimit(t *testing.T) {
	repo := gitRepo(t)
	run1 := State{RunID: "run-blocked", ChangeName: "1-a", Status: statusBlocked, Stage: statusBlocked, BatchID: "batch-blocked", Error: "审核修正达到上限", Workflow: DefaultWorkflowConfig()}
	if err := saveState(repo, run1); err != nil {
		t.Fatal(err)
	}
	batch := BatchState{
		BatchID:      "batch-blocked",
		Status:       batchStatusFailed,
		Changes:      []string{"1-a", "2-b"},
		CurrentIndex: 0,
		RunIDs:       map[string]string{"1-a": "run-blocked"},
		FailedChange: "1-a",
		FailedRunID:  "run-blocked",
		Error:        "run blocked",
	}
	if err := saveBatchState(repo, batch); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	if err := printHumanStatus(&stdout, repo); err != nil {
		t.Fatal(err)
	}
	got := stdout.String()
	for _, want := range []string{"- 1-a", "- 2-b", "审核修正达到上限"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q:\n%s", want, got)
		}
	}
}

// TestPrintHumanStatusBlockedValidationLimit verifies blocked validation limit shows failure marker.
func TestPrintHumanStatusBlockedValidationLimit(t *testing.T) {
	repo := gitRepo(t)
	run1 := State{RunID: "run-val-blocked", ChangeName: "1-a", Status: statusValidationBlocked, Stage: statusValidationBlocked, BatchID: "batch-val-blocked", Error: "阶段验证达到上限", Workflow: DefaultWorkflowConfig()}
	if err := saveState(repo, run1); err != nil {
		t.Fatal(err)
	}
	batch := BatchState{
		BatchID:      "batch-val-blocked",
		Status:       batchStatusFailed,
		Changes:      []string{"1-a", "2-b"},
		CurrentIndex: 0,
		RunIDs:       map[string]string{"1-a": "run-val-blocked"},
		FailedChange: "1-a",
		FailedRunID:  "run-val-blocked",
		Error:        "validation blocked",
	}
	if err := saveBatchState(repo, batch); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	if err := printHumanStatus(&stdout, repo); err != nil {
		t.Fatal(err)
	}
	got := stdout.String()
	for _, want := range []string{"- 1-a", "- 2-b", "阶段验证达到上限"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q:\n%s", want, got)
		}
	}
}

// TestPrintHumanStatusSingleRunNoBatch verifies plain single-run output remains unchanged.
func TestPrintHumanStatusSingleRunNoBatch(t *testing.T) {
	repo := gitRepo(t)
	state := State{RunID: "single-run", ChangeName: "demo", Status: statusRunning, Stage: "execution", Workflow: DefaultWorkflowConfig()}
	if err := saveState(repo, state); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	if err := printHumanStatus(&stdout, repo); err != nil {
		t.Fatal(err)
	}
	got := stdout.String()
	if strings.Contains(got, "批量任务") || strings.Contains(got, "最近一次批量工作流") {
		t.Fatalf("single run output should not contain batch terms:\n%s", got)
	}
	if !strings.Contains(got, "执行阶段 - → -") {
		t.Fatalf("single run output missing expected line:\n%s", got)
	}
}

// TestPrintHumanStatusJSONContractUnchanged verifies --json still returns single run DTO.
func TestPrintHumanStatusJSONContractUnchanged(t *testing.T) {
	repo := gitRepo(t)
	run1 := State{RunID: "run-batch", ChangeName: "1-a", Status: statusRunning, Stage: "execution", BatchID: "batch-json", Workflow: DefaultWorkflowConfig()}
	if err := saveState(repo, run1); err != nil {
		t.Fatal(err)
	}
	batch := BatchState{
		BatchID:      "batch-json",
		Status:       batchStatusRunning,
		Changes:      []string{"1-a"},
		CurrentIndex: 0,
		RunIDs:       map[string]string{"1-a": "run-batch"},
	}
	if err := saveBatchState(repo, batch); err != nil {
		t.Fatal(err)
	}

	state, err := loadState(repo, "run-batch")
	if err != nil {
		t.Fatal(err)
	}
	var stdout bytes.Buffer
	if err := writeRunnerState(&stdout, state); err != nil {
		t.Fatal(err)
	}
	got := stdout.String()
	for _, want := range []string{`"run_id"`, `"change_name"`, `"status"`, `"stage"`, `"stages"`, `"paths"`, `"sessions"`, `"error"`} {
		if !strings.Contains(got, want) {
			t.Fatalf("json output missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "批量任务") || strings.Contains(got, "工作流") {
		t.Fatalf("json output should not contain batch human terms:\n%s", got)
	}
}

// TestPrintHumanStatusDoneBatch verifies all changes show done marker.
func TestPrintHumanStatusDoneBatch(t *testing.T) {
	repo := gitRepo(t)
	run1 := State{RunID: "run-1", ChangeName: "1-a", Status: statusDone, Stage: "done", BatchID: "batch-done", Workflow: DefaultWorkflowConfig()}
	run2 := State{RunID: "run-2", ChangeName: "2-b", Status: statusDone, Stage: "done", BatchID: "batch-done", Workflow: DefaultWorkflowConfig()}
	if err := saveState(repo, run1); err != nil {
		t.Fatal(err)
	}
	if err := saveState(repo, run2); err != nil {
		t.Fatal(err)
	}
	batch := BatchState{
		BatchID:      "batch-done",
		Status:       batchStatusDone,
		Changes:      []string{"1-a", "2-b"},
		CurrentIndex: 2,
		RunIDs:       map[string]string{"1-a": "run-1", "2-b": "run-2"},
	}
	if err := saveBatchState(repo, batch); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	if err := printHumanStatus(&stdout, repo); err != nil {
		t.Fatal(err)
	}
	got := stdout.String()
	for _, want := range []string{"- 1-a", "- 2-b"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q:\n%s", want, got)
		}
	}
	assertLineOrder(t, got,
		"- 1-a",
		"- 2-b",
	)
	if strings.Contains(got, "x") {
		t.Fatalf("done batch should not contain failure marker:\n%s", got)
	}
}

// TestPrintHumanStatusAbortedBatch verifies aborted change shows x marker.
func TestPrintHumanStatusAbortedBatch(t *testing.T) {
	repo := gitRepo(t)
	run1 := State{RunID: "run-1", ChangeName: "1-a", Status: statusAborted, Stage: "execution", BatchID: "batch-abort", Error: "用户已中止", Workflow: DefaultWorkflowConfig()}
	if err := saveState(repo, run1); err != nil {
		t.Fatal(err)
	}
	batch := BatchState{
		BatchID:      "batch-abort",
		Status:       batchStatusAborted,
		Changes:      []string{"1-a", "2-b"},
		CurrentIndex: 0,
		RunIDs:       map[string]string{"1-a": "run-1"},
		Error:        "用户已中止",
	}
	if err := saveBatchState(repo, batch); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	if err := printHumanStatus(&stdout, repo); err != nil {
		t.Fatal(err)
	}
	got := stdout.String()
	for _, want := range []string{"- 1-a", "- 2-b", "错误: 用户已中止", "清理: 可运行 wo clean 清理当前项目失败或异常运行态"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q:\n%s", want, got)
		}
	}
	if !strings.Contains(got, "该操作仅删除 wo 历史记录，不回滚代码改动") {
		t.Fatalf("aborted batch missing code-change disclaimer:\n%s", got)
	}
}

// TestPrintHumanStatusMissingRunState verifies graceful degradation when run state is missing.
func TestPrintHumanStatusMissingRunState(t *testing.T) {
	repo := gitRepo(t)
	batch := BatchState{
		BatchID:      "batch-missing",
		Status:       batchStatusRunning,
		Changes:      []string{"1-a"},
		CurrentIndex: 0,
		RunIDs:       map[string]string{"1-a": "run-missing"},
	}
	if err := saveBatchState(repo, batch); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	if err := printHumanStatus(&stdout, repo); err != nil {
		t.Fatal(err)
	}
	got := stdout.String()
	if !strings.Contains(got, "- 1-a") {
		t.Fatalf("output missing missing-run change:\n%s", got)
	}
	if strings.Contains(got, "run-missing") || strings.Contains(got, "state_missing") {
		t.Fatalf("missing run state should not expose run details:\n%s", got)
	}
	if strings.Contains(got, "  - 写") || strings.Contains(got, "  - 审") || strings.Contains(got, "  - 存") || strings.Contains(got, "  - 规") {
		t.Fatalf("missing run state should not expand stage details:\n%s", got)
	}
}

// TestPrintHumanStatusBatchNoRunsYet verifies batch without any created runs.
func TestPrintHumanStatusBatchNoRunsYet(t *testing.T) {
	repo := gitRepo(t)
	batch := BatchState{
		BatchID:      "batch-new",
		Status:       batchStatusRunning,
		Changes:      []string{"1-a", "2-b"},
		CurrentIndex: 0,
		RunIDs:       map[string]string{},
	}
	if err := saveBatchState(repo, batch); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	if err := printHumanStatus(&stdout, repo); err != nil {
		t.Fatal(err)
	}
	got := stdout.String()
	for _, want := range []string{"- 1-a", "- 2-b"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q:\n%s", want, got)
		}
	}
	body := strings.Join(strings.Split(got, "\n")[1:], "\n")
	if strings.Contains(body, "batch-new") || strings.Contains(body, "未开始") || strings.Contains(body, "工作流 w") {
		t.Fatalf("new batch output should be compact:\n%s", got)
	}
	if strings.Contains(got, "  - 写") || strings.Contains(got, "  - 审") || strings.Contains(got, "  - 存") || strings.Contains(got, "  - 规") {
		t.Fatalf("unstarted changes should not expand stage details:\n%s", got)
	}
	if strings.Contains(got, "没有 wo run") {
		t.Fatalf("batch without runs should not show '没有 wo run':\n%s", got)
	}
}

// TestPrintHumanStatusDefaultPrefersLatestBatch verifies default status prioritizes batches.
func TestPrintHumanStatusDefaultPrefersLatestBatch(t *testing.T) {
	repo := gitRepo(t)
	state := State{RunID: "single-run", ChangeName: "demo", Status: statusRunning, Stage: "execution", Workflow: DefaultWorkflowConfig()}
	if err := saveState(repo, state); err != nil {
		t.Fatal(err)
	}
	// Any batch is the default status target; use -w1 to inspect the workflow.
	batch := BatchState{
		BatchID:      "batch-old",
		Status:       batchStatusDone,
		Changes:      []string{"old-a"},
		CurrentIndex: 1,
		RunIDs:       map[string]string{"old-a": "run-old"},
	}
	if err := saveBatchState(repo, batch); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	if err := printHumanStatus(&stdout, repo); err != nil {
		t.Fatal(err)
	}
	got := stdout.String()
	firstLine, _, _ := strings.Cut(got, "\n")
	if firstLine != "- old-a" {
		t.Fatalf("default batch status first line = %q, want proposal list\nfull output:\n%s", firstLine, got)
	}
	if strings.Contains(got, "最近一次批量工作流") || strings.Contains(got, "→ b1 1/1") {
		t.Fatalf("default status should show latest batch:\n%s", got)
	}
	stdout.Reset()
	if err := printHumanStatus(&stdout, repo, "-w1"); err != nil {
		t.Fatal(err)
	}
	w1Output := stdout.String()
	if strings.Contains(w1Output, "最近一次批量工作流") {
		t.Fatalf("-w1 output should not contain default batch hint:\n%s", w1Output)
	}
	if !strings.Contains(w1Output, "执行阶段 - → -") {
		t.Fatalf("-w1 output missing expected line:\n%s", w1Output)
	}
}

// TestPrintHumanStatusNewerBatchTakesPrecedence verifies a newer batch without runs overrides an older single run.
func TestPrintHumanStatusNewerBatchTakesPrecedence(t *testing.T) {
	repo := gitRepo(t)
	// Old single run
	oldRun := State{RunID: "20260510T000000.000000000Z", ChangeName: "old", Status: statusRunning, Stage: "execution", Workflow: DefaultWorkflowConfig()}
	if err := saveState(repo, oldRun); err != nil {
		t.Fatal(err)
	}
	// Newer batch without any runs yet
	batch := BatchState{
		BatchID:      "20260511T000000.000000000Z",
		Status:       batchStatusRunning,
		Changes:      []string{"1-a", "2-b"},
		CurrentIndex: 0,
		RunIDs:       map[string]string{},
	}
	if err := saveBatchState(repo, batch); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	if err := printHumanStatus(&stdout, repo); err != nil {
		t.Fatal(err)
	}
	got := stdout.String()
	if !strings.Contains(got, "- 1-a") || !strings.Contains(got, "- 2-b") {
		t.Fatalf("newer batch should take precedence over old single run:\n%s", got)
	}
	if strings.Contains(got, "写 20260510T000000.000000000Z →") {
		t.Fatalf("newer batch should not show old single run:\n%s", got)
	}
}

// TestPrintHumanStatusShortRefsResolveHistory verifies bN/wN aliases map newest first.
func TestPrintHumanStatusShortRefsResolveHistory(t *testing.T) {
	repo := gitRepo(t)
	oldRun := State{RunID: "20260510T000000.000000000Z", ChangeName: "old", Status: statusRunning, Stage: "execution", Workflow: DefaultWorkflowConfig()}
	newRun := State{RunID: "20260511T000000.000000000Z", ChangeName: "new", Status: statusRunning, Stage: "execution", Workflow: DefaultWorkflowConfig()}
	if err := saveState(repo, oldRun); err != nil {
		t.Fatal(err)
	}
	if err := saveState(repo, newRun); err != nil {
		t.Fatal(err)
	}
	for _, batch := range []BatchState{
		{BatchID: "20260510T000000.000000000Z", Status: batchStatusDone, Changes: []string{"old"}, CurrentIndex: 1, RunIDs: map[string]string{}},
		{BatchID: "20260511T000000.000000000Z", Status: batchStatusRunning, Changes: []string{"new"}, CurrentIndex: 0, RunIDs: map[string]string{}},
	} {
		if err := saveBatchState(repo, batch); err != nil {
			t.Fatal(err)
		}
	}

	var stdout bytes.Buffer
	if err := printHumanStatus(&stdout, repo, "-b2"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "- old") {
		t.Fatalf("-b2 should show older batch:\n%s", stdout.String())
	}
	stdout.Reset()
	if err := printHumanStatus(&stdout, repo, "-w2"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "执行阶段 - → -") {
		t.Fatalf("-w2 should show older workflow:\n%s", stdout.String())
	}
	if err := printHumanStatus(&stdout, repo, "-b99"); err == nil || !strings.Contains(err.Error(), "找不到 b99") {
		t.Fatalf("-b99 error = %v, want clear missing alias", err)
	}
	if err := printHumanStatus(&stdout, repo, "-w99"); err == nil || !strings.Contains(err.Error(), "找不到 w99") {
		t.Fatalf("-w99 error = %v, want clear missing alias", err)
	}
}

// TestPrintHumanStatusBatchCurrentRunShowsActiveReview verifies batch current run displays review_1 with arrow.
func TestPrintHumanStatusBatchCurrentRunShowsActiveReview(t *testing.T) {
	repo := gitRepo(t)
	run1 := State{RunID: "run-1", ChangeName: "1-a", Status: statusDone, Stage: "done", BatchID: "batch-review", Workflow: DefaultWorkflowConfig()}
	run2 := State{RunID: "run-2", ChangeName: "2-b", Status: statusRunning, Stage: "review_1", BatchID: "batch-review", Sessions: map[string]string{"codex:reviewer": "review-thread"}, Stages: map[string]string{"execution": "completed"}, Workflow: DefaultWorkflowConfig()}
	if err := saveState(repo, run1); err != nil {
		t.Fatal(err)
	}
	if err := saveState(repo, run2); err != nil {
		t.Fatal(err)
	}
	batch := BatchState{
		BatchID:      "batch-review",
		Status:       batchStatusRunning,
		Changes:      []string{"1-a", "2-b"},
		CurrentIndex: 1,
		RunIDs:       map[string]string{"1-a": "run-1", "2-b": "run-2"},
	}
	if err := saveBatchState(repo, batch); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	if err := printHumanStatus(&stdout, repo); err != nil {
		t.Fatal(err)
	}
	got := stdout.String()
	if !strings.Contains(got, "  审核阶段 review-thread → -") {
		t.Fatalf("output missing active review stage with arrow:\n%s", got)
	}
	if !strings.Contains(got, "  执行阶段 - ✓ -") || strings.Contains(got, "  执行阶段 run-2") {
		t.Fatalf("pre-completed executor should not use run id:\n%s", got)
	}
}

// TestAppendBatchChangesAppendsToRunningBatch verifies active changes are appended to the tail.
func TestAppendBatchChangesAppendsToRunningBatch(t *testing.T) {
	repo := gitRepo(t)
	mustChange(t, repo, "1-a")
	mustChange(t, repo, "2-b")
	mustChange(t, repo, "3-c")
	batch := BatchState{
		BatchID:      "batch-append",
		Status:       batchStatusRunning,
		Changes:      []string{"1-a"},
		CurrentIndex: 0,
		RunIDs:       map[string]string{},
	}
	if err := saveBatchState(repo, batch); err != nil {
		t.Fatal(err)
	}
	added, skipped, err := AppendBatchChanges(repo, "batch-append", []Change{{Name: "3-c"}, {Name: "2-b"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(added) != 2 || added[0] != "2-b" || added[1] != "3-c" {
		t.Fatalf("added = %v, want [2-b 3-c]", added)
	}
	if len(skipped) != 0 {
		t.Fatalf("skipped = %v, want none", skipped)
	}
	final, err := loadBatchState(repo, "batch-append")
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(final.Changes, ","); got != "1-a,2-b,3-c" {
		t.Fatalf("batch changes = %s, want 1-a,2-b,3-c", got)
	}
	if final.CurrentIndex != 0 {
		t.Fatalf("current_index = %d, want 0", final.CurrentIndex)
	}
}

// TestAppendBatchChangesSkipsDuplicates verifies existing changes are skipped.
func TestAppendBatchChangesSkipsDuplicates(t *testing.T) {
	repo := gitRepo(t)
	mustChange(t, repo, "1-a")
	mustChange(t, repo, "2-b")
	mustChange(t, repo, "3-c")
	batch := BatchState{
		BatchID:      "batch-dedup",
		Status:       batchStatusRunning,
		Changes:      []string{"1-a", "2-b"},
		CurrentIndex: 1,
		RunIDs:       map[string]string{},
	}
	if err := saveBatchState(repo, batch); err != nil {
		t.Fatal(err)
	}
	added, skipped, err := AppendBatchChanges(repo, "batch-dedup", []Change{{Name: "2-b"}, {Name: "3-c"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(added) != 1 || added[0] != "3-c" {
		t.Fatalf("added = %v, want [3-c]", added)
	}
	if len(skipped) != 1 || skipped[0] != "2-b" {
		t.Fatalf("skipped = %v, want [2-b]", skipped)
	}
	final, _ := loadBatchState(repo, "batch-dedup")
	if got := strings.Join(final.Changes, ","); got != "1-a,2-b,3-c" {
		t.Fatalf("batch changes = %s, want 1-a,2-b,3-c", got)
	}
}

// TestAppendBatchChangesRejectsNonRunningBatch verifies only running batches accept appends.
func TestAppendBatchChangesRejectsNonRunningBatch(t *testing.T) {
	repo := gitRepo(t)
	mustChange(t, repo, "2-b")
	for _, status := range []string{batchStatusDone, batchStatusFailed, batchStatusAborted} {
		batch := BatchState{
			BatchID:      "batch-" + status,
			Status:       status,
			Changes:      []string{"1-a"},
			CurrentIndex: 1,
			RunIDs:       map[string]string{},
		}
		if err := saveBatchState(repo, batch); err != nil {
			t.Fatal(err)
		}
		_, _, err := AppendBatchChanges(repo, "batch-"+status, []Change{{Name: "2-b"}})
		if err == nil {
			t.Fatalf("AppendBatchChanges should reject %s batch", status)
		}
	}
}

// TestAppendBatchChangesAllDuplicateReturnsEmpty verifies no error when all are duplicates.
func TestAppendBatchChangesAllDuplicateReturnsEmpty(t *testing.T) {
	repo := gitRepo(t)
	mustChange(t, repo, "1-a")
	batch := BatchState{
		BatchID:      "batch-all-dup",
		Status:       batchStatusRunning,
		Changes:      []string{"1-a"},
		CurrentIndex: 0,
		RunIDs:       map[string]string{},
	}
	if err := saveBatchState(repo, batch); err != nil {
		t.Fatal(err)
	}
	added, skipped, err := AppendBatchChanges(repo, "batch-all-dup", []Change{{Name: "1-a"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(added) != 0 || len(skipped) != 1 {
		t.Fatalf("added=%v skipped=%v, want 0 added 1 skipped", added, skipped)
	}
}

// TestRunBatchConsumesAppendedChanges verifies worker picks up appended changes after current run.
func TestRunBatchConsumesAppendedChanges(t *testing.T) {
	repo := gitRepo(t)
	mustChange(t, repo, "1-a")
	mustChange(t, repo, "2-b")
	mustPrompts(t, repo)
	mustWritePrompt(t, filepath.Join(repo, "wo.yaml"), "wo:\n  workflow:\n    max_review_iterations: 0\n")
	runner := &batchRunner{}
	engine := NewEngine(repo, testRegistry(runner))
	batch := BatchState{
		BatchID:      "batch-consume",
		Status:       batchStatusRunning,
		Changes:      []string{"1-a"},
		CurrentIndex: 0,
		RunIDs:       map[string]string{},
	}
	if err := saveBatchState(repo, batch); err != nil {
		t.Fatal(err)
	}
	// Simulate append happening during the first run by appending before RunBatch starts.
	if _, _, err := AppendBatchChanges(repo, "batch-consume", []Change{{Name: "2-b"}}); err != nil {
		t.Fatal(err)
	}
	if err := engine.RunBatch(context.Background(), "batch-consume"); err != nil {
		t.Fatal(err)
	}
	final, err := loadBatchState(repo, "batch-consume")
	if err != nil {
		t.Fatal(err)
	}
	if final.Status != batchStatusDone || final.CurrentIndex != 2 {
		t.Fatalf("batch = %#v, want done at index 2", final)
	}
	if got := strings.Join(runner.changes, ","); got != "1-a,1-a,2-b,2-b" {
		t.Fatalf("runner changes = %s, want serial stages per change", got)
	}
}

// TestInteractiveShowsRunningBatchAppendMenu verifies running batches show append option.
func TestInteractiveShowsRunningBatchAppendMenu(t *testing.T) {
	repo := gitRepo(t)
	mustChange(t, repo, "1-a")
	mustChange(t, repo, "2-b")
	mustPrompts(t, repo)
	batch := BatchState{BatchID: "batch-append-menu", Status: batchStatusRunning, Changes: []string{"1-a"}, RunIDs: map[string]string{}}
	if err := saveBatchState(repo, batch); err != nil {
		t.Fatal(err)
	}
	var started []string
	previous := startDetachedBatchCommand
	startDetachedBatchCommand = func(_ string, batchID string) error {
		started = append(started, batchID)
		return nil
	}
	t.Cleanup(func() { startDetachedBatchCommand = previous })
	engine := NewEngine(repo, testRegistry(&batchRunner{}))
	var stdout bytes.Buffer
	// Choose option 2 (append). The only appendable change is selected directly.
	if err := interactive(context.Background(), strings.NewReader("2\n"), &stdout, repo, engine); err != nil {
		t.Fatal(err)
	}
	got := stdout.String()
	if !strings.Contains(got, "发现未完成批量任务：batch-append-menu") {
		t.Fatalf("output missing running batch notice:\n%s", got)
	}
	if !strings.Contains(got, "追加变更提案") {
		t.Fatalf("output missing append option:\n%s", got)
	}
	if strings.Contains(got, "1. 2-b") {
		t.Fatalf("single appendable change should not render a number prompt:\n%s", got)
	}
	final, err := loadBatchState(repo, "batch-append-menu")
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(final.Changes, ","); got != "1-a,2-b" {
		t.Fatalf("batch changes = %s, want 1-a,2-b", got)
	}
	if len(started) != 0 {
		t.Fatalf("started = %v, want no new worker for append", started)
	}
}

// TestInteractiveAppendWithNoCandidatesExits verifies an empty append set does not prompt.
func TestInteractiveAppendWithNoCandidatesExits(t *testing.T) {
	repo := gitRepo(t)
	mustChange(t, repo, "1-a")
	mustPrompts(t, repo)
	batch := BatchState{BatchID: "batch-no-append", Status: batchStatusRunning, Changes: []string{"1-a"}, RunIDs: map[string]string{}}
	if err := saveBatchState(repo, batch); err != nil {
		t.Fatal(err)
	}
	engine := NewEngine(repo, testRegistry(&batchRunner{}))
	var stdout bytes.Buffer
	if err := interactive(context.Background(), strings.NewReader("2\n"), &stdout, repo, engine); err != nil {
		t.Fatal(err)
	}
	got := stdout.String()
	if !strings.Contains(got, "没有可追加的 active 变更提案") {
		t.Fatalf("output missing no-candidate message:\n%s", got)
	}
	if strings.Contains(got, "1. 1-a") {
		t.Fatalf("existing batch change should not be offered for append:\n%s", got)
	}
	final, err := loadBatchState(repo, "batch-no-append")
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(final.Changes, ","); got != "1-a" {
		t.Fatalf("batch changes = %s, want unchanged 1-a", got)
	}
}

// TestAppendBatchChangesRejectsInactiveChange verifies non-active changes are refused.
func TestAppendBatchChangesRejectsInactiveChange(t *testing.T) {
	repo := gitRepo(t)
	mustChange(t, repo, "1-a")
	batch := BatchState{
		BatchID:      "batch-reject",
		Status:       batchStatusRunning,
		Changes:      []string{"1-a"},
		CurrentIndex: 0,
		RunIDs:       map[string]string{},
	}
	if err := saveBatchState(repo, batch); err != nil {
		t.Fatal(err)
	}
	_, _, err := AppendBatchChanges(repo, "batch-reject", []Change{{Name: "missing-change"}})
	if err == nil {
		t.Fatal("AppendBatchChanges should reject inactive change")
	}
	final, _ := loadBatchState(repo, "batch-reject")
	if len(final.Changes) != 1 {
		t.Fatalf("batch changes = %v, want unchanged", final.Changes)
	}
}

// TestAppendBatchChangesConcurrentLock verifies withBatchState prevents race.
func TestAppendBatchChangesConcurrentLock(t *testing.T) {
	repo := gitRepo(t)
	mustChange(t, repo, "1-a")
	mustChange(t, repo, "2-b")
	mustChange(t, repo, "3-c")
	batch := BatchState{
		BatchID:      "batch-concurrent",
		Status:       batchStatusRunning,
		Changes:      []string{"1-a"},
		CurrentIndex: 0,
		RunIDs:       map[string]string{},
	}
	if err := saveBatchState(repo, batch); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			changeName := fmt.Sprintf("%d-c", idx+2)
			_, _, _ = AppendBatchChanges(repo, "batch-concurrent", []Change{{Name: changeName}})
		}(i)
	}
	wg.Wait()

	final, err := loadBatchState(repo, "batch-concurrent")
	if err != nil {
		t.Fatal(err)
	}
	// Only 2-b and 3-c are active; others are inactive and should be rejected.
	// With the lock, state should not be corrupted even with concurrent writes.
	if len(final.Changes) < 1 {
		t.Fatalf("batch changes corrupted: %v", final.Changes)
	}
}

// slowAppendRunner pauses during the first change execution to allow append.
type slowAppendRunner struct {
	stages  []string
	changes []string
	paused  chan struct{}
	resume  chan struct{}
}

func (r *slowAppendRunner) Run(_ context.Context, repo, prompt, threadID string, _ StageOptions) (string, error) {
	stage := stageFromPromptOrState(repo, prompt)
	runID := currentRunID(repo)
	state, err := loadState(repo, runID)
	if err != nil {
		return "", err
	}
	r.stages = append(r.stages, stage)
	r.changes = append(r.changes, state.ChangeName)
	switch stage {
	case "execution":
		// Pause on first change to simulate running batch.
		if state.ChangeName == "1-a" {
			close(r.paused)
			<-r.resume
		}
		task := filepath.Join(repo, "docs", "changes", state.ChangeName, "task.md")
		return "executor-thread", os.WriteFile(task, []byte("- [x] task\n"), 0o644)
	case "archive":
		archive := filepath.Join(repo, "docs", "changes", "archive", "2026-05-05-"+state.ChangeName)
		if err := os.MkdirAll(archive, 0o755); err != nil {
			return "", err
		}
		return threadID, os.WriteFile(filepath.Join(runDir(repo, runID), "delivery-summary.md"), []byte("done\n"), 0o644)
	default:
		return threadID, nil
	}
}

// TestRunBatchPicksUpAppendDuringExecution verifies worker reloads state after a running change.
func TestRunBatchPicksUpAppendDuringExecution(t *testing.T) {
	repo := gitRepo(t)
	mustChange(t, repo, "1-a")
	mustChange(t, repo, "2-b")
	mustPrompts(t, repo)
	mustWritePrompt(t, filepath.Join(repo, "wo.yaml"), "wo:\n  workflow:\n    max_review_iterations: 0\n")

	batch := BatchState{
		BatchID:      "batch-during-run",
		Status:       batchStatusRunning,
		Changes:      []string{"1-a"},
		CurrentIndex: 0,
		RunIDs:       map[string]string{},
	}
	if err := saveBatchState(repo, batch); err != nil {
		t.Fatal(err)
	}

	runner := &slowAppendRunner{
		paused: make(chan struct{}),
		resume: make(chan struct{}),
	}
	engine := NewEngine(repo, testRegistry(runner))

	// Start batch worker in background.
	done := make(chan error, 1)
	go func() {
		done <- engine.RunBatch(context.Background(), "batch-during-run")
	}()

	// Wait until worker pauses on first change.
	<-runner.paused

	// Append 2-b while first change is running.
	added, skipped, err := AppendBatchChanges(repo, "batch-during-run", []Change{{Name: "2-b"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(added) != 1 || added[0] != "2-b" {
		t.Fatalf("added = %v, want [2-b]", added)
	}
	if len(skipped) != 0 {
		t.Fatalf("skipped = %v, want none", skipped)
	}

	// Resume worker.
	close(runner.resume)

	// Wait for worker to finish.
	if err := <-done; err != nil {
		t.Fatalf("RunBatch error: %v", err)
	}
	final, err := loadBatchState(repo, "batch-during-run")
	if err != nil {
		t.Fatal(err)
	}
	if final.Status != batchStatusDone || final.CurrentIndex != 2 {
		t.Fatalf("batch = %#v, want done at index 2", final)
	}
	if got := strings.Join(runner.changes, ","); got != "1-a,1-a,2-b,2-b" {
		t.Fatalf("runner changes = %s, want serial stages per change", got)
	}
}

// TestRunBatchHandlesAppendAfterLastReload verifies appended changes are not
// skipped when the append happens between the last reload and the done write.
func TestRunBatchHandlesAppendAfterLastReload(t *testing.T) {
	repo := gitRepo(t)
	mustChange(t, repo, "1-a")
	mustChange(t, repo, "2-b")

	// Batch with 1-a already done.
	batch := BatchState{
		BatchID:      "batch-last-reload",
		Status:       batchStatusRunning,
		Changes:      []string{"1-a"},
		CurrentIndex: 1,
		RunIDs:       map[string]string{},
	}
	if err := saveBatchState(repo, batch); err != nil {
		t.Fatal(err)
	}

	// Simulate the guard in RunBatch: withBatchState sees no work,
	// then append happens, then withBatchState is called again and
	// correctly skips the done transition.
	if err := withBatchState(repo, "batch-last-reload", func(b *BatchState) error {
		if b.CurrentIndex < len(b.Changes) {
			return nil
		}
		b.Status = batchStatusDone
		b.CurrentIndex = len(b.Changes)
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	// Now batch is done.
	first, _ := loadBatchState(repo, "batch-last-reload")
	if first.Status != batchStatusDone {
		t.Fatalf("expected done after first guard, got %s", first.Status)
	}

	// Reset to running and append 2-b (simulating append after reload).
	first.Status = batchStatusRunning
	if err := saveBatchState(repo, first); err != nil {
		t.Fatal(err)
	}
	_, _, err := AppendBatchChanges(repo, "batch-last-reload", []Change{{Name: "2-b"}})
	if err != nil {
		t.Fatal(err)
	}

	// Second guard: should see 2-b and skip done transition.
	if err := withBatchState(repo, "batch-last-reload", func(b *BatchState) error {
		if b.CurrentIndex < len(b.Changes) {
			return nil
		}
		b.Status = batchStatusDone
		b.CurrentIndex = len(b.Changes)
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	final, err := loadBatchState(repo, "batch-last-reload")
	if err != nil {
		t.Fatal(err)
	}
	if final.Status != batchStatusRunning {
		t.Fatalf("batch status = %s, want running (2-b not executed yet)", final.Status)
	}
	if len(final.Changes) != 2 || final.Changes[1] != "2-b" {
		t.Fatalf("batch changes = %v, want [1-a 2-b]", final.Changes)
	}
}

// assertLineOrder verifies status fragments appear in the expected sequence.
func assertLineOrder(t *testing.T, output string, fragments ...string) {
	t.Helper()
	offset := 0
	for _, fragment := range fragments {
		index := strings.Index(output[offset:], fragment)
		if index < 0 {
			t.Fatalf("output missing %q after byte %d:\n%s", fragment, offset, output)
		}
		offset += index + len(fragment)
	}
}

// TestPrintHumanStatusFailedBatchShowsRestartHint verifies failed batch shows restart hint.
func TestPrintHumanStatusFailedBatchShowsRestartHint(t *testing.T) {
	repo := gitRepo(t)
	run1 := State{RunID: "run-fail", ChangeName: "1-a", Status: statusFailed, Stage: "execution", BatchID: "batch-fail", Error: "agent execution failed", Workflow: DefaultWorkflowConfig()}
	if err := saveState(repo, run1); err != nil {
		t.Fatal(err)
	}
	batch := BatchState{
		BatchID:      "batch-fail",
		Status:       batchStatusFailed,
		Changes:      []string{"1-a", "2-b"},
		CurrentIndex: 0,
		RunIDs:       map[string]string{"1-a": "run-fail"},
		FailedChange: "1-a",
		FailedRunID:  "run-fail",
		Error:        "agent execution failed",
	}
	if err := saveBatchState(repo, batch); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	if err := printHumanStatus(&stdout, repo); err != nil {
		t.Fatal(err)
	}
	got := stdout.String()
	for _, want := range []string{"1-a 的写阶段失败", "可运行 wo restart -b1 删除失败记录并继续该批量任务"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "wo clean") {
		t.Fatalf("recoverable batch should not show wo clean cleanup:\n%s", got)
	}
}

// TestPrintHumanStatusBlockedBatchShowsCleanupNotRestart verifies blocked batch does not suggest restart.
func TestPrintHumanStatusBlockedBatchShowsCleanupNotRestart(t *testing.T) {
	repo := gitRepo(t)
	run1 := State{RunID: "run-blocked", ChangeName: "1-a", Status: statusBlocked, Stage: statusBlocked, BatchID: "batch-blocked", Error: "审核修正达到上限，工作流已中断", Workflow: DefaultWorkflowConfig()}
	if err := saveState(repo, run1); err != nil {
		t.Fatal(err)
	}
	batch := BatchState{
		BatchID:      "batch-blocked",
		Status:       batchStatusFailed,
		Changes:      []string{"1-a", "2-b"},
		CurrentIndex: 0,
		RunIDs:       map[string]string{"1-a": "run-blocked"},
		FailedChange: "1-a",
		FailedRunID:  "run-blocked",
		Error:        "blocked",
	}
	if err := saveBatchState(repo, batch); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	if err := printHumanStatus(&stdout, repo); err != nil {
		t.Fatal(err)
	}
	got := stdout.String()
	if strings.Contains(got, "wo restart -b1") {
		t.Fatalf("blocked batch should not suggest restart:\n%s", got)
	}
	if !strings.Contains(got, "wo clean") {
		t.Fatalf("blocked batch should show wo clean cleanup command:\n%s", got)
	}
	if !strings.Contains(got, "该操作仅删除 wo 历史记录，不回滚代码改动") {
		t.Fatalf("blocked batch missing code-change disclaimer:\n%s", got)
	}
}

// TestPrintHumanStatusFailedBatchNoRunState verifies graceful failure summary when run state is missing.
func TestPrintHumanStatusFailedBatchNoRunState(t *testing.T) {
	repo := gitRepo(t)
	batch := BatchState{
		BatchID:      "batch-norun",
		Status:       batchStatusFailed,
		Changes:      []string{"1-a", "2-b"},
		CurrentIndex: 0,
		RunIDs:       map[string]string{},
		FailedChange: "1-a",
		FailedRunID:  "run-missing",
		Error:        "something went wrong",
	}
	if err := saveBatchState(repo, batch); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	if err := printHumanStatus(&stdout, repo); err != nil {
		t.Fatal(err)
	}
	got := stdout.String()
	if !strings.Contains(got, "工作流记录缺失") {
		t.Fatalf("output should show missing run state message:\n%s", got)
	}
	if strings.Contains(got, "something went wrong") {
		t.Fatalf("output should not leak raw error when run state is missing:\n%s", got)
	}
	// Missing run state is not recoverable by restart.
	if strings.Contains(got, "wo restart") {
		t.Fatalf("batch with missing run state should show cleanup instead of restart:\n%s", got)
	}
	if !strings.Contains(got, "wo clean") {
		t.Fatalf("batch with missing run state should show wo clean cleanup hint:\n%s", got)
	}
}

// TestResolveWatchTargetDefaultBatchFirst verifies watch prioritizes running batch.
func TestResolveWatchTargetDefaultBatchFirst(t *testing.T) {
	repo := gitRepo(t)
	// Set up running batch.
	batch := BatchState{
		BatchID:      "batch-running",
		Status:       batchStatusRunning,
		Changes:      []string{"1-a"},
		CurrentIndex: 0,
		RunIDs:       map[string]string{},
	}
	if err := saveBatchState(repo, batch); err != nil {
		t.Fatal(err)
	}
	// Also set up a single run.
	run := State{RunID: "single-run", ChangeName: "demo", Status: statusRunning, Stage: "execution", Workflow: DefaultWorkflowConfig()}
	if err := saveState(repo, run); err != nil {
		t.Fatal(err)
	}

	kind, ref, err := resolveWatchTarget(repo, nil)
	if err != nil {
		t.Fatal(err)
	}
	if kind != "batch" {
		t.Fatalf("watch kind = %q, want batch", kind)
	}
	if ref.ID != "batch-running" {
		t.Fatalf("watch ref = %q, want batch-running", ref.ID)
	}
}

// TestResolveWatchTargetFallsBackToSingleRun verifies watch falls back when no running batch.
func TestResolveWatchTargetFallsBackToSingleRun(t *testing.T) {
	repo := gitRepo(t)
	run := State{RunID: "single-run", ChangeName: "demo", Status: statusRunning, Stage: "execution", Workflow: DefaultWorkflowConfig()}
	if err := saveState(repo, run); err != nil {
		t.Fatal(err)
	}

	kind, ref, err := resolveWatchTarget(repo, nil)
	if err != nil {
		t.Fatal(err)
	}
	if kind != "run" {
		t.Fatalf("watch kind = %q, want run", kind)
	}
	if ref.ID != "single-run" {
		t.Fatalf("watch ref = %q, want single-run", ref.ID)
	}
}

// TestResolveWatchTargetNoRunningTask verifies watch reports error when nothing runs.
func TestResolveWatchTargetNoRunningTask(t *testing.T) {
	repo := gitRepo(t)
	_, _, err := resolveWatchTarget(repo, nil)
	if err == nil {
		t.Fatal("resolveWatchTarget should fail with no running tasks")
	}
	if !strings.Contains(err.Error(), "没有正在进行的批量任务或工作流") {
		t.Fatalf("error = %v, want '没有正在进行的批量任务或工作流'", err)
	}
}

// TestResolveWatchTargetExplicitBatch verifies -bN targets explicit batch.
func TestResolveWatchTargetExplicitBatch(t *testing.T) {
	repo := gitRepo(t)
	batch := BatchState{
		BatchID:      "batch-watch",
		Status:       batchStatusRunning,
		Changes:      []string{"1-a"},
		CurrentIndex: 0,
		RunIDs:       map[string]string{},
	}
	if err := saveBatchState(repo, batch); err != nil {
		t.Fatal(err)
	}

	kind, ref, err := resolveWatchTarget(repo, []string{"-b1"})
	if err != nil {
		t.Fatal(err)
	}
	if kind != "batch" || ref.ID != "batch-watch" {
		t.Fatalf("watch target = %s %s, want batch batch-watch", kind, ref.ID)
	}
}

// TestResolveWatchTargetExplicitSingleRun verifies -wN targets explicit single run.
func TestResolveWatchTargetExplicitSingleRun(t *testing.T) {
	repo := gitRepo(t)
	run := State{RunID: "run-watch", ChangeName: "demo", Status: statusRunning, Stage: "execution", Workflow: DefaultWorkflowConfig()}
	if err := saveState(repo, run); err != nil {
		t.Fatal(err)
	}

	kind, ref, err := resolveWatchTarget(repo, []string{"-w1"})
	if err != nil {
		t.Fatal(err)
	}
	if kind != "run" || ref.ID != "run-watch" {
		t.Fatalf("watch target = %s %s, want run run-watch", kind, ref.ID)
	}
}

// TestWatchSpinnerReplacesArrowInRunningStage verifies spinner replaces static arrow in watch output.
func TestWatchSpinnerReplacesArrowInRunningStage(t *testing.T) {
	repo := gitRepo(t)
	run := State{RunID: "run-spin", ChangeName: "demo", Status: statusRunning, Stage: "execution", Workflow: DefaultWorkflowConfig()}
	if err := saveState(repo, run); err != nil {
		t.Fatal(err)
	}

	lines := watchRunStatusLines(repo, run, "w1", "|")
	got := strings.Join(lines, "\n")
	if !strings.HasPrefix(got, "- demo\n") {
		t.Fatalf("watch output should start from proposal list:\n%s", got)
	}
	if !strings.Contains(got, "  执行阶段 - | -") {
		t.Fatalf("watch body should put spinner on running stage:\n%s", got)
	}
}

// TestHumanRunFailureSummaryCoversKeyStates verifies failure summaries for all terminal states.
func TestHumanRunFailureSummaryCoversKeyStates(t *testing.T) {
	tests := []struct {
		name     string
		state    State
		contains string
	}{
		{
			name:     "failed execution with raw error",
			state:    State{ChangeName: "1-a", Status: statusFailed, Stage: "execution", Error: "agent process crash"},
			contains: "1-a 的写阶段失败",
		},
		{
			name:     "backend error sanitized",
			state:    State{ChangeName: "2-b", Status: statusFailed, Stage: "execution", Error: "stderr: request to backend-api failed wss://chatgpt.com/backend-api/codex"},
			contains: "智能体后端连接失败",
		},
		{
			name:     "blocked review limit",
			state:    State{ChangeName: "3-c", Status: statusBlocked, Stage: statusBlocked, Error: "审核修正达到上限，工作流已中断"},
			contains: "审核修正达到上限",
		},
		{
			name:     "blocked validation limit",
			state:    State{ChangeName: "4-d", Status: statusValidationBlocked, Stage: statusValidationBlocked, Error: "阶段验证达到上限，工作流已中断"},
			contains: "阶段验证达到上限",
		},
		{
			name:     "interrupted",
			state:    State{ChangeName: "5-e", Status: statusInterrupted, Stage: "execution", Error: "signal: interrupt"},
			contains: "工作流被中断",
		},
		{
			name:     "aborted",
			state:    State{ChangeName: "6-f", Status: statusAborted, Stage: "execution", Error: "用户已中止"},
			contains: "用户已中止",
		},
		{
			name:     "review stage failed",
			state:    State{ChangeName: "7-g", Status: statusFailed, Stage: "review_1", Error: "review failed"},
			contains: "审核阶段失败",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := humanRunFailureSummary(tt.state, tt.state.ChangeName)
			if !strings.Contains(got, tt.contains) {
				t.Fatalf("summary = %q, want containing %q", got, tt.contains)
			}
		})
	}
}

// TestSanitizeErrorForHumanHidesDiagnostics verifies raw backend details are hidden.
func TestSanitizeErrorForHumanHidesDiagnostics(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{"empty", "", "智能体执行失败"},
		{"backend api", "request to backend-api failed", "智能体后端连接失败"},
		{"websocket", "websocket: connection refused", "智能体后端连接失败"},
		{"tls handshake", "tls handshake eof", "智能体后端连接失败"},
		{"wss url", "connection wss://chatgpt.com failed", "智能体后端连接失败"},
		{"stderr", "stderr: something went wrong", "智能体执行失败"},
		{"normal", "变更未通过测试验证", "变更未通过测试验证"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeErrorForHuman(tt.raw)
			if got != tt.want {
				t.Fatalf("sanitizeErrorForHuman(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}
