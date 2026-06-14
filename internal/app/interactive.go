// Package app owns the human interactive oz flow workflow menu.
package app

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// interactive shows the terminal menu and dispatches the selected action.
func interactive(ctx context.Context, stdin io.Reader, stdout io.Writer, repo string, engine *Engine) error {
	reader := bufio.NewReader(stdin)
	var supersededRun string
	batchID, stoppedBatches, err := FindStartupBatches(repo)
	if err != nil {
		return err
	}
	if batchID != "" {
		fmt.Fprintf(stdout, "发现未完成批量任务：%s\n", batchID)
		fmt.Fprintln(stdout, "1. 恢复未完成批量任务")
		fmt.Fprintln(stdout, "2. 追加变更提案")
		fmt.Fprintln(stdout, "3. 进入规划阶段并追加新提案")
		fmt.Fprintln(stdout, "4. 中止未完成批量任务")
		fmt.Fprintln(stdout, "5. 开始新的执行")
		choice, err := promptChoice(reader, stdout, 5)
		if err != nil {
			return err
		}
		switch choice {
		case 1:
			return startDetachedBatchCommand(repo, batchID)
		case 2:
			changes, err := chooseAppendableBatchChanges(reader, stdout, repo, batchID)
			if err != nil {
				return err
			}
			return appendSelectedBatchChanges(stdout, repo, batchID, changes)
		case 3:
			if _, _, err := runPlanning(ctx, repo); err != nil {
				return err
			}
			changes, err := appendableBatchChanges(repo, batchID)
			if err != nil {
				return err
			}
			return appendCandidateChanges(reader, stdout, repo, batchID, changes)
		case 4:
			if err := AbortBatch(repo, batchID); err != nil {
				return err
			}
			return nil
		}
	}
	run, stopped, err := FindStartupRuns(repo)
	if err != nil {
		return err
	}
	if len(stoppedBatches) > 0 || len(stopped) > 0 {
		printStoppedHistory(stdout, repo, stoppedBatches, stopped)
	}
	if run != "" {
		fmt.Fprintf(stdout, "发现未完成 run：%s\n", run)
		if state, err := loadState(repo, run); err == nil {
			printHumanStageChecklist(stdout, state)
		}
		fmt.Fprintln(stdout, "1. 恢复未完成 run")
		fmt.Fprintln(stdout, "2. 中止未完成 run")
		fmt.Fprintln(stdout, "3. 开始新的执行")
		choice, err := promptChoice(reader, stdout, 3)
		if err != nil {
			return err
		}
		switch choice {
		case 1:
			return engine.ResumeDetachedAfterUserChoice(ctx, run)
		case 2:
			if err := AbortRun(repo, run); err != nil {
				return err
			}
			return nil
		case 3:
			supersededRun = run
		}
	}

	changes, err := ListChanges(repo)
	if err != nil {
		return err
	}
	planned := false
	if len(changes) == 0 {
		tool, sessionID, err := runPlanning(ctx, repo)
		if err != nil {
			return err
		}
		engine.PlanningTool = tool
		engine.PlanningSessionID = sessionID
		planned = true
		changes, err = ListChanges(repo)
		if err != nil {
			return err
		}
		if len(changes) == 0 {
			return nil
		}
	} else {
		fmt.Fprintln(stdout, "1. 进入规划阶段")
		fmt.Fprintln(stdout, "2. 选择已有变更")
		choice, err := promptChoice(reader, stdout, 2)
		if err != nil {
			return err
		}
		if choice == 1 {
			tool, sessionID, err := runPlanning(ctx, repo)
			if err != nil {
				return err
			}
			engine.PlanningTool = tool
			engine.PlanningSessionID = sessionID
			planned = true
			changes, err = ListChanges(repo)
			if err != nil {
				return err
			}
			if len(changes) == 0 {
				return nil
			}
		}
	}
	changes = SortChangesByNumericPrefix(changes)
	if planned && len(changes) == 1 {
		if supersededRun != "" {
			if err := ArchiveSupersededRun(repo, supersededRun); err != nil {
				return err
			}
		}
		return engine.SubmitBatch(ctx, changes)
	}
	changes, err = chooseFromChanges(reader, stdout, changes)
	if err != nil {
		return err
	}
	if supersededRun != "" {
		if err := ArchiveSupersededRun(repo, supersededRun); err != nil {
			return err
		}
	}
	return engine.SubmitBatch(ctx, changes)
}

// chooseAppendableBatchChanges prompts only for changes not already in the batch.
func chooseAppendableBatchChanges(reader *bufio.Reader, stdout io.Writer, repo, batchID string) ([]Change, error) {
	changes, err := appendableBatchChanges(repo, batchID)
	if err != nil {
		return nil, err
	}
	if len(changes) == 0 {
		fmt.Fprintln(stdout, "没有可追加的 active 变更提案")
		return nil, nil
	}
	return chooseFromChanges(reader, stdout, changes)
}

// appendableBatchChanges loads sorted active changes that are not queued yet.
func appendableBatchChanges(repo, batchID string) ([]Change, error) {
	batch, err := loadBatchState(repo, batchID)
	if err != nil {
		return nil, err
	}
	changes, err := ListChanges(repo)
	if err != nil {
		return nil, err
	}
	return FilterChangesNotInBatch(SortChangesByNumericPrefix(changes), batch), nil
}

// appendCandidateChanges appends all of one candidate or prompts for many.
func appendCandidateChanges(reader *bufio.Reader, stdout io.Writer, repo, batchID string, changes []Change) error {
	switch len(changes) {
	case 0:
		fmt.Fprintln(stdout, "没有可追加的 active 变更提案")
		return nil
	case 1:
		return appendSelectedBatchChanges(stdout, repo, batchID, changes)
	default:
		selected, err := chooseFromChanges(reader, stdout, changes)
		if err != nil {
			return err
		}
		return appendSelectedBatchChanges(stdout, repo, batchID, selected)
	}
}

// appendSelectedBatchChanges writes selected active changes into an existing batch.
func appendSelectedBatchChanges(stdout io.Writer, repo, batchID string, changes []Change) error {
	if len(changes) == 0 {
		return nil
	}
	added, skipped, err := AppendBatchChanges(repo, batchID, changes)
	if err != nil {
		return err
	}
	if len(added) == 0 {
		fmt.Fprintln(stdout, "没有追加新变更，所选提案已在批量任务中")
	} else {
		fmt.Fprintf(stdout, "已追加 %d 个变更到批量任务 %s\n", len(added), batchID)
		for _, name := range added {
			fmt.Fprintf(stdout, "- %s\n", name)
		}
	}
	if len(skipped) > 0 {
		fmt.Fprintf(stdout, "已跳过 %d 个重复提案\n", len(skipped))
		for _, name := range skipped {
			fmt.Fprintf(stdout, "- %s\n", name)
		}
	}
	return nil
}

// chooseFromChanges renders a prepared active-change candidate list.
func chooseFromChanges(reader *bufio.Reader, stdout io.Writer, changes []Change) ([]Change, error) {
	if len(changes) == 1 {
		return append([]Change(nil), changes...), nil
	}
	for i, change := range changes {
		fmt.Fprintf(stdout, "%d. %s\n", i+1, change.Name)
	}
	fmt.Fprint(stdout, "> ")
	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return nil, err
	}
	return ParseChangeSelection(line, changes)
}

// promptChoice reads a one-based numeric menu selection.
func promptChoice(reader *bufio.Reader, stdout io.Writer, max int) (int, error) {
	fmt.Fprint(stdout, "> ")
	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return 0, err
	}
	n, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil || n < 1 || n > max {
		return 0, fmt.Errorf("无效选择")
	}
	return n, nil
}
