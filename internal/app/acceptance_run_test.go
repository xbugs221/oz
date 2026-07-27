// Package app tests required_tests execution for active change acceptance contracts.
package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/xbugs221/oz/internal/acceptance"
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
	wantLogPath := "test-results/acceptance-run/1-demo/pass-test.log"
	if len(result.Tests) != 1 || result.Tests[0].LogPath != wantLogPath {
		t.Fatalf("public acceptance log path = %#v, want %q", result.Tests, wantLogPath)
	}
	if len(result.Tests[0].LogHash) != sha256.Size*2 {
		t.Fatalf("public acceptance log hash = %q", result.Tests[0].LogHash)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("passing acceptance run should not report diagnostics: %#v", result.Diagnostics)
	}
	if len(result.Coverage) != 1 || result.Coverage[0].Tests[0] != "pass-test" || result.Coverage[0].Evidence[0] != "evidence-a" {
		t.Fatalf("passing acceptance run should expose coverage trace: %#v", result.Coverage)
	}
	if len(result.Producers) != 1 || result.Producers[0].EvidenceID != "evidence-a" || !result.Producers[0].Verified {
		t.Fatalf("passing acceptance run should expose verified producer trace: %#v", result.Producers)
	}
}

// TestRunAcceptanceLogWriteFailureFailsGate verifies command success cannot hide missing durable logs.
func TestRunAcceptanceLogWriteFailureFailsGate(t *testing.T) {
	repo := acceptanceRunRepo(
		t,
		"1-demo",
		"pass-test",
		`mkdir -p test-results/demo && printf ok > test-results/demo/runtime.log`,
		"test-results/demo/runtime.log",
	)
	logPath := filepath.Join(repo, "test-results", "acceptance-run", "1-demo", "pass-test.log")
	if err := os.MkdirAll(logPath, 0o755); err != nil {
		t.Fatal(err)
	}
	result, err := runAcceptanceRequiredTests(context.Background(), repo, "1-demo")
	if err == nil {
		t.Fatal("acceptance run should fail when its required-test log cannot be written")
	}
	if result.Valid || result.Summary.Failed != 1 || len(result.Tests) != 1 ||
		result.Tests[0].Status != validationStatusFailed || result.Tests[0].ExitCode != 0 {
		t.Fatalf("log persistence failure result = %#v", result)
	}
	if !hasAcceptanceDiagnostic(result.Diagnostics, "required_test_log_write_failed") {
		t.Fatalf("missing log persistence diagnostic: %#v", result.Diagnostics)
	}
}

// TestRunAcceptanceLogWriteReplacesLinks verifies logs never overwrite symbolic or hard-link targets.
func TestRunAcceptanceLogWriteReplacesLinks(t *testing.T) {
	for _, linkKind := range []string{"symlink", "hardlink"} {
		t.Run(linkKind, func(t *testing.T) {
			repo := t.TempDir()
			resultDirRel := "test-results/acceptance-run/1-demo/run/audit_1/attempt-1"
			resultDir, err := ensureAcceptanceArtifactDirectory(repo, resultDirRel)
			if err != nil {
				t.Fatal(err)
			}
			logRel := filepath.ToSlash(filepath.Join(resultDirRel, acceptanceLogName(0, "pass-test")))
			logPath := filepath.Join(repo, filepath.FromSlash(logRel))
			outside := filepath.Join(t.TempDir(), "outside.log")
			const sentinel = "outside sentinel\n"
			if err := os.WriteFile(outside, []byte(sentinel), 0o644); err != nil {
				t.Fatal(err)
			}
			if linkKind == "symlink" {
				if err := os.Symlink(outside, logPath); err != nil {
					t.Fatal(err)
				}
			} else if err := os.Link(outside, logPath); err != nil {
				t.Skipf("hard links unavailable: %v", err)
			}

			results, diagnostics := runAcceptanceTests(
				context.Background(),
				repo,
				resultDir,
				resultDirRel,
				true,
				[]AcceptanceTest{{ID: "pass-test", Command: "printf 'runner output\\n'"}},
			)
			if len(diagnostics) != 0 || len(results) != 1 || results[0].Status != validationStatusPassed {
				t.Fatalf("linked log persistence result=%#v diagnostics=%#v", results, diagnostics)
			}
			outsideData, err := os.ReadFile(outside)
			if err != nil {
				t.Fatal(err)
			}
			if string(outsideData) != sentinel {
				t.Fatalf("%s target was overwritten: %q", linkKind, outsideData)
			}
			info, err := os.Lstat(logPath)
			if err != nil {
				t.Fatal(err)
			}
			if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
				t.Fatalf("final log is not a regular file: %s", info.Mode())
			}
		})
	}
}

// TestWriteAcceptanceRunResultReplacesLinks verifies result JSON never follows an attacker-planted link.
func TestWriteAcceptanceRunResultReplacesLinks(t *testing.T) {
	for _, linkKind := range []string{"symlink", "hardlink"} {
		t.Run(linkKind, func(t *testing.T) {
			repo := t.TempDir()
			result := AcceptanceRunResult{
				Change:     "1-demo",
				Valid:      true,
				Status:     validationStatusPassed,
				ResultPath: "test-results/acceptance-run/1-demo/result.json",
			}
			resultPath, err := acceptanceArtifactFilePath(repo, result.ResultPath, true)
			if err != nil {
				t.Fatal(err)
			}
			outside := filepath.Join(t.TempDir(), "outside.json")
			const sentinel = "outside sentinel\n"
			if err := os.WriteFile(outside, []byte(sentinel), 0o644); err != nil {
				t.Fatal(err)
			}
			if linkKind == "symlink" {
				if err := os.Symlink(outside, resultPath); err != nil {
					t.Fatal(err)
				}
			} else if err := os.Link(outside, resultPath); err != nil {
				t.Skipf("hard links unavailable: %v", err)
			}

			if err := writeAcceptanceRunResult(repo, result); err != nil {
				t.Fatal(err)
			}
			outsideData, err := os.ReadFile(outside)
			if err != nil {
				t.Fatal(err)
			}
			if string(outsideData) != sentinel {
				t.Fatalf("%s target was overwritten: %q", linkKind, outsideData)
			}
			info, err := os.Lstat(resultPath)
			if err != nil {
				t.Fatal(err)
			}
			if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
				t.Fatalf("final result is not a regular file: %s", info.Mode())
			}
		})
	}
}

// TestAcceptanceArtifactWriteRejectsLinkedParent keeps the entire result namespace inside the repository.
func TestAcceptanceArtifactWriteRejectsLinkedParent(t *testing.T) {
	repo := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(repo, "test-results")); err != nil {
		t.Fatal(err)
	}
	relative := "test-results/acceptance-run/1-demo/result.json"
	if err := writeAcceptanceArtifactFile(repo, relative, []byte("{}\n")); err == nil {
		t.Fatal("acceptance artifact write followed a linked parent")
	}
	if _, err := os.Stat(filepath.Join(outside, "acceptance-run", "1-demo", "result.json")); !os.IsNotExist(err) {
		t.Fatalf("linked parent received an artifact: %v", err)
	}
}

// TestRunAcceptanceRejectsUnsafeEvidenceTypes verifies only safe readable regular files satisfy evidence.
func TestRunAcceptanceRejectsUnsafeEvidenceTypes(t *testing.T) {
	const evidencePath = "test-results/demo/runtime.log"
	t.Run("repository escape symlink", func(t *testing.T) {
		repo := acceptanceRunRepo(
			t,
			"1-demo",
			"pass-test",
			`printf '%s\n' test-results/demo/runtime.log >/dev/null`,
			evidencePath,
		)
		outside := filepath.Join(t.TempDir(), "external.log")
		if err := os.WriteFile(outside, []byte("external\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(repo, filepath.FromSlash(evidencePath))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, path); err != nil {
			t.Fatal(err)
		}
		assertUnsafeAcceptanceEvidence(t, repo, "1-demo")
	})
	t.Run("fifo", func(t *testing.T) {
		repo := acceptanceRunRepo(
			t,
			"1-demo",
			"pass-test",
			`printf '%s\n' test-results/demo/runtime.log >/dev/null`,
			evidencePath,
		)
		path := filepath.Join(repo, filepath.FromSlash(evidencePath))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := syscall.Mkfifo(path, 0o600); err != nil {
			t.Fatal(err)
		}
		assertUnsafeAcceptanceEvidence(t, repo, "1-demo")
	})
}

// assertUnsafeAcceptanceEvidence checks that unsafe evidence reaches the persisted result diagnostics.
func assertUnsafeAcceptanceEvidence(t *testing.T, repo, changeName string) {
	t.Helper()
	result, err := runAcceptanceRequiredTests(context.Background(), repo, changeName)
	if err == nil {
		t.Fatal("acceptance run should fail for unsafe required evidence")
	}
	if result.Valid || result.Summary.EvidenceMissing != 1 || len(result.Evidence) != 1 ||
		result.Evidence[0].Status != "unsafe" {
		t.Fatalf("unsafe evidence result = %#v", result)
	}
	if !hasAcceptanceDiagnostic(result.Diagnostics, "required_evidence_runtime_unsafe") {
		t.Fatalf("missing unsafe evidence diagnostic: %#v", result.Diagnostics)
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
	if !hasAcceptanceDiagnostic(result.Diagnostics, "required_evidence_runtime_missing") {
		t.Fatalf("missing evidence should emit lifecycle diagnostics: %#v", result.Diagnostics)
	}
}

// TestRunAcceptanceLifecycleDiagnosticsFailGate verifies lifecycle errors are not advisory only.
func TestRunAcceptanceLifecycleDiagnosticsFailGate(t *testing.T) {
	repo := t.TempDir()
	change := "1-demo"
	changeDir := filepath.Join(repo, "docs", "changes", change)
	testsDir := filepath.Join(changeDir, "tests")
	if err := os.MkdirAll(testsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	scriptRel := "docs/changes/" + change + "/tests/pass.sh"
	script := filepath.Join(repo, filepath.FromSlash(scriptRel))
	writeTestFile(t, script, "#!/usr/bin/env bash\n# 文件功能目的：生成 runtime evidence 以验证 lifecycle gate。\nset -euo pipefail\nmkdir -p test-results/demo\nprintf ok > test-results/demo/runtime.log\n")
	if err := os.Chmod(script, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(changeDir, "acceptance.json"), `{
  "summary": "invalid lifecycle fixture",
  "coverage": [{
    "spec": "需求：invalid lifecycle fixture / 场景：非法 required test path",
    "tests": ["bad-test"],
    "evidence": ["runtime-log"],
    "risk": "fixture only"
  }],
  "required_tests": [{
    "id": "bad-test",
    "source": "change_contract",
    "path": "../outside/evil.sh",
    "command": "bash `+scriptRel+` # ../outside/evil.sh",
    "purpose": "produce runtime-log at test-results/demo/runtime.log",
    "assertions": ["required test writes runtime-log to test-results/demo/runtime.log"]
  }],
  "required_evidence": [{
    "id": "runtime-log",
    "kind": "runtime_log",
    "path": "test-results/demo/runtime.log",
    "purpose": "prove runtime evidence exists"
  }]
}`)

	result, err := runAcceptanceRequiredTests(context.Background(), repo, change)
	if err == nil {
		t.Fatal("run-acceptance should fail when lifecycle diagnostics contain errors")
	}
	if result.Valid || result.Status != validationStatusFailed {
		t.Fatalf("lifecycle error should fail result: %#v", result)
	}
	if result.Summary.Failed != 0 || result.Summary.EvidenceMissing != 0 {
		t.Fatalf("fixture should isolate lifecycle gate failure: %#v", result.Summary)
	}
	if !hasAcceptanceDiagnostic(result.Diagnostics, "required_test_path") {
		t.Fatalf("expected required_test_path diagnostic: %#v", result.Diagnostics)
	}
	if len(result.Producers) != 1 || result.Producers[0].Verified {
		t.Fatalf("invalid lifecycle producer trace should not be verified: %#v", result.Producers)
	}
}

func hasAcceptanceDiagnostic(diagnostics []acceptance.LifecycleDiagnostic, code string) bool {
	// hasAcceptanceDiagnostic checks diagnostics by code without depending on ordering.
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == code {
			return true
		}
	}
	return false
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
