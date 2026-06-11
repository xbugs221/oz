#!/usr/bin/env bash
# Purpose: verify the public OMO-style parallel helper config and prompt-only execution contract.
set -euo pipefail

ROOT=$(git rev-parse --show-toplevel)
TMPDIR=$(mktemp -d)
trap 'rm -rf "$TMPDIR"' EXIT

cp -a "$ROOT/." "$TMPDIR/repo"
cd "$TMPDIR/repo"

cat > tests/app/parallel_config_contract_test.gotest <<'EOF'
package app

import (
    "strings"
    "testing"
)

func TestDefaultWorkflowConfigYAMLIncludesOMOParallelGroups(t *testing.T) {
    yaml := DefaultWorkflowConfigYAML

    required := []string{
        "parallel:",
        "enabled: true",
        "planning_context:",
        "implementation_context:",
        "review:",
        "qa:",
        "需求分析员",
        "代码库侦察员",
        "外部资料研究员",
        "目标核对审核员",
        "代码质量审核员",
        "测试有效性审核员",
        "安全风险审核员",
        "上下文一致性审核员",
        "CLI/API 测试员",
        "浏览器路径测试员",
        "证据采集员",
        "回归场景测试员",
    }
    for _, want := range required {
        if !strings.Contains(yaml, want) {
            t.Fatalf("DefaultWorkflowConfigYAML missing %q; parallel OMO-style config skeleton is not exposed", want)
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
EOF

OZ_MIGRATED_APP_RUN='TestDefaultWorkflowConfigYAMLIncludesOMOParallelGroups' \
    go test ./tests/app -run TestMigratedAppTestsRunWithGoToolchain -count=1

cat > tests/app/parallel_prompt_only_contract_test.gotest <<'EOF'
package app

import (
    "os"
    "os/exec"
    "path/filepath"
    "strings"
    "testing"
)

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
    for _, forbidden := range []string{"--subagent", "--agent", "legacy-agent agent", "pi --subagent"} {
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
EOF

OZ_MIGRATED_APP_RUN='TestParallelMemberMetadataStillRequiresCodexPiPreflight|TestParallelPromptExplainsPromptOnlySubagentContract|TestPiRunArgsNeverReceiveSubagentStyleFlags' \
    go test ./tests/app -run TestMigratedAppTestsRunWithGoToolchain -count=1
