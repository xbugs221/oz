// Package app tests the built-in default workflow configuration.
package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDefaultWorkflowConfigPinsCodexModel verifies the default config passes the pinned model to each Codex command.
func TestDefaultWorkflowConfigPinsCodexModel(t *testing.T) {
	const wantModel = "gpt-5.6-sol"

	config := DefaultWorkflowConfig()
	for stage, options := range config.Stages {
		if options.Tool != "codex" {
			continue
		}
		if options.Model != wantModel {
			t.Errorf("stage %q model = %q, want %q", stage, options.Model, wantModel)
		}
		if args := codexCommandArgsForTest(stage, options); !containsCommandArgPair(args, "-m", wantModel) {
			t.Errorf("stage %q args = %q, want -m %s", stage, args, wantModel)
		}
	}
}

func TestConfigCommandOnlyAcceptsGlobal(t *testing.T) {
	for _, args := range [][]string{nil, {"--global"}} {
		if _, err := parseConfigCommandOptions(args); err != nil {
			t.Errorf("parseConfigCommandOptions(%q): %v", args, err)
		}
	}
	for _, args := range [][]string{{"--profile", "mada-code"}, {"--list-profiles"}, {"--unknown"}} {
		if _, err := parseConfigCommandOptions(args); err == nil {
			t.Errorf("parseConfigCommandOptions(%q) unexpectedly succeeded", args)
		}
	}
}

// TestUserSelectedModelReachesCodexCommand verifies a repository stage override becomes the Codex -m argument.
func TestUserSelectedModelReachesCodexCommand(t *testing.T) {
	const wantModel = "user-selected-model"
	repo := t.TempDir()
	body := []byte(`stages:
  execution:
    agent: codex
    model: user-selected-model
`)
	if err := os.WriteFile(filepath.Join(repo, "oz-flow.yaml"), body, 0o644); err != nil {
		t.Fatal(err)
	}

	config, err := LoadWorkflowConfig(repo)
	if err != nil {
		t.Fatalf("load user workflow config: %v", err)
	}

	options, err := config.StageOption("execution")
	if err != nil {
		t.Fatal(err)
	}
	args := codexExecArgs("/tmp/repo", "", options)
	if !containsCommandArgPair(args, "-m", wantModel) {
		t.Fatalf("codex args = %q, want -m %s", args, wantModel)
	}
}

// codexCommandArgsForTest returns the real planning or sealed-stage argument path used by Codex.
func codexCommandArgsForTest(stage string, options StageOptions) []string {
	if stage == "planning" {
		cmd := codexPlanningCommand(context.Background(), "codex", "prompt", strings.NewReader(""), options)
		return cmd.Args[1:]
	}
	return codexExecArgs("/tmp/repo", "", options)
}

// containsCommandArgPair reports whether one command flag is immediately followed by its value.
func containsCommandArgPair(args []string, flag, value string) bool {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == flag && args[i+1] == value {
			return true
		}
	}
	return false
}
