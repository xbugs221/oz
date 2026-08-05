package app

import (
	"os"
	"path/filepath"
	"testing"
)

// TestClaudeAcceptedByConfigMerge proves agent: claude survives workflow config merge
// and reaches the sealed-run argument builder as the configured tool/model.
func TestClaudeAcceptedByConfigMerge(t *testing.T) {
	repo := t.TempDir()
	body := []byte(`stages:
  execution:
    agent: claude
    model: sonnet
    reasoning: medium
`)
	if err := os.WriteFile(filepath.Join(repo, "oz-flow.yaml"), body, 0o644); err != nil {
		t.Fatal(err)
	}
	config, err := LoadWorkflowConfig(repo)
	if err != nil {
		t.Fatalf("load user workflow config with agent: claude: %v", err)
	}
	options, err := config.StageOption("execution")
	if err != nil {
		t.Fatal(err)
	}
	if options.Tool != "claude" {
		t.Fatalf("execution tool = %q, want claude", options.Tool)
	}
	if options.Model != "sonnet" {
		t.Fatalf("model = %q, want sonnet", options.Model)
	}
	args := claudeRunArgs(repo, "prompt", "", options)
	if !containsArg(args, "-p") || !containsArgPair(args, "--model", "sonnet") || !containsArgPair(args, "--effort", "medium") {
		t.Fatalf("claude args = %v, missing sealed flags", args)
	}
	if !validAgentTool("claude") {
		t.Fatal("validAgentTool should accept claude")
	}
}

// TestClaudeRejectedAsUnknownAliasWhenMisspelled confirms the allowlist still rejects typos.
func TestClaudeRejectedAsUnknownAliasWhenMisspelled(t *testing.T) {
	if validAgentTool("claude-code") {
		t.Fatal("validAgentTool should reject claude-code alias")
	}
}
