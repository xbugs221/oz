// Package app dispatches repository-scoped oz flow commands after base startup wiring.
package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
)

// dispatchRepositoryCommand routes commands that require a repository, context, and engine.
func dispatchRepositoryCommand(ctx context.Context, args []string, stdout io.Writer, repo string, engine *Engine) error {
	switch args[0] {
	case "config":
		options, err := parseConfigCommandOptions(args[1:])
		if err != nil {
			return err
		}
		if options.ListProfiles {
			printWorkflowProfiles(stdout)
			return nil
		}
		if options.Global {
			path, err := WriteWorkflowConfigProfile("", true, options.Profile)
			if err != nil {
				return err
			}
			fmt.Fprintf(stdout, "已创建全局配置 %s\n", path)
			return nil
		}
		path, err := WriteWorkflowConfigProfile(repo, false, options.Profile)
		if err != nil {
			return err
		}
		fmt.Fprintf(stdout, "已创建 %s\n", filepath.Base(path))
		return nil
	case "init":
		return fmt.Errorf("oz flow init 已移除，请改用 oz flow config")
	case "install":
		return fmt.Errorf("oz flow install 已移除，prompt 已内嵌在 oz-flow.yaml 中；请改用 oz flow config 或 oz flow config --global")
	case "contract":
		if !hasFlag(args[1:], "--json") {
			return fmt.Errorf("用法：oz flow contract --json")
		}
		return writeRunnerContract(stdout)
	case "list-changes":
		if !hasFlag(args[1:], "--json") {
			return fmt.Errorf("用法：oz flow list-changes --json")
		}
		changes, err := ListChanges(repo)
		if err != nil {
			return err
		}
		return writeChangeList(stdout, changes)
	case "run":
		return dispatchRunCommand(ctx, args, stdout, repo, engine)
	case "r":
		if len(args) != 1 {
			return fmt.Errorf("用法：oz flow r")
		}
		return submitAllActiveChanges(ctx, stdout, repo, engine)
	case "resume":
		return dispatchResumeCommand(ctx, args, stdout, repo, engine)
	case "batch":
		return dispatchBatchCommand(ctx, args, stdout, repo, engine)
	case "restart":
		return dispatchRestartCommand(ctx, args, stdout, repo, engine)
	case "status":
		return dispatchStatusCommand(args, stdout, repo)
	case "abort":
		return dispatchAbortCommand(args, stdout, repo)
	case "clean":
		for _, arg := range args[1:] {
			if arg != "--agent-sessions" {
				return fmt.Errorf("用法：oz flow clean [--agent-sessions]")
			}
		}
		return runClean(stdout, repo, args[1:]...)
	case "watch":
		return runWatch(stdout, repo, args[1:]...)
	case "--list-changes":
		return printChanges(stdout, repo)
	case "--resume":
		return engine.Resume(ctx)
	case "--run":
		if len(args) != 2 {
			return fmt.Errorf("用法：oz flow --run <change-name>")
		}
		if err := ValidateChange(repo, args[1]); err != nil {
			return err
		}
		return engine.SubmitBatch(ctx, []Change{{Name: args[1]}})
	default:
		return fmt.Errorf("未知命令 %q", args[0])
	}
}

// dispatchRunCommand handles human and JSON run entry points.
func dispatchRunCommand(ctx context.Context, args []string, stdout io.Writer, repo string, engine *Engine) error {
	if len(args) == 1 {
		return submitAllActiveChanges(ctx, stdout, repo, engine)
	}
	if !hasFlag(args[1:], "--json") {
		return fmt.Errorf("用法：oz flow run --change <change-name> [--engine go-dag] --json")
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
	return engine.StartJSON(ctx, changeName, stdout)
}

// dispatchResumeCommand handles JSON runner resume and preserves failure DTO output.
func dispatchResumeCommand(ctx context.Context, args []string, stdout io.Writer, repo string, engine *Engine) error {
	if !hasFlag(args[1:], "--json") {
		return fmt.Errorf("用法：oz flow resume --run-id <run-id> --json")
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
				writeErr := writeRunnerState(stdout, state)
				warnWorkflowWrite("write locked runner state", writeErr)
				return errors.Join(err, writeErr)
			}
			state = failedState(state, err)
			saveErr := saveState(repo, state)
			warnWorkflowWrite("save failed resume state", saveErr)
			writeErr := writeFailedRunnerState(stdout, state, err)
			warnWorkflowWrite("write failed resume runner state", writeErr)
			return errors.Join(err, saveErr, writeErr)
		}
		writeErr := writeFailedRunnerError(stdout, runID, err)
		warnWorkflowWrite("write failed resume runner error", writeErr)
		return errors.Join(err, writeErr)
	}
	return nil
}

// dispatchBatchCommand handles batch continuation and append commands.
func dispatchBatchCommand(ctx context.Context, args []string, stdout io.Writer, repo string, engine *Engine) error {
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
			return fmt.Errorf("用法：oz flow batch append --batch-id <batch-id> --change <change-name> [--change <change-name> ...]")
		}
		changes := make([]Change, 0, len(changeNames))
		for _, name := range changeNames {
			changes = append(changes, Change{Name: name})
		}
		return appendSelectedBatchChanges(stdout, repo, batchID, changes)
	}
	if !hasFlag(args[1:], "--json") {
		return fmt.Errorf("用法：oz flow batch --batch-id <batch-id> --json")
	}
	engine.Output = nil
	return engine.RunBatch(ctx, batchID)
}

// dispatchRestartCommand handles JSON restart and human restart aliases.
func dispatchRestartCommand(ctx context.Context, args []string, stdout io.Writer, repo string, engine *Engine) error {
	if hasFlag(args[1:], "--json") {
		runID, runErr := optionalFlagValue(args[1:], "--run-id")
		batchID, batchErr := optionalFlagValue(args[1:], "--batch-id")
		if runErr != nil || batchErr != nil || (runID == "" && batchID == "") || (runID != "" && batchID != "") {
			return fmt.Errorf("用法：oz flow restart --run-id <run-id> --json 或 oz flow restart --batch-id <batch-id> --json")
		}
		engine.Output = nil
		if runID != "" {
			return engine.RestartRunJSON(ctx, runID, stdout)
		}
		return engine.RestartBatchJSON(ctx, batchID)
	}
	return restartHuman(ctx, stdout, repo, engine, args[1:])
}

// dispatchStatusCommand separates JSON status from human status rendering.
func dispatchStatusCommand(args []string, stdout io.Writer, repo string) error {
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
}

// dispatchAbortCommand aborts a JSON runner workflow and emits the final state.
func dispatchAbortCommand(args []string, stdout io.Writer, repo string) error {
	if !hasFlag(args[1:], "--json") {
		return fmt.Errorf("用法：oz flow abort --run-id <run-id> --json")
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
}
