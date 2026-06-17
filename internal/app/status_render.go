// Package app renders shared status views for human status and watch output.
package app

import (
	"fmt"
)

// watchStatusLines renders batch or single-run status with spinner for watch refresh.
func watchStatusLines(repo, kind string, ref StatusRef, spinner string) []string {
	if kind == "batch" {
		batch, err := loadBatchState(repo, ref.ID)
		if err != nil {
			return []string{fmt.Sprintf("批量任务 %s 状态读取失败: %v", ref.Alias, err)}
		}
		return watchBatchStatusLines(repo, &batch, ref.Alias, spinner)
	}
	state, err := loadState(repo, ref.ID)
	if err != nil {
		return []string{fmt.Sprintf("工作流 %s 状态读取失败: %v", ref.Alias, err)}
	}
	return watchRunStatusLines(repo, state, ref.Alias, spinner)
}

// watchBatchStatusLines formats a batch with spinner in the running stage.
func watchBatchStatusLines(repo string, batch *BatchState, batchAlias string, spinner string) []string {
	var lines []string
	if batchAlias == "" {
		batchAlias = batch.BatchID
	}

	for _, changeName := range batch.Changes {
		runID := batch.RunIDs[changeName]
		if runID != "" {
			if state, err := loadState(repo, runID); err == nil {
				runRefs, _ := ListRunRefs(repo)
				runAlias := RunAliasForID(runRefs, runID)
				view := buildHumanStatusView(repo, state, runAlias, spinner)
				lines = append(lines, statusHeaderText(changeName, view))
				for _, line := range compactStatusLines(view) {
					lines = append(lines, fmt.Sprintf("  %s", line))
				}
				if batch.Status == batchStatusRunning && isStaleRunningRun(repo, state) {
					lines = append(lines, fmt.Sprintf("  提示: 当前 run 的 lock 已失效，可运行 oz flow restart -%s 重试当前批量阶段", batchAlias))
				}
				continue
			}
		}
		lines = append(lines, fmt.Sprintf("- %s", changeName))
	}
	if batch.Status == batchStatusFailed || batch.Status == batchStatusAborted {
		lines = append(lines, batchFailureLines(repo, *batch, batchAlias)...)
	}

	return lines
}

// watchRunStatusLines formats a single run with spinner in the running stage.
func watchRunStatusLines(repo string, state State, runAlias string, spinner string) []string {
	if runAlias == "" {
		runAlias = state.RunID
	}
	lines := runProposalStatusLines(repo, state, runAlias, spinner)
	if state.BatchID == "" && isRestartableRunState(state) && (state.Status == statusFailed || state.Status == statusInterrupted) {
		lines = append(lines, "提示: 可运行 oz flow restart 重启最近失败任务")
	}
	return lines
}

// runProposalStatusLines renders one workflow under its change proposal list item.
func runProposalStatusLines(repo string, state State, runAlias string, runningMarker string) []string {
	view := buildHumanStatusView(repo, state, runAlias, runningMarker)
	lines := []string{statusHeaderText(state.ChangeName, view)}
	for _, line := range compactStatusLines(view) {
		lines = append(lines, fmt.Sprintf("  %s", line))
	}
	if isStaleRunningRun(repo, state) {
		lines = append(lines, "  提示: 当前 run 的 lock 已失效，可运行 oz flow restart 重试当前阶段")
	}
	return lines
}
