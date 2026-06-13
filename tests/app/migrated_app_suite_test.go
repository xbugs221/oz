// Package app_test keeps the legacy dynamic migrated-test runner available for shell contracts.
package app_test

import (
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestMigratedAppTestsRunWithGoToolchain compiles production app code with dynamic migrated tests.
func TestMigratedAppTestsRunWithGoToolchain(t *testing.T) {
	// Some root shell contracts still create short-lived .gotest files under
	// tests/app, so this test builds a temporary module copy where Go can
	// execute those dynamic inputs as package app.
	root := repoRoot(t)
	gotestFiles := listFiles(t, filepath.Join(root, "tests", "app"), ".gotest")
	if len(gotestFiles) == 0 {
		if strings.TrimSpace(os.Getenv("OZ_MIGRATED_APP_RUN")) != "" {
			t.Fatalf("OZ_MIGRATED_APP_RUN=%q was set, but tests/app has no dynamic .gotest files to execute", os.Getenv("OZ_MIGRATED_APP_RUN"))
		}
		t.Skip("tests/app has no dynamic .gotest files to execute")
	}

	work := t.TempDir()
	copyFile(t, filepath.Join(root, "go.mod"), filepath.Join(work, "go.mod"))
	copyFile(t, filepath.Join(root, "README.md"), filepath.Join(work, "README.md"))
	if fileExists(filepath.Join(root, "go.sum")) {
		copyFile(t, filepath.Join(root, "go.sum"), filepath.Join(work, "go.sum"))
	}
	copyDir(t, filepath.Join(root, "internal"), filepath.Join(work, "internal"), ".go")
	copyDir(t, filepath.Join(root, "cmd"), filepath.Join(work, "cmd"), ".go")
	copyDir(t, filepath.Join(root, "docs"), filepath.Join(work, "docs"), "")
	copyDir(t, filepath.Join(root, "profiles-template"), filepath.Join(work, "profiles-template"), "")
	copyDir(t, filepath.Join(root, "prompts-template"), filepath.Join(work, "prompts-template"), "")
	copyDir(t, filepath.Join(root, "skills"), filepath.Join(work, "skills"), "")
	for _, path := range gotestFiles {
		target := filepath.Join(work, "internal", "app", strings.TrimSuffix(filepath.Base(path), ".gotest")+".go")
		copyFile(t, path, target)
	}

	args := []string{"test", "./internal/app", "-timeout=30s"}
	if pattern := strings.TrimSpace(os.Getenv("OZ_MIGRATED_APP_RUN")); pattern != "" {
		args = append(args, "-run", pattern, "-count=1")
	}
	cmd := exec.Command("go", args...)
	cmd.Dir = work
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("migrated app tests failed: %v\n%s", err, out)
	}
	if strings.Contains(string(out), "no tests to run") {
		t.Fatalf("migrated app tests matched no tests for %q:\n%s", os.Getenv("OZ_MIGRATED_APP_RUN"), out)
	}
	if !strings.Contains(string(out), "ok") {
		t.Fatalf("migrated app tests output did not report a passing package:\n%s", out)
	}
}

// repoRoot walks up from the test package to the module root.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if fileExists(filepath.Join(dir, "go.mod")) {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}

// listFiles returns files under dir whose names end in suffix.
func listFiles(t *testing.T, dir, suffix string) []string {
	t.Helper()
	var files []string
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), suffix) {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

// copyDir copies matching regular files from src to dst while preserving layout.
func copyDir(t *testing.T, src, dst, suffix string) {
	t.Helper()
	err := filepath.WalkDir(src, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if suffix == ".go" && strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}
		if suffix != "" && !strings.HasSuffix(entry.Name(), suffix) {
			return nil
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		copyFile(t, path, filepath.Join(dst, rel))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// copyFile copies one regular file, creating parent directories as needed.
func copyFile(t *testing.T, src, dst string) {
	t.Helper()
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(src)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, data, info.Mode().Perm()); err != nil {
		t.Fatal(err)
	}
}

// fileExists reports whether path exists.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
