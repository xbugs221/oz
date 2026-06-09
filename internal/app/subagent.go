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
	iteration := nodeIteration(args, stage)
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

	var sessionID string
	var result ParallelMemberResult
	var schemaErr error
	for attempt := 1; attempt <= 3; attempt++ {
		attemptHead, attemptDiff, err := gitSnapshot(e.Repo)
		if err != nil {
			return err
		}
		if attempt > 1 {
			retryPrompt := artifactRetryPrompt(groupName, member, artifactPath, schemaErr)
			sessionID, err = tool.NewRunner().Run(ctx, e.Repo, retryPrompt, sessionID, options)
		} else {
			sessionID, err = tool.NewRunner().Run(ctx, e.Repo, prompt, "", options)
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
	if state.Sessions == nil {
		state.Sessions = map[string]string{}
	}
	if sessionID != "" {
		state.Sessions[sessionStateKey(options.Tool, "subagent:"+configName+":"+member.Name+":"+strconv.Itoa(iteration))] = sessionID
		_ = saveState(e.Repo, state)
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
		"你是只读 subagent。不得修改源码、测试、文档或运行态之外的文件。",
		"SUBAGENT_GROUP=" + groupName,
		"SUBAGENT_NAME=" + member.Name,
		"SUBAGENT_PURPOSE=" + member.Purpose,
		"SUBAGENT_OUTPUT=" + output,
		"",
		"请将单成员 JSON artifact 写入 SUBAGENT_OUTPUT，只写一个 JSON object，不要 Markdown 或解释文字。",
		memberArtifactSchemaPrompt(),
	}, "\n") + "\n"
}

func nodeIteration(args []string, stage string) int {
	if value, err := optionalFlagValue(args, "--iteration"); err == nil && value != "" {
		n, _ := strconv.Atoi(value)
		return n
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
			for _, field := range []string{"title", "severity", "evidence", "recommendation"} {
				if v, ok := obj[field]; ok && v != nil {
					if _, isString := v.(string); !isString {
						return ParallelMemberResult{}, fmt.Errorf("findings 第 %d 项的 %s 必须是字符串", i+1, field)
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
		"你是只读 subagent。不得修改源码、测试、文档或运行态之外的文件。",
		"SUBAGENT_GROUP=" + groupName,
		"SUBAGENT_NAME=" + member.Name,
		"SUBAGENT_PURPOSE=" + member.Purpose,
		"SUBAGENT_OUTPUT=" + artifactPath,
		"",
		"之前生成的 SUBAGENT_OUTPUT 格式不正确：" + schemaErr.Error(),
		memberArtifactSchemaPrompt(),
		"请只重写 SUBAGENT_OUTPUT，修正上述格式错误，不要修改其他文件。",
		"请将修正后的单成员 JSON artifact 写入 SUBAGENT_OUTPUT。",
	}, "\n") + "\n"
}

func memberArtifactSchemaPrompt() string {
	return strings.Join([]string{
		"JSON 顶层只允许字段：name, purpose, status, summary, evidence, findings。",
		"name 必须等于 SUBAGENT_NAME；purpose/status/summary 必须是非空字符串。",
		"evidence 必须是字符串数组。",
		"findings 必须是对象数组；每个对象只允许 title, severity, evidence, recommendation 四个字符串字段。",
		"findings[].severity 使用 blocker/major/minor；info、warning、note 这类低风险口径写成 minor。",
		"不要使用 category、description、detail、location、level、type 等额外字段；需要分类或位置时写入 title/evidence/recommendation 字符串。",
	}, "\n")
}
