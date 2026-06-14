// Package app builds prompts for read-only subagent helpers.
package app

import (
	"fmt"
	"path/filepath"
	"strings"
)

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
		"- 写入后必须运行：oz flow validate-member-artifact --artifact \"$ARTIFACT_PATH\" --group " + groupName + " --member " + member.Name + " --change " + context.ChangeName,
		"- 最终回复只简短说明已写入和校验结果；不要把 JSON 作为最终回复传输。",
		"ARTIFACT_PATH 文件字段必须严格为：",
		memberArtifactSchemaPrompt(),
	}, "\n") + "\n"
}

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
		"写入后必须运行：oz flow validate-member-artifact --artifact \"$ARTIFACT_PATH\" --group " + groupName + " --member " + member.Name + " --change " + context.ChangeName,
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
