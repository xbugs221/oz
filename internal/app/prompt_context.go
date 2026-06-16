// Package app builds sealed-run prompt templates and stable prompt context paths.
package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"gopkg.in/yaml.v3"
)

// promptSnapshot stores the effective prompt bodies frozen for one sealed run.
type promptSnapshot struct {
	Prompts map[string]string `yaml:"prompts"`
}

// promptTemplateContext is the data exposed to named prompt templates.
type promptTemplateContext struct {
	RunID                        string
	ChangeName                   string
	Stage                        string
	StageKind                    string
	Iteration                    int
	MaxReviewIterations          int
	StatePath                    string
	ChangePath                   string
	AcceptancePath               string
	AcceptanceSummaryPath        string
	ReviewPath                   string
	QAPath                       string
	FixSummaryPath               string
	PreviousReviewPaths          []string
	PreviousQAPaths              []string
	PreviousFixSummaryPaths      []string
	PreviousReviewCount          int
	PreviousFixSummaryCount      int
	LatestPreviousReviewPath     string
	LatestPreviousFixSummaryPath string
	HasPreviousReview            bool
	HasPreviousFixSummary        bool
	PlanningContextPath          string
	ParallelContextPath          string
	ParallelReviewPath           string
	ParallelQAPath               string
	HasPlanningContext           bool
	HasParallelContext           bool
	HasParallelReview            bool
	HasParallelQA                bool
	DeliverySummaryPath          string
	BaselineHead                 string
	RoleSessionKey               string
	RoleSessionID                string
	HasRoleSession               bool
	IsFirstRoleTurn              bool
	FixEscalated                 bool
	FixEscalationReasoning       string
	ConsecutiveReviewFailures    int
	RepeatedFindingTitles        []string
}

// promptForStage reads and renders the YAML prompt for a sealed stage.
func promptForStage(repo string, state State) (string, error) {
	if state.ChangeName != "" {
		if err := validateChangeNameForPath(state.ChangeName); err != nil {
			return "", err
		}
	}
	name, err := promptNameForStage(state.Stage)
	if err != nil {
		return "", err
	}
	var templateText string
	if state.RunID != "" {
		templateText, err = runPromptTemplate(repo, state.RunID, name)
		if err != nil {
			return "", err
		}
	} else {
		config := DefaultWorkflowConfig()
		if state.Workflow.Prompts != nil {
			config = state.Workflow
		} else if loaded, loadErr := LoadWorkflowConfig(repo); loadErr == nil {
			config = loaded
		}
		templateText, err = promptForName(config, name)
		if err != nil {
			return "", err
		}
	}
	context, err := promptContext(repo, state)
	if err != nil {
		return "", err
	}
	prompt, err := renderPromptTemplate(name, templateText, context)
	if err != nil {
		return "", err
	}
	if failurePrompt := validationFailurePrompt(repo, state); failurePrompt != "" {
		return failurePrompt, nil
	}
	return prompt, nil
}

// promptForName reads a named prompt from the effective YAML config.
func promptForName(config WorkflowConfig, name string) (string, error) {
	key, err := promptKeyForName(name)
	if err != nil {
		return "", err
	}
	body := config.Prompts[key]
	if body == "" {
		return "", fmt.Errorf("配置缺少 prompts.%s", key)
	}
	return body, nil
}

// promptKeyForName resolves a prompt name to its workflow YAML key.
func promptKeyForName(name string) (string, error) {
	role, ok := roleByPromptName(name)
	if !ok {
		return "", fmt.Errorf("未知 prompt %q", name)
	}
	return role.PromptKey, nil
}

// runPromptTemplate reads the prompt snapshot saved when a sealed run starts.
func runPromptTemplate(repo, runID, name string) (string, error) {
	key, keyErr := promptKeyForName(name)
	if keyErr != nil {
		return "", keyErr
	}
	snapshotPath := filepath.Join(runDir(repo, runID), "prompt-snapshot.yaml")
	data, err := os.ReadFile(snapshotPath)
	if err == nil {
		var snapshot promptSnapshot
		if err := yaml.Unmarshal(data, &snapshot); err != nil {
			return "", fmt.Errorf("读取 prompt 快照 %s 失败: %w", snapshotPath, err)
		}
		body := snapshot.Prompts[key]
		if body == "" {
			return "", fmt.Errorf("prompt 快照缺少 prompts.%s", key)
		}
		return body, nil
	}
	if !os.IsNotExist(err) {
		return "", err
	}
	return "", fmt.Errorf("run %s 缺少 prompt 快照 prompt-snapshot.yaml", runID)
}

// snapshotRunPrompts freezes sealed-run prompts so resume cannot drift.
func snapshotRunPrompts(repo, runID string) error {
	root := runDir(repo, runID)
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	config, err := LoadWorkflowConfig(repo)
	if err != nil {
		return err
	}
	snapshot := promptSnapshot{Prompts: map[string]string{}}
	for _, key := range rolePromptKeys() {
		body := config.Prompts[key]
		if body == "" {
			return fmt.Errorf("配置缺少 prompts.%s", key)
		}
		snapshot.Prompts[key] = body
	}
	data, err := yaml.Marshal(snapshot)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(root, "prompt-snapshot.yaml"), data, 0o644)
}

// renderPromptTemplate injects run metadata and fails on unknown template variables.
func renderPromptTemplate(name, body string, context promptTemplateContext) (string, error) {
	tmpl, err := template.New(name).Option("missingkey=error").Parse(body)
	if err != nil {
		return "", err
	}
	var out strings.Builder
	if err := tmpl.Execute(&out, context); err != nil {
		return "", err
	}
	return out.String(), nil
}

// promptContext computes stable run paths for the current workflow stage.
func promptContext(repo string, state State) (promptTemplateContext, error) {
	ensureWorkflowConfig(&state)
	kind := stageKind(state.Stage)
	iteration, err := stageIteration(state.Stage)
	if err != nil {
		return promptTemplateContext{}, err
	}
	runPath := runDir(repo, state.RunID)
	roleSessionKey, roleSessionID := promptRoleSession(state)
	context := promptTemplateContext{
		RunID:                   state.RunID,
		ChangeName:              state.ChangeName,
		Stage:                   state.Stage,
		StageKind:               kind,
		Iteration:               iteration,
		MaxReviewIterations:     state.Workflow.MaxReviewIterations,
		StatePath:               filepath.Join(runPath, "state.json"),
		ChangePath:              "docs/changes/" + state.ChangeName,
		AcceptancePath:          acceptancePath(repo, state.ChangeName),
		AcceptanceSummaryPath:   acceptancePath(repo, state.ChangeName),
		DeliverySummaryPath:     filepath.Join(runPath, "delivery-summary.md"),
		BaselineHead:            state.BaselineHead,
		RoleSessionKey:          roleSessionKey,
		RoleSessionID:           roleSessionID,
		HasRoleSession:          roleSessionID != "",
		IsFirstRoleTurn:         roleSessionID == "",
		PreviousReviewPaths:     []string{},
		PreviousQAPaths:         []string{},
		PreviousFixSummaryPaths: []string{},
		RepeatedFindingTitles:   []string{},
	}
	if state.Workflow.Parallel.Enabled {
		context.PlanningContextPath = parallelArtifactPath(runPath, "planning_context", iteration)
		context.ParallelContextPath = parallelArtifactPath(runPath, "implementation_context", iteration)
		context.ParallelReviewPath = parallelArtifactPath(runPath, "review", iteration)
		context.ParallelQAPath = parallelArtifactPath(runPath, "qa", iteration)
		context.HasPlanningContext = parallelGroupConfigured(state.Workflow, "planning_context") && (kind == "planning" || kind == "execution" || kind == "review" || kind == "qa")
		context.HasParallelContext = parallelGroupConfigured(state.Workflow, "implementation_context") && (kind == "execution" || kind == "review" || kind == "qa")
		context.HasParallelReview = parallelGroupConfigured(state.Workflow, "review") && kind == "review"
		context.HasParallelQA = parallelGroupConfigured(state.Workflow, "qa") && kind == "qa"
	}
	escalation, err := fixEscalation(repo, state)
	if err != nil {
		return promptTemplateContext{}, err
	}
	if escalation.Enabled {
		context.FixEscalated = escalation.Enabled
		context.FixEscalationReasoning = escalation.Reasoning
		context.ConsecutiveReviewFailures = escalation.ConsecutiveFailures
		context.RepeatedFindingTitles = escalation.RepeatedFindingTitles
	}
	if iteration > 0 {
		context.ReviewPath = filepath.Join(runPath, fmt.Sprintf("review-%d.json", iteration))
		context.QAPath = filepath.Join(runPath, fmt.Sprintf("qa-%d.json", iteration))
		context.FixSummaryPath = filepath.Join(runPath, fmt.Sprintf("fix-%d-summary.md", iteration))
		for i := 1; i < iteration; i++ {
			context.PreviousReviewPaths = append(context.PreviousReviewPaths, filepath.Join(runPath, fmt.Sprintf("review-%d.json", i)))
			context.PreviousFixSummaryPaths = append(context.PreviousFixSummaryPaths, filepath.Join(runPath, fmt.Sprintf("fix-%d-summary.md", i)))
		}
		context.PreviousReviewCount = len(context.PreviousReviewPaths)
		context.PreviousFixSummaryCount = len(context.PreviousFixSummaryPaths)
		context.HasPreviousReview = context.PreviousReviewCount > 0
		context.HasPreviousFixSummary = context.PreviousFixSummaryCount > 0
		if context.HasPreviousReview {
			context.LatestPreviousReviewPath = context.PreviousReviewPaths[context.PreviousReviewCount-1]
		}
		if context.HasPreviousFixSummary {
			context.LatestPreviousFixSummaryPath = context.PreviousFixSummaryPaths[context.PreviousFixSummaryCount-1]
		}
	}
	if kind == "archive" {
		for i := 1; i <= state.Workflow.MaxReviewIterations; i++ {
			reviewStage := fmt.Sprintf("review_%d", i)
			if state.Stages[reviewStage] == "" {
				continue
			}
			context.PreviousReviewPaths = append(context.PreviousReviewPaths, filepath.Join(runPath, fmt.Sprintf("review-%d.json", i)))
			qaStage := fmt.Sprintf("qa_%d", i)
			if state.Stages[qaStage] != "" {
				context.PreviousQAPaths = append(context.PreviousQAPaths, filepath.Join(runPath, fmt.Sprintf("qa-%d.json", i)))
			}
			fixStage := fmt.Sprintf("fix_%d", i)
			if state.Stages[fixStage] != "" {
				context.PreviousFixSummaryPaths = append(context.PreviousFixSummaryPaths, filepath.Join(runPath, fmt.Sprintf("fix-%d-summary.md", i)))
			}
		}
	}
	return context, nil
}

// promptRoleSession returns the current stage role's backend-scoped session identity.
func promptRoleSession(state State) (string, string) {
	options, err := state.Workflow.StageOption(state.Stage)
	if err != nil || options.Tool == "" {
		return "", ""
	}
	key := sessionStateKey(options.Tool, stageSessionRole(state.Stage))
	return key, state.Sessions[key]
}
