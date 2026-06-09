// Package app tests repository workflow configuration parsing.
package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLoadWorkflowConfigUsesDefaults verifies missing wo.yaml produces built-in behavior.
func TestLoadWorkflowConfigUsesDefaults(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	config, err := LoadWorkflowConfig(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if config.MaxReviewIterations != 30 {
		t.Fatalf("max iterations = %d, want 30", config.MaxReviewIterations)
	}
	if _, err := config.StageOption("review_30"); err != nil {
		t.Fatal(err)
	}
	planning, err := config.StageOption("planning")
	if err != nil {
		t.Fatal(err)
	}
	if !planning.Fast {
		t.Fatalf("planning fast = %v, want true", planning.Fast)
	}
	if config.Validation.MaxAttemptsPerStage != 3 {
		t.Fatalf("validation attempts = %d, want 3", config.Validation.MaxAttemptsPerStage)
	}
	if config.Engine != "go-dag" {
		t.Fatalf("engine = %q, want go-dag", config.Engine)
	}
	if !config.Parallel.Enabled {
		t.Fatal("parallel enabled = false, want enabled by default")
	}
	for _, group := range []string{"planning_context", "implementation_context", "review", "qa"} {
		if _, ok := config.Parallel.Groups[group]; !ok {
			t.Fatalf("parallel group %q missing from defaults", group)
		}
	}
	if !strings.Contains(config.Prompts["planning"], "oz-plan") ||
		!strings.Contains(config.Prompts["execution"], "oz-exec") ||
		!strings.Contains(config.Prompts["qa"], "截图") ||
		!strings.Contains(config.Prompts["fix"], "只修复当前 review/QA artifact 中列出的 findings") {
		t.Fatalf("default prompts missing built-in oz guidance: %#v", config.Prompts)
	}
	if _, ok := config.Prompts["acceptance"]; ok {
		t.Fatalf("default prompts must not include sealed acceptance stage: %#v", config.Prompts)
	}
	if _, ok := config.Prompts["writing"]; ok {
		t.Fatalf("default prompts must not include deprecated writing key: %#v", config.Prompts)
	}
}

// TestWriteWorkflowConfigWritesDefaultYAML verifies config creates the repository config.
func TestInitWorkflowConfigWritesDefaultMCYAML(t *testing.T) {
	repo := t.TempDir()
	path, err := WriteWorkflowConfig(repo, false)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(path) != "wo.yaml" {
		t.Fatalf("path = %s, want wo.yaml", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != DefaultWorkflowConfigYAML {
		t.Fatalf("wo.yaml =\n%s\nwant:\n%s", data, DefaultWorkflowConfigYAML)
	}
	config, err := LoadWorkflowConfig(repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(config.Validation.Commands) != 0 {
		t.Fatalf("validation commands = %v, want disabled default", config.Validation.Commands)
	}
	if !strings.Contains(string(data), "parallel:") || !strings.Contains(string(data), "name: 需求分析员") {
		t.Fatalf("default wo.yaml missing parallel helper skeleton:\n%s", data)
	}
	if config.Validation.MaxAttemptsPerStage != 3 {
		t.Fatalf("validation attempts = %d, want 3", config.Validation.MaxAttemptsPerStage)
	}
	if fileExists(filepath.Join(repo, ".wo")) {
		t.Fatal("wo config must not create .wo")
	}
	if _, err := WriteWorkflowConfig(repo, false); err == nil {
		t.Fatal("expected existing config error")
	}
}

// TestDefaultWorkflowConfigMatchesUserDocs verifies audited defaults do not drift.
func TestDefaultWorkflowConfigMatchesUserDocs(t *testing.T) {
	defaults := []string{
		"max_review_iterations: 30",
		"reasoning: xhigh",
		"reasoning: high",
		"max_attempts_per_stage: 3",
		"commands: []",
	}
	for _, path := range []string{
		filepath.Join("..", "..", "README.md"),
		filepath.Join("..", "..", "docs", "specs", "codex-workflow-cli", "spec.md"),
	} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		body := string(data)
		for _, want := range defaults {
			if !strings.Contains(body, want) {
				t.Fatalf("%s missing default %q", path, want)
			}
		}
	}
}

// TestWriteWorkflowConfigGlobalWritesHomeYAML verifies global config stays out of ~/.wo.
func TestWriteWorkflowConfigGlobalWritesHomeYAML(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)
	global, err := WriteWorkflowConfig(repo, true)
	if err != nil {
		t.Fatal(err)
	}
	if global != filepath.Join(home, "wo.yaml") {
		t.Fatalf("global path = %s", global)
	}
	if fileExists(filepath.Join(home, ".wo")) {
		t.Fatal("wo config --global must not create ~/.wo")
	}
	if data, err := os.ReadFile(global); err != nil || !strings.Contains(string(data), "prompts:") {
		t.Fatalf("global config = %q, %v", data, err)
	}
}

// TestLoadWorkflowConfigAppliesOverrides verifies defaults, stages, and iterations merge.
func TestLoadWorkflowConfigAppliesOverrides(t *testing.T) {
	repo := t.TempDir()
	mustWritePrompt(t, filepath.Join(repo, "wo.yaml"), `
wo:
  workflow:
    max_review_iterations: 4
    defaults:
      cli: codex
      model: base-model
      reasoning: medium
      fast: false
    stages:
      review:
        cli: opencode
        model: review-model
        reasoning: high
        fast: false
      writing:
        fast: true
      archive:
        tool: pi
        model: archive-model
    iterations:
      review_4:
        model: review-four-model
        reasoning: xhigh
    validation:
      max_attempts_per_stage: 5
      commands:
        - pnpm run typecheck
        - pnpm run test:spec
`)
	config, err := LoadWorkflowConfig(repo)
	if err != nil {
		t.Fatal(err)
	}
	if config.MaxReviewIterations != 4 {
		t.Fatalf("max iterations = %d, want 4", config.MaxReviewIterations)
	}
	review, _ := config.StageOption("review_4")
	if review.Tool != "opencode" || review.Model != "review-four-model" || review.Reasoning != "xhigh" || review.Fast {
		t.Fatalf("review_4 = %#v, want opencode/review-four-model/xhigh/false", review)
	}
	fix, _ := config.StageOption("fix_2")
	if fix.Tool != "codex" || fix.Model != "base-model" || fix.Reasoning != "medium" || !fix.Fast {
		t.Fatalf("fix_2 = %#v, want codex/base-model/medium/true", fix)
	}
	archive, _ := config.StageOption("archive")
	if archive.Tool != "pi" || archive.Model != "archive-model" {
		t.Fatalf("archive = %#v, want pi/archive-model from tool field", archive)
	}
	if config.Validation.MaxAttemptsPerStage != 5 || len(config.Validation.Commands) != 2 {
		t.Fatalf("validation = %#v, want two commands and five attempts", config.Validation)
	}
}

// TestLoadWorkflowConfigParsesParallelOverrides verifies user fan-out helpers merge into the effective snapshot.
func TestLoadWorkflowConfigParsesParallelOverrides(t *testing.T) {
	repo := t.TempDir()
	mustWritePrompt(t, filepath.Join(repo, "wo.yaml"), `
wo:
  workflow:
    parallel:
      enabled: true
      groups:
        implementation_context:
          mode: advisory
          members:
            - name: 代码库侦察员
              purpose: 汇总 execution 需要读取的文件和测试模式
              stage: before_execution
              tool: codex
              subagent: explore
              required: true
`)
	config, err := LoadWorkflowConfig(repo)
	if err != nil {
		t.Fatal(err)
	}
	if !config.Parallel.Enabled {
		t.Fatal("parallel enabled = false, want true")
	}
	group, ok := config.Parallel.Groups["implementation_context"]
	if !ok {
		t.Fatal("implementation_context group missing")
	}
	if group.Mode != "advisory" || len(group.Members) != 1 {
		t.Fatalf("implementation_context = %#v, want one advisory member", group)
	}
	member := group.Members[0]
	if member.Name != "代码库侦察员" || member.Tool != "codex" || !member.Required {
		t.Fatalf("member = %#v, want required codex code scout", member)
	}
}

// TestParallelArtifactPathUsesStableRunLocalNames verifies helper results have predictable evidence paths.
func TestParallelArtifactPathUsesStableRunLocalNames(t *testing.T) {
	runPath := filepath.Join("state", "runs", "run-1")
	cases := []struct {
		group     string
		iteration int
		want      string
	}{
		{group: "planning_context", iteration: 0, want: filepath.Join(runPath, "parallel-planning-context.json")},
		{group: "implementation_context", iteration: 0, want: filepath.Join(runPath, "parallel-implementation-context.json")},
		{group: "review", iteration: 2, want: filepath.Join(runPath, "parallel-review-2.json")},
		{group: "qa", iteration: 3, want: filepath.Join(runPath, "parallel-qa-3.json")},
	}
	for _, tc := range cases {
		got := parallelArtifactPath(runPath, tc.group, tc.iteration)
		if got != tc.want {
			t.Fatalf("parallelArtifactPath(%q, %d) = %q, want %q", tc.group, tc.iteration, got, tc.want)
		}
	}
}

// TestRequiredAgentToolsIgnoresPromptOnlyParallelMembers verifies helper metadata does not require a CLI.
func TestRequiredAgentToolsIgnoresPromptOnlyParallelMembers(t *testing.T) {
	config := DefaultWorkflowConfig()
	disabled := requiredAgentTools(config)
	if strings.Contains(strings.Join(disabled, ","), "opencode") {
		t.Fatalf("disabled parallel tools = %v, want sealed stage tools only", disabled)
	}

	config.Parallel.Enabled = true
	enabled := requiredAgentTools(config)
	if strings.Contains(strings.Join(enabled, ","), "opencode") {
		t.Fatalf("enabled parallel tools = %v, want sealed stage tools only", enabled)
	}
}

// TestLoadWorkflowConfigSeparatesExecutionAndFixStages verifies new stage keys override writing fallback.
func TestLoadWorkflowConfigSeparatesExecutionAndFixStages(t *testing.T) {
	repo := t.TempDir()
	mustWritePrompt(t, filepath.Join(repo, "wo.yaml"), `
wo:
  workflow:
    max_review_iterations: 2
    stages:
      writing:
        model: legacy-model
      execution:
        model: execution-model
      fix:
        model: fix-model
`)
	config, err := LoadWorkflowConfig(repo)
	if err != nil {
		t.Fatal(err)
	}
	execution, _ := config.StageOption("execution")
	fix, _ := config.StageOption("fix_2")
	if execution.Model != "execution-model" {
		t.Fatalf("execution model = %q, want execution-model", execution.Model)
	}
	if fix.Model != "fix-model" {
		t.Fatalf("fix model = %q, want fix-model", fix.Model)
	}
}

// TestLoadWorkflowConfigKeepsGlobalExecutionFixWhenRepoOmitsStages verifies prompt-only repo config does not reset runtime options.
func TestLoadWorkflowConfigKeepsGlobalExecutionFixWhenRepoOmitsStages(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)
	mustWritePrompt(t, filepath.Join(home, "wo.yaml"), `
wo:
  workflow:
    stages:
      execution:
        model: global-execution
      fix:
        model: global-fix
`)
	mustWritePrompt(t, filepath.Join(repo, "wo.yaml"), `
wo:
  prompts:
    planning: prompt-only override
`)
	config, err := LoadWorkflowConfig(repo)
	if err != nil {
		t.Fatal(err)
	}
	execution, _ := config.StageOption("execution")
	fix, _ := config.StageOption("fix_1")
	if execution.Model != "global-execution" || fix.Model != "global-fix" {
		t.Fatalf("execution=%#v fix=%#v, want global runtime options preserved", execution, fix)
	}
}

// TestLoadWorkflowConfigRejectsInvalidInput verifies user config errors fail before runs start.
func TestLoadWorkflowConfigRejectsInvalidInput(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{name: "bad yaml", body: "wo: ["},
		{name: "bad reasoning", body: "wo:\n  workflow:\n    defaults:\n      reasoning: huge\n"},
		{name: "bad tool", body: "wo:\n  workflow:\n    defaults:\n      tool: unknown\n"},
		{name: "pi ai alias", body: "wo:\n  workflow:\n    defaults:\n      cli: pi-ai\n"},
		{name: "negative iterations", body: "wo:\n  workflow:\n    max_review_iterations: -1\n"},
		{name: "empty validation command", body: "wo:\n  workflow:\n    validation:\n      commands: ['']\n"},
		{name: "bad validation attempts", body: "wo:\n  workflow:\n    validation:\n      max_attempts_per_stage: 0\n      commands: ['true']\n"},
		{name: "bad parallel mode", body: "wo:\n  workflow:\n    parallel:\n      groups:\n        review:\n          mode: swarm\n          members:\n            - name: 审核员\n              purpose: 审核\n"},
		{name: "empty parallel member name", body: "wo:\n  workflow:\n    parallel:\n      groups:\n        review:\n          mode: gate_input\n          members:\n            - name: ''\n              purpose: 审核\n"},
		{name: "bad parallel member tool", body: "wo:\n  workflow:\n    parallel:\n      groups:\n        review:\n          mode: gate_input\n          members:\n            - name: 审核员\n              purpose: 审核\n              tool: unknown\n"},
		{name: "unknown field", body: "wo:\n  workflow:\n    surprise: true\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := t.TempDir()
			mustWritePrompt(t, filepath.Join(repo, "wo.yaml"), tc.body)
			if _, err := LoadWorkflowConfig(repo); err == nil {
				t.Fatal("expected config error")
			}
		})
	}
}

// TestLoadWorkflowConfigRejectsDuplicateFiles verifies wo.yaml and wo.yml do not merge.
func TestLoadWorkflowConfigRejectsDuplicateFiles(t *testing.T) {
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "wo.yaml"), []byte("wo: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "wo.yml"), []byte("wo: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadWorkflowConfig(repo); err == nil {
		t.Fatal("expected duplicate config file error")
	}
}
