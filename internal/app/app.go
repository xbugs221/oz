// Package app contains terminal orchestration for the Codex/oz workflow.
package app

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"
)

// Run parses command arguments and starts the interactive workflow.
func Run(args []string, stdin io.Reader, stdout, stderr io.Writer) error {
	if len(args) > 0 {
		if args[0] == "--version" {
			fmt.Fprintln(stdout, resolvedVersion())
			return nil
		}
		if args[0] == "--help" || args[0] == "-h" {
			printHelp(stdout)
			return nil
		}
		if args[0] == "validate-review" {
			return runValidateReviewArtifact(args[1:], stdout)
		}
		if args[0] == "validate-qa" {
			return runValidateQAArtifact(args[1:], stdout)
		}
		if args[0] == "update" {
			if len(args) != 1 {
				return fmt.Errorf("用法：oz flow update")
			}
			return runUpdate(stdout)
		}
		if args[0] == "config" {
			options, err := parseConfigCommandOptions(args[1:])
			if err != nil || !options.Global {
				return runWithRepository(args, stdin, stdout)
			}
			return handleFlowConfigCommand(context.Background(), args, stdout, "", nil)
		}
		if args[0] == "graph" {
			repo, err := GitRoot(".")
			if err != nil {
				return err
			}
			return runGraph(repo, args[1:], stdout)
		}
	}
	return runWithRepository(args, stdin, stdout)
}

// runWithRepository wires repository state, workflow tools, and registry dispatch.
func runWithRepository(args []string, stdin io.Reader, stdout io.Writer) error {
	repo, err := GitRoot(".")
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if len(args) == 0 || flowCommandRequiresWorkflowTools(args[0]) {
		if err := ensureBaseWorkflowCommands(); err != nil {
			return err
		}
	}
	registry := NewAgentRegistry()
	engine := NewEngine(repo, registry)
	engine.Output = stdout
	engine.inPlaceProgress = supportsInPlaceProgress(stdout)

	if len(args) > 0 {
		if spec, ok := flowCommandByName(args[0]); ok {
			return spec.Handler(ctx, args, stdout, repo, engine)
		}
		return fmt.Errorf("未知命令 %q", args[0])
	}

	return interactive(ctx, stdin, stdout, repo, engine)
}

// submitAllActiveChanges starts or extends the active change queue.
func submitAllActiveChanges(ctx context.Context, stdout io.Writer, repo string, engine *Engine) error {
	changes, err := ListChanges(repo)
	if err != nil {
		return err
	}
	if len(changes) == 0 {
		fmt.Fprintln(stdout, "没有 active 变更提案")
		return nil
	}
	batchID, err := FindUnfinishedBatch(repo)
	if err != nil {
		return err
	}
	if batchID != "" {
		batch, err := loadBatchState(repo, batchID)
		if err != nil {
			return err
		}
		appendable := FilterChangesNotInBatch(SortChangesByNumericPrefix(changes), batch)
		if len(appendable) == 0 {
			fmt.Fprintf(stdout, "已有运行中的批量任务 %s，没有可追加的 active 变更提案\n", batchID)
			return nil
		}
		fmt.Fprintf(stdout, "已有运行中的批量任务 %s，追加新的 active 变更提案\n", batchID)
		return appendSelectedBatchChanges(stdout, repo, batchID, appendable)
	}
	return engine.SubmitBatch(ctx, changes)
}

// flowCommandRequiresWorkflowTools reports whether a registered command invokes workflow backends.
func flowCommandRequiresWorkflowTools(name string) bool {
	spec, ok := flowCommandByName(name)
	return !ok || spec.NeedsWorkflowTools
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
				fmt.Fprintf(stdout, "  归档: 可运行 %s 归档失败记录后开启新工作流\n", archiveCommandForAlias(alias))
			} else {
				fmt.Fprintf(stdout, "  归档: 可运行 %s 归档失败记录后开启新工作流\n", archiveCommandForAlias(alias))
				fmt.Fprintf(stdout, "  清理: 可运行 oz flow clean 清理当前项目失败或异常运行态\n")
				fmt.Fprintf(stdout, "        该操作仅删除 oz flow 历史记录，不回滚代码改动\n")
			}
		} else if batch.Status == batchStatusAborted {
			fmt.Fprintf(stdout, "  清理: 可运行 oz flow clean 清理当前项目失败或异常运行态\n")
			fmt.Fprintf(stdout, "        该操作仅删除 oz flow 历史记录，不回滚代码改动\n")
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
				fmt.Fprintf(stdout, "  归档: 可运行 %s 归档失败记录后开启新工作流\n", archiveCommandForAlias(runAlias))
			} else {
				if isArchivableRun(state) {
					fmt.Fprintf(stdout, "  归档: 可运行 %s 归档失败记录后开启新工作流\n", archiveCommandForAlias(runAlias))
				}
				fmt.Fprintf(stdout, "  清理: 可运行 oz flow clean 清理当前项目失败或异常运行态\n")
				fmt.Fprintf(stdout, "        该操作仅删除 oz flow 历史记录，不回滚代码改动\n")
			}
		}
	}
}

// restartCommandForAlias returns the shortest human restart command for a status alias.
func restartCommandForAlias(alias string) string {
	if strings.HasPrefix(alias, "b") || strings.HasPrefix(alias, "w") {
		return "oz flow restart -" + alias
	}
	return "oz flow restart"
}

// archiveCommandForAlias returns the shortest human archive command for a status alias.
func archiveCommandForAlias(alias string) string {
	if strings.HasPrefix(alias, "b") || strings.HasPrefix(alias, "w") {
		return "oz flow archive -" + alias
	}
	return "oz flow archive"
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
	if state.Stage == statusBlocked || state.Stage == statusValidationBlocked || state.Stage == statusAcceptanceContractBlocked {
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
	case state.Status == statusAcceptanceContractBlocked || state.Stage == statusAcceptanceContractBlocked:
		reason := state.Error
		if reason == "" {
			reason = "验收合同预检未通过，不能自动继续"
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
	case stage == statusAcceptanceContractBlocked:
		return "验收预检"
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
		if len(args) == 0 && err.Error() == "没有 oz flow run" {
			fmt.Fprintln(stdout, err)
			return nil
		}
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
	for _, line := range runProposalStatusLines(repo, state, ref.Alias, "→") {
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
	fmt.Fprintln(stdout, "oz flow 通过封闭的 Codex/Pi 工作流执行 oz 变更。")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "用法：")
	fmt.Fprintln(stdout, "  oz flow")
	fmt.Fprintln(stdout, "  oz flow config [--global] [--profile <name>]")
	fmt.Fprintln(stdout, "  oz flow config --list-profiles")
	fmt.Fprintln(stdout, "  oz flow run | oz flow r")
	fmt.Fprintln(stdout, "  oz flow status")
	fmt.Fprintln(stdout, "  oz flow restart")
	fmt.Fprintln(stdout, "  oz flow stop")
	fmt.Fprintln(stdout, "  oz flow archive")
	fmt.Fprintln(stdout, "  oz flow loop")
	fmt.Fprintln(stdout, "  oz flow clean [--agent-sessions] [--dry-run] [--json]")
	fmt.Fprintln(stdout, "  oz flow watch")
	fmt.Fprintln(stdout, "  oz flow update")
	fmt.Fprintln(stdout, "  oz flow --list-changes")
	fmt.Fprintln(stdout, "  oz flow --run <change-name>")
	fmt.Fprintln(stdout, "  oz flow --resume")
	fmt.Fprintln(stdout, "  oz flow validate-review --artifact <artifact-path> [--json]")
	fmt.Fprintln(stdout, "  oz flow validate-qa --artifact <artifact-path> --acceptance <acceptance-path> [--json]")
	fmt.Fprintln(stdout, "  oz flow --version")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "人类交互命令：")
	fmt.Fprintln(stdout, "  oz flow                         进入规划、选择 active change，或恢复 run")
	fmt.Fprintln(stdout, "  oz flow clean [--agent-sessions] [--dry-run] [--json] 清理当前项目失败或异常运行态")
	fmt.Fprintln(stdout, "  oz flow config [--global] [--profile <name>] 写入仓库 oz-flow.yaml 或用户 ~/oz-flow.yaml（默认写入 default profile）")
	fmt.Fprintln(stdout, "  oz flow config --list-profiles  查看可用的 profile 列表")
	fmt.Fprintln(stdout, "  oz flow run | oz flow r              直接全选 active change 并启动任务队列")
	fmt.Fprintln(stdout, "  oz flow status                  打印最新 run 进度")
	fmt.Fprintln(stdout, "  oz flow restart [-bN|-wN]       重启最近失败或中断的批量任务/工作流")
	fmt.Fprintln(stdout, "  oz flow stop                    停止当前正在进行的批量工作流")
	fmt.Fprintln(stdout, "  oz flow archive [-bN|-wN]       归档最近失败的批量任务/工作流运行态")
	fmt.Fprintln(stdout, "  oz flow loop                    每分钟监控批量工作流，失败后归档并从未完成变更继续")
	fmt.Fprintln(stdout, "  oz flow watch [-bN|-wN]         持续刷新运行中的任务状态")
	fmt.Fprintln(stdout, "  oz flow update                  更新 oz flow 和 oz，并输出备份回滚命令")
	fmt.Fprintln(stdout, "  oz flow --list-changes          打印 active docs/changes 名称")
	fmt.Fprintln(stdout, "  oz flow --run <change-name>     为 active change 启动可追加任务队列")
	fmt.Fprintln(stdout, "  oz flow --resume                恢复最新未完成 run")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "Runner JSON 命令：")
	fmt.Fprintln(stdout, "  oz flow validate-review --artifact <artifact-path> [--json]")
	fmt.Fprintln(stdout, "  oz flow validate-qa --artifact <artifact-path> --acceptance <acceptance-path> [--json]")
	fmt.Fprintln(stdout, "  oz flow contract --json")
	fmt.Fprintln(stdout, "  oz flow list-changes --json")
	fmt.Fprintln(stdout, "  oz flow run --change <change-name> --json")
	fmt.Fprintln(stdout, "  oz flow run-acceptance --change <change-name> --json")
	fmt.Fprintln(stdout, "  oz flow resume --run-id <run-id> --json")
	fmt.Fprintln(stdout, "  oz flow restart --run-id <run-id> --json")
	fmt.Fprintln(stdout, "  oz flow restart --batch-id <batch-id> --json")
	fmt.Fprintln(stdout, "  oz flow status --run-id <run-id> --json")
	fmt.Fprintln(stdout, "  oz flow abort --run-id <run-id> --json")
	fmt.Fprintln(stdout, "  oz flow batch --batch-id <batch-id> --json")
	fmt.Fprintln(stdout, "  oz flow batch append --batch-id <batch-id> --change <change-name> --json")
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, "文件：")
	fmt.Fprintln(stdout, "  oz-flow.yaml                    仓库工作流配置，可用 oz flow config 创建")
	fmt.Fprintln(stdout, "  ~/oz-flow.yaml                  用户级默认工作流配置")
	fmt.Fprintln(stdout, "  docs/changes/<name>/       active oz changes")
	fmt.Fprintln(stdout, "  ${XDG_STATE_HOME:-~/.local/state}/oz/flow/repos/<repo-key>/runs/<run-id>/")
	fmt.Fprintln(stdout, "                             sealed run 状态、prompt 快照和 artifact")
}

// printStageChecklist writes a plain text workflow status summary.
func printStageChecklist(stdout io.Writer, state State) {
	for _, line := range compactStatusLines(buildHumanStatusView("", state, state.RunID, "→")) {
		fmt.Fprintln(stdout, line)
	}
}

// printHumanStageChecklist shows the durable current stage when no live runtime map exists.
func printHumanStageChecklist(stdout io.Writer, state State) {
	runtime := map[string]stageRuntime{}
	if state.Status == statusRunning && state.Stage != "" {
		runtime[state.Stage] = stageRuntime{}
	}
	_ = runtime
	for _, line := range compactStatusLines(buildHumanStatusView("", state, state.RunID, "→")) {
		fmt.Fprintln(stdout, line)
	}
	if state.BatchID == "" && isRestartableRunState(state) && (state.Status == statusFailed || state.Status == statusInterrupted) {
		fmt.Fprintln(stdout, "提示: 可运行 oz flow restart 重启最近失败任务")
	}
}

// printHumanStatusStageChecklist shows workflow status with run-local human artifacts.
func printHumanStatusStageChecklist(stdout io.Writer, repo string, state State) {
	runtime := map[string]stageRuntime{}
	if state.Status == statusRunning && state.Stage != "" {
		runtime[state.Stage] = stageRuntime{}
	}
	_ = runtime
	for _, line := range compactStatusLines(buildHumanStatusView(repo, state, state.RunID, "→")) {
		fmt.Fprintln(stdout, line)
	}
	if state.BatchID == "" && isRestartableRunState(state) && (state.Status == statusFailed || state.Status == statusInterrupted) {
		fmt.Fprintln(stdout, "提示: 可运行 oz flow restart 重启最近失败任务")
	}
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

// spinnerFrames are the ASCII spinner animation frames for oz flow watch.
var spinnerFrames = []string{"|", "/", "-", "\\"}

// runWatch implements the oz flow watch command with continuous status refresh.
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
			fmt.Fprint(stdout, "\x1b[H\x1b[2J")
		}
		for _, line := range lines {
			fmt.Fprintln(stdout, line)
		}
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
		return "", StatusRef{}, fmt.Errorf("用法：oz flow watch [-bN|-wN]")
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
		return "", StatusRef{}, fmt.Errorf("用法：oz flow watch [-bN|-wN]")
	}
}

type configCommandOptions struct {
	Global       bool
	Profile      string
	ListProfiles bool
}

func parseConfigCommandOptions(args []string) (configCommandOptions, error) {
	options := configCommandOptions{Profile: "default"}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--global":
			options.Global = true
		case "--list-profiles":
			options.ListProfiles = true
		case "--profile":
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "-") {
				return configCommandOptions{}, fmt.Errorf("缺少 --profile 的值")
			}
			options.Profile = args[i+1]
			i++
		default:
			return configCommandOptions{}, fmt.Errorf("用法：oz flow config [--global] [--profile <name>] 或 oz flow config --list-profiles")
		}
	}
	if options.ListProfiles && (options.Global || options.Profile != "default" || len(args) > 1) {
		return configCommandOptions{}, fmt.Errorf("用法：oz flow config --list-profiles")
	}
	return options, nil
}

func printWorkflowProfiles(stdout io.Writer) {
	for _, profile := range BuiltInWorkflowProfiles() {
		fmt.Fprintf(stdout, "%s\t%s\t适用场景：%s\n", profile.Name, profile.Description, profile.Scenario)
	}
}
