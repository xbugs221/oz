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
	"sync"
	"time"
)

var mergeSubagentSessionState = mergeState
var subagentAttemptTimeout = 10 * time.Minute

type artifactCaptureSetter interface {
	SetArtifactCapture(*artifactCapture)
}

type artifactCapture struct {
	mu      sync.Mutex
	builder strings.Builder
}

// Append records text emitted by a read-only subagent backend.
func (c *artifactCapture) Append(text string) {
	if c == nil || text == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.builder.WriteString(text)
}

// String returns captured text in emission order.
func (c *artifactCapture) String() string {
	if c == nil {
		return ""
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.builder.String()
}

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
		if attempt > 1 {
			if err := removeStaleMemberArtifact(artifactPath); err != nil {
				return e.failNodeState(state, err)
			}
		}
		attemptHead, attemptDiff, err := gitSnapshot(e.Repo)
		if err != nil {
			return err
		}
		attemptRunFiles, err := runArtifactFileSnapshot(runDir(e.Repo, state.RunID))
		if err != nil {
			return e.failNodeState(state, err)
		}
		attemptState, err := loadState(e.Repo, state.RunID)
		if err != nil {
			return e.failNodeState(state, err)
		}
		runner := tool.NewRunner()
		if runner, ok := runner.(progressSetter); ok {
			runner.SetProgress(&subagentProgressWriter{engine: e, state: &state, sessionKey: sessionKey})
		}
		capture := &artifactCapture{}
		if runner, ok := runner.(artifactCaptureSetter); ok {
			runner.SetArtifactCapture(capture)
		}
		attemptCtx, cancelAttempt := subagentAttemptContext(ctx)
		if attempt > 1 {
			retryPrompt := artifactRetryPrompt(groupName, member, artifactPath, schemaErr, promptContext)
			sessionID, err = runner.Run(attemptCtx, e.Repo, retryPrompt, sessionID, options)
		} else {
			sessionID, err = runner.Run(attemptCtx, e.Repo, prompt, "", options)
		}
		if attemptCtx.Err() == context.DeadlineExceeded {
			err = fmt.Errorf("%w: subagent %s 第 %d 次执行超过 %s，可由 go-dag 重试", errGoDAGRetryableNode, member.Name, attempt, subagentAttemptTimeout)
		}
		cancelAttempt()
		if boundaryErr := e.checkSubagentReadOnlyBoundary(state, member, attempt, artifactPath, attemptHead, attemptDiff, attemptRunFiles, attemptState, sessionKey); boundaryErr != nil {
			return boundaryErr
		}
		if err != nil {
			return e.failNodeState(state, fmt.Errorf("%w: subagent %s 第 %d 次执行失败，可由 go-dag 重试：%v", errGoDAGRetryableNode, member.Name, attempt, err))
		}
		if fileExists(artifactPath) {
			result, schemaErr = readNormalizeValidateMemberArtifact(artifactPath, configName, member, state.ChangeName)
			if schemaErr == nil {
				break
			}
		} else {
			if err := materializeCapturedMemberArtifact(artifactPath, capture, member, state.ChangeName); err != nil {
				schemaErr = err
				if attempt == 3 {
					result = subagentArtifactFailureResult(member, state.ChangeName, artifactPath, schemaErr)
					break
				}
				continue
			}
			result, schemaErr = readNormalizeValidateMemberArtifact(artifactPath, configName, member, state.ChangeName)
			if schemaErr == nil {
				break
			}
		}
		if attempt == 3 {
			result = subagentArtifactFailureResult(member, state.ChangeName, artifactPath, schemaErr)
			break
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

// subagentArtifactFailureResult records helper delivery failure as stage input instead of a workflow blocker.
func subagentArtifactFailureResult(member ParallelMemberConfig, changeName, artifactPath string, err error) ParallelMemberResult {
	summary := "helper artifact delivery failed; main stage should proceed with remaining context"
	evidence := []string{"artifact delivery failed: " + artifactPath}
	if err != nil {
		evidence = append(evidence, "error: "+err.Error())
	}
	return ParallelMemberResult{
		Name:       member.Name,
		ChangeName: changeName,
		Purpose:    member.Purpose,
		Status:     "failed",
		Summary:    summary,
		Evidence:   evidence,
		Required:   member.Required,
	}
}

// subagentAttemptContext bounds one helper invocation while preserving parent cancellation.
func subagentAttemptContext(parent context.Context) (context.Context, context.CancelFunc) {
	if subagentAttemptTimeout <= 0 {
		return context.WithCancel(parent)
	}
	return context.WithTimeout(parent, subagentAttemptTimeout)
}

// checkSubagentReadOnlyBoundary enforces the read-only contract after every subagent tool attempt.
func (e *Engine) checkSubagentReadOnlyBoundary(state State, member ParallelMemberConfig, attempt int, artifactPath, beforeHead, beforeDiff string, beforeRunFiles map[string]string, beforeState State, sessionKey string) error {
	afterHead, afterDiff, err := gitSnapshot(e.Repo)
	if err != nil {
		return err
	}
	runGuard, err := classifyRunArtifactChanges(runDir(e.Repo, state.RunID), beforeRunFiles, filepath.Dir(artifactPath), beforeState, sessionKey)
	if err != nil {
		return e.failNodeState(state, err)
	}
	if beforeHead == afterHead && beforeDiff == afterDiff && !runGuard.Blocked {
		return nil
	}
	guard, err := classifyGitSnapshotChangeWithAllowed(e.Repo, state.ChangeName, beforeHead, beforeDiff, afterHead, afterDiff, []string{filepath.Dir(artifactPath)})
	if err != nil {
		return e.failNodeState(state, err)
	}
	if guard.Blocked || runGuard.Blocked {
		detail := guard.Detail()
		if runGuard.Blocked {
			detail = runGuard.Detail()
			if guard.Blocked {
				detail = guard.Detail() + "; " + runGuard.Detail()
			}
		}
		return e.failNodeState(state, fmt.Errorf("subagent %s 第 %d 次尝试破坏只读边界：检测到当前 run 相关路径或源码变化（%s），artifact=%s", member.Name, attempt, detail, artifactPath))
	}
	return nil
}

type runArtifactGuard struct {
	Blocked bool
	Paths   []string
}

// Detail formats run artifact paths that explain a filesystem boundary decision.
func (guard runArtifactGuard) Detail() string {
	if len(guard.Paths) == 0 {
		return "run artifact 变化"
	}
	limit := len(guard.Paths)
	if limit > 5 {
		limit = 5
	}
	detail := strings.Join(guard.Paths[:limit], ", ")
	if len(guard.Paths) > limit {
		detail += fmt.Sprintf(" 等 %d 个路径", len(guard.Paths))
	}
	return detail
}

// runArtifactFileSnapshot records current run files so repo-external artifacts stay guarded.
func runArtifactFileSnapshot(root string) (map[string]string, error) {
	files := map[string]string{}
	if _, err := os.Stat(root); os.IsNotExist(err) {
		return files, nil
	} else if err != nil {
		return nil, err
	}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(rel)] = fmt.Sprintf("%d:%x", info.Size(), sha1.Sum(data))
		return nil
	})
	if err != nil {
		return nil, err
	}
	return files, nil
}

// classifyRunArtifactChanges blocks run-local writes outside subagent-owned artifact/progress paths.
func classifyRunArtifactChanges(root string, before map[string]string, allowedDir string, beforeState State, sessionKey string) (runArtifactGuard, error) {
	after, err := runArtifactFileSnapshot(root)
	if err != nil {
		return runArtifactGuard{}, err
	}
	allowedRel, err := filepath.Rel(root, allowedDir)
	if err != nil {
		allowedRel = ""
	}
	allowedRel = strings.TrimSuffix(filepath.ToSlash(allowedRel), "/")
	var blocked []string
	for _, path := range changedRunArtifactPaths(before, after) {
		if allowedRel != "" && allowedRel != "." && (path == allowedRel || strings.HasPrefix(path, allowedRel+"/")) {
			continue
		}
		if isWritableParallelMemberArtifact(path, after) {
			continue
		}
		if path == "state.json" {
			ok, err := stateJSONOnlySubagentProgressChange(root, beforeState, sessionKey)
			if err != nil {
				return runArtifactGuard{}, err
			}
			if ok {
				continue
			}
		}
		blocked = append(blocked, path)
	}
	return runArtifactGuard{Blocked: len(blocked) > 0, Paths: blocked}, nil
}

// isWritableParallelMemberArtifact allows sibling helpers to create or rewrite their own member.json concurrently.
func isWritableParallelMemberArtifact(path string, after map[string]string) bool {
	if after[path] == "" {
		return false
	}
	parts := strings.Split(filepath.ToSlash(path), "/")
	if len(parts) < 4 || parts[0] != "parallel-members" {
		return false
	}
	return parts[len(parts)-1] == "member.json" && strings.HasSuffix(parts[len(parts)-2], ".artifact")
}

// stateJSONOnlySubagentProgressChange allows framework-owned subagent progress persistence.
func stateJSONOnlySubagentProgressChange(root string, before State, sessionKey string) (bool, error) {
	if strings.TrimSpace(sessionKey) == "" {
		return false, nil
	}
	data, err := os.ReadFile(filepath.Join(root, "state.json"))
	if err != nil {
		return false, err
	}
	var after State
	if err := json.Unmarshal(data, &after); err != nil {
		return false, err
	}
	if after.Sessions == nil {
		return false, nil
	}
	if !subagentSessionChangesAllowed(before.Sessions, after.Sessions) {
		return false, nil
	}
	if !subagentDAGNodeChangesAllowed(before.Workflow, before.DAGNodes, after.DAGNodes) {
		return false, nil
	}
	normalized := after
	normalized.Sessions = copyStringMap(before.Sessions)
	normalized.DAGNodes = copyDAGNodeMap(before.DAGNodes)
	normalized.Processes = append([]ProcessState(nil), before.Processes...)
	beforeData, err := json.Marshal(before)
	if err != nil {
		return false, err
	}
	normalizedData, err := json.Marshal(normalized)
	if err != nil {
		return false, err
	}
	return bytes.Equal(beforeData, normalizedData), nil
}

// copyStringMap duplicates state maps before normalizing allowed deltas.
func copyStringMap(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}
	copied := make(map[string]string, len(values))
	for key, value := range values {
		copied[key] = value
	}
	return copied
}

// copyDAGNodeMap duplicates DAG node state before normalizing framework progress deltas.
func copyDAGNodeMap(values map[string]DAGNodeState) map[string]DAGNodeState {
	if values == nil {
		return nil
	}
	copied := make(map[string]DAGNodeState, len(values))
	for key, value := range values {
		copied[key] = value
	}
	return copied
}

// subagentSessionChangesAllowed limits state.json deltas to subagent session additions or updates.
func subagentSessionChangesAllowed(before, after map[string]string) bool {
	for key, value := range before {
		if after[key] != value {
			return false
		}
	}
	for key, value := range after {
		if before != nil && before[key] == value {
			continue
		}
		if value == "" || !isSubagentSessionKey(key) {
			return false
		}
	}
	return true
}

// isSubagentSessionKey reports whether a persisted session belongs to a helper member.
func isSubagentSessionKey(key string) bool {
	_, role, ok := strings.Cut(key, ":")
	return ok && strings.HasPrefix(role, "subagent:")
}

// subagentDAGNodeChangesAllowed limits state.json DAG progress deltas to configured subagent nodes.
func subagentDAGNodeChangesAllowed(workflow WorkflowConfig, before, after map[string]DAGNodeState) bool {
	for key, value := range before {
		afterValue, ok := after[key]
		if !ok {
			return false
		}
		if afterValue != value && !workflowSubagentNodeID(workflow, key) {
			return false
		}
	}
	for key := range after {
		if _, ok := before[key]; ok {
			continue
		}
		if !workflowSubagentNodeID(workflow, key) {
			return false
		}
	}
	return true
}

// workflowSubagentNodeID reports whether a DAG node id belongs to a configured helper member.
func workflowSubagentNodeID(workflow WorkflowConfig, id string) bool {
	spec := BuildWorkflowSpec("", workflow)
	for _, node := range spec.Nodes {
		if node.ID == id && node.Type == "subagent" {
			return true
		}
	}
	return false
}

// changedRunArtifactPaths returns files whose content appeared, disappeared, or changed.
func changedRunArtifactPaths(before, after map[string]string) []string {
	seen := map[string]bool{}
	var changed []string
	for path, beforeSig := range before {
		seen[path] = true
		if after[path] != beforeSig {
			changed = append(changed, path)
		}
	}
	for path := range after {
		if !seen[path] {
			changed = append(changed, path)
		}
	}
	return uniqueSortedPaths(changed)
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
	artifactDir := filepath.Dir(output)
	return strings.Join([]string{
		"你是 " + member.Name + "，职责：" + member.Purpose + "。",
		"",
		"只处理当前提案：" + context.ChangeName + "。",
		"先读取：",
		"CURRENT_CHANGE=" + context.ChangeName,
		"STATE_PATH=" + context.StatePath,
		"CHANGE_PATH=" + context.ChangePath,
		"ACCEPTANCE_PATH=" + context.AcceptancePath,
		"SUBAGENT_NAME=" + member.Name,
		"SUBAGENT_PURPOSE=" + member.Purpose,
		"ARTIFACT_DIR=" + artifactDir,
		"ARTIFACT_PATH=" + output,
		"",
		"判断范围：",
		"- 先判断当前提案是否与你的职责相关；无关时立即输出 relevant:false、irrelevant_reason、status:\"skipped\"、findings:[]，不要继续探索。",
		"- 当前提案违反 spec/acceptance 的可复现问题，写入 findings。",
		"- 历史债务、无关问题、扩展建议，写 scope=0，不得阻断。",
		"- 已满足项、正向确认、无操作项，不写 findings，只写 summary/evidence。",
		"- blocker/major 只用于当前提案范围内的明确失败。",
		"",
		"交付合同：",
		"- 只在 ARTIFACT_DIR 内写入结果文件，目标固定为 ARTIFACT_PATH。",
		"- 写入后必须运行：wo validate-member-artifact --artifact \"$ARTIFACT_PATH\" --group " + groupName + " --member " + member.Name + " --change " + context.ChangeName,
		"- 最终回复只简短说明已写入和校验结果；不要把 JSON 作为最终回复传输。",
		"ARTIFACT_PATH 文件字段必须严格为：",
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
	dirName := memberArtifactFileName(member) + ".artifact"
	if iteration > 0 {
		return filepath.Join(runDir(repo, runID), "parallel-members", group, strconv.Itoa(iteration), dirName, "member.json")
	}
	return filepath.Join(runDir(repo, runID), "parallel-members", group, dirName, "member.json")
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
	if strings.TrimSpace(result.Summary) == "" && result.Relevant != nil && !*result.Relevant {
		result.Summary = result.IrrelevantReason
	}
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
	normalizedData, err := normalizeCapturedMemberRawJSON(raw)
	if err != nil {
		return ParallelMemberResult{}, err
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
	dec := json.NewDecoder(bytes.NewReader(normalizedData))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&result); err != nil {
		return ParallelMemberResult{}, err
	}
	return result, nil
}

// normalizeCapturedMemberRawJSON removes harmless model-side compatibility noise before strict decode.
func normalizeCapturedMemberRawJSON(raw map[string]interface{}) ([]byte, error) {
	if value, ok := raw["secrets"]; ok {
		arr, isArray := value.([]interface{})
		if !isArray || len(arr) != 0 {
			return nil, fmt.Errorf("字段 secrets 不属于 member artifact schema；如发现敏感信息，只能写入 evidence 或 findings，不能添加 secrets 字段")
		}
		delete(raw, "secrets")
	}
	return json.Marshal(raw)
}

// artifactRetryPrompt builds a prompt that resumes the same subagent session to repair the artifact file.
func artifactRetryPrompt(groupName string, member ParallelMemberConfig, artifactPath string, schemaErr error, context subagentContext) string {
	artifactDir := filepath.Dir(artifactPath)
	return strings.Join([]string{
		"你是 " + member.Name + "，职责：" + member.Purpose + "。",
		"",
		"上次 artifact 文件格式错误：" + schemaErr.Error(),
		"请基于当前提案修正 ARTIFACT_PATH 指向的 JSON 文件。",
		"只处理当前提案：" + context.ChangeName + "。",
		"",
		"CURRENT_CHANGE=" + context.ChangeName,
		"STATE_PATH=" + context.StatePath,
		"CHANGE_PATH=" + context.ChangePath,
		"ACCEPTANCE_PATH=" + context.AcceptancePath,
		"SUBAGENT_NAME=" + member.Name,
		"SUBAGENT_PURPOSE=" + member.Purpose,
		"ARTIFACT_DIR=" + artifactDir,
		"ARTIFACT_PATH=" + artifactPath,
		"",
		"判断范围：",
		"- 先判断当前提案是否与你的职责相关；无关时立即输出 relevant:false、irrelevant_reason、status:\"skipped\"、findings:[]，不要继续探索。",
		"- 当前提案违反 spec/acceptance 的可复现问题，写入 findings。",
		"- 历史债务、无关问题、扩展建议，写 scope=0，不得阻断。",
		"- 已满足项、正向确认、无操作项，不写 findings，只写 summary/evidence。",
		"- blocker/major 只用于当前提案范围内的明确失败。",
		"只在 ARTIFACT_DIR 内写入结果文件，目标固定为 ARTIFACT_PATH。",
		"写入后必须运行：wo validate-member-artifact --artifact \"$ARTIFACT_PATH\" --group " + groupName + " --member " + member.Name + " --change " + context.ChangeName,
		"最终回复只简短说明已写入和校验结果；不要把 JSON 作为最终回复传输。",
		memberArtifactSchemaPrompt(),
	}, "\n") + "\n"
}

func memberArtifactSchemaPrompt() string {
	return strings.Join([]string{
		`{"name":"` + "{{SUBAGENT_NAME}}" + `","change_name":"` + "{{CURRENT_CHANGE}}" + `","purpose":"` + "{{SUBAGENT_PURPOSE}}" + `","status":0|1|"success"|"failed"|"skipped","relevant":true|false,"irrelevant_reason":"string","summary":"string","evidence":["string"],"findings":[{"title":"string","severity":1|2|3,"scope":1|2|0,"evidence":"string","recommendation":"string"}]}`,
		"只允许这些顶层字段；evidence 每一项必须是字符串，不能是对象或数组；change_name 必须等于 CURRENT_CHANGE；relevant:false 必须提供 irrelevant_reason 且 findings 必须为空。",
	}, "\n")
}

// materializeCapturedMemberArtifact lets read-only backends return artifact JSON via stdout.
func materializeCapturedMemberArtifact(path string, capture *artifactCapture, member ParallelMemberConfig, expectedChange string) error {
	if fileExists(path) {
		return nil
	}
	data, err := extractCapturedMemberJSONObject(capture.String(), member, expectedChange)
	if err != nil {
		return fmt.Errorf("未从最终回复捕获到合法 member artifact JSON object：%w；最终回复必须只包含一个裸 JSON object，不能使用 markdown 代码块或解释文字；JSON 字符串中的引号必须转义；evidence 必须是字符串数组", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

// removeStaleMemberArtifact clears the previous invalid artifact before schema retry.
func removeStaleMemberArtifact(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove stale subagent artifact %s: %w", path, err)
	}
	return nil
}

// extractCapturedMemberJSONObject returns the best member artifact object embedded in assistant text.
func extractCapturedMemberJSONObject(text string, member ParallelMemberConfig, expectedChange string) ([]byte, error) {
	text = strings.TrimSpace(text)
	var best []byte
	bestScore := 0
	for index := 0; index < len(text); index++ {
		if text[index] != '{' {
			continue
		}
		var raw json.RawMessage
		dec := json.NewDecoder(strings.NewReader(text[index:]))
		if err := dec.Decode(&raw); err != nil {
			continue
		}
		cleaned := bytes.TrimSpace(raw)
		if len(cleaned) == 0 || cleaned[0] != '{' {
			continue
		}
		score, ok := scoreCapturedMemberJSONObject(cleaned, member, expectedChange)
		if !ok {
			continue
		}
		if score > bestScore {
			bestScore = score
			best = append([]byte(nil), cleaned...)
		}
	}
	if len(best) > 0 {
		return best, nil
	}
	return nil, fmt.Errorf("captured member artifact JSON object not found")
}

// scoreCapturedMemberJSONObject ranks complete member artifacts above nested finding JSON.
func scoreCapturedMemberJSONObject(data []byte, member ParallelMemberConfig, expectedChange string) (int, bool) {
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return 0, false
	}
	name, ok := capturedStringField(raw, "name")
	if !ok {
		return 0, false
	}
	changeName, ok := capturedStringField(raw, "change_name")
	if !ok {
		return 0, false
	}
	if _, ok := capturedStringField(raw, "summary"); !ok {
		return 0, false
	}
	if artifactScalarText(raw["status"]) == "" {
		return 0, false
	}
	score := 10
	if name == strings.TrimSpace(member.Name) {
		score += 50
	}
	expectedChange = strings.TrimSpace(expectedChange)
	if expectedChange == "" || changeName == expectedChange {
		score += 100
	}
	if _, ok := raw["evidence"]; ok {
		score++
	}
	if _, ok := raw["findings"]; ok {
		score++
	}
	return score, true
}

// capturedStringField reads a required top-level string from a captured JSON object.
func capturedStringField(raw map[string]interface{}, field string) (string, bool) {
	value, ok := raw[field].(string)
	if !ok {
		return "", false
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return "", false
	}
	return value, true
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
