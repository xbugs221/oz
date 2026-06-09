// Package app defines the workflow role table shared by prompt, runtime, and status mapping.
package app

import (
	"fmt"
	"strings"
)

// stageRole describes one business role in the workflow state machine.
type stageRole struct {
	Name       string
	PromptKey  string
	PromptName string
	Session    string
	Label      string
	Iterated   bool
	OptionsKey string
	Default    StageOptions
	Legacy     bool
}

var workflowRoles = []stageRole{
	{Name: "planning", PromptKey: "planning", PromptName: "wo-discuss", Session: "planner", Label: "规", OptionsKey: "planning", Default: StageOptions{Tool: "codex", Reasoning: "xhigh", Fast: true}},
	{Name: "acceptance", PromptKey: "acceptance", PromptName: "wo-acceptance", Session: "acceptance", Label: "验", OptionsKey: "acceptance", Default: StageOptions{Tool: "codex", Reasoning: "high", Fast: false}, Legacy: true},
	{Name: "execution", PromptKey: "execution", PromptName: "wo-start", Session: "executor", Label: "写", OptionsKey: "execution", Default: StageOptions{Tool: "codex", Reasoning: "low", Fast: false}},
	{Name: "review", PromptKey: "review", PromptName: "wo-review", Session: "reviewer", Label: "审", Iterated: true, OptionsKey: "review", Default: StageOptions{Tool: "codex", Reasoning: "high", Fast: false}},
	{Name: "qa", PromptKey: "qa", PromptName: "wo-qa", Session: "qa", Label: "测", Iterated: true, OptionsKey: "qa", Default: StageOptions{Tool: "codex", Reasoning: "high", Fast: false}},
	{Name: "fix", PromptKey: "fix", PromptName: "wo-fix", Session: "fixer", Label: "修", Iterated: true, OptionsKey: "fix", Default: StageOptions{Tool: "codex", Reasoning: "low", Fast: false}},
	{Name: "archive", PromptKey: "archive", PromptName: "wo-done", Session: "archiver", Label: "存", OptionsKey: "archive", Default: StageOptions{Tool: "codex", Reasoning: "low", Fast: false}},
}

// roleForStage returns the workflow role that owns an expanded stage name.
func roleForStage(stage string) (stageRole, error) {
	for _, role := range workflowRoles {
		if role.Name == stage {
			return role, nil
		}
		if role.Iterated && strings.HasPrefix(stage, role.Name+"_") {
			return role, nil
		}
	}
	return stageRole{}, fmt.Errorf("未知阶段 %q", stage)
}

func roleByName(name string) (stageRole, bool) {
	for _, role := range workflowRoles {
		if role.Name == name {
			return role, true
		}
	}
	return stageRole{}, false
}

func roleByPromptName(name string) (stageRole, bool) {
	for _, role := range workflowRoles {
		if role.PromptName == name {
			return role, true
		}
	}
	return stageRole{}, false
}

func rolePromptKeys() []string {
	keys := make([]string, 0, len(workflowRoles))
	for _, role := range workflowRoles {
		if role.Legacy {
			continue
		}
		keys = append(keys, role.PromptKey)
	}
	return keys
}

func roleStageKinds() []string {
	kinds := make([]string, 0, len(workflowRoles)+1)
	for _, role := range workflowRoles {
		kinds = append(kinds, role.OptionsKey)
	}
	return append(kinds, "writing")
}

func statusRoles() []stageRole {
	roles := make([]stageRole, 0, len(workflowRoles))
	for _, role := range workflowRoles {
		if !role.Legacy {
			roles = append(roles, role)
		}
	}
	return roles
}
