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
	promptContext, err := subagentPromptContext(e.Repo, state)
	if err != nil {
		return e.failNodeState(state, err)
	}
	prompt := subagentPrompt(groupName, member, artifactPath, promptContext)
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
			retryPrompt := artifactRetryPrompt(groupName, member, artifactPath, schemaErr, promptContext)
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
		result, schemaErr = readNormalizeValidateMemberArtifact(artifactPath, configName, member, state.ChangeName)
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
		guard, err := classifyGitSnapshotChange(e.Repo, state.ChangeName, beforeHead, beforeDiff, afterHead, afterDiff)
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

// checkSubagentReadOnlyBoundary enforces the read-only contract after every subagent tool attempt.
func (e *Engine) checkSubagentReadOnlyBoundary(state State, member ParallelMemberConfig, attempt int, artifactPath, beforeHead, beforeDiff string) error {
	afterHead, afterDiff, err := gitSnapshot(e.Repo)
	if err != nil {
		return err
	}
	if beforeHead == afterHead && beforeDiff == afterDiff {
		return nil
	}
	guard, err := classifyGitSnapshotChange(e.Repo, state.ChangeName, beforeHead, beforeDiff, afterHead, afterDiff)
	if err != nil {
		return e.failNodeState(state, err)
	}
	if guard.Blocked {
		return e.failNodeState(state, fmt.Errorf("subagent %s 第 %d 次尝试破坏只读边界：检测到当前 run 相关路径或源码变化（%s），artifact=%s", member.Name, attempt, guard.Detail(), artifactPath))
	}
	return nil
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

type subagentContext struct {
	ChangeName     string
	StatePath      string
	ChangePath     string
	AcceptancePath string
	BaselineHead   string
}

// subagentPromptContext builds the sealed-run identity block shared by helper prompts.
func subagentPromptContext(repo string, state State) (subagentContext, error) {
	if state.ChangeName == "" {
		return subagentContext{}, fmt.Errorf("subagent prompt 缺少当前 change_name")
	}
	if err := validateChangeNameForPath(state.ChangeName); err != nil {
		return subagentContext{}, err
	}
	return subagentContext{
		ChangeName:     state.ChangeName,
		StatePath:      filepath.Join(runDir(repo, state.RunID), "state.json"),
		ChangePath:     filepath.Join("docs", "changes", state.ChangeName),
		AcceptancePath: acceptancePath(repo, state.ChangeName),
		BaselineHead:   state.BaselineHead,
	}, nil
}

func subagentPrompt(groupName string, member ParallelMemberConfig, output string, context subagentContext) string {
	return strings.Join([]string{
		"只读，聚焦当前提案范围，" + member.Purpose,
		subagentReadOnlyBoundaryPrompt(),
		"当前 sealed run 只处理 CURRENT_CHANGE；必须先读取 STATE_PATH、CHANGE_PATH 和 ACCEPTANCE_PATH，再给出结论。",
		"如果看到其它 docs/changes/*，只能作为背景证据；不得把其它提案的任务、测试或缺口写成当前提案 blocker/major finding。",
		"当前提案问题写 findings；历史债务或无关问题写 scope=out_of_scope_existing，不要阻断。",
		"正向确认、已满足项或无操作项不要写入 findings；只能写入 summary/evidence。",
		"只有违反 acceptance/spec 的可复现行为失败才能标为 blocker/major；更深覆盖建议、可维护性建议或未承诺扩展写 minor 或 out_of_scope_existing。",
		"CURRENT_CHANGE=" + context.ChangeName,
		"STATE_PATH=" + context.StatePath,
		"CHANGE_PATH=" + context.ChangePath,
		"ACCEPTANCE_PATH=" + context.AcceptancePath,
		"BASELINE_HEAD=" + context.BaselineHead,
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
func readNormalizeValidateMemberArtifact(path string, group string, member ParallelMemberConfig, expectedChange string) (ParallelMemberResult, error) {
	result, err := readAndValidateMemberArtifact(path)
	if err != nil {
		return ParallelMemberResult{}, fmt.Errorf("group=%s member=%s artifact=%s: %w", group, member.Name, path, err)
	}
	if err := validateSubagentArtifactChange(path, group, member, result.ChangeName, expectedChange); err != nil {
		return ParallelMemberResult{}, err
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
		if subagentFindingBlocksOtherChange(result.Findings[i], expectedChange) {
			return ParallelMemberResult{}, fmt.Errorf("group=%s member=%s field=findings[%d] artifact=%s: 当前提案 blocker/major 不得指向其它 docs/changes 提案", group, member.Name, i, path)
		}
	}
	if err := validateMemberResult(result); err != nil {
		return ParallelMemberResult{}, fmt.Errorf("group=%s member=%s artifact=%s: %w", group, member.Name, path, err)
	}
	return result, nil
}

// subagentFindingBlocksOtherChange detects cross-proposal blockers before fan-in.
func subagentFindingBlocksOtherChange(finding Finding, expectedChange string) bool {
	if strings.TrimSpace(expectedChange) == "" || !isCurrentChangeFindingHardBlocking(finding) {
		return false
	}
	text := strings.Join([]string{finding.Title, finding.Evidence, finding.Recommendation}, "\n")
	for _, changeName := range referencedDocsChangeNames(text) {
		if changeName != "" && changeName != expectedChange {
			return true
		}
	}
	return false
}

// referencedDocsChangeNames extracts docs/changes/<change-name> mentions from finding text.
func referencedDocsChangeNames(text string) []string {
	const prefix = "docs/changes/"
	var names []string
	remaining := text
	for {
		index := strings.Index(remaining, prefix)
		if index < 0 {
			return names
		}
		rest := remaining[index+len(prefix):]
		name := docsChangeNamePrefix(rest)
		if name != "" {
			names = append(names, name)
		}
		remaining = rest
	}
}

// docsChangeNamePrefix returns the first path segment after docs/changes/.
func docsChangeNamePrefix(text string) string {
	text = strings.TrimLeft(text, "`\"'([<")
	if text == "" || strings.HasPrefix(text, "archive/") || strings.HasPrefix(text, ".") {
		return ""
	}
	end := len(text)
	for i, r := range text {
		if r == '/' || r == '\\' || r == '`' || r == '"' || r == '\'' || r == ')' || r == ']' || r == '>' || r == '，' || r == '。' || r == ',' || r == ';' || r == ':' || r == '\n' || r == '\t' || r == ' ' {
			end = i
			break
		}
	}
	return strings.TrimSpace(text[:end])
}

// validateSubagentArtifactChange proves helper output belongs to the sealed current change.
func validateSubagentArtifactChange(path string, group string, member ParallelMemberConfig, got, expected string) error {
	expected = strings.TrimSpace(expected)
	if expected == "" {
		return nil
	}
	got = strings.TrimSpace(got)
	if got == "" {
		return fmt.Errorf("group=%s member=%s field=change_name artifact=%s: change_name 必须等于当前提案 %q", group, member.Name, path, expected)
	}
	if got != expected {
		return fmt.Errorf("group=%s member=%s field=change_name artifact=%s: change_name %q 不匹配当前提案 %q", group, member.Name, path, got, expected)
	}
	return nil
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
func artifactRetryPrompt(groupName string, member ParallelMemberConfig, artifactPath string, schemaErr error, context subagentContext) string {
	return strings.Join([]string{
		"只读，聚焦当前提案范围，" + member.Purpose,
		subagentReadOnlyBoundaryPrompt(),
		"当前 sealed run 只处理 CURRENT_CHANGE；必须先读取 STATE_PATH、CHANGE_PATH 和 ACCEPTANCE_PATH，再重写 artifact。",
		"如果看到其它 docs/changes/*，只能作为背景证据；不得把其它提案写成当前提案 blocker/major finding。",
		"当前提案问题写 findings；历史债务或无关问题写 scope=out_of_scope_existing，不要阻断。",
		"CURRENT_CHANGE=" + context.ChangeName,
		"STATE_PATH=" + context.StatePath,
		"CHANGE_PATH=" + context.ChangePath,
		"ACCEPTANCE_PATH=" + context.AcceptancePath,
		"BASELINE_HEAD=" + context.BaselineHead,
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

// subagentReadOnlyBoundaryPrompt tells helper agents where validation side effects may live.
func subagentReadOnlyBoundaryPrompt() string {
	return "只读边界：不要新增、修改或删除仓库文件；如需构建或运行测试，所有 -o、日志、截图、trace、coverage 等运行输出只能写入 test-results/ 或系统临时目录；例如 Go 二进制用 go build -o test-results/<name> ./cmd/<name>，结束时不得留下 test-results/ 之外的构建产物。"
}

func memberArtifactSchemaPrompt() string {
	return strings.Join([]string{
		"name, change_name(必须等于 CURRENT_CHANGE), purpose, status(0=ok,1=fail), summary, evidence[], findings[{title,severity(1/2/3),scope(1=当前,2=回归,0=无关),evidence,recommendation}]",
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

// readOnlyBoundaryDetail summarizes the git snapshot delta that violated read-only mode.
func readOnlyBoundaryDetail(beforeHead, beforeDiff, afterHead, afterDiff string) string {
	var parts []string
	if beforeHead != afterHead {
		parts = append(parts, fmt.Sprintf("HEAD %s -> %s", shortCommit(beforeHead), shortCommit(afterHead)))
	}
	if diff := statusDeltaSummary(beforeDiff, afterDiff); diff != "" {
		parts = append(parts, diff)
	}
	if len(parts) == 0 {
		return "worktree changed"
	}
	return strings.Join(parts, "；")
}

// statusDeltaSummary returns added and removed porcelain status lines.
func statusDeltaSummary(before, after string) string {
	added, removed := statusDelta(before, after)
	var parts []string
	if len(added) > 0 {
		parts = append(parts, "新增/变更："+strings.Join(limitStatusLines(added), " | "))
	}
	if len(removed) > 0 {
		parts = append(parts, "消失："+strings.Join(limitStatusLines(removed), " | "))
	}
	return strings.Join(parts, "；")
}

// statusDelta compares git porcelain status strings while preserving display order.
func statusDelta(before, after string) ([]string, []string) {
	beforeLines := statusLines(before)
	afterLines := statusLines(after)
	beforeSet := map[string]bool{}
	for _, line := range beforeLines {
		beforeSet[line] = true
	}
	afterSet := map[string]bool{}
	for _, line := range afterLines {
		afterSet[line] = true
	}
	var added []string
	for _, line := range afterLines {
		if !beforeSet[line] {
			added = append(added, line)
		}
	}
	var removed []string
	for _, line := range beforeLines {
		if !afterSet[line] {
			removed = append(removed, line)
		}
	}
	return added, removed
}

// statusLines splits a porcelain status snapshot into non-empty lines.
func statusLines(status string) []string {
	var lines []string
	for _, line := range strings.Split(status, "\n") {
		if strings.TrimSpace(line) != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

// limitStatusLines caps diagnostics so workflow errors stay readable.
func limitStatusLines(lines []string) []string {
	const maxLines = 8
	if len(lines) <= maxLines {
		return lines
	}
	limited := append([]string{}, lines[:maxLines]...)
	limited = append(limited, fmt.Sprintf("... 还有 %d 项", len(lines)-maxLines))
	return limited
}

// shortCommit returns a readable prefix for full git commit ids.
func shortCommit(commit string) string {
	commit = strings.TrimSpace(commit)
	if len(commit) <= 12 {
		return commit
	}
	return commit[:12]
}
