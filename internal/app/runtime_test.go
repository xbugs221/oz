// Package app tests durable runtime path selection and repository isolation.
package app

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestRuntimeRootPrefersXDGStateHome verifies explicit state roots isolate runtime data.
func TestRuntimeRootPrefersXDGStateHome(t *testing.T) {
	stateHome := t.TempDir()
	t.Setenv("XDG_STATE_HOME", stateHome)
	t.Setenv("LOCALAPPDATA", filepath.Join(t.TempDir(), "localappdata"))

	got, err := runtimeRootForGOOS("linux")
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Join(stateHome, "wo") {
		t.Fatalf("runtime root = %s, want XDG state dir", got)
	}
}

// TestRuntimeRootUsesPlatformDefaults verifies the fallback state directory contract.
func TestRuntimeRootUsesPlatformDefaults(t *testing.T) {
	home := t.TempDir()
	local := filepath.Join(t.TempDir(), "localappdata")
	t.Setenv("XDG_STATE_HOME", "")
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("LOCALAPPDATA", local)

	linuxRoot, err := runtimeRootForGOOS("linux")
	if err != nil {
		t.Fatal(err)
	}
	if linuxRoot != filepath.Join(home, ".local", "state", "wo") && runtime.GOOS != "windows" {
		t.Fatalf("linux runtime root = %s, want home state dir", linuxRoot)
	}
	windowsRoot, err := runtimeRootForGOOS("windows")
	if err != nil {
		t.Fatal(err)
	}
	if windowsRoot != filepath.Join(local, "wo") {
		t.Fatalf("windows runtime root = %s, want LOCALAPPDATA", windowsRoot)
	}
}

// TestRepoKeyIsStableAndIsolatesSameBasename verifies repos cannot share state by name alone.
func TestRepoKeyIsStableAndIsolatesSameBasename(t *testing.T) {
	first := filepath.Join(t.TempDir(), "project")
	second := filepath.Join(t.TempDir(), "project")

	if repoKey(first) != repoKey(first) {
		t.Fatal("repo key should be stable for one path")
	}
	if repoKey(first) == repoKey(second) {
		t.Fatalf("same basename repos share key: %s", repoKey(first))
	}
	if !strings.HasPrefix(repoKey(first), "project-") {
		t.Fatalf("repo key = %s, want readable basename prefix", repoKey(first))
	}
}

// TestRunAndBatchDirsUseUserStateRepoScope verifies runtime paths leave the repository.
func TestRunAndBatchDirsUseUserStateRepoScope(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "repo")
	stateHome := t.TempDir()
	t.Setenv("XDG_STATE_HOME", stateHome)

	run := runDir(repo, "run-1")
	batch := batchDir(repo, "batch-1")
	wantBase := filepath.Join(stateHome, "wo", "repos", repoKey(repo))
	for _, path := range []string{run, batch} {
		if !strings.HasPrefix(path, wantBase+string(filepath.Separator)) {
			t.Fatalf("runtime path = %s, want under %s", path, wantBase)
		}
		if strings.HasPrefix(path, repo+string(filepath.Separator)) {
			t.Fatalf("runtime path stayed in repo: %s", path)
		}
	}
}
