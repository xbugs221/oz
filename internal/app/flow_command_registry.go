// Package app defines the single registry for oz flow command behavior.
package app

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
)

type flowCommandHandler func(context.Context, []string, io.Writer, string, *Engine) error

type flowCommandSpec struct {
	Name               string
	NeedsRepo          bool
	NeedsWorkflowTools bool
	Handler            flowCommandHandler
}

var flowCommandRegistry = []flowCommandSpec{
	{Name: "config", NeedsRepo: true, Handler: handleFlowConfigCommand},
	{Name: "init", NeedsRepo: true, Handler: removedFlowInitCommand},
	{Name: "install", NeedsRepo: true, Handler: removedFlowInstallCommand},
	{Name: "contract", NeedsRepo: true, Handler: handleFlowContractCommand},
	{Name: "list-changes", NeedsRepo: true, Handler: handleFlowListChangesCommand},
	{Name: "run", NeedsRepo: true, NeedsWorkflowTools: true, Handler: dispatchRunCommand},
	{Name: "run-acceptance", NeedsRepo: true, Handler: handleRunAcceptanceCommand},
	{Name: "r", NeedsRepo: true, NeedsWorkflowTools: true, Handler: handleSubmitAllActiveCommand},
	{Name: "resume", NeedsRepo: true, NeedsWorkflowTools: true, Handler: dispatchResumeCommand},
	{Name: "batch", NeedsRepo: true, NeedsWorkflowTools: true, Handler: dispatchBatchCommand},
	{Name: "restart", NeedsRepo: true, NeedsWorkflowTools: true, Handler: dispatchRestartCommand},
	{Name: "stop", NeedsRepo: true, Handler: handleStopCommand},
	{Name: "archive", NeedsRepo: true, Handler: handleArchiveCommand},
	{Name: "loop", NeedsRepo: true, NeedsWorkflowTools: true, Handler: dispatchLoopCommand},
	{Name: "status", NeedsRepo: true, Handler: handleStatusCommand},
	{Name: "abort", NeedsRepo: true, Handler: handleAbortCommand},
	{Name: "clean", NeedsRepo: true, Handler: handleCleanCommand},
	{Name: "watch", NeedsRepo: true, Handler: handleWatchCommand},
	{Name: "--list-changes", NeedsRepo: true, Handler: handlePrintChangesCommand},
	{Name: "--resume", NeedsRepo: true, NeedsWorkflowTools: true, Handler: handleResumeLatestCommand},
	{Name: "--run", NeedsRepo: true, NeedsWorkflowTools: true, Handler: handleRunAliasCommand},
}

// flowCommandByName returns the command behavior used by preflight and dispatch.
func flowCommandByName(name string) (flowCommandSpec, bool) {
	for _, spec := range flowCommandRegistry {
		if spec.Name == name {
			return spec, true
		}
	}
	return flowCommandSpec{}, false
}

func handleFlowConfigCommand(ctx context.Context, args []string, stdout io.Writer, repo string, engine *Engine) error {
	_ = ctx
	_ = engine
	options, err := parseConfigCommandOptions(args[1:])
	if err != nil {
		return err
	}
	if options.ListProfiles {
		printWorkflowProfiles(stdout)
		return nil
	}
	targetRepo := repo
	if options.Global {
		targetRepo = ""
	}
	path, err := WriteWorkflowConfigProfile(targetRepo, options.Global, options.Profile)
	if err != nil {
		return err
	}
	if options.Global {
		fmt.Fprintf(stdout, "已创建全局配置 %s\n", path)
		return nil
	}
	fmt.Fprintf(stdout, "已创建 %s\n", filepath.Base(path))
	return nil
}

func removedFlowInitCommand(context.Context, []string, io.Writer, string, *Engine) error {
	return fmt.Errorf("oz flow init 已移除，请改用 oz flow config")
}

func removedFlowInstallCommand(context.Context, []string, io.Writer, string, *Engine) error {
	return fmt.Errorf("oz flow install 已移除，prompt 已内嵌在 oz-flow.yaml 中；请改用 oz flow config 或 oz flow config --global")
}

func handleFlowContractCommand(ctx context.Context, args []string, stdout io.Writer, repo string, engine *Engine) error {
	_ = ctx
	_ = repo
	_ = engine
	if !hasFlag(args[1:], "--json") {
		return fmt.Errorf("用法：oz flow contract --json")
	}
	return writeRunnerContract(stdout)
}

func handleFlowListChangesCommand(ctx context.Context, args []string, stdout io.Writer, repo string, engine *Engine) error {
	_ = ctx
	_ = engine
	if !hasFlag(args[1:], "--json") {
		return fmt.Errorf("用法：oz flow list-changes --json")
	}
	changes, err := ListChanges(repo)
	if err != nil {
		return err
	}
	return writeChangeList(stdout, changes)
}

func handleRunAcceptanceCommand(ctx context.Context, args []string, stdout io.Writer, repo string, engine *Engine) error {
	_ = ctx
	_ = engine
	return dispatchRunAcceptanceCommand(ctx, args, stdout, repo)
}

func handleSubmitAllActiveCommand(ctx context.Context, args []string, stdout io.Writer, repo string, engine *Engine) error {
	if len(args) != 1 {
		return fmt.Errorf("用法：oz flow r")
	}
	return submitAllActiveChanges(ctx, stdout, repo, engine)
}

func handleStopCommand(ctx context.Context, args []string, stdout io.Writer, repo string, engine *Engine) error {
	_ = ctx
	_ = engine
	return dispatchStopCommand(args, stdout, repo)
}

func handleArchiveCommand(ctx context.Context, args []string, stdout io.Writer, repo string, engine *Engine) error {
	_ = ctx
	_ = engine
	return dispatchArchiveCommand(args, stdout, repo)
}

func handleStatusCommand(ctx context.Context, args []string, stdout io.Writer, repo string, engine *Engine) error {
	_ = ctx
	_ = engine
	return dispatchStatusCommand(args, stdout, repo)
}

func handleAbortCommand(ctx context.Context, args []string, stdout io.Writer, repo string, engine *Engine) error {
	_ = ctx
	_ = engine
	return dispatchAbortCommand(args, stdout, repo)
}

func handleCleanCommand(ctx context.Context, args []string, stdout io.Writer, repo string, engine *Engine) error {
	_ = ctx
	_ = engine
	for _, arg := range args[1:] {
		if arg != "--agent-sessions" && arg != "--dry-run" && arg != "--json" {
			return fmt.Errorf("用法：oz flow clean [--agent-sessions] [--dry-run] [--json]")
		}
	}
	return runClean(stdout, repo, args[1:]...)
}

func handleWatchCommand(ctx context.Context, args []string, stdout io.Writer, repo string, engine *Engine) error {
	_ = ctx
	_ = engine
	return runWatch(stdout, repo, args[1:]...)
}

func handlePrintChangesCommand(ctx context.Context, args []string, stdout io.Writer, repo string, engine *Engine) error {
	_ = ctx
	_ = args
	_ = engine
	return printChanges(stdout, repo)
}

func handleResumeLatestCommand(ctx context.Context, args []string, stdout io.Writer, repo string, engine *Engine) error {
	_ = args
	_ = stdout
	_ = repo
	return engine.Resume(ctx)
}

func handleRunAliasCommand(ctx context.Context, args []string, stdout io.Writer, repo string, engine *Engine) error {
	_ = stdout
	if len(args) != 2 {
		return fmt.Errorf("用法：oz flow --run <change-name>")
	}
	if err := ValidateChange(repo, args[1]); err != nil {
		return err
	}
	return engine.SubmitBatch(ctx, []Change{{Name: args[1]}})
}
