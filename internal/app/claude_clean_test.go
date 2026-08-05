package app

import (
	"os"
	"path/filepath"
	"testing"
)

// TestCleanClaudeSessionFilesRemovesByUUIDAndRespectsProtection proves cleanJSONLSessionFiles
// walks the claude projects/<encoded-cwd>/<uuid>.jsonl tree and only deletes targeted, unprotected sessions.
func TestCleanClaudeSessionFilesRemovesByUUIDAndRespectsProtection(t *testing.T) {
	root := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", root)
	projects := filepath.Join(root, "projects", "-tmp-repo")
	if err := os.MkdirAll(projects, 0o755); err != nil {
		t.Fatal(err)
	}
	target := "aaaa-1111-2222-3333"
	keep := "bbbb-4444-5555-6666"
	for _, id := range []string{target, keep} {
		if err := os.WriteFile(filepath.Join(projects, id+".jsonl"), []byte("{}\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	removed := cleanClaudeSessionFiles(map[string]bool{target: true})
	if removed != 1 {
		t.Fatalf("removed = %d, want 1", removed)
	}
	if _, err := os.Stat(filepath.Join(projects, target+".jsonl")); !os.IsNotExist(err) {
		t.Fatalf("target file should be removed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(projects, keep+".jsonl")); err != nil {
		t.Fatalf("untargeted file should remain: %v", err)
	}
}

// TestCollectAgentSessionsIncludesClaudeKey proves the claude: prefix is collected for cleanup.
func TestCollectAgentSessionsIncludesClaudeKey(t *testing.T) {
	state := State{Sessions: map[string]string{
		sessionStateKey("claude", "executor"): "sess-claude-1",
		sessionStateKey("codex", "qa"):        "sess-codex-2",
		"orphan":                              "sess-orphan-3",
		"":                                    "empty-session",
	}}
	got := map[string]bool{}
	collectAgentSessions(state, got)
	if !got["sess-claude-1"] || !got["sess-codex-2"] {
		t.Fatalf("collected = %v, want both claude and codex sessions", got)
	}
	if got["sess-orphan-3"] {
		t.Fatalf("orphan key should not be collected: %v", got)
	}
}

// TestClaudeSessionsRootRespectsEnv proves CLAUDE_CONFIG_DIR overrides the default root.
func TestClaudeSessionsRootRespectsEnv(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", "/custom/claude")
	if got := claudeSessionsRoot(); got != "/custom/claude/projects" {
		t.Fatalf("claudeSessionsRoot = %q, want /custom/claude/projects", got)
	}
}
