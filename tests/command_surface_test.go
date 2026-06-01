// Package tests verifies the command surface required by the simplified oz CLI proposal.
package tests

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func repoRoot(t *testing.T) string {
	// repoRoot walks upward from this test package to find the module root.
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}

func runOz(t *testing.T, dir string, args ...string) (string, error) {
	// runOz executes the real CLI entry point against a temporary project.
	t.Helper()
	cmdArgs := append([]string{"run", "./cmd/oz"}, args...)
	cmd := exec.Command("go", cmdArgs...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func TestSimplifiedCommandHelp(t *testing.T) {
	// TestSimplifiedCommandHelp covers daily commands and automation interfaces in top-level help.
	root := repoRoot(t)
	stdout, err := runOz(t, root, "--help")
	if err != nil {
		t.Fatalf("help failed: %v\n%s", err, stdout)
	}
	for _, want := range []string{
		"list | l [--json]",
		"install | i [--global | -g]",
		"create",
		"status <change> [--json]",
		"validate <change> [--json]",
		"archive <change> --yes",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("help missing %q:\n%s", want, stdout)
		}
	}
	for _, removed := range []string{"init", "plan", "exec"} {
		if strings.Contains(stdout, removed) {
			t.Fatalf("help still includes removed command %q:\n%s", removed, stdout)
		}
	}
}

func TestRemovedStageCommandsFail(t *testing.T) {
	// TestRemovedStageCommandsFail covers removed stages and keeps create from accepting stage arguments.
	root := repoRoot(t)
	for _, args := range [][]string{{"init"}, {"create", "需求"}, {"exec"}, {"plan"}} {
		stdout, err := runOz(t, root, args...)
		if err == nil {
			t.Fatalf("%v unexpectedly succeeded:\n%s", args, stdout)
		}
	}
}
