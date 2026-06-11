// Package app tests run with isolated user state so temporary repositories do
// not leak durable runtime files into the developer's real home directory.
package app

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

var testOzBinary string

// TestMain installs package-wide state and builds the real oz test binary once.
func TestMain(m *testing.M) {
	stateHome, err := os.MkdirTemp("", "wo-app-test-state-*")
	if err != nil {
		panic(err)
	}
	if err := os.Setenv("XDG_STATE_HOME", stateHome); err != nil {
		panic(err)
	}
	var binDir string
	if os.Getenv("WO_FAKE_OZ") != "1" {
		binDir, err = os.MkdirTemp("", "wo-app-test-bin-*")
		if err != nil {
			panic(err)
		}
		testOzBinary = filepath.Join(binDir, executableName("oz"))
		root, err := testRepoRoot()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		cmd := exec.Command("go", "build", "-o", testOzBinary, "./cmd/oz")
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			fmt.Fprintf(os.Stderr, "build test oz binary failed: %v\n%s", err, out)
			os.Exit(1)
		}
	}
	code := m.Run()
	_ = os.RemoveAll(stateHome)
	if binDir != "" {
		_ = os.RemoveAll(binDir)
	}
	os.Exit(code)
}

func testRepoRoot() (string, error) {
	// testRepoRoot walks upward from the package directory to find the module root.
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("go.mod not found")
		}
		dir = parent
	}
}

func installRealOz(t *testing.T) {
	// installRealOz points oz subprocess calls at the once-built repository binary.
	t.Helper()
	if testOzBinary == "" {
		t.Fatal("test oz binary was not built")
	}
	previous := ozCommand
	previousPrefix := ozCommandPrefix
	ozCommand = testOzBinary
	ozCommandPrefix = nil
	t.Cleanup(func() {
		ozCommand = previous
		ozCommandPrefix = previousPrefix
	})
}
