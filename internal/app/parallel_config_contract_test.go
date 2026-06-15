// Package app contains long-lived regression tests migrated from shell-injected contracts.
package app

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultWorkflowConfigYAMLIncludesTreeParallelHelpers(t *testing.T) {
	yaml := DefaultWorkflowConfigYAML

	required := []string{
		"parallel: true",
		"subagent_guard: advisory",
		"stages:",
		"before:",
		"execution:",
		"review:",
		"qa:",
		"代码库侦察员",
		"外部资料研究员",
		"目标核对审核员",
		"测试有效性审核员",
		"安全风险审核员",
		"上下文一致性审核员",
		"CLI/API 测试员",
		"浏览器路径测试员",
		"回归场景测试员",
	}
	for _, want := range required {
		if !strings.Contains(yaml, want) {
			t.Fatalf("DefaultWorkflowConfigYAML missing %q; tree-shaped parallel helper config is not exposed", want)
		}
	}

	forbiddenPrimaryNames := []string{
		"Sisyphus",
		"Prometheus",
		"Metis",
		"Momus",
		"Oracle",
	}
	for _, name := range forbiddenPrimaryNames {
		if strings.Contains(yaml, "name: "+name) {
			t.Fatalf("DefaultWorkflowConfigYAML exposes mythological agent name %q as a primary user-visible member name", name)
		}
	}
}

func TestSubagentGuardConfigModes(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string
	}{
		{name: "default", body: "parallel: true\n", want: subagentGuardModeAdvisory},
		{name: "advisory", body: "parallel: true\nsubagent_guard: advisory\n", want: subagentGuardModeAdvisory},
		{name: "strict", body: "parallel: true\nsubagent_guard: strict\n", want: subagentGuardModeStrict},
		{name: "off", body: "parallel: true\nsubagent_guard: off\n", want: subagentGuardModeOff},
		{name: "false", body: "parallel: true\nsubagent_guard: false\n", want: subagentGuardModeOff},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			workflow, err := workflowConfigFromYAML([]byte(tt.body), tt.name+".yaml", nil)
			if err != nil {
				t.Fatal(err)
			}
			if workflow.SubagentGuard != tt.want {
				t.Fatalf("subagent guard = %q, want %q", workflow.SubagentGuard, tt.want)
			}
		})
	}
	if _, err := workflowConfigFromYAML([]byte("parallel: true\nsubagent_guard: maybe\n"), "bad.yaml", nil); err == nil {
		t.Fatal("invalid subagent_guard should fail")
	}
}

func TestParallelMemberMetadataStillRequiresCodexPiPreflight(t *testing.T) {
	workflow := DefaultWorkflowConfig()
	workflow.Parallel.Enabled = true
	workflow.Parallel.Groups = map[string]ParallelGroupConfig{
		"implementation_context": {
			Mode: "advisory",
			Members: []ParallelMemberConfig{
				{
					Name:     "代码库侦察员",
					Purpose:  "汇总 execution 需要读取的文件和测试模式",
					Stage:    "before_execution",
					Tool:     "pi",
					Subagent: "explore",
				},
			},
		},
	}

	got := strings.Join(requiredAgentTools(), ",")
	if got != "codex,pi,agy" {
		t.Fatalf("required tools = %s, want mandatory sealed-run clis codex,pi,agy", got)
	}
}

func TestParallelPromptExplainsPromptOnlySubagentContract(t *testing.T) {
	repo := parallelContractGitRepo(t)
	workflow := DefaultWorkflowConfig()
	workflow.Parallel.Enabled = true
	workflow.Parallel.Groups = map[string]ParallelGroupConfig{
		"implementation_context": {
			Mode: "advisory",
			Members: []ParallelMemberConfig{
				{
					Name:     "代码库侦察员",
					Purpose:  "汇总 execution 需要读取的文件和测试模式",
					Stage:    "before_execution",
					Tool:     "pi",
					Subagent: "explore",
				},
			},
		},
	}

	prompt, err := promptForStage(repo, State{ChangeName: "demo", Stage: "execution", Sealed: true, Workflow: workflow})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"parallel-implementation-context.json",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("execution prompt missing %q:\n%s", want, prompt)
		}
	}
	for _, forbidden := range []string{"--subagent", "--agent", "old-agent agent", "pi --subagent"} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("execution prompt should not bind subagent to backend CLI via %q:\n%s", forbidden, prompt)
		}
	}
}

func parallelContractGitRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	runParallelContractGit(t, repo, "init")
	runParallelContractGit(t, repo, "config", "user.email", "test@example.com")
	runParallelContractGit(t, repo, "config", "user.name", "Test")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runParallelContractGit(t, repo, "add", ".")
	runParallelContractGit(t, repo, "commit", "-m", "init")
	return repo
}

func runParallelContractGit(t *testing.T, repo string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = repo
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}

func TestPiRunArgsNeverReceiveSubagentStyleFlags(t *testing.T) {
	args := strings.Join(piRunArgs("prompt", "s-1", StageOptions{Model: "anthropic/claude-sonnet", Reasoning: "high"}), " ")
	for _, want := range []string{"--mode json", "--model anthropic/claude-sonnet", "--thinking high", "--session s-1"} {
		if !strings.Contains(args, want) {
			t.Fatalf("pi args missing %q: %s", want, args)
		}
	}
	for _, forbidden := range []string{"--subagent", "--agent", "explore", "librarian", "metis"} {
		if strings.Contains(args, forbidden) {
			t.Fatalf("pi args leaked subagent metadata %q: %s", forbidden, args)
		}
	}
}
