// Package app renders built-in workflow configuration profiles.
package app

import (
	"fmt"
	"strings"

	profilestemplate "github.com/xbugs221/oz/profiles-template"
)

// BuiltInWorkflowProfiles returns the ordered profile registry shown by the CLI.
func BuiltInWorkflowProfiles() []WorkflowProfile {
	return []WorkflowProfile{
		{Name: "default", Description: "默认代码工作流", Scenario: "通用 oz 提案执行、审查和 QA"},
		{Name: "mada-code", Description: "MADA 代码实现/审查 profile", Scenario: "需要多角色代码侦察、实现审核和回归测试"},
		{Name: "mada-decision", Description: "MADA 决策 profile", Scenario: "技术选型、推荐、学习路线和取舍评估"},
		{Name: "mada-research", Description: "MADA 调研 profile", Scenario: "资料调研、证据审计和结论交叉验证"},
	}
}

// WorkflowProfileYAML renders a built-in profile template into the final wo.yaml body.
func WorkflowProfileYAML(profile string) (string, error) {
	if !knownWorkflowProfile(profile) {
		return "", fmt.Errorf("未知 profile %q，可用 profile: %s", profile, strings.Join(workflowProfileNames(), ", "))
	}
	data, err := profilestemplate.FS.ReadFile(profile + ".yaml")
	if err != nil {
		return "", err
	}
	return renderWorkflowProfileTemplate(string(data)), nil
}

func indentPrompt(body string) string {
	body = strings.TrimRight(body, "\n")
	if body == "" {
		return "      "
	}
	lines := strings.Split(body, "\n")
	for i := range lines {
		lines[i] = "      " + lines[i]
	}
	return strings.Join(lines, "\n")
}

func renderWorkflowProfileTemplate(template string) string {
	prompts := defaultPromptSet()
	for _, key := range rolePromptKeys() {
		template = strings.ReplaceAll(template, fmt.Sprintf(`{{ prompt "%s" }}`, key), indentPrompt(prompts[key]))
	}
	if !strings.HasSuffix(template, "\n") {
		template += "\n"
	}
	return template
}

func knownWorkflowProfile(profile string) bool {
	for _, item := range BuiltInWorkflowProfiles() {
		if item.Name == profile {
			return true
		}
	}
	return false
}

func workflowProfileNames() []string {
	profiles := BuiltInWorkflowProfiles()
	names := make([]string, 0, len(profiles))
	for _, item := range profiles {
		names = append(names, item.Name)
	}
	return names
}
