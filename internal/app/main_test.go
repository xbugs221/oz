// Package app builds the repository's current oz binary for CLI-backed integration tests.
package app

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// TestMain makes integration tests exercise the checked-out source instead of a stale installed oz.
func TestMain(m *testing.M) {
	tempDir, err := os.MkdirTemp("", "oz-app-tests-*")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	binary := filepath.Join(tempDir, "oz")
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		fmt.Fprintln(os.Stderr, "resolve app test source path")
		os.Exit(1)
	}
	build := exec.Command("go", "build", "-o", binary, "./cmd/oz")
	build.Dir = filepath.Clean(filepath.Join(filepath.Dir(sourceFile), "..", ".."))
	if output, buildErr := build.CombinedOutput(); buildErr != nil {
		fmt.Fprintf(os.Stderr, "build current oz for tests: %v\n%s", buildErr, output)
		os.Exit(1)
	}
	ozCommand = binary
	code := m.Run()
	_ = os.RemoveAll(tempDir)
	os.Exit(code)
}
