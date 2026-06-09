// Package app contains terminal orchestration for the Codex/oz workflow.
package app

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Run parses command arguments and starts the interactive workflow.
func Run(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	if len(args) > 0 {
		switch args[0] {
		case "--version":
			fmt.Fprintln(stdout, resolvedVersion())
			return nil
		case "--help", "-h":
			printHelp(stdout)
			return nil
		case "validate-review":
			return runValidateReviewArtifact(args[1:], stdout)
		case "update":
			if len(args) != 1 {
				return fmt.Errorf("用法：wo update")
			}
			return runUpdate(stdout)
		case "config":
			if !hasFlag(args[1:], "--global") {
				break
			}
			for _, arg := range args[1:] {
				if arg != "--global" {
					return fmt.Errorf("用法：wo config [--global]")
				}
			}
			path, err := WriteWorkflowConfig("", true)
			if err != nil {
				return err
			}
			fmt.Fprintf(stdout, "已创建全局配置 %s\n", path)
			return nil
		case "graph":
			repo, err := GitRoot(".")
			if err != nil {
				return err
			}
			return runGraph(repo, args[1:], stdout)
		}
	}
	repo, err := GitRoot(".")
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if commandNeedsWorkflowTools(args) {
		if err := ensureBaseWorkflowCommands(); err != nil {
			return err
		}
	}
	registry := NewAgentRegistry()
	engine := NewEngine(repo, registry)
	engine.Output = stdout
	engine.inPlaceProgress = supportsInPlaceProgress(stdout)

	if len(args) > 0 {
		switch args[0] {
		case "config":
			for _, arg := range args[1:] {
				if arg != "--global" {
					return fmt.Errorf("用法：wo config [--global]")
				}
			}
			global := hasFlag(args[1:], "--global")
			if global {
				path, err := WriteWorkflowConfig("", true)
				if err != nil {
					return err
				}
				fmt.Fprintf(stdout, "已创建全局配置 %s\n", path)
				return nil
			}
			path, err := WriteWorkflowConfig(repo, global)
			if err != nil {
				return err
			}
			fmt.Fprintf(stdout, "已创建 %s\n", filepath.Base(path))
			return nil
		case "init":
			return fmt.Errorf("wo init 已移除，请改用 wo config")
		case "install":
			return fmt.Errorf("wo install 已移除，prompt 已内嵌在 wo.yaml 中；请改用 wo config 或 wo config --global")
		case "contract":
			if !hasFlag(args[1:], "--json") {
				return fmt.Errorf("用法：wo contract --json")
			}
			return writeRunnerContract(stdout)
		case "list-changes":
			if !hasFlag(args[1:], "--json") {
				return fmt.Errorf("用法：wo list-changes --json")
			}
			changes, err := ListChanges(repo)
			if err != nil {
				return err
			}
			return writeChangeList(stdout, changes)
		case "run":
			if len(args) == 1 {
				return submitAllActiveChanges(ctx, stdout, repo, engine)
			}
			if !hasFlag(args[1:], "--json") {
				return fmt.Errorf("用法：wo run --change <change-name> [--engine go-dag] --json")
			}
			changeName, err := requireFlagValue(args[1:], "--change")
			if err != nil {
				return err
			}
			engine.Output = nil
			if engineName, _ := optionalFlagValue(args[1:], "--engine"); engineName != "" {
				if engineName != "go-dag" {
					return fmt.Errorf("workflow.engine 只支持 go-dag")
				}
			}
			if err := engine.StartJSON(ctx, changeName, stdout); err != nil {
				return err
			}
			return nil
		case "r":
			if len(args) != 1 {
				return fmt.Errorf("用法：wo r")
			}
			return submitAllActiveChanges(ctx, stdout, repo, engine)
		case "resume":
			if !hasFlag(args[1:], "--json") {
				return fmt.Errorf("用法：wo resume --run-id <run-id> --json")
			}
			runID, err := requireFlagValue(args[1:], "--run-id")
			if err != nil {
				return err
			}
			engine.Output = nil
			if err := engine.ResumeRunJSON(ctx, runID, stdout); err != nil {
				if state, loadErr := loadState(repo, runID); loadErr == nil {
					if isRunLockedError(err) {
						state.Error = err.Error()
						_ = writeRunnerState(stdout, state)
						return err
					}
					state = failedState(state, err)
					_ = saveState(repo, state)
					_ = writeFailedRunnerState(stdout, state, err)
				} else {
					_ = writeFailedRunnerError(stdout, runID, err)
				}
				return err
			}
			return nil
		case "batch":
			batchID, err := requireFlagValue(args[1:], "--batch-id")
			if err != nil {
				return err
			}
			if (len(args) > 1 && args[1] == "append") || hasFlag(args[1:], "--append") {
				var changeNames []string
				for i := 0; i < len(args[1:]); i++ {
					if args[1:][i] == "--change" && i+1 < len(args[1:]) {
						changeNames = append(changeNames, args[1:][i+1])
						i++
					}
				}
				if len(changeNames) == 0 {
					return fmt.Errorf("用法：wo batch append --batch-id <batch-id> --change <change-name> [--change <change-name> ...]")
				}
				changes := make([]Change, 0, len(changeNames))
				for _, name := range changeNames {
					changes = append(changes, Change{Name: name})
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
			if !hasFlag(args[1:], "--json") {
				return fmt.Errorf("用法：wo batch --batch-id <batch-id> --json")
			}
			engine.Output = nil
			return engine.RunBatch(ctx, batchID)
		case "restart":
			if hasFlag(args[1:], "--json") {
				runID, runErr := optionalFlagValue(args[1:], "--run-id")
				batchID, batchErr := optionalFlagValue(args[1:], "--batch-id")
				if runErr != nil || batchErr != nil || (runID == "" && batchID == "") || (runID != "" && batchID != "") {
					return fmt.Errorf("用法：wo restart --run-id <run-id> --json 或 wo restart --batch-id <batch-id> --json")
				}
				engine.Output = nil
				if runID != "" {
					return engine.RestartRunJSON(ctx, runID, stdout)
				}
				return engine.RestartBatchJSON(ctx, batchID)
			}
			return restartHuman(ctx, stdout, repo, engine, args[1:])
		case "status":
			if hasFlag(args[1:], "--json") {
				runID, err := requireFlagValue(args[1:], "--run-id")
				if err != nil {
					return err
				}
				state, err := loadState(repo, runID)
				if err != nil {
					_ = writeFailedRunnerError(stdout, runID, err)
					return err
				}
				runRefs, _ := ListRunRefs(repo)
				return writeJSON(stdout, runnerStateFromStatusView(repo, state, RunAliasForID(runRefs, runID)))
			}
			return printHumanStatus(stdout, repo, args[1:]...)
		case "abort":
			if !hasFlag(args[1:], "--json") {
				return fmt.Errorf("用法：wo abort --run-id <run-id> --json")
			}
			runID, err := requireFlagValue(args[1:], "--run-id")
			if err != nil {
				return err
			}
			if err := AbortRun(repo, runID); err != nil {
				_ = writeFailedRunnerError(stdout, runID, err)
				return err
			}
			state, err := loadState(repo, runID)
			if err != nil {
				return err
			}
			return writeRunnerState(stdout, state)
		case "clean":
			if len(args) != 1 {
				return fmt.Errorf("用法：wo clean")
			}
			return runClean(stdout, repo)
		case "watch":
			return runWatch(stdout, repo, args[1:]...)
		case "--list-changes":
			return printChanges(stdout, repo)
		case "--resume":
			return engine.Resume(ctx)
		case "--run":
			if len(args) != 2 {
				return fmt.Errorf("用法：wo --run <change-name>")
			}
			if err := ValidateChange(repo, args[1]); err != nil {
				return err
			}
			return engine.SubmitBatch(ctx, []Change{{Name: args[1]}})
		default:
			return fmt.Errorf("未知命令 %q", args[0])
		}
	}

	return interactive(ctx, stdin, stdout, repo, engine)
}

// submitAllActiveChanges starts a queue for every currently active oz change.
func submitAllActiveChanges(ctx context.Context, stdout io.Writer, repo string, engine *Engine) error {
	changes, err := ListChanges(repo)
	if err != nil {
		return err
	}
	if len(changes) == 0 {
		fmt.Fprintln(stdout, "没有 active 变更提案")
		return nil
	}
	return engine.SubmitBatch(ctx, changes)
}

// commandNeedsWorkflowTools reports whether the CLI path will invoke workflow backends.
func commandNeedsWorkflowTools(args []string) bool {
	if len(args) == 0 {
		return true
	}
	switch args[0] {
	case "--help", "-h", "--version", "config", "init", "install", "contract", "validate-review", "list-changes", "status", "update", "abort", "watch", "clean", "graph", "--list-changes":
		return false
	default:
		return true
	}
}

// GitRoot returns the absolute root of the current git repository.
func GitRoot(dir string) (string, error) {
	gitPath, err := resolveCommand("git")
	if err != nil {
		return "", err
	}
	cmd := commandContext(context.Background(), gitPath, "rev-parse", "--show-toplevel")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("当前目录不在 git 仓库内：%w", err)
	}
	return filepath.Clean(strings.TrimSpace(string(out))), nil
}

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

// runPlanning starts a normal Codex TUI session for human planning.
func runPlanning(ctx context.Context, repo string) (string, string, error) {
	rendered, options, err := planningPrompt(repo)
	if err != nil {
		return "", "", err
	}
	registry := NewAgentRegistry()
	tool, err := registry.Tool(options.Tool)
	if err != nil {
		return "", "", err
	}
	if err := tool.Resolve(); err != nil {
		return "", "", err
	}
	cmd, err := tool.PlanningCommand(ctx, repo, rendered, os.Stdin, options)
	if err != nil {
		return "", "", err
	}
	sessionFile, err := os.CreateTemp("", "wo-planning-session-*")
	if err != nil {
		return "", "", err
	}
	sessionPath := sessionFile.Name()
	_ = sessionFile.Close()
	defer os.Remove(sessionPath)
	env := cmd.Env
	if env == nil {
		env = os.Environ()
	}
	cmd.Env = append(env, "WO_PLANNING_SESSION_FILE="+sessionPath)
	cmd.Dir = repo
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", "", err
	}
	return options.Tool, planningSessionIDFromFile(sessionPath), nil
}

func planningSessionIDFromFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// planningPrompt renders the human planning prompt through the public template name.
func planningPrompt(repo string) (string, StageOptions, error) {
	workflow, err := LoadWorkflowConfig(repo)
	if err != nil {
		return "", StageOptions{}, err
	}
	name, err := promptNameForStage("planning")
	if err != nil {
		return "", StageOptions{}, err
	}
	prompt, err := promptForName(workflow, name)
	if err != nil {
		return "", StageOptions{}, err
	}
	context := promptContext(repo, State{RunID: "planning", Stage: "planning", Workflow: workflow})
	rendered, err := renderPromptTemplate(name, prompt, context)
	if err != nil {
		return "", StageOptions{}, err
	}
	options, err := workflow.StageOption("planning")
	if err != nil {
		return "", StageOptions{}, err
	}
	return rendered, options, nil
}

// codexPlanningCommand keeps human planning interactive while passing the seed prompt directly.
func codexPlanningCommand(ctx context.Context, path, prompt string, stdin io.Reader, options StageOptions) *exec.Cmd {
	args := []string{"--dangerously-bypass-approvals-and-sandbox"}
	if options.Model != "" {
		args = append(args, "-m", options.Model)
	}
	if options.Reasoning != "" {
		args = append(args, "-c", "model_reasoning_effort="+options.Reasoning)
	}
	if options.Fast {
		args = append(args, "--enable", "fast_mode")
	} else {
		args = append(args, "--disable", "fast_mode")
	}
	args = append(args, prompt)
	cmd := commandContext(ctx, path, args...)
	cmd.Stdin = stdin
	return cmd
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

// printStoppedHistory writes stopped batch and workflow notices in a structured form.
func printStoppedHistory(stdout io.Writer, repo string, batches []BatchState, runs []State) {
	batchRefs, _ := ListBatchRefs(repo)
	runRefs, _ := ListRunRefs(repo)
	fmt.Fprintln(stdout, "检测到已停止的历史任务：")
	for _, batch := range batches {
		alias := batchAliasForID(batchRefs, batch.BatchID)
		fmt.Fprintf(stdout, "- 批量任务 %s %s %s\n", alias, batch.BatchID, batch.Status)
		if batch.FailedChange != "" {
			fmt.Fprintf(stdout, "  change: %s\n", batch.FailedChange)
		}
		if batch.FailedRunID != "" {
			runAlias := RunAliasForID(runRefs, batch.FailedRunID)
			if runAlias == "" {
				runAlias = batch.FailedRunID
			}
			fmt.Fprintf(stdout, "  run: %s %s\n", runAlias, batch.FailedRunID)
		}
		if summary := humanBatchFailureSummary(repo, batch, batch.FailedChange, batch.FailedRunID); summary != "" {
			fmt.Fprintf(stdout, "  reason: %s\n", summary)
		}
		if batch.Status == batchStatusFailed {
			if isBatchRestartRecoverable(repo, batch) {
				fmt.Fprintf(stdout, "  提示: 可运行 %s 删除失败记录并继续该批量任务\n", restartCommandForAlias(alias))
			} else {
				fmt.Fprintf(stdout, "  清理: 可运行 wo clean 清理当前项目失败或异常运行态\n")
				fmt.Fprintf(stdout, "        该操作仅删除 wo 历史记录，不回滚代码改动\n")
			}
		} else if batch.Status == batchStatusAborted {
			fmt.Fprintf(stdout, "  清理: 可运行 wo clean 清理当前项目失败或异常运行态\n")
			fmt.Fprintf(stdout, "        该操作仅删除 wo 历史记录，不回滚代码改动\n")
		}
	}
	for _, state := range runs {
		runAlias := RunAliasForID(runRefs, state.RunID)
		if runAlias == "" {
			runAlias = state.RunID
		}
		fmt.Fprintf(stdout, "- 工作流 %s %s %s\n", runAlias, state.RunID, state.Status)
		if state.ChangeName != "" {
			fmt.Fprintf(stdout, "  change: %s\n", state.ChangeName)
		}
		if reason := stoppedRunReason(state); reason != "" {
			fmt.Fprintf(stdout, "  reason: %s\n", reason)
		}
		if state.BatchID == "" {
			if isRestartableRunCandidate(repo, state) {
				fmt.Fprintf(stdout, "  提示: 可运行 %s 重启该工作流\n", restartCommandForAlias(runAlias))
			} else {
				fmt.Fprintf(stdout, "  清理: 可运行 wo clean 清理当前项目失败或异常运行态\n")
				fmt.Fprintf(stdout, "        该操作仅删除 wo 历史记录，不回滚代码改动\n")
			}
		}
	}
}

// restartCommandForAlias returns the shortest human restart command for a status alias.
func restartCommandForAlias(alias string) string {
	if strings.HasPrefix(alias, "b") || strings.HasPrefix(alias, "w") {
		return "wo restart -" + alias
	}
	return "wo restart"
}

// batchAliasForID returns a batch short alias or falls back to the real id.
func batchAliasForID(refs []StatusRef, batchID string) string {
	for _, ref := range refs {
		if ref.ID == batchID {
			return ref.Alias
		}
	}
	return batchID
}

// stoppedBatchReason chooses the shortest useful reason for a stopped batch.
func stoppedBatchReason(repo string, batch BatchState) string {
	if batch.FailedRunID != "" {
		if state, err := loadState(repo, batch.FailedRunID); err == nil {
			if reason := stoppedRunReason(state); reason != "" {
				return reason
			}
		}
	}
	if batch.Status != "" {
		return batch.Status
	}
	return batch.Error
}

// stoppedRunReason chooses a stable status enum before falling back to error text.
func stoppedRunReason(state State) string {
	if state.Status != "" && state.Status != statusFailed {
		return state.Status
	}
	if state.Stage == statusBlocked || state.Stage == statusValidationBlocked {
		return state.Stage
	}
	if state.Status != "" {
		return state.Status
	}
	return sanitizeErrorForHuman(state.Error)
}

// humanRunFailureSummary produces a Chinese failure reason with change name and stage role.
func humanRunFailureSummary(state State, changeName string) string {
	if changeName == "" {
		changeName = state.ChangeName
	}
	prefix := ""
	if changeName != "" {
		prefix = changeName + " 的"
	}
	stageRole := humanStageRole(state.Stage)

	switch {
	case state.Status == statusBlocked || state.Stage == statusBlocked:
		reason := state.Error
		if reason == "" {
			reason = "审核修正达到上限，不能自动继续"
		}
		return prefix + stageRole + "失败：" + reason
	case state.Status == statusValidationBlocked || state.Stage == statusValidationBlocked:
		reason := state.Error
		if reason == "" {
			reason = "阶段验证达到上限，不能自动继续"
		}
		return prefix + stageRole + "失败：" + reason
	case state.Status == statusAborted || state.Status == "aborted":
		reason := state.Error
		if reason == "" {
			reason = "用户已中止"
		}
		return prefix + reason
	case state.Status == statusInterrupted:
		return prefix + stageRole + "失败：工作流被中断"
	case state.Status == statusFailed:
		return prefix + stageRole + "失败：" + sanitizeErrorForHuman(state.Error)
	default:
		return prefix + stageRole + "失败：" + sanitizeErrorForHuman(state.Error)
	}
}

// humanStageRole maps stage names to short Chinese role labels.
func humanStageRole(stage string) string {
	switch {
	case stage == "execution":
		return "写阶段"
	case strings.HasPrefix(stage, "review_"):
		return "审核阶段"
	case strings.HasPrefix(stage, "fix_"):
		return "修正阶段"
	case stage == "archive":
		return "归档阶段"
	case stage == statusBlocked:
		return "审核阶段"
	case stage == statusValidationBlocked:
		return "阶段验证"
	default:
		return "当前阶段"
	}
}

// sanitizeErrorForHuman hides raw backend diagnostics from human-readable output.
func sanitizeErrorForHuman(raw string) string {
	if raw == "" {
		return "智能体执行失败"
	}
	lower := strings.ToLower(raw)
	switch {
	case strings.Contains(lower, "backend-api") || strings.Contains(lower, "wss://") ||
		strings.Contains(lower, "websocket") || strings.Contains(lower, "tls handshake"):
		return "智能体后端连接失败"
	case strings.Contains(lower, "stderr"):
		return "智能体执行失败"
	default:
		return raw
	}
}

// printChanges writes active change names for script-friendly usage.
func printChanges(stdout io.Writer, repo string) error {
	changes, err := ListChanges(repo)
	if err != nil {
		return err
	}
	for _, change := range changes {
		fmt.Fprintln(stdout, change.Name)
	}
	return nil
}

// printHumanStatus writes the selected human status target.
func printHumanStatus(stdout io.Writer, repo string, args ...string) error {
	kind, ref, err := ResolveStatusTarget(repo, args)
	if err != nil {
		return err
	}
	if kind == "batch" {
		batch, err := loadBatchState(repo, ref.ID)
		if err != nil {
			return err
		}
		runRefs, err := ListRunRefs(repo)
		if err != nil {
			return err
		}
		if len(args) == 0 {
			fmt.Fprintf(stdout, "正在查看 %s 最近一次批量工作流，如需查看普通工作流，请使用 wo status -w1\n", projectName(repo))
		}
		lines := batchStatusLines(repo, &batch, ref.Alias, runRefs)
		for _, line := range lines {
			fmt.Fprintln(stdout, line)
		}
		printUpdateHint(stdout)
		return nil
	}

	state, err := loadState(repo, ref.ID)
	if err != nil {
		return err
	}
	for _, line := range compactStatusLines(buildStatusView(repo, state, ref.Alias, "→")) {
		fmt.Fprintln(stdout, line)
	}
	printUpdateHint(stdout)
	return nil
}

// projectName returns the repository directory name for human status hints.
func projectName(repo string) string {
	name := filepath.Base(filepath.Clean(repo))
	if name == "." || name == string(filepath.Separator) {
		return "当前项目"
	}
	return name
}

// printHelp writes supported command-line options.
func printHelp(stdout io.Writer) {
	fmt.Fprintln(stdout, "wo 通过封闭的 Codex/OpenCode/Pi 工作流执行 oz 变更。")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "用法：")
	fmt.Fprintln(stdout, "  wo")
	fmt.Fprintln(stdout, "  wo config [--global]")
	fmt.Fprintln(stdout, "  wo run | wo r")
	fmt.Fprintln(stdout, "  wo status")
	fmt.Fprintln(stdout, "  wo restart")
	fmt.Fprintln(stdout, "  wo clean")
	fmt.Fprintln(stdout, "  wo watch")
	fmt.Fprintln(stdout, "  wo update")
	fmt.Fprintln(stdout, "  wo --list-changes")
	fmt.Fprintln(stdout, "  wo --run <change-name>")
	fmt.Fprintln(stdout, "  wo --resume")
	fmt.Fprintln(stdout, "  wo validate-review --artifact <artifact-path> [--json]")
	fmt.Fprintln(stdout, "  wo --version")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "人类交互命令：")
	fmt.Fprintln(stdout, "  wo                         进入规划、选择 active change，或恢复 run")
	fmt.Fprintln(stdout, "  wo clean                   清理当前项目失败或异常运行态")
	fmt.Fprintln(stdout, "  wo config [--global]       写入仓库 wo.yaml 或用户 ~/wo.yaml")
	fmt.Fprintln(stdout, "  wo run | wo r              直接全选 active change 并启动任务队列")
	fmt.Fprintln(stdout, "  wo status                  打印最新 run 进度")
	fmt.Fprintln(stdout, "  wo restart [-bN|-wN]       重启最近失败或中断的批量任务/工作流")
	fmt.Fprintln(stdout, "  wo watch [-bN|-wN]         持续刷新运行中的任务状态")
	fmt.Fprintln(stdout, "  wo update                  更新 wo 和 oz，并输出备份回滚命令")
	fmt.Fprintln(stdout, "  wo --list-changes          打印 active docs/changes 名称")
	fmt.Fprintln(stdout, "  wo --run <change-name>     为 active change 启动可追加任务队列")
	fmt.Fprintln(stdout, "  wo --resume                恢复最新未完成 run")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Runner JSON 命令：")
	fmt.Fprintln(stdout, "  wo validate-review --artifact <artifact-path> [--json]")
	fmt.Fprintln(stdout, "  wo contract --json")
	fmt.Fprintln(stdout, "  wo list-changes --json")
	fmt.Fprintln(stdout, "  wo run --change <change-name> --json")
	fmt.Fprintln(stdout, "  wo resume --run-id <run-id> --json")
	fmt.Fprintln(stdout, "  wo restart --run-id <run-id> --json")
	fmt.Fprintln(stdout, "  wo restart --batch-id <batch-id> --json")
	fmt.Fprintln(stdout, "  wo status --run-id <run-id> --json")
	fmt.Fprintln(stdout, "  wo abort --run-id <run-id> --json")
	fmt.Fprintln(stdout, "  wo batch --batch-id <batch-id> --json")
	fmt.Fprintln(stdout, "  wo batch append --batch-id <batch-id> --change <change-name> --json")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "文件：")
	fmt.Fprintln(stdout, "  wo.yaml                    仓库工作流配置，可用 wo config 创建")
	fmt.Fprintln(stdout, "  ~/wo.yaml                  用户级默认工作流配置")
	fmt.Fprintln(stdout, "  docs/changes/<name>/       active oz changes")
	fmt.Fprintln(stdout, "  ${XDG_STATE_HOME:-~/.local/state}/wo/repos/<repo-key>/runs/<run-id>/")
	fmt.Fprintln(stdout, "                             sealed run 状态、prompt 快照和 artifact")
}

// printStageChecklist writes a plain text workflow status summary.
func printStageChecklist(stdout io.Writer, state State) {
	for _, line := range stageChecklistLines(state, nil) {
		fmt.Fprintln(stdout, line)
	}
}

// printHumanStageChecklist shows the durable current stage when no live runtime map exists.
func printHumanStageChecklist(stdout io.Writer, state State) {
	runtime := map[string]stageRuntime{}
	if state.Status == statusRunning && state.Stage != "" {
		runtime[state.Stage] = stageRuntime{}
	}
	for _, line := range stageChecklistLines(state, runtime) {
		fmt.Fprintln(stdout, line)
	}
	if state.BatchID == "" && isRestartableRunState(state) && (state.Status == statusFailed || state.Status == statusInterrupted) {
		fmt.Fprintln(stdout, "提示: 可运行 wo restart 重启最近失败任务")
	}
}

// printHumanStatusStageChecklist shows workflow status with run-local human artifacts.
func printHumanStatusStageChecklist(stdout io.Writer, repo string, state State) {
	runtime := map[string]stageRuntime{}
	if state.Status == statusRunning && state.Stage != "" {
		runtime[state.Stage] = stageRuntime{}
	}
	for _, line := range stageChecklistLinesWithParallel(repo, state, runtime) {
		fmt.Fprintln(stdout, line)
	}
	if state.BatchID == "" && isRestartableRunState(state) && (state.Status == statusFailed || state.Status == statusInterrupted) {
		fmt.Fprintln(stdout, "提示: 可运行 wo restart 重启最近失败任务")
	}
}

// stageChecklistLines formats workflow status with optional runtime metadata.
func stageChecklistLines(state State, runtime map[string]stageRuntime) []string {
	return stageChecklistLinesForRepo("", state, runtime)
}

// stageChecklistLinesWithParallel formats human status lines with run-local parallel summaries.
func stageChecklistLinesWithParallel(repo string, state State, runtime map[string]stageRuntime) []string {
	return stageChecklistLinesForRepo(repo, state, runtime)
}

// stageChecklistLinesForRepo formats workflow status and optionally attaches run-local artifacts.
func stageChecklistLinesForRepo(repo string, state State, runtime map[string]stageRuntime) []string {
	engine := state.Engine
	if engine == "" {
		engine = state.Workflow.Engine
	}
	if engine == "" {
		engine = "go-dag"
	}
	var lines []string
	lines = append(lines, "- 引擎 "+engine)
	for _, item := range visibleSessionItems(state, runtime) {
		parts := []string{"-", item.label}
		if item.sessionID != "" {
			parts = append(parts, item.sessionID)
		}
		markers := strings.Repeat("✓", item.completed) + item.running
		if markers != "" {
			parts = append(parts, markers)
		}
		line := strings.Join(parts, " ")
		lines = append(lines, line)
		if repo != "" {
			lines = append(lines, parallelStatusLinesForRole(repo, state, item.role, "  ")...)
		}
	}
	if state.Status == statusBlocked || state.Stage == statusBlocked {
		reason := state.Error
		if reason == "" {
			reason = "审核修正达到上限，工作流已中断"
		}
		lines = append(lines, fmt.Sprintf("- 状态 %s x %s", statusBlocked, reason))
	}
	if state.Status == statusValidationBlocked || state.Stage == statusValidationBlocked {
		reason := state.Error
		if reason == "" {
			reason = "阶段验证达到上限，工作流已中断"
		}
		lines = append(lines, fmt.Sprintf("- 状态 %s x %s", statusValidationBlocked, reason))
	}
	if len(lines) == 0 {
		return []string{"- 写 未知 →"}
	}
	lines = append(lines, stageDurationSummaryLines(state, time.Now().UTC())...)
	return lines
}

type stageDurationItem struct {
	label   string
	stage   string
	minutes float64
}

// stageDurationSummaryLines formats human duration totals for executed stages only.
func stageDurationSummaryLines(state State, now time.Time) []string {
	items := stageDurationItems(state, now)
	if len(items) == 0 {
		return nil
	}
	parts := make([]string, 0, len(items))
	total := 0.0
	for _, item := range items {
		parts = append(parts, formatMinutes(item.minutes))
		total += item.minutes
	}
	lines := []string{fmt.Sprintf("- 耗时 %s分钟=%s", formatMinutes(total), strings.Join(parts, "+"))}
	for _, item := range items {
		lines = append(lines, fmt.Sprintf("  - %s %s %s分钟", item.label, item.stage, formatMinutes(item.minutes)))
	}
	return lines
}

type stageDurationBucket struct {
	label     string
	stage     string
	minutes   float64
	stageName string
	count     int
}

// stageDurationItems collects valid timing records in workflow display order.
func stageDurationItems(state State, now time.Time) []stageDurationItem {
	if len(state.StageTimings) == 0 {
		return nil
	}
	var order []string
	buckets := map[string]*stageDurationBucket{}
	for _, stage := range workflowStagesForState(state) {
		timing, ok := state.StageTimings[stage]
		if !ok || timing.StartedAt == "" {
			continue
		}
		startedAt, err := time.Parse(time.RFC3339Nano, timing.StartedAt)
		if err != nil {
			continue
		}
		finishedAt := now
		if timing.FinishedAt != "" {
			finishedAt, err = time.Parse(time.RFC3339Nano, timing.FinishedAt)
			if err != nil {
				continue
			}
		} else if state.Stage != stage || state.Status != statusRunning {
			continue
		}
		duration := finishedAt.Sub(startedAt)
		if duration < 0 {
			continue
		}
		role, err := roleForStage(stage)
		if err != nil {
			continue
		}
		key := role.Name
		bucket, ok := buckets[key]
		if !ok {
			bucket = &stageDurationBucket{label: role.Label, stage: role.Name, stageName: stage}
			buckets[key] = bucket
			order = append(order, key)
		}
		bucket.minutes += duration.Minutes()
		bucket.count++
	}
	items := make([]stageDurationItem, 0, len(order))
	for _, key := range order {
		bucket := buckets[key]
		stage := bucket.stage
		if bucket.count == 1 {
			stage = bucket.stageName
		}
		items = append(items, stageDurationItem{
			label:   bucket.label,
			stage:   stage,
			minutes: bucket.minutes,
		})
	}
	return items
}

// formatMinutes renders minutes with at most two decimal places for status text.
func formatMinutes(minutes float64) string {
	text := strconv.FormatFloat(minutes, 'f', 2, 64)
	text = strings.TrimRight(text, "0")
	text = strings.TrimRight(text, ".")
	if text == "" || text == "-0" {
		return "0"
	}
	return text
}

// stageTool returns the configured backend name for session lookup.
func stageTool(state State, stage string) string {
	ensureWorkflowConfig(&state)
	if options, ok := state.Workflow.Stages[stage]; ok && options.Tool != "" {
		return options.Tool
	}
	return "codex"
}

// stageChecklistSignature identifies the currently visible workflow state.
func stageChecklistSignature(state State) string {
	return stageChecklistSignatureWithRuntime(state, nil)
}

// stageChecklistSignatureWithRuntime identifies visible workflow state for duplicate suppression.
func stageChecklistSignatureWithRuntime(state State, runtime map[string]stageRuntime) string {
	parts := []string{state.RunID, state.Status, state.Stage}
	for _, item := range visibleSessionItems(state, runtime) {
		parts = append(parts, item.role+":"+item.sessionID+":"+strconv.Itoa(item.completed)+":"+item.running)
	}
	return strings.Join(parts, "|")
}

type visibleSessionItem struct {
	role      string
	label     string
	sessionID string
	completed int
	running   string
}

const preWorkflowCompletedLabel = "工作流开始之前就已完成"

// visibleSessionItems groups launched workflow stages by human session role.
func visibleSessionItems(state State, runtime map[string]stageRuntime) []visibleSessionItem {
	stages := workflowStagesForState(state)
	var items []visibleSessionItem
	for _, workflowRole := range statusRoles() {
		role := workflowRole.Session
		if role == "planner" {
			item := visibleSessionItem{role: role, label: sessionRoleLabel(role), sessionID: plannerSessionID(state)}
			if item.sessionID == "" {
				item.sessionID = preWorkflowCompletedLabel
				item.completed = 1
			}
			items = append(items, item)
			continue
		}
		item := visibleSessionItem{role: role, label: sessionRoleLabel(role), sessionID: sessionRoleID(state, role, stages, runtime)}
		for _, stage := range stages {
			if stageSessionRole(stage) != role {
				continue
			}
			if state.Stages[stage] == "completed" {
				item.completed++
			}
			if stage == state.Stage && state.Status == statusRunning && state.Stages[stage] != "completed" {
				item.running = "→"
			}
		}
		occurred := roleOccurred(state, role, stages, runtime)
		if item.completed == 0 && item.running == "" && item.sessionID == "" && !occurred {
			continue
		}
		if role == "executor" && item.sessionID == "" && item.completed > 0 && item.running == "" {
			item.sessionID = preWorkflowCompletedLabel
		}
		if item.sessionID == "" && (item.completed > 0 || item.running != "" || occurred) {
			item.sessionID = "未知"
		}
		items = append(items, item)
	}
	return items
}

func plannerSessionID(state State) string {
	ensureWorkflowConfig(&state)
	tool := "codex"
	if options, ok := state.Workflow.Stages["planning"]; ok && options.Tool != "" {
		tool = options.Tool
	}
	for _, key := range []string{sessionStateKey(tool, "planner"), sessionStateKey(tool, "planning"), "planner"} {
		if id := state.Sessions[key]; id != "" {
			return id
		}
	}
	return ""
}

// sessionRoleID returns the public session id for a grouped role line.
func sessionRoleID(state State, role string, stages []string, runtime map[string]stageRuntime) string {
	for _, stage := range stages {
		if stageSessionRole(stage) != role {
			continue
		}
		if meta, ok := runtime[stage]; ok {
			if meta.Thread != "" {
				return meta.Thread
			}
			if meta.Failed {
				return "失败"
			}
		}
	}
	for _, stage := range stages {
		if stageSessionRole(stage) != role {
			continue
		}
		if id := state.Sessions[sessionStateKey(stageTool(state, stage), role)]; id != "" {
			return id
		}
	}
	return ""
}

// roleOccurred reports whether any stage for a role has entered durable or live state.
func roleOccurred(state State, role string, stages []string, runtime map[string]stageRuntime) bool {
	for _, stage := range stages {
		if stageSessionRole(stage) != role {
			continue
		}
		if state.Stages[stage] != "" {
			return true
		}
		if state.Stage == stage && state.Status != "" && state.Status != statusDone {
			return true
		}
		if _, ok := runtime[stage]; ok {
			return true
		}
	}
	return false
}

// sessionRoleLabel maps agent roles to the short human status vocabulary.
func sessionRoleLabel(role string) string {
	for _, workflowRole := range statusRoles() {
		if workflowRole.Session == role {
			return workflowRole.Label
		}
	}
	return "写"
}

// supportsInPlaceProgress reports whether stdout is a terminal-like file.
func supportsInPlaceProgress(stdout io.Writer) bool {
	file, ok := stdout.(*os.File)
	if !ok {
		return false
	}
	info, err := file.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

// spinnerFrames are the ASCII spinner animation frames for wo watch.
var spinnerFrames = []string{"|", "/", "-", "\\"}

// runWatch implements the wo watch command with continuous status refresh.
func runWatch(stdout io.Writer, repo string, args ...string) error {
	targetKind, targetRef, err := resolveWatchTarget(repo, args)
	if err != nil {
		if strings.Contains(err.Error(), "没有正在进行的批量任务或工作流") {
			fmt.Fprintln(stdout, "当前没有正在进行的批量任务或工作流")
			return nil
		}
		return err
	}
	tty := supportsInPlaceProgress(stdout)
	frameIdx := 0
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	// Listen for interrupt signal to allow timeout-based testing.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)
	defer signal.Stop(sigCh)

	render := func() {
		var lines []string
		lines = append(lines, watchStatusLines(repo, targetKind, targetRef, spinnerFrames[frameIdx%len(spinnerFrames)])...)
		if tty && frameIdx > 0 {
			fmt.Fprintf(stdout, "\x1b[%dA\x1b[J", prevWatchLines)
		}
		for _, line := range lines {
			fmt.Fprintln(stdout, line)
		}
		prevWatchLines = len(lines)
		frameIdx++
	}

	render()
	for {
		select {
		case <-ticker.C:
			render()
		case <-sigCh:
			return nil
		}
	}
}

// prevWatchLines tracks how many lines to clear on the next watch refresh.
var prevWatchLines int

// resolveWatchTarget picks the watch target: explicit -bN/-wN or default to running batch > single-run.
func resolveWatchTarget(repo string, args []string) (kind string, ref StatusRef, err error) {
	if len(args) == 0 {
		// Default: running batch first, then running single-run.
		batchRefs, _ := ListBatchRefs(repo)
		for _, batchRef := range batchRefs {
			batch, loadErr := loadBatchState(repo, batchRef.ID)
			if loadErr == nil && batch.Status == batchStatusRunning {
				return "batch", batchRef, nil
			}
		}
		runID, runErr := FindUnfinishedRun(repo)
		if runErr == nil && runID != "" {
			runRefs, _ := ListRunRefs(repo)
			for _, runRef := range runRefs {
				if runRef.ID == runID {
					return "run", runRef, nil
				}
			}
			return "run", StatusRef{Alias: "w1", ID: runID}, nil
		}
		return "", StatusRef{}, fmt.Errorf("当前没有正在进行的批量任务或工作流")
	}
	if len(args) != 1 {
		return "", StatusRef{}, fmt.Errorf("用法：wo watch [-bN|-wN]")
	}
	arg := args[0]
	switch {
	case strings.HasPrefix(arg, "-b"):
		ref, err := resolveIndexedRef(repo, arg, "-b", ListBatchRefs)
		return "batch", ref, err
	case strings.HasPrefix(arg, "-w"):
		ref, err := resolveIndexedRef(repo, arg, "-w", ListRunRefs)
		return "run", ref, err
	default:
		return "", StatusRef{}, fmt.Errorf("用法：wo watch [-bN|-wN]")
	}
}

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
	currentPos := batch.CurrentIndex + 1
	if batch.Status == batchStatusDone {
		currentPos = len(batch.Changes)
	}
	if batchAlias == "" {
		batchAlias = batch.BatchID
	}
	lines = append(lines, fmt.Sprintf("%s %s %d/%d", spinner, batchAlias, currentPos, len(batch.Changes)))
	if batch.Status == batchStatusFailed || batch.Status == batchStatusAborted {
		lines = append(lines, batchFailureLines(repo, *batch, batchAlias)...)
	}

	for _, changeName := range batch.Changes {
		runID := batch.RunIDs[changeName]
		lines = append(lines, fmt.Sprintf("- %s", changeName))
		if runID != "" {
			if state, err := loadState(repo, runID); err == nil {
				runRefs, _ := ListRunRefs(repo)
				runAlias := RunAliasForID(runRefs, runID)
				for _, line := range compactStatusLines(buildStatusView(repo, state, runAlias, "→")) {
					lines = append(lines, fmt.Sprintf("  %s", line))
				}
			}
		}
	}

	return lines
}

// watchRunStatusLines formats a single run with spinner in the running stage.
func watchRunStatusLines(repo string, state State, runAlias string, spinner string) []string {
	var lines []string
	if runAlias == "" {
		runAlias = state.RunID
	}
	lines = append(lines, compactStatusLines(buildStatusView(repo, state, runAlias, spinner))...)
	if state.BatchID == "" && isRestartableRunState(state) && (state.Status == statusFailed || state.Status == statusInterrupted) {
		lines = append(lines, "提示: 可运行 wo restart 重启最近失败任务")
	}
	return lines
}

// watchStageChecklistLines is like stageChecklistLines but replaces → with spinner.
func watchStageChecklistLines(state State, runtime map[string]stageRuntime, spinner string) []string {
	var lines []string
	for _, item := range visibleSessionItems(state, runtime) {
		parts := []string{"-", item.label}
		if item.sessionID != "" {
			parts = append(parts, item.sessionID)
		}
		markers := strings.Repeat("✓", item.completed)
		if item.running == "→" {
			markers += spinner
		} else {
			markers += item.running
		}
		if markers != "" {
			parts = append(parts, markers)
		}
		line := strings.Join(parts, " ")
		lines = append(lines, line)
	}
	if state.Status == statusBlocked || state.Stage == statusBlocked {
		reason := state.Error
		if reason == "" {
			reason = "审核修正达到上限，工作流已中断"
		}
		lines = append(lines, fmt.Sprintf("- 状态 %s x %s", statusBlocked, reason))
	}
	if state.Status == statusValidationBlocked || state.Stage == statusValidationBlocked {
		reason := state.Error
		if reason == "" {
			reason = "阶段验证达到上限，工作流已中断"
		}
		lines = append(lines, fmt.Sprintf("- 状态 %s x %s", statusValidationBlocked, reason))
	}
	if len(lines) == 0 {
		return []string{"- 写 未知 " + spinner}
	}
	return lines
}
