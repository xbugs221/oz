// Package app executes read-only parallel subagents for the built-in Go DAG scheduler.
package app

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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
	prompt := subagentPrompt(groupName, member, artifactPath)
	sessionKey := sessionStateKey(options.Tool, "subagent:"+configName+":"+member.Name+":"+strconv.Itoa(iteration))

	var sessionID string
	var result ParallelMemberResult
	var schemaErr error
	for attempt := 1; attempt <= 3; attempt++ {
		attemptHead, attemptDiff, err := gitSnapshot(e.Repo)
		if err != nil {
			return err
		}
		runner := tool.NewRunner()
		if runner, ok := runner.(progressSetter); ok {
			runner.SetProgress(&subagentProgressWriter{engine: e, state: &state, sessionKey: sessionKey})
		}
		if attempt > 1 {
			retryPrompt := artifactRetryPrompt(groupName, member, artifactPath, schemaErr)
			sessionID, err = runner.Run(ctx, e.Repo, retryPrompt, sessionID, options)
		} else {
			sessionID, err = runner.Run(ctx, e.Repo, prompt, "", options)
		}
		if boundaryErr := e.checkSubagentReadOnlyBoundary(state, member, attempt, artifactPath, attemptHead, attemptDiff); boundaryErr != nil {
			return boundaryErr
		}
		if err != nil {
			return e.failNodeState(state, err)
		}
		result, schemaErr = readNormalizeValidateMemberArtifact(artifactPath, configName, member)
		if schemaErr == nil {
			break
		}
		if attempt == 3 {
			return e.failNodeState(state, fmt.Errorf("subagent %s artifact 格式校验失败（%s）：%w", member.Name, artifactPath, schemaErr))
		}
	}

	if err := writeMemberArtifact(artifactPath, result); err != nil {
		return e.failNodeState(state, err)
	}
	afterHead, afterDiff, err := gitSnapshot(e.Repo)
	if err != nil {
		return err
	}
	if beforeHead != afterHead || beforeDiff != afterDiff {
		return e.failNodeState(state, fmt.Errorf("subagent 只读边界被破坏：检测到源码或 worktree 变化"))
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

// checkSubagentReadOnlyBoundary enforces the read-only contract after every subagent tool attempt.
func (e *Engine) checkSubagentReadOnlyBoundary(state State, member ParallelMemberConfig, attempt int, artifactPath, beforeHead, beforeDiff string) error {
	afterHead, afterDiff, err := gitSnapshot(e.Repo)
	if err != nil {
		return err
	}
	if beforeHead == afterHead && beforeDiff == afterDiff {
		return nil
	}
	return e.failNodeState(state, fmt.Errorf("subagent %s 第 %d 次尝试破坏只读边界：检测到源码或 worktree 变化，artifact=%s", member.Name, attempt, artifactPath))
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

func subagentPrompt(groupName string, member ParallelMemberConfig, output string) string {
	return strings.Join([]string{
		"只读，聚焦当前提案范围，" + member.Purpose,
		"当前提案问题写 findings；历史债务或无关问题写 scope=out_of_scope_existing，不要阻断。",
		"正向确认、已满足项或无操作项不要写入 findings；只能写入 summary/evidence。",
		"只有违反 acceptance/spec 的可复现行为失败才能标为 blocker/major；更深覆盖建议、可维护性建议或未承诺扩展写 minor 或 out_of_scope_existing。",
		"SUBAGENT_GROUP=" + groupName,
		"SUBAGENT_NAME=" + member.Name,
		"SUBAGENT_PURPOSE=" + member.Purpose,
		"SUBAGENT_OUTPUT=" + output,
		"",
		"把一个 JSON object 写到 SUBAGENT_OUTPUT，字段：",
		memberArtifactSchemaPrompt(),
	}, "\n") + "\n"
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

func memberArtifactPath(repo, runID, group string, iteration int, member string) string {
	slugName := memberArtifactFileName(member)
	if iteration > 0 {
		return filepath.Join(runDir(repo, runID), "parallel-members", group, strconv.Itoa(iteration), slugName+".json")
	}
	return filepath.Join(runDir(repo, runID), "parallel-members", group, slugName+".json")
}

func memberArtifactFileName(member string) string {
	base := slug(member)
	sum := fmt.Sprintf("%x", sha1.Sum([]byte(member)))[:10]
	return base + "-" + sum
}

func readMemberArtifact(path string) (ParallelMemberResult, error) {
	var result ParallelMemberResult
	data, err := os.ReadFile(path)
	if err != nil {
		return result, err
	}
	dec := json.NewDecoder(bytes.NewReader(bytes.TrimSpace(data)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&result); err != nil {
		return result, err
	}
	return result, nil
}

func validateMemberResult(result ParallelMemberResult) error {
	artifact := ParallelArtifact{Group: "member", Mode: "member", Summary: "member", Members: []ParallelMemberResult{result}}
	return ValidateParallelArtifact(artifact)
}

// readNormalizeValidateMemberArtifact enforces the member artifact contract at the subagent boundary.
func readNormalizeValidateMemberArtifact(path string, group string, member ParallelMemberConfig) (ParallelMemberResult, error) {
	result, err := readAndValidateMemberArtifact(path)
	if err != nil {
		return ParallelMemberResult{}, fmt.Errorf("group=%s member=%s artifact=%s: %w", group, member.Name, path, err)
	}
	result.Purpose = nonEmpty(result.Purpose, member.Purpose)
	result.Status = normalizeMemberStatus(result.Status)
	result.Required = member.Required
	if strings.TrimSpace(result.Name) == "" {
		return ParallelMemberResult{}, fmt.Errorf("group=%s member=%s field=name artifact=%s: name 不能为空", group, member.Name, path)
	}
	if result.Name != member.Name {
		return ParallelMemberResult{}, fmt.Errorf("group=%s member=%s field=name artifact=%s: member name %q 不匹配配置 %q", group, member.Name, path, result.Name, member.Name)
	}
	for i := range result.Findings {
		severity, ok := normalizeFindingSeverity(result.Findings[i].Severity)
		if !ok {
			return ParallelMemberResult{}, fmt.Errorf("group=%s member=%s field=findings[%d].severity value=%q artifact=%s: severity 无法归一化", group, member.Name, i, result.Findings[i].Severity, path)
		}
		result.Findings[i].Severity = severity
		scope, ok := normalizeFindingScope(result.Findings[i].Scope)
		if !ok {
			return ParallelMemberResult{}, fmt.Errorf("group=%s member=%s field=findings[%d].scope value=%q artifact=%s: scope 无法归一化", group, member.Name, i, result.Findings[i].Scope, path)
		}
		result.Findings[i].Scope = scope
	}
	if err := validateMemberResult(result); err != nil {
		return ParallelMemberResult{}, fmt.Errorf("group=%s member=%s artifact=%s: %w", group, member.Name, path, err)
	}
	return result, nil
}

func writeMemberArtifact(path string, result ParallelMemberResult) error {
	return writeJSONFile(path, result)
}

func nonEmpty(value, fallback string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return fallback
}

// readAndValidateMemberArtifact reads a member artifact and performs strict schema gate checks.
func readAndValidateMemberArtifact(path string) (ParallelMemberResult, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ParallelMemberResult{}, err
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return ParallelMemberResult{}, fmt.Errorf("JSON 解析失败：%w", err)
	}
	if ev, ok := raw["evidence"]; ok && ev != nil {
		arr, isArray := ev.([]interface{})
		if !isArray {
			return ParallelMemberResult{}, fmt.Errorf("evidence 必须是字符串数组")
		}
		for i, item := range arr {
			if _, isString := item.(string); !isString {
				return ParallelMemberResult{}, fmt.Errorf("evidence 第 %d 项必须是字符串，当前是 %T", i+1, item)
			}
		}
	}
	if fi, ok := raw["findings"]; ok && fi != nil {
		arr, isArray := fi.([]interface{})
		if !isArray {
			return ParallelMemberResult{}, fmt.Errorf("findings 必须是对象数组")
		}
		for i, item := range arr {
			obj, isObj := item.(map[string]interface{})
			if !isObj {
				return ParallelMemberResult{}, fmt.Errorf("findings 第 %d 项必须是对象", i+1)
			}
			for _, field := range []string{"title", "evidence", "recommendation"} {
				if v, ok := obj[field]; ok && v != nil {
					if _, isString := v.(string); !isString {
						return ParallelMemberResult{}, fmt.Errorf("findings 第 %d 项的 %s 必须是字符串", i+1, field)
					}
				}
			}
			for _, field := range []string{"severity", "scope"} {
				if v, ok := obj[field]; ok && v != nil {
					if !isStringOrNumber(v) {
						return ParallelMemberResult{}, fmt.Errorf("findings 第 %d 项的 %s 必须是字符串或数字", i+1, field)
					}
				}
			}
		}
	}
	var result ParallelMemberResult
	dec := json.NewDecoder(bytes.NewReader(bytes.TrimSpace(data)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&result); err != nil {
		return ParallelMemberResult{}, err
	}
	return result, nil
}

// artifactRetryPrompt builds a prompt that resumes the same subagent session to rewrite only SUBAGENT_OUTPUT.
func artifactRetryPrompt(groupName string, member ParallelMemberConfig, artifactPath string, schemaErr error) string {
	return strings.Join([]string{
		"只读，聚焦当前提案范围，" + member.Purpose,
		"当前提案问题写 findings；历史债务或无关问题写 scope=out_of_scope_existing，不要阻断。",
		"SUBAGENT_GROUP=" + groupName,
		"SUBAGENT_NAME=" + member.Name,
		"SUBAGENT_PURPOSE=" + member.Purpose,
		"SUBAGENT_OUTPUT=" + artifactPath,
		"",
		"SUBAGENT_OUTPUT 格式错误：" + schemaErr.Error(),
		memberArtifactSchemaPrompt(),
		"只重写 SUBAGENT_OUTPUT。",
	}, "\n") + "\n"
}

func memberArtifactSchemaPrompt() string {
	return strings.Join([]string{
		"name, purpose, status(0=ok,1=fail), summary, evidence[], findings[{title,severity(1/2/3),scope(1=当前,2=回归,0=无关),evidence,recommendation}]",
	}, "\n")
}

func isStringOrNumber(value interface{}) bool {
	switch value.(type) {
	case string, float64:
		return true
	default:
		return false
	}
}
