// Package app loads repository workflow configuration and expands per-stage options.
package app

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	promptstemplate "github.com/xbugs221/oz/prompts-template"
	"gopkg.in/yaml.v3"
)

const defaultMaxReviewIterations = 5

var (
	validReasoning = map[string]bool{"low": true, "medium": true, "high": true, "xhigh": true}
	stageKinds     = roleStageKinds()
	iterationStage = regexp.MustCompile(`^(review|qa|fix)_([1-9][0-9]*)$`)
)

// StageOptions describes the agent runtime knobs for one effective workflow stage.
type StageOptions struct {
	Tool      string `json:"tool" yaml:"tool"`
	Model     string `json:"model,omitempty" yaml:"model"`
	Reasoning string `json:"reasoning" yaml:"reasoning"`
	Fast      bool   `json:"fast" yaml:"fast"`
}

// WorkflowConfig is the effective sealed-run workflow snapshot stored in state.json.
type WorkflowConfig struct {
	Engine              string                  `json:"engine,omitempty" yaml:"engine"`
	MaxReviewIterations int                     `json:"max_review_iterations" yaml:"max_review_iterations"`
	Stages              map[string]StageOptions `json:"stages" yaml:"stages"`
	Parallel            ParallelConfig          `json:"parallel,omitempty" yaml:"parallel"`
	Validation          ValidationConfig        `json:"validation,omitempty" yaml:"validation"`
	Prompts             map[string]string       `json:"-" yaml:"prompts,omitempty"`
}

// ParallelConfig describes optional read-only fan-out helpers around the sealed workflow.
type ParallelConfig struct {
	Enabled bool                           `json:"enabled" yaml:"enabled"`
	Groups  map[string]ParallelGroupConfig `json:"groups,omitempty" yaml:"groups"`
}

// ParallelGroupConfig describes one named fan-out group and how its results affect a stage.
type ParallelGroupConfig struct {
	Mode    string                 `json:"mode" yaml:"mode"`
	Members []ParallelMemberConfig `json:"members" yaml:"members"`
}

// ParallelMemberConfig describes one user-visible helper role in a parallel group.
type ParallelMemberConfig struct {
	Name     string `json:"name" yaml:"name"`
	Purpose  string `json:"purpose" yaml:"purpose"`
	Stage    string `json:"stage,omitempty" yaml:"stage,omitempty"`
	Tool     string `json:"tool,omitempty" yaml:"tool,omitempty"`
	Subagent string `json:"subagent,omitempty" yaml:"subagent,omitempty"`
	Required bool   `json:"required,omitempty" yaml:"required,omitempty"`
}

// ValidationConfig describes deterministic commands that must pass before stage advance.
type ValidationConfig struct {
	Commands            []string `json:"commands,omitempty" yaml:"commands"`
	MaxAttemptsPerStage int      `json:"max_attempts_per_stage,omitempty" yaml:"max_attempts_per_stage"`
}

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
}

type parallelConfigInput struct {
	Enabled *bool                               `yaml:"enabled"`
	Groups  map[string]parallelGroupConfigInput `yaml:"groups"`
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
	Subagent string `yaml:"subagent"`
	Required bool   `yaml:"required"`
}

type validationConfigInput struct {
	Commands            []string `yaml:"commands"`
	MaxAttemptsPerStage *int     `yaml:"max_attempts_per_stage"`
}

type stageOptionsInput struct {
	CLI       *string `yaml:"cli"`
	Tool      *string `yaml:"tool"`
	Model     *string `yaml:"model"`
	Reasoning *string `yaml:"reasoning"`
	Fast      *bool   `yaml:"fast"`
}

func (input stageOptionsInput) hasValues() bool {
	return input.CLI != nil || input.Tool != nil || input.Model != nil || input.Reasoning != nil || input.Fast != nil
}

// DefaultWorkflowConfigYAML is kept for tests that compare the generated config text.
var DefaultWorkflowConfigYAML = mustDefaultWorkflowConfigYAML()

// LoadWorkflowConfig reads wo.yaml or wo.yml and returns an expanded effective config.
func LoadWorkflowConfig(repo string) (WorkflowConfig, error) {
	config := DefaultWorkflowConfig()
	if path, err := globalWorkflowConfigPath(); err != nil {
		return WorkflowConfig{}, err
	} else if fileExists(path) {
		if err := mergeWorkflowConfigFile(&config, path); err != nil {
			return WorkflowConfig{}, err
		}
	}
	path, err := workflowConfigPath(repo)
	if err != nil {
		return WorkflowConfig{}, err
	}
	if path != "" {
		if err := mergeWorkflowConfigFile(&config, path); err != nil {
			return WorkflowConfig{}, err
		}
	}
	normalizeWorkflowConfig(&config)
	return config, nil
}

// DefaultWorkflowConfig returns the built-in runtime behavior used without wo.yaml.
func DefaultWorkflowConfig() WorkflowConfig {
	config, _ := workflowConfigFromInput(workflowConfigInput{}, nil)
	config.Prompts = defaultPromptSet()
	normalizePromptConfig(config.Prompts)
	normalizeWorkflowConfig(&config)
	return config
}

// StageOption returns the effective Codex options for a stage.
func (c WorkflowConfig) StageOption(stage string) (StageOptions, error) {
	option, ok := c.Stages[stage]
	if !ok {
		return StageOptions{}, fmt.Errorf("workflow config 缺少阶段 %q", stage)
	}
	return option, nil
}

// workflowStagesForConfig expands execution, review/QA/fix rounds, and archive.
func workflowStagesForConfig(config WorkflowConfig) []string {
	stages := []string{"execution"}
	for i := 1; i <= config.MaxReviewIterations; i++ {
		stages = append(stages, fmt.Sprintf("review_%d", i), fmt.Sprintf("qa_%d", i), fmt.Sprintf("fix_%d", i))
	}
	return append(stages, "archive")
}

func globalWorkflowConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("解析用户主目录失败：%w", err)
	}
	return filepath.Join(home, "wo.yaml"), nil
}

func mergeWorkflowConfigFile(config *WorkflowConfig, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var file woConfigFile
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&file); err != nil {
		return fmt.Errorf("解析 %s 失败：%w", filepath.Base(path), err)
	}
	next, err := workflowConfigFromInput(file.MC.Workflow, config)
	if err != nil {
		return fmt.Errorf("%s 无效：%w", filepath.Base(path), err)
	}
	if file.MC.Prompts != nil {
		if next.Prompts == nil {
			next.Prompts = map[string]string{}
		}
		for key, body := range file.MC.Prompts {
			if !slices.Contains(stageKinds, key) {
				return fmt.Errorf("%s 无效：未知 prompt %q", filepath.Base(path), key)
			}
			next.Prompts[key] = body
		}
		if writing := file.MC.Prompts["writing"]; writing != "" {
			if file.MC.Prompts["execution"] == "" {
				next.Prompts["execution"] = writing
			}
			if file.MC.Prompts["fix"] == "" {
				next.Prompts["fix"] = writing
			}
		}
	}
	normalizePromptConfig(next.Prompts)
	*config = next
	return nil
}

func workflowConfigPath(repo string) (string, error) {
	yamlPath := filepath.Join(repo, "wo.yaml")
	ymlPath := filepath.Join(repo, "wo.yml")
	yamlExists := fileExists(yamlPath)
	ymlExists := fileExists(ymlPath)
	if yamlExists && ymlExists {
		return "", fmt.Errorf("wo.yaml 和 wo.yml 不能同时存在")
	}
	if yamlExists {
		return yamlPath, nil
	}
	if ymlExists {
		return ymlPath, nil
	}
	return "", nil
}

// InitWorkflowConfig writes the default config without overwriting user files.
func InitWorkflowConfig(repo string) (string, error) {
	return WriteWorkflowConfig(repo, false)
}

// WriteWorkflowConfig writes either repository or user-level default configuration.
func WriteWorkflowConfig(repo string, global bool) (string, error) {
	path := filepath.Join(repo, "wo.yaml")
	if global {
		var err error
		path, err = globalWorkflowConfigPath()
		if err != nil {
			return "", err
		}
	}
	if fileExists(path) {
		return "", fmt.Errorf("%s 已存在", filepath.Base(path))
	}
	if !global && fileExists(filepath.Join(repo, "wo.yml")) {
		return "", fmt.Errorf("wo.yml 已存在")
	}
	if err := os.WriteFile(path, []byte(DefaultWorkflowConfigYAML), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func workflowConfigFromInput(input workflowConfigInput, baseConfig *WorkflowConfig) (WorkflowConfig, error) {
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
		if option, ok := baseConfig.Stages["acceptance"]; ok {
			byKind["acceptance"] = option
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
		if input.Engine != "go-dag" {
			return WorkflowConfig{}, fmt.Errorf("workflow.engine 只支持 go-dag")
		}
		engine = input.Engine
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
	if input.Stages != nil {
		writing, hasWriting := byKind["writing"]
		_, explicitExecution := input.Stages["execution"]
		_, explicitFix := input.Stages["fix"]
		if hasWriting && input.Stages["writing"].hasValues() && !explicitExecution {
			byKind["execution"] = writing
		}
		if hasWriting && input.Stages["writing"].hasValues() && !explicitFix {
			byKind["fix"] = writing
		}
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
		if strings.TrimSpace(member.Name) == "" {
			return ParallelGroupConfig{}, fmt.Errorf("members[%d].name 不能为空", i)
		}
		if strings.TrimSpace(member.Purpose) == "" {
			return ParallelGroupConfig{}, fmt.Errorf("members[%d].purpose 不能为空", i)
		}
		if tool != "" && !validAgentTool(tool) {
			return ParallelGroupConfig{}, fmt.Errorf("members[%d].tool 无效：%q", i, tool)
		}
		group.Members = append(group.Members, ParallelMemberConfig{
			Name:     member.Name,
			Purpose:  member.Purpose,
			Stage:    member.Stage,
			Tool:     tool,
			Subagent: member.Subagent,
			Required: member.Required,
		})
	}
	return group, nil
}

func validateParallelConfig(config ParallelConfig) error {
	for name, group := range config.Groups {
		switch group.Mode {
		case "advisory", "gate_input":
		default:
			return fmt.Errorf("parallel.groups.%s.mode 无效：%q", name, group.Mode)
		}
		if len(group.Members) == 0 {
			return fmt.Errorf("parallel.groups.%s.members 不能为空", name)
		}
	}
	return nil
}

// validationConfigFromInput validates user-supplied quality gate commands.
func validationConfigFromInput(input validationConfigInput, base ValidationConfig) (ValidationConfig, error) {
	config := ValidationConfig{Commands: append([]string(nil), base.Commands...), MaxAttemptsPerStage: base.MaxAttemptsPerStage}
	if input.Commands != nil {
		config.Commands = append([]string(nil), input.Commands...)
	}
	for _, command := range config.Commands {
		if strings.TrimSpace(command) == "" {
			return ValidationConfig{}, fmt.Errorf("validation command 不能为空")
		}
	}
	if input.MaxAttemptsPerStage != nil {
		if *input.MaxAttemptsPerStage < 1 {
			return ValidationConfig{}, fmt.Errorf("validation.max_attempts_per_stage 必须是正数")
		}
		config.MaxAttemptsPerStage = *input.MaxAttemptsPerStage
	}
	normalizeValidationConfig(&config)
	return config, nil
}

// normalizeWorkflowConfig backfills fields missing in older state snapshots.
func normalizeWorkflowConfig(config *WorkflowConfig) {
	if config == nil {
		return
	}
	for stage, options := range config.Stages {
		if options.Tool == "" {
			options.Tool = "codex"
		}
		config.Stages[stage] = options
	}
	if config.Engine == "" {
		config.Engine = "go-dag"
	}
	normalizeValidationConfig(&config.Validation)
	if len(config.Parallel.Groups) == 0 {
		config.Parallel = defaultParallelConfig()
	}
}

// normalizeValidationConfig backfills the default retry budget for older snapshots.
func normalizeValidationConfig(config *ValidationConfig) {
	if config == nil {
		return
	}
	if config.MaxAttemptsPerStage == 0 {
		config.MaxAttemptsPerStage = 3
	}
}

func mergeStageOptions(base *StageOptions, override stageOptionsInput) error {
	if override.CLI != nil {
		override.Tool = override.CLI
	}
	if override.Tool != nil {
		if !validAgentTool(*override.Tool) {
			return fmt.Errorf("无效 tool %q", *override.Tool)
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
	return nil
}

func clonePrompts(prompts map[string]string) map[string]string {
	if prompts == nil {
		cloned := defaultPromptSet()
		normalizePromptConfig(cloned)
		return cloned
	}
	cloned := map[string]string{}
	for key, body := range prompts {
		cloned[key] = body
	}
	normalizePromptConfig(cloned)
	return cloned
}

func cloneParallelConfig(config ParallelConfig) ParallelConfig {
	cloned := ParallelConfig{Enabled: config.Enabled, Groups: map[string]ParallelGroupConfig{}}
	for name, group := range config.Groups {
		members := append([]ParallelMemberConfig(nil), group.Members...)
		cloned.Groups[name] = ParallelGroupConfig{Mode: group.Mode, Members: members}
	}
	return cloned
}

func defaultParallelConfig() ParallelConfig {
	return ParallelConfig{
		Enabled: true,
		Groups: map[string]ParallelGroupConfig{
			"planning_context": {
				Mode: "advisory",
				Members: []ParallelMemberConfig{
					{Name: "需求分析员", Purpose: "找出需求歧义、风险和遗漏", Stage: "planning", Tool: "pi", Subagent: "metis"},
					{Name: "代码库侦察员", Purpose: "搜索现有模块、测试入口和实现约定", Stage: "planning", Tool: "pi", Subagent: "explore"},
					{Name: "外部资料研究员", Purpose: "查询外部库文档和开源实现", Stage: "planning", Tool: "pi", Subagent: "librarian"},
				},
			},
			"implementation_context": {
				Mode: "advisory",
				Members: []ParallelMemberConfig{
					{Name: "代码库侦察员", Purpose: "汇总 execution 需要读取的文件和测试模式", Stage: "before_execution", Tool: "pi", Subagent: "explore"},
					{Name: "外部资料研究员", Purpose: "查询 execution 依赖的外部库文档和开源实现", Stage: "before_execution", Tool: "pi", Subagent: "librarian"},
				},
			},
			"review": {
				Mode: "gate_input",
				Members: []ParallelMemberConfig{
					{Name: "目标核对审核员", Purpose: "核对 proposal/spec/task 是否满足"},
					{Name: "代码质量审核员", Purpose: "检查类型、边界和可维护性"},
					{Name: "测试有效性审核员", Purpose: "判断测试是否真实覆盖场景"},
					{Name: "安全风险审核员", Purpose: "检查权限、输入、泄漏和破坏性操作"},
					{Name: "上下文一致性审核员", Purpose: "检查是否违背现有架构约定"},
				},
			},
			"qa": {
				Mode: "gate_input",
				Members: []ParallelMemberConfig{
					{Name: "CLI/API 测试员", Purpose: "执行命令行或接口真实路径"},
					{Name: "浏览器路径测试员", Purpose: "执行页面真实用户路径"},
					{Name: "证据采集员", Purpose: "采集截图、trace、console、network 或 runtime log"},
					{Name: "回归场景测试员", Purpose: "覆盖邻近功能回归"},
				},
			},
		},
	}
}

func normalizePromptConfig(prompts map[string]string) {
	if prompts == nil || prompts["writing"] == "" {
		return
	}
	if prompts["execution"] == "" {
		prompts["execution"] = prompts["writing"]
	}
	if prompts["fix"] == "" {
		prompts["fix"] = prompts["writing"]
	}
}

func defaultStageOptionsByKind() map[string]StageOptions {
	options := map[string]StageOptions{}
	for _, role := range workflowRoles {
		options[role.OptionsKey] = role.Default
	}
	options["writing"] = options["execution"]
	return options
}

func defaultPromptSet() map[string]string {
	names := map[string]string{
		"planning":  "wo-discuss.md",
		"execution": "wo-start.md",
		"review":    "wo-review.md",
		"qa":        "wo-qa.md",
		"fix":       "wo-fix.md",
		"archive":   "wo-done.md",
	}
	prompts := map[string]string{}
	for key, file := range names {
		data, err := promptstemplate.FS.ReadFile(file)
		if err != nil {
			panic(err)
		}
		prompts[key] = string(data)
	}
	return prompts
}

func mustDefaultWorkflowConfigYAML() string {
	prompts := defaultPromptSet()
	return strings.Join([]string{
		"wo:",
		"  workflow:",
		"    engine: go-dag",
		"    max_review_iterations: 5",
		"    stages:",
		"      planning:",
		"        cli: codex",
		"        reasoning: xhigh",
		"      execution:",
		"        cli: codex",
		"        reasoning: low",
		"      fix:",
		"        cli: codex",
		"        reasoning: low",
		"      review:",
		"        cli: codex",
		"        reasoning: high",
		"      qa:",
		"        cli: codex",
		"        reasoning: high",
		"      archive:",
		"        cli: codex",
		"        reasoning: low",
		"    parallel:",
		"      enabled: true",
		"      groups:",
		"        planning_context:",
		"          mode: advisory",
		"          members:",
		"            - name: 需求分析员",
		"              purpose: 找出需求歧义、风险和遗漏",
		"              stage: planning",
		"              tool: pi",
		"              subagent: metis",
		"            - name: 代码库侦察员",
		"              purpose: 搜索现有模块、测试入口和实现约定",
		"              stage: planning",
		"              tool: pi",
		"              subagent: explore",
		"            - name: 外部资料研究员",
		"              purpose: 查询外部库文档和开源实现",
		"              stage: planning",
		"              tool: pi",
		"              subagent: librarian",
		"        implementation_context:",
		"          mode: advisory",
		"          members:",
		"            - name: 代码库侦察员",
		"              purpose: 汇总 execution 需要读取的文件和测试模式",
		"              stage: before_execution",
		"              tool: pi",
		"              subagent: explore",
		"            - name: 外部资料研究员",
		"              purpose: 查询 execution 依赖的外部库文档和开源实现",
		"              stage: before_execution",
		"              tool: pi",
		"              subagent: librarian",
		"        review:",
		"          mode: gate_input",
		"          members:",
		"            - name: 目标核对审核员",
		"              purpose: 核对 proposal/spec/task 是否满足",
		"            - name: 代码质量审核员",
		"              purpose: 检查类型、边界和可维护性",
		"            - name: 测试有效性审核员",
		"              purpose: 判断测试是否真实覆盖场景",
		"            - name: 安全风险审核员",
		"              purpose: 检查权限、输入、泄漏和破坏性操作",
		"            - name: 上下文一致性审核员",
		"              purpose: 检查是否违背现有架构约定",
		"        qa:",
		"          mode: gate_input",
		"          members:",
		"            - name: CLI/API 测试员",
		"              purpose: 执行命令行或接口真实路径",
		"            - name: 浏览器路径测试员",
		"              purpose: 执行页面真实用户路径",
		"            - name: 证据采集员",
		"              purpose: 采集截图、trace、console、network 或 runtime log",
		"            - name: 回归场景测试员",
		"              purpose: 覆盖邻近功能回归",
		"    validation:",
		"      max_attempts_per_stage: 3",
		"      commands: []",
		"  prompts:",
		"    planning: |",
		indentPrompt(prompts["planning"]),
		"    execution: |",
		indentPrompt(prompts["execution"]),
		"    review: |",
		indentPrompt(prompts["review"]),
		"    qa: |",
		indentPrompt(prompts["qa"]),
		"    fix: |",
		indentPrompt(prompts["fix"]),
		"    archive: |",
		indentPrompt(prompts["archive"]),
		"",
	}, "\n")
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
