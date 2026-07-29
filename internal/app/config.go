// Package app loads repository workflow configuration and expands per-stage options.
package app

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	promptstemplate "github.com/xbugs221/oz/prompts-template"
	"gopkg.in/yaml.v3"
)

const (
	defaultMaxReviewIterations    = 5
	defaultMaxRepairIterations    = 5
	defaultMaxAuditIterations     = 3
	repairWorkflowGeneration      = "repair-v1"
	qualityLoopWorkflowGeneration = "quality-loop-v1"
)

var (
	validReasoning   = map[string]bool{"low": true, "medium": true, "high": true, "xhigh": true}
	validPermissions = map[string]bool{"default": true, "danger-full-access": true, "sandbox": true}
	stageKinds       = roleStageKinds()
)

// StageOptions describes the agent runtime knobs for one effective workflow stage.
type StageOptions struct {
	Tool        string `json:"tool" yaml:"tool"`
	Model       string `json:"model,omitempty" yaml:"model"`
	Reasoning   string `json:"reasoning" yaml:"reasoning"`
	Fast        bool   `json:"fast" yaml:"fast"`
	Permissions string `json:"permissions" yaml:"permissions"`
}

// WorkflowConfig is the effective sealed-run workflow snapshot stored in state.json.
type WorkflowConfig struct {
	Engine              string                  `json:"engine,omitempty" yaml:"-"`
	Generation          string                  `json:"generation,omitempty" yaml:"-"`
	MaxAuditIterations  int                     `json:"max_audit_iterations,omitempty" yaml:"max_audit_iterations"`
	MaxRepairIterations int                     `json:"max_repair_iterations,omitempty" yaml:"max_repair_iterations"`
	MaxReviewIterations int                     `json:"max_review_iterations,omitempty" yaml:"max_review_iterations,omitempty"`
	SubagentGuard       string                  `json:"subagent_guard,omitempty" yaml:"subagent_guard,omitempty"`
	Stages              map[string]StageOptions `json:"stages" yaml:"stages"`
	Parallel            ParallelConfig          `json:"parallel,omitempty" yaml:"parallel"`
	Validation          ValidationConfig        `json:"validation,omitempty" yaml:"validation"`
	Prompts             map[string]string       `json:"-" yaml:"prompts,omitempty"`
	Warnings            []string                `json:"warnings,omitempty" yaml:"-"`
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
	Model    string `json:"model,omitempty" yaml:"model,omitempty"`
	Subagent string `json:"subagent,omitempty" yaml:"subagent,omitempty"`
	Required bool   `json:"required,omitempty" yaml:"required,omitempty"`
}

// ValidationConfig describes deterministic commands that must pass before stage advance.
type ValidationConfig struct {
	Commands            []ValidationCommand `json:"commands,omitempty" yaml:"commands"`
	MaxAttemptsPerStage int                 `json:"max_attempts_per_stage,omitempty" yaml:"max_attempts_per_stage"`
}

// ValidationCommand describes one deterministic validation command.
type ValidationCommand struct {
	Run        string   `json:"run,omitempty" yaml:"run,omitempty"`
	Executable string   `json:"executable" yaml:"executable"`
	Args       []string `json:"args,omitempty" yaml:"args"`
}

// DefaultWorkflowConfigYAML is kept for tests that compare the generated config text.
var DefaultWorkflowConfigYAML = mustDefaultWorkflowConfigYAML()

// LoadWorkflowConfig reads oz-flow.yaml or oz-flow.yml and returns an expanded effective config.
func LoadWorkflowConfig(repo string) (WorkflowConfig, error) {
	config := DefaultWorkflowConfig()
	path, err := workflowConfigPath(repo)
	if err != nil {
		return WorkflowConfig{}, err
	}
	if path != "" {
		if err := mergeWorkflowConfigFile(&config, path); err != nil {
			return WorkflowConfig{}, err
		}
	} else if globalPath, err := globalWorkflowConfigPath(); err != nil {
		return WorkflowConfig{}, err
	} else if fileExists(globalPath) {
		if err := mergeWorkflowConfigFile(&config, globalPath); err != nil {
			return WorkflowConfig{}, err
		}
	}
	normalizeWorkflowConfig(&config)
	emitWorkflowConfigWarnings(config)
	return config, nil
}

// DefaultWorkflowConfig returns the built-in runtime behavior used without oz-flow.yaml.
func DefaultWorkflowConfig() WorkflowConfig {
	config, err := workflowConfigFromYAML([]byte(DefaultWorkflowConfigYAML), "built-in default configuration", nil)
	if err != nil {
		panic(err)
	}
	normalizeWorkflowConfig(&config)
	return config
}

// StageOption returns the effective agent options for a concrete or dynamically appended stage.
func (c WorkflowConfig) StageOption(stage string) (StageOptions, error) {
	option, ok := c.Stages[stage]
	if ok {
		return option, nil
	}
	if usesQualityLoop(c) {
		parsed, err := parseWorkflowStage(stage)
		if err == nil {
			template := map[string]string{
				workflowStageAudit:          "audit_1",
				workflowStageTargetedRepair: "targeted_repair_1",
				workflowStageQA:             "qa_1",
			}[parsed.Kind]
			if template != "" {
				if option, ok := c.Stages[template]; ok {
					return option, nil
				}
			}
		}
	}
	return StageOptions{}, fmt.Errorf("workflow config 缺少阶段 %q", stage)
}

// workflowStagesForConfig returns the initial quality-loop stages or expands a sealed finite snapshot.
func workflowStagesForConfig(config WorkflowConfig) []string {
	stages := []string{"execution"}
	if usesQualityLoop(config) {
		return append(stages, "audit_1", "qa_1", "archive")
	}
	if usesRepairWorkflow(config) {
		if config.MaxRepairIterations == 0 {
			return append(stages, "qa_1", "archive")
		}
		for i := 1; i <= config.MaxRepairIterations; i++ {
			stages = append(stages, fmt.Sprintf("repair_%d", i), fmt.Sprintf("qa_%d", i))
		}
		return append(stages, "archive")
	}
	for i := 1; i <= config.MaxReviewIterations; i++ {
		stages = append(stages, fmt.Sprintf("review_%d", i), fmt.Sprintf("qa_%d", i), fmt.Sprintf("fix_%d", i))
	}
	return append(stages, "archive")
}

// usesQualityLoop reports whether a snapshot uses dynamic quality-driven stages.
func usesQualityLoop(config WorkflowConfig) bool {
	return config.Generation == qualityLoopWorkflowGeneration
}

// usesRepairWorkflow distinguishes sealed finite repair runs from legacy review/fix snapshots.
func usesRepairWorkflow(config WorkflowConfig) bool {
	if usesQualityLoop(config) {
		return false
	}
	if config.MaxReviewIterations > 0 {
		return false
	}
	if config.MaxRepairIterations > 0 {
		return true
	}
	return config.Generation == repairWorkflowGeneration
}

func globalWorkflowConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("解析用户主目录失败：%w", err)
	}
	return filepath.Join(home, "oz-flow.yaml"), nil
}

func mergeWorkflowConfigFile(config *WorkflowConfig, path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	next, err := workflowConfigFromYAML(data, filepath.Base(path), config)
	if err != nil {
		return err
	}
	*config = next
	return nil
}

func workflowConfigPath(repo string) (string, error) {
	yamlPath := filepath.Join(repo, "oz-flow.yaml")
	ymlPath := filepath.Join(repo, "oz-flow.yml")
	yamlExists := fileExists(yamlPath)
	ymlExists := fileExists(ymlPath)
	if yamlExists && ymlExists {
		return "", fmt.Errorf("oz-flow.yaml 和 oz-flow.yml 不能同时存在")
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

// WriteWorkflowConfig writes the built-in configuration to the repository or user level.
func WriteWorkflowConfig(repo string, global bool) (string, error) {
	path := filepath.Join(repo, "oz-flow.yaml")
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
	if !global && fileExists(filepath.Join(repo, "oz-flow.yml")) {
		return "", fmt.Errorf("oz-flow.yml 已存在")
	}
	if err := os.WriteFile(path, []byte(DefaultWorkflowConfigYAML), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func workflowConfigFromYAML(data []byte, source string, baseConfig *WorkflowConfig) (WorkflowConfig, error) {
	hasOldRoot, err := hasLegacyWorkflowRoot(data)
	if err != nil {
		return WorkflowConfig{}, fmt.Errorf("解析 %s 失败：%w", source, err)
	}
	if hasOldRoot {
		return WorkflowConfig{}, fmt.Errorf("%s 无效：旧字段 wo/workflow 已删除，请使用根节点 stages 配置", source)
	}

	var input workflowConfigInput
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&input); err != nil {
		return WorkflowConfig{}, fmt.Errorf("解析 %s 失败：%w", source, err)
	}
	next, err := workflowConfigFromInput(input, baseConfig)
	if err != nil {
		return WorkflowConfig{}, fmt.Errorf("%s 无效：%w", source, err)
	}
	if input.Prompts != nil {
		if next.Prompts == nil {
			next.Prompts = map[string]string{}
		}
		if _, hasRepair := input.Prompts["repair"]; hasRepair && (input.Prompts["review"] != "" || input.Prompts["fix"] != "") {
			return WorkflowConfig{}, fmt.Errorf("%s 无效：prompts.repair 不能与弃用的 review/fix prompt 同时出现", source)
		}
		for key, body := range input.Prompts {
			if !slices.Contains(rolePromptKeys(), key) {
				return WorkflowConfig{}, fmt.Errorf("%s 无效：未知 prompt %q", source, key)
			}
			next.Prompts[key] = body
		}
		if _, hasRepair := input.Prompts["repair"]; !hasRepair {
			if body, hasFix := input.Prompts["fix"]; hasFix {
				next.Prompts["repair"] = body
				next.Warnings = append(next.Warnings, "prompts.fix 已弃用并迁移为 prompts.repair")
			} else if body, hasReview := input.Prompts["review"]; hasReview {
				next.Prompts["repair"] = body
				next.Warnings = append(next.Warnings, "prompts.review 已弃用并迁移为 prompts.repair")
			}
		}
	}
	normalizePromptConfig(next.Prompts)
	return next, nil
}

// emitWorkflowConfigWarnings makes compatibility migrations visible to CLI users.
func emitWorkflowConfigWarnings(config WorkflowConfig) {
	for _, warning := range config.Warnings {
		fmt.Fprintf(os.Stderr, "oz flow warning: %s\n", warning)
	}
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
		if options.Permissions == "" {
			options.Permissions = "default"
		}
		config.Stages[stage] = options
	}
	if config.Engine == "" {
		config.Engine = "go-dag"
	}
	normalizeValidationConfig(&config.Validation)
	config.SubagentGuard = ""
	config.Parallel = ParallelConfig{}
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

func clonePrompts(prompts map[string]string) map[string]string {
	if prompts == nil {
		cloned := defaultPromptSet()
		return cloned
	}
	cloned := map[string]string{}
	for key, body := range prompts {
		cloned[key] = body
	}
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
	return ParallelConfig{}
}

func normalizePromptConfig(prompts map[string]string) {
}

func defaultStageOptionsByKind() map[string]StageOptions {
	options := map[string]StageOptions{}
	for _, role := range workflowRoles {
		options[role.OptionsKey] = role.Default
	}
	return options
}

func defaultPromptSet() map[string]string {
	names := map[string]string{
		"planning":  "oz-flow-discuss.md",
		"execution": "oz-flow-start.md",
		"repair":    "oz-flow-repair.md",
		"review":    "oz-flow-review.md",
		"qa":        "oz-flow-qa.md",
		"fix":       "oz-flow-fix.md",
		"archive":   "oz-flow-done.md",
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
	template := `max_audit_iterations: 3
stages:
  planning:
    agent: codex
    model: gpt-5.6-sol
    reasoning: xhigh
  execution:
    agent: codex
    model: gpt-5.6-sol
    reasoning: medium
  repair:
    agent: codex
    model: gpt-5.6-sol
    reasoning: high
  qa:
    agent: codex
    model: gpt-5.6-sol
    reasoning: high
  archive:
    agent: codex
    model: gpt-5.6-sol
    reasoning: low
validation:
  limit: 3
  commands: []
prompts:
  planning: |
{{ prompt "planning" }}
  execution: |
{{ prompt "execution" }}
  repair: |
{{ prompt "repair" }}
  qa: |
{{ prompt "qa" }}
  archive: |
{{ prompt "archive" }}`
	prompts := defaultPromptSet()
	for _, key := range rolePromptKeys() {
		body := strings.TrimRight(prompts[key], "\n")
		lines := strings.Split(body, "\n")
		for i := range lines {
			lines[i] = "      " + lines[i]
		}
		template = strings.ReplaceAll(template, fmt.Sprintf(`{{ prompt "%s" }}`, key), strings.Join(lines, "\n"))
	}
	return template + "\n"
}
