// Package app defines the YAML schema inputs used to build workflow configs.
package app

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

var iterationStage = regexp.MustCompile(`^(review|qa|fix)_([1-9][0-9]*)$`)

type woConfigFile struct {
	MC woConfig `yaml:"wo"`
}

type woConfig struct {
	Workflow workflowConfigInput `yaml:"workflow"`
	Prompts  map[string]string   `yaml:"prompts"`
}

type workflowConfigInput struct {
	Engine              string                       `yaml:"engine"`
	MaxReviewIterations *int                         `yaml:"max_review_iterations"`
	Defaults            stageOptionsInput            `yaml:"defaults"`
	Stages              map[string]stageOptionsInput `yaml:"stages"`
	Iterations          map[string]stageOptionsInput `yaml:"iterations"`
	Parallel            parallelConfigInput          `yaml:"parallel"`
	Subagents           parallelConfigInput          `yaml:"subagents"`
	Validation          validationConfigInput        `yaml:"validation"`
	Prompts             map[string]string            `yaml:"prompts"`
}

type parallelConfigInput struct {
	Enabled *bool                               `yaml:"enabled"`
	Groups  map[string]parallelGroupConfigInput `yaml:"groups"`
}

// UnmarshalYAML accepts the KISS `parallel: true|false` scalar config shape.
func (input *parallelConfigInput) UnmarshalYAML(value *yaml.Node) error {
	if value.Kind == yaml.ScalarNode {
		var enabled bool
		if err := value.Decode(&enabled); err != nil {
			return err
		}
		input.Enabled = &enabled
		return nil
	}
	type raw parallelConfigInput
	var decoded raw
	if err := value.Decode(&decoded); err != nil {
		return err
	}
	*input = parallelConfigInput(decoded)
	return nil
}

type parallelGroupConfigInput struct {
	Mode    string                      `yaml:"mode"`
	Members []parallelMemberConfigInput `yaml:"members"`
}

type parallelMemberConfigInput struct {
	Name     string `yaml:"name"`
	Purpose  string `yaml:"purpose"`
	Stage    string `yaml:"stage"`
	CLI      string `yaml:"cli"`
	Tool     string `yaml:"tool"`
	Agent    string `yaml:"agent"`
	Model    string `yaml:"model"`
	Subagent string `yaml:"subagent"`
	Required bool   `yaml:"required"`
}

type validationConfigInput struct {
	Commands            []ValidationCommand `yaml:"commands"`
	MaxAttemptsPerStage *int                `yaml:"max_attempts_per_stage"`
	Limit               *int                `yaml:"limit"`
}

type stageOptionsInput struct {
	CLI         *string                     `yaml:"cli"`
	Tool        *string                     `yaml:"tool"`
	Agent       *string                     `yaml:"agent"`
	Model       *string                     `yaml:"model"`
	Reasoning   *string                     `yaml:"reasoning"`
	Fast        *bool                       `yaml:"fast"`
	Permissions *string                     `yaml:"permissions"`
	Before      []parallelMemberConfigInput `yaml:"before"`
}

// hasValues reports whether a stage override contains any user field.
func (input stageOptionsInput) hasValues() bool {
	return input.CLI != nil || input.Tool != nil || input.Agent != nil || input.Model != nil || input.Reasoning != nil || input.Fast != nil || input.Permissions != nil || input.Before != nil
}

// hasValues reports whether the workflow input contains any user field.
func (input workflowConfigInput) hasValues() bool {
	return input.Engine != "" || input.MaxReviewIterations != nil || input.Defaults.hasValues() || input.Stages != nil || input.Iterations != nil || input.Parallel.Enabled != nil || input.Parallel.Groups != nil || input.Subagents.Enabled != nil || input.Subagents.Groups != nil || input.Validation.MaxAttemptsPerStage != nil || input.Validation.Limit != nil || input.Validation.Commands != nil
}

func hasLegacyWorkflowRoot(data []byte) (bool, error) {
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return false, err
	}
	if len(root.Content) == 0 || root.Content[0].Kind != yaml.MappingNode {
		return false, nil
	}
	for i := 0; i+1 < len(root.Content[0].Content); i += 2 {
		key := root.Content[0].Content[i].Value
		value := root.Content[0].Content[i+1]
		if key == "workflow" {
			return true, nil
		}
		if key == "wo" && mappingHasKey(value, "workflow") {
			return true, nil
		}
	}
	return false, nil
}

func mappingHasKey(node *yaml.Node, key string) bool {
	if node == nil || node.Kind != yaml.MappingNode {
		return false
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return true
		}
	}
	return false
}

func workflowConfigFromInput(input workflowConfigInput, baseConfig *WorkflowConfig) (WorkflowConfig, error) {
	if input.Defaults.hasValues() {
		return WorkflowConfig{}, fmt.Errorf("defaults 是旧字段，已删除")
	}
	if input.Iterations != nil {
		return WorkflowConfig{}, fmt.Errorf("iterations 是旧字段，已删除")
	}
	if input.Subagents.Enabled != nil || input.Subagents.Groups != nil {
		return WorkflowConfig{}, fmt.Errorf("subagents 是旧字段，已删除")
	}
	if input.Parallel.Groups != nil {
		return WorkflowConfig{}, fmt.Errorf("parallel.groups 是旧字段，已删除；请使用 stages.<stage>.before")
	}
	maxIterations := defaultMaxReviewIterations
	engine := "go-dag"
	var basePrompts map[string]string
	byKind := defaultStageOptionsByKind()
	validation := ValidationConfig{MaxAttemptsPerStage: 3}
	parallel := defaultParallelConfig()
	if baseConfig != nil {
		engine = baseConfig.Engine
		maxIterations = baseConfig.MaxReviewIterations
		basePrompts = clonePrompts(baseConfig.Prompts)
		validation = baseConfig.Validation
		parallel = cloneParallelConfig(baseConfig.Parallel)
		if option, ok := baseConfig.Stages["planning"]; ok {
			byKind["planning"] = option
		}
		if option, ok := baseConfig.Stages["execution"]; ok {
			byKind["execution"] = option
		}
		if option, ok := baseConfig.Stages["fix_1"]; ok {
			byKind["fix"] = option
		}
		if option, ok := baseConfig.Stages["review_1"]; ok {
			byKind["review"] = option
		}
		if option, ok := baseConfig.Stages["qa_1"]; ok {
			byKind["qa"] = option
		}
		if option, ok := baseConfig.Stages["archive"]; ok {
			byKind["archive"] = option
		}
	}
	if input.MaxReviewIterations != nil {
		if *input.MaxReviewIterations < 0 {
			return WorkflowConfig{}, fmt.Errorf("max_review_iterations 必须是非负数")
		}
		maxIterations = *input.MaxReviewIterations
	}
	if strings.TrimSpace(input.Engine) != "" {
		return WorkflowConfig{}, fmt.Errorf("engine 是旧字段，已删除")
	}
	for _, kind := range stageKinds {
		base := byKind[kind]
		if err := mergeStageOptions(&base, input.Defaults); err != nil {
			return WorkflowConfig{}, fmt.Errorf("defaults: %w", err)
		}
		if input.Stages != nil {
			for key := range input.Stages {
				if !slices.Contains(stageKinds, key) {
					return WorkflowConfig{}, fmt.Errorf("未知阶段类型 %q", key)
				}
			}
			if override, ok := input.Stages[kind]; ok {
				if err := mergeStageOptions(&base, override); err != nil {
					return WorkflowConfig{}, fmt.Errorf("stages.%s: %w", kind, err)
				}
			}
		}
		byKind[kind] = base
	}
	config := WorkflowConfig{Engine: engine, MaxReviewIterations: maxIterations, Stages: map[string]StageOptions{
		"planning":  byKind["planning"],
		"execution": byKind["execution"],
		"archive":   byKind["archive"],
	}, Prompts: basePrompts}
	validation, err := validationConfigFromInput(input.Validation, validation)
	if err != nil {
		return WorkflowConfig{}, err
	}
	parallel, err = parallelConfigFromInput(input.Parallel, parallel)
	if err != nil {
		return WorkflowConfig{}, err
	}
	parallel, err = parallelConfigFromInput(input.Subagents, parallel)
	if err != nil {
		return WorkflowConfig{}, err
	}
	parallel, err = parallelConfigFromStages(input.Stages, parallel)
	if err != nil {
		return WorkflowConfig{}, err
	}
	config.Validation = validation
	config.Parallel = parallel
	for i := 1; i <= maxIterations; i++ {
		config.Stages[fmt.Sprintf("review_%d", i)] = byKind["review"]
		config.Stages[fmt.Sprintf("qa_%d", i)] = byKind["qa"]
		config.Stages[fmt.Sprintf("fix_%d", i)] = byKind["fix"]
	}
	for key, override := range input.Iterations {
		if !iterationStage.MatchString(key) {
			return WorkflowConfig{}, fmt.Errorf("未知轮次阶段 %q", key)
		}
		base, ok := config.Stages[key]
		if !ok {
			return WorkflowConfig{}, fmt.Errorf("轮次阶段 %q 超出 max_review_iterations", key)
		}
		if err := mergeStageOptions(&base, override); err != nil {
			return WorkflowConfig{}, fmt.Errorf("iterations.%s: %w", key, err)
		}
		config.Stages[key] = base
	}
	return config, nil
}

func mergeStageOptions(base *StageOptions, override stageOptionsInput) error {
	if override.CLI != nil {
		return fmt.Errorf("cli 是旧字段，已删除；请使用 agent")
	}
	if override.Tool != nil {
		return fmt.Errorf("tool 是旧字段，已删除；请使用 agent")
	}
	if override.Permissions != nil {
		return fmt.Errorf("permissions 是旧字段，已删除")
	}
	if override.Agent != nil {
		override.Tool = override.Agent
	}
	if override.Tool != nil {
		if !validAgentTool(*override.Tool) {
			return fmt.Errorf("未知 agent tool %q", *override.Tool)
		}
		base.Tool = *override.Tool
	}
	if override.Model != nil {
		base.Model = *override.Model
	}
	if override.Reasoning != nil {
		if !validReasoning[*override.Reasoning] {
			return fmt.Errorf("无效 reasoning %q", *override.Reasoning)
		}
		base.Reasoning = *override.Reasoning
	}
	if override.Fast != nil {
		base.Fast = *override.Fast
	}
	if override.Permissions != nil {
		if !validPermissions[*override.Permissions] {
			return fmt.Errorf("无效 permissions %q", *override.Permissions)
		}
		base.Permissions = *override.Permissions
	}
	return nil
}
