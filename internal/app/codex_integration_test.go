// Package app contains opt-in Codex CLI integration smoke tests.
package app

import (
	"context"
	"os"
	"testing"
)

// TestCodexCLIIntegrationSmoke is opt-in because it requires a configured Codex CLI.
func TestCodexCLIIntegrationSmoke(t *testing.T) {
	if os.Getenv("CCFLOW_CODEX_INTEGRATION") != "1" {
		t.Skip("set CCFLOW_CODEX_INTEGRATION=1 to run codex exec smoke test")
	}
	repo := gitRepo(t)
	runner := NewCodexCLI()
	threadID, err := runner.Run(context.Background(), repo, "Say ok and make no file changes.", "", StageOptions{Reasoning: "low", Fast: false})
	if err != nil {
		t.Fatal(err)
	}
	if threadID == "" {
		t.Fatal("missing thread id")
	}
	if _, err := runner.Run(context.Background(), repo, "Say ok again.", threadID, StageOptions{Reasoning: "low", Fast: false}); err != nil {
		t.Fatal(err)
	}
}
