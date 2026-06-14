// Package app expands and validates workflow parallel helper configuration.
package app

import (
	"fmt"
	"strings"
)

// parallelConfigFromInput overlays user-provided parallel helper settings.
func parallelConfigFromInput(input parallelConfigInput, base ParallelConfig) (ParallelConfig, error) {
	config := cloneParallelConfig(base)
	if input.Enabled != nil {
		config.Enabled = *input.Enabled
	}
	if input.Groups != nil {
		config.Groups = map[string]ParallelGroupConfig{}
		for name, group := range input.Groups {
			parsed, err := parallelGroupConfigFromInput(group)
			if err != nil {
				return ParallelConfig{}, fmt.Errorf("parallel.groups.%s: %w", name, err)
			}
			config.Groups[name] = parsed
		}
	}
	if err := validateParallelConfig(config); err != nil {
		return ParallelConfig{}, err
	}
	return config, nil
}

func parallelGroupConfigFromInput(input parallelGroupConfigInput) (ParallelGroupConfig, error) {
	group := ParallelGroupConfig{Mode: input.Mode}
	group.Members = make([]ParallelMemberConfig, 0, len(input.Members))
	for i, member := range input.Members {
		tool := member.Tool
		if tool == "" {
			tool = member.CLI
		}
		if tool == "" {
			tool = member.Agent
		}
		if tool == "" {
			tool = "pi"
		}
		if strings.TrimSpace(member.Name) == "" {
			return ParallelGroupConfig{}, fmt.Errorf("members[%d].name 不能为空", i)
		}
		if strings.TrimSpace(member.Purpose) == "" {
			return ParallelGroupConfig{}, fmt.Errorf("members[%d].purpose 不能为空", i)
		}
		if tool != "" && !validAgentTool(tool) {
			return ParallelGroupConfig{}, fmt.Errorf("members[%d].tool 未知 agent tool %q", i, tool)
		}
		group.Members = append(group.Members, ParallelMemberConfig{
			Name:     member.Name,
			Purpose:  member.Purpose,
			Stage:    member.Stage,
			Tool:     tool,
			Model:    member.Model,
			Subagent: member.Subagent,
			Required: member.Required,
		})
	}
	return group, nil
}

func rejectStageBeforeLegacyMemberFields(member parallelMemberConfigInput, index int) error {
	// stages.<stage>.before[] is the tree-shaped config surface and rejects legacy aliases.
	if member.CLI != "" {
		return fmt.Errorf("members[%d].cli 是旧字段，已删除；请使用 agent", index)
	}
	if member.Tool != "" {
		return fmt.Errorf("members[%d].tool 是旧字段，已删除；请使用 agent", index)
	}
	if member.Stage != "" {
		return fmt.Errorf("members[%d].stage 是旧字段，已删除；stage 由 stages.<stage>.before 自动决定", index)
	}
	return nil
}

func parallelConfigFromStages(stages map[string]stageOptionsInput, base ParallelConfig) (ParallelConfig, error) {
	config := cloneParallelConfig(base)
	if stages == nil {
		return config, validateParallelConfig(config)
	}
	stageToGroup := map[string]string{"planning": "planning_context", "execution": "implementation_context", "review": "review", "qa": "qa"}
	stageToAnchor := map[string]string{"planning": "planning", "execution": "before_execution", "review": "before_review", "qa": "before_qa"}
	for stage, input := range stages {
		if len(input.Before) == 0 {
			continue
		}
		groupName, ok := stageToGroup[stage]
		if !ok {
			return ParallelConfig{}, fmt.Errorf("stages.%s.before 不支持", stage)
		}
		members := make([]parallelMemberConfigInput, 0, len(input.Before))
		for i, member := range input.Before {
			if err := rejectStageBeforeLegacyMemberFields(member, i); err != nil {
				return ParallelConfig{}, fmt.Errorf("stages.%s.before: %w", stage, err)
			}
			member.Stage = stageToAnchor[stage]
			members = append(members, member)
		}
		group, err := parallelGroupConfigFromInput(parallelGroupConfigInput{Mode: stageMode(stage), Members: members})
		if err != nil {
			return ParallelConfig{}, fmt.Errorf("stages.%s.before: %w", stage, err)
		}
		if config.Groups == nil {
			config.Groups = map[string]ParallelGroupConfig{}
		}
		config.Groups[groupName] = group
	}
	return config, validateParallelConfig(config)
}

func stageMode(stage string) string {
	switch stage {
	case "planning", "execution":
		return "advisory"
	default:
		return "gate_input"
	}
}

func validateParallelConfig(config ParallelConfig) error {
	for name, group := range config.Groups {
		allowedStages, ok := allowedParallelMemberStages(name)
		if !ok {
			return fmt.Errorf("parallel.groups.%s 未知并行组", name)
		}
		switch group.Mode {
		case "advisory", "gate_input":
		default:
			return fmt.Errorf("parallel.groups.%s.mode 无效：%q", name, group.Mode)
		}
		if len(group.Members) == 0 {
			return fmt.Errorf("parallel.groups.%s.members 不能为空", name)
		}
		for i, member := range group.Members {
			stage := strings.TrimSpace(member.Stage)
			if stage == "" {
				continue
			}
			if !allowedStages[stage] {
				return fmt.Errorf("parallel.groups.%s.members[%d].stage 不能挂载到 %q", name, i, stage)
			}
		}
	}
	return nil
}

// allowedParallelMemberStages defines the only stage anchors the built-in DAG can schedule.
func allowedParallelMemberStages(groupName string) (map[string]bool, bool) {
	switch groupName {
	case "planning_context":
		return map[string]bool{"planning": true}, true
	case "implementation_context":
		return map[string]bool{"before_execution": true, "execution": true}, true
	case "review":
		return map[string]bool{"before_review": true, "review": true}, true
	case "qa":
		return map[string]bool{"before_qa": true, "qa": true}, true
	default:
		return nil, false
	}
}
