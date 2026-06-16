// Package app tests required_tests execution for active change acceptance contracts.
package app

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRunAcceptanceCommandPassesAndWritesResult verifies the runner command emits the persisted result JSON.
func TestRunAcceptanceCommandPassesAndWritesResult(t *testing.T) {
	repo := acceptanceRunRepo(t, "1-demo", "pass-test", `mkdir -p test-results/demo && printf ok > test-results/demo/runtime.log`, "test-results/demo/runtime.log")
	var stdout bytes.Buffer
	err := dispatchRunAcceptanceCommand(context.Background(), []string{"run-acceptance", "--change", "1-demo", "--json"}, &stdout, repo)
	if err != nil {
		t.Fatalf("run-acceptance should pass: %v", err)
	}
	result := decodeAcceptanceRunResult(t, stdout.String())
	if !result.Valid || result.Status != validationStatusPassed || result.Summary.Passed != 1 {
		t.Fatalf("unexpected result: %#v", result)
	}
	if !fileExists(filepath.Join(repo, filepath.FromSlash(result.ResultPath))) {
		t.Fatalf("missing result file %s", result.ResultPath)
	}
}

// TestRunAcceptanceFailureDoesNotShortCircuit verifies later required tests still execute after failure.
func TestRunAcceptanceFailureDoesNotShortCircuit(t *testing.T) {
	repo := t.TempDir()
	change := "1-demo"
	writeAcceptanceRunChange(t, repo, change, []acceptanceRunFixtureTest{
		{id: "fail-test", body: `mkdir -p test-results/demo && printf fail > test-results/demo/fail.log && exit 7`},
		{id: "pass-after-failure", body: `mkdir -p test-results/demo && printf pass > test-results/demo/pass.log`},
	}, []string{"test-results/demo/fail.log", "test-results/demo/pass.log"})
	result, err := runAcceptanceRequiredTests(context.Background(), repo, change)
	if err == nil {
		t.Fatal("run-acceptance should fail when one required test exits nonzero")
	}
	if result.Summary.Total != 2 || result.Summary.Passed != 1 || result.Summary.Failed != 1 {
		t.Fatalf("bad summary: %#v", result.Summary)
	}
	if !fileExists(filepath.Join(repo, "test-results/demo/pass.log")) {
		t.Fatal("second required test did not run after failure")
	}
}

// TestRunAcceptanceMissingEvidenceFails verifies passing tests cannot hide missing runtime evidence.
func TestRunAcceptanceMissingEvidenceFails(t *testing.T) {
	repo := acceptanceRunRepo(t, "1-demo", "pass-test", `printf no-evidence`, "test-results/demo/missing.log")
	result, err := runAcceptanceRequiredTests(context.Background(), repo, "1-demo")
	if err == nil {
		t.Fatal("run-acceptance should fail when evidence is missing")
	}
	if result.Valid || result.Summary.EvidenceMissing != 1 {
		t.Fatalf("missing evidence should fail result: %#v", result)
	}
}

// TestRunAcceptanceRejectsPathTraversal verifies change names cannot escape docs/changes.
func TestRunAcceptanceRejectsPathTraversal(t *testing.T) {
	_, err := runAcceptanceRequiredTests(context.Background(), t.TempDir(), "../outside")
	if err == nil || !strings.Contains(err.Error(), "非法路径片段") {
		t.Fatalf("expected path traversal rejection, got %v", err)
	}
}

// TestRunAcceptanceLogNameCannotEscapeResultDir verifies test ids are mapped to local filenames.
func TestRunAcceptanceLogNameCannotEscapeResultDir(t *testing.T) {
	name := safeAcceptanceLogName("../../evil/test")
	if strings.ContainsAny(name, `/\`) || strings.Contains(name, "..") || name == "" {
		t.Fatalf("unsafe log name: %q", name)
	}
}

type acceptanceRunFixtureTest struct {
	id   string
	body string
}

func acceptanceRunRepo(t *testing.T, change, testID, body, evidence string) string {
	t.Helper()
	repo := t.TempDir()
	writeAcceptanceRunChange(t, repo, change, []acceptanceRunFixtureTest{{id: testID, body: body}}, []string{evidence})
	return repo
}

func writeAcceptanceRunChange(t *testing.T, repo, change string, tests []acceptanceRunFixtureTest, evidence []string) {
	t.Helper()
	changeDir := filepath.Join(repo, "docs", "changes", change)
	testsDir := filepath.Join(changeDir, "tests")
	if err := os.MkdirAll(testsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	var requiredTests []string
	var coverageTests []string
	for _, test := range tests {
		script := filepath.Join(testsDir, test.id+".sh")
		writeTestFile(t, script, "#!/usr/bin/env bash\n# 文件功能目的：测试 acceptance run required_tests 执行。\nset -euo pipefail\n"+test.body+"\n")
		if err := os.Chmod(script, 0o755); err != nil {
			t.Fatal(err)
		}
		coverageTests = append(coverageTests, `"`+test.id+`"`)
		requiredTests = append(requiredTests, `{
      "id": "`+test.id+`",
      "source": "change_contract",
      "path": "docs/changes/`+change+`/tests/`+test.id+`.sh",
      "command": "bash docs/changes/`+change+`/tests/`+test.id+`.sh",
      "purpose": "execute `+test.id+`",
      "assertions": ["required test `+test.id+` records a business-level acceptance result"]
    }`)
	}
	var requiredEvidence []string
	var coverageEvidence []string
	for i, path := range evidence {
		id := "evidence-" + string(rune('a'+i))
		coverageEvidence = append(coverageEvidence, `"`+id+`"`)
		requiredEvidence = append(requiredEvidence, `{
      "id": "`+id+`",
      "kind": "runtime_log",
      "path": "`+path+`",
      "purpose": "runtime evidence for acceptance run"
    }`)
	}
	body := `{
  "summary": "acceptance run fixture",
  "coverage": [{
    "spec": "需求：acceptance run fixture / 场景：执行 required tests",
    "tests": [` + strings.Join(coverageTests, ",") + `],
    "evidence": [` + strings.Join(coverageEvidence, ",") + `],
    "risk": "fixture only"
  }],
  "required_tests": [` + strings.Join(requiredTests, ",") + `],
  "required_evidence": [` + strings.Join(requiredEvidence, ",") + `]
}`
	writeTestFile(t, filepath.Join(changeDir, "acceptance.json"), body)
}

func decodeAcceptanceRunResult(t *testing.T, body string) AcceptanceRunResult {
	t.Helper()
	var result AcceptanceRunResult
	if err := json.Unmarshal([]byte(strings.TrimSpace(body)), &result); err != nil {
		t.Fatalf("decode acceptance run JSON: %v\n%s", err, body)
	}
	return result
}
