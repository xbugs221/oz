// Package app reads, validates, and materializes subagent member artifacts.
package app

import (
	"bytes"
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

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
