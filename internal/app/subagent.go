// Package app executes read-only parallel subagents for the built-in Go DAG scheduler.
package app

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
)

var mergeSubagentSessionState = mergeState

// nodeRunSubagent executes one configured read-only helper member with artifact schema retry.
func (e *Engine) nodeRunSubagent(ctx context.Context, state State, args []string, stdout io.Writer) error {
	groupName, err := requireFlagValue(args, "--group")
	if err != nil {
		return err
	}
	memberName, err := requireFlagValue(args, "--member")
	if err != nil {
		return err
	}
	stage, err := requireFlagValue(args, "--stage")
	if err != nil {
		return err
	}
	iteration, err := nodeIteration(args, stage)
	if err != nil {
		return e.failNodeState(state, err)
	}
	if state.Status != statusRunning || state.Stage != stage {
		return writeNodeResult(stdout, nodeResult{Status: "skipped", RunID: state.RunID, Stage: stage, Group: groupName, Member: memberName})
	}
	configName := configGroupName(groupName)
	_, member, err := configuredParallelMember(state.Workflow, configName, memberName)
	if err != nil {
		return e.failNodeState(state, err)
	}
	artifactPath := memberArtifactPath(e.Repo, state.RunID, configName, iteration, member.Name)
	beforeHead, beforeDiff, err := gitSnapshot(e.Repo)
	if err != nil {
		return err
	}
	options, err := e.subagentOptions(state, stage, member)
	if err != nil {
		return e.failNodeState(state, err)
	}
	tool, err := e.Registry.Tool(options.Tool)
	if err != nil {
		return e.failNodeState(state, err)
	}
	promptContext, err := subagentPromptContext(e.Repo, state)
	if err != nil {
		return e.failNodeState(state, err)
	}
	prompt := subagentPrompt(groupName, member, artifactPath, promptContext)
	sessionKey := sessionStateKey(options.Tool, "subagent:"+configName+":"+member.Name+":"+strconv.Itoa(iteration))

	attempts, err := e.runSubagentAttempts(subagentAttemptsRequest{
		Tool:          tool,
		State:         state,
		GroupName:     groupName,
		ConfigName:    configName,
		Member:        member,
		ArtifactPath:  artifactPath,
		SessionKey:    sessionKey,
		Prompt:        prompt,
		PromptContext: promptContext,
		Options:       options,
		Context:       ctx,
	})
	if err != nil {
		return err
	}
	sessionID := attempts.SessionID
	result := attempts.Member

	if err := writeMemberArtifact(artifactPath, result); err != nil {
		return e.failNodeState(state, err)
	}
	afterHead, afterDiff, err := gitSnapshot(e.Repo)
	if err != nil {
		return err
	}
	if beforeHead != afterHead || beforeDiff != afterDiff {
		guard, err := classifyGitSnapshotChangeWithAllowed(e.Repo, state.ChangeName, beforeHead, beforeDiff, afterHead, afterDiff, []string{filepath.Dir(artifactPath)})
		if err != nil {
			return e.failNodeState(state, err)
		}
		if guard.Blocked {
			return e.failNodeState(state, fmt.Errorf("subagent 只读边界被破坏：检测到当前 run 相关路径或源码变化（%s），artifact=%s", guard.Detail(), artifactPath))
		}
	}
	if sessionID != "" {
		if state.Sessions == nil {
			state.Sessions = map[string]string{}
		}
		state.Sessions[sessionKey] = sessionID
		if err := mergeSubagentSessionState(e.Repo, state.RunID, func(latest *State) {
			latest.Sessions[sessionKey] = sessionID
		}); err != nil {
			warnWorkflowWrite("record subagent session", err)
			return e.failNodeState(state, fmt.Errorf("record subagent session: %w", err))
		}
	}
	return writeNodeResult(stdout, nodeResult{Status: "completed", RunID: state.RunID, Stage: stage, Group: groupName, Member: memberName, Artifact: artifactPath})
}

// subagentOptions resolves member-specific backend settings.
func (e *Engine) subagentOptions(state State, stage string, member ParallelMemberConfig) (StageOptions, error) {
	options, err := state.Workflow.StageOption(stage)
	if err != nil {
		return StageOptions{}, err
	}
	if member.Tool != "" {
		options.Tool = member.Tool
	}
	if member.Model != "" {
		options.Model = member.Model
	}
	if options.Tool == "pi" {
		options.Permissions = "sandbox"
	}
	return options, nil
}

func configuredParallelMember(workflow WorkflowConfig, groupName, memberName string) (ParallelGroupConfig, ParallelMemberConfig, error) {
	group, ok := workflow.Parallel.Groups[groupName]
	if !workflow.Parallel.Enabled || !ok {
		return ParallelGroupConfig{}, ParallelMemberConfig{}, fmt.Errorf("parallel group %q 未启用", groupName)
	}
	for _, member := range group.Members {
		if member.Name == memberName {
			return group, member, nil
		}
	}
	return ParallelGroupConfig{}, ParallelMemberConfig{}, fmt.Errorf("parallel group %q 缺少成员 %q", groupName, memberName)
}

func nodeIteration(args []string, stage string) (int, error) {
	if value, err := optionalFlagValue(args, "--iteration"); err == nil && value != "" {
		n, parseErr := strconv.Atoi(value)
		if parseErr != nil || n < 0 {
			return 0, fmt.Errorf("invalid --iteration %q", value)
		}
		return n, nil
	}
	return stageIteration(stage)
}
