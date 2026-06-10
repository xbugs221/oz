// Package app contains opt-in OpenCode CLI integration smoke tests.
package app

import (
	"context"
	"os"
	"testing"
)

// TestOpenCodeCLIIntegrationSmoke is opt-in because it requires a configured OpenCode CLI.
func TestOpenCodeCLIIntegrationSmoke(t *testing.T) {
	if os.Getenv("CCFLOW_OPENCODE_INTEGRATION") != "1" {
		t.Skip("set CCFLOW_OPENCODE_INTEGRATION=1 to run opencode run smoke test")
	}
	repo := gitRepo(t)
	path, err := resolveCommand("opencode")
	if err != nil {
		t.Fatal(err)
	}
	runner := OpenCodeCLI{Path: path}
	sessionID, err := runner.Run(context.Background(), repo, "Say ok and make no file changes.", "", StageOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if sessionID == "" {
		t.Fatal("missing session id")
	}
	if _, err := runner.Run(context.Background(), repo, "Say ok again.", sessionID, StageOptions{}); err != nil {
		t.Fatal(err)
	}
}
