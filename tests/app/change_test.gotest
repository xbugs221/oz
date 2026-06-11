// Package app tests oz discovery behavior for workflow selection.
package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestListChangesUsesOzListJSON verifies active changes come from oz instead of local scanning.
func TestListChangesUsesOzListJSON(t *testing.T) {
	repo := t.TempDir()
	installFakeOz(t, "1-a-change", "2-b-change")

	changes, err := ListChanges(repo)
	if err != nil {
		t.Fatal(err)
	}
	got := []string{}
	for _, change := range changes {
		got = append(got, change.Name)
	}
	want := []string{"1-a-change", "2-b-change"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("changes = %v, want %v", got, want)
	}
}

// TestChangeTasksDoneUsesOzStatus verifies execution completion is read from oz status.
func TestChangeTasksDoneUsesOzStatus(t *testing.T) {
	repo := t.TempDir()
	mustChange(t, repo, "demo")
	done, err := ChangeTasksDone(repo, "demo")
	if err != nil {
		t.Fatal(err)
	}
	if done {
		t.Fatal("unchecked oz status tasks should not be done")
	}
	if err := os.WriteFile(filepath.Join(repo, "docs", "changes", "demo", "task.md"), []byte("- [x] task\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	done, err = ChangeTasksDone(repo, "demo")
	if err != nil {
		t.Fatal(err)
	}
	if !done {
		t.Fatal("checked oz status tasks should be done")
	}
}

// TestRunOzJSONMissingExecutableFails verifies oz absence cannot fabricate success.
func TestRunOzJSONMissingExecutableFails(t *testing.T) {
	previous := ozCommand
	previousPrefix := ozCommandPrefix
	ozCommand = "wo-missing-oz-for-test"
	ozCommandPrefix = nil
	t.Cleanup(func() {
		ozCommand = previous
		ozCommandPrefix = previousPrefix
	})
	if err := ValidateChange(t.TempDir(), "demo"); err == nil {
		t.Fatal("ValidateChange succeeded without oz executable")
	}
	if done, err := ChangeTasksDone(t.TempDir(), "demo"); err == nil || done {
		t.Fatalf("ChangeTasksDone = %v, %v; want missing oz error and not done", done, err)
	}
}

// TestChangeNameRejectsPathTraversal verifies local path boundaries do not rely on oz alone.
func TestChangeNameRejectsPathTraversal(t *testing.T) {
	repo := t.TempDir()
	for _, name := range []string{"", "../demo", "nested/demo", `nested\demo`, "demo..backup"} {
		if err := ValidateChange(repo, name); err == nil {
			t.Fatalf("ValidateChange(%q) succeeded, want path validation error", name)
		}
		if done, err := ChangeTasksDone(repo, name); err == nil || done {
			t.Fatalf("ChangeTasksDone(%q) = %v, %v; want path validation error", name, done, err)
		}
		if _, err := promptForStage(repo, State{RunID: "run-1", ChangeName: name, Stage: "execution", Sealed: true, Workflow: DefaultWorkflowConfig()}); err == nil {
			t.Fatalf("promptForStage(%q) succeeded, want path validation error", name)
		}
	}
}

// TestParseChangeSelectionSupportsSingleListRangeAndDedup verifies user-facing menu syntax.
func TestParseChangeSelectionSupportsSingleListRangeAndDedup(t *testing.T) {
	changes := []Change{{Name: "1-a"}, {Name: "2-b"}, {Name: "3-c"}}
	selected, err := ParseChangeSelection("3,1-2,2", changes)
	if err != nil {
		t.Fatal(err)
	}
	got := []string{}
	for _, change := range selected {
		got = append(got, change.Name)
	}
	want := "3-c,1-a,2-b"
	if strings.Join(got, ",") != want {
		t.Fatalf("selection = %v, want %s", got, want)
	}
}

// TestParseChangeSelectionSupportsSelectAll verifies a/A selects every visible candidate.
func TestParseChangeSelectionSupportsSelectAll(t *testing.T) {
	changes := []Change{{Name: "1-a"}, {Name: "2-b"}, {Name: "3-c"}}
	for _, input := range []string{"a", "A"} {
		selected, err := ParseChangeSelection(input, changes)
		if err != nil {
			t.Fatal(err)
		}
		got := []string{}
		for _, change := range selected {
			got = append(got, change.Name)
		}
		if strings.Join(got, ",") != "1-a,2-b,3-c" {
			t.Fatalf("ParseChangeSelection(%q) = %v, want all changes", input, got)
		}
	}
}

// TestParseChangeSelectionRejectsInvalidInput verifies no run is created from ambiguous input.
func TestParseChangeSelectionRejectsInvalidInput(t *testing.T) {
	changes := []Change{{Name: "1-a"}, {Name: "2-b"}}
	for _, input := range []string{"", "abc", "3", "2-1", "1--2", "1,", "1,a", "a,2"} {
		if _, err := ParseChangeSelection(input, changes); err == nil {
			t.Fatalf("ParseChangeSelection(%q) succeeded, want error", input)
		}
	}
}

// TestSortChangesByNumericPrefix verifies numbered changes run first in numeric order.
func TestSortChangesByNumericPrefix(t *testing.T) {
	changes := []Change{{Name: "x"}, {Name: "5-c"}, {Name: "3-a"}, {Name: "3-b"}, {Name: "abc"}}
	sorted := SortChangesByNumericPrefix(changes)
	got := []string{}
	for _, change := range sorted {
		got = append(got, change.Name)
	}
	want := "3-a,3-b,5-c,x,abc"
	if strings.Join(got, ",") != want {
		t.Fatalf("sorted = %v, want %s", got, want)
	}
}

// mustChange creates a minimal valid oz change for tests.
func mustChange(t *testing.T, repo, name string) {
	t.Helper()
	installFakeOz(t)
	root := filepath.Join(repo, "docs", "changes", name)
	if err := os.MkdirAll(filepath.Join(root, "tests"), 0o755); err != nil {
		t.Fatal(err)
	}
	for rel, body := range map[string]string{
		"proposal.md": "proposal",
		"design.md":   "design",
		"spec.md":     "spec",
		"task.md":     "- [ ] task\n",
	} {
		if err := os.WriteFile(filepath.Join(root, rel), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "acceptance.json"), []byte(acceptanceJSON()), 0o644); err != nil {
		t.Fatal(err)
	}
}

// installFakeOz points tests at a tiny oz-compatible JSON fixture command.
func installFakeOz(t *testing.T, listNames ...string) {
	t.Helper()
	previous := ozCommand
	previousPrefix := ozCommandPrefix
	ozCommand = os.Args[0]
	ozCommandPrefix = []string{"-test.run=TestFakeOzCommand", "--"}
	t.Setenv("WO_FAKE_OZ", "1")
	t.Setenv("WO_FAKE_OZ_LIST", strings.Join(listNames, " "))
	t.Cleanup(func() {
		ozCommand = previous
		ozCommandPrefix = previousPrefix
	})
}

// TestFakeOzCommand runs as a child process when tests need an oz executable.
func TestFakeOzCommand(t *testing.T) {
	if os.Getenv("WO_FAKE_OZ") != "1" {
		return
	}
	args := os.Args
	for i, arg := range args {
		if arg == "--" {
			fakeOzMain(args[i+1:])
			os.Exit(0)
		}
	}
	fmt.Fprintln(os.Stderr, "missing fake oz args")
	os.Exit(1)
}

func fakeOzMain(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "missing fake oz command")
		os.Exit(1)
	}
	switch args[0] {
	case "list":
		fakeOzList()
	case "status":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "missing change")
			os.Exit(1)
		}
		fakeOzStatus(args[1])
	case "validate":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "missing change")
			os.Exit(1)
		}
		if !fakeChangeKnown(args[1]) {
			fmt.Printf(`{"valid":false,"change":%q,"errors":["change not found"],"warnings":[],"artifacts":{}}`+"\n", args[1])
			os.Exit(1)
		}
		fmt.Printf(`{"valid":true,"change":%q,"errors":[],"warnings":[],"artifacts":{}}`+"\n", args[1])
	default:
		fmt.Fprintf(os.Stderr, "unexpected oz command: %s\n", strings.Join(args, " "))
		os.Exit(1)
	}
}

func fakeChangeKnown(change string) bool {
	for _, name := range strings.Fields(os.Getenv("WO_FAKE_OZ_LIST")) {
		if name == change {
			return true
		}
	}
	info, err := os.Stat(filepath.Join("docs", "changes", change))
	return err == nil && info.IsDir()
}

func fakeOzList() {
	names := strings.Fields(os.Getenv("WO_FAKE_OZ_LIST"))
	if len(names) == 0 {
		entries, _ := os.ReadDir(filepath.Join("docs", "changes"))
		for _, entry := range entries {
			if entry.IsDir() && entry.Name() != "archive" {
				names = append(names, entry.Name())
			}
		}
	}
	fmt.Print(`{"changes":[`)
	for i, name := range names {
		if i > 0 {
			fmt.Print(",")
		}
		fmt.Printf(`{"name":%q}`, name)
	}
	fmt.Println(`]}`)
}

func fakeOzStatus(change string) {
	taskPath := filepath.Join("docs", "changes", change, "task.md")
	total, done := fakeTaskProgress(taskPath)
	status := "incomplete"
	if total > 0 && done == total {
		status = "ready"
	}
	fmt.Printf(`{"change":%q,"status":%q,"tasks":{"total":%d,"done":%d}}`+"\n", change, status, total, done)
}

func fakeTaskProgress(path string) (int, int) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, 0
	}
	total, done := 0, 0
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "- [ ]") || strings.HasPrefix(trimmed, "- [x]") || strings.HasPrefix(trimmed, "- [X]") {
			total++
		}
		if strings.HasPrefix(trimmed, "- [x]") || strings.HasPrefix(trimmed, "- [X]") {
			done++
		}
	}
	return total, done
}
