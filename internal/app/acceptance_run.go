// Package app executes active change acceptance contracts for runner gates.
package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/xbugs221/oz/internal/acceptance"
)

const acceptanceRunKind = "acceptance_run"

// AcceptanceRunResult is the stable JSON result consumed by runners and QA reviewers.
type AcceptanceRunResult struct {
	Change      string                           `json:"change"`
	Valid       bool                             `json:"valid"`
	Status      string                           `json:"status"`
	ResultPath  string                           `json:"result_path"`
	StartedAt   string                           `json:"started_at"`
	FinishedAt  string                           `json:"finished_at"`
	Summary     AcceptanceRunSummary             `json:"summary"`
	Tests       []AcceptanceRunTestResult        `json:"tests"`
	Evidence    []AcceptanceRunEvidenceResult    `json:"evidence"`
	Coverage    []AcceptanceRunCoverageResult    `json:"coverage,omitempty"`
	Producers   []AcceptanceRunProducerResult    `json:"producers,omitempty"`
	Diagnostics []acceptance.LifecycleDiagnostic `json:"diagnostics"`
}

// AcceptanceRunSummary records aggregate counts for fast gate decisions.
type AcceptanceRunSummary struct {
	Total           int `json:"total"`
	Passed          int `json:"passed"`
	Failed          int `json:"failed"`
	EvidenceTotal   int `json:"evidence_total"`
	EvidencePresent int `json:"evidence_present"`
	EvidenceMissing int `json:"evidence_missing"`
}

// AcceptanceRunTestResult records one required_tests command execution.
type AcceptanceRunTestResult struct {
	ID         string `json:"id"`
	Source     string `json:"source"`
	Path       string `json:"path"`
	Command    string `json:"command"`
	Status     string `json:"status"`
	ExitCode   int    `json:"exit_code"`
	LogPath    string `json:"log_path"`
	DurationMS int64  `json:"duration_ms"`
}

// AcceptanceRunEvidenceResult records whether one required_evidence artifact exists.
type AcceptanceRunEvidenceResult struct {
	ID     string `json:"id"`
	Kind   string `json:"kind"`
	Path   string `json:"path"`
	Status string `json:"status"`
}

// AcceptanceRunCoverageResult records which required tests and evidence cover one spec.
type AcceptanceRunCoverageResult struct {
	Spec     string   `json:"spec"`
	Tests    []string `json:"tests"`
	Evidence []string `json:"evidence"`
}

// AcceptanceRunProducerResult records the required_tests that are expected to produce evidence.
type AcceptanceRunProducerResult struct {
	EvidenceID string   `json:"evidence_id"`
	Path       string   `json:"path"`
	Tests      []string `json:"tests"`
	Verified   bool     `json:"verified"`
}

// dispatchRunAcceptanceCommand parses the runner command and writes JSON even for failed tests.
func dispatchRunAcceptanceCommand(ctx context.Context, args []string, stdout io.Writer, repo string) error {
	if !hasFlag(args[1:], "--json") {
		return fmt.Errorf("用法：oz flow run-acceptance --change <change-name> --json")
	}
	changeName, err := requireFlagValue(args[1:], "--change")
	if err != nil {
		return err
	}
	result, runErr := runAcceptanceRequiredTests(ctx, repo, changeName)
	writeErr := writeJSON(stdout, result)
	if runErr != nil {
		return errors.Join(runErr, writeErr)
	}
	return writeErr
}

// runAcceptanceRequiredTests executes all required tests and checks declared runtime evidence.
func runAcceptanceRequiredTests(ctx context.Context, repo, changeName string) (AcceptanceRunResult, error) {
	if err := validateChangeNameForPath(changeName); err != nil {
		return AcceptanceRunResult{}, err
	}
	contract, err := ReadAcceptance(acceptancePath(repo, changeName))
	if err != nil {
		return AcceptanceRunResult{}, err
	}
	resultDir := filepath.Join(repo, "test-results", "acceptance-run", changeName)
	if err := os.MkdirAll(resultDir, 0o755); err != nil {
		return AcceptanceRunResult{}, err
	}
	result := AcceptanceRunResult{
		Change:     changeName,
		Status:     validationStatusPassed,
		ResultPath: filepath.ToSlash(filepath.Join("test-results", "acceptance-run", changeName, "result.json")),
		StartedAt:  time.Now().UTC().Format(time.RFC3339Nano),
	}
	result.Tests = runAcceptanceTests(ctx, repo, resultDir, contract.RequiredTests)
	result.Evidence = checkAcceptanceEvidence(repo, contract.RequiredEvidence)
	lifecycle := acceptance.ValidateLifecycle(repo, contract)
	result.Diagnostics = append(result.Diagnostics, lifecycle.Diagnostics...)
	result.Coverage = buildAcceptanceRunCoverage(contract.Coverage)
	result.Producers = buildAcceptanceRunProducers(repo, contract, lifecycle.Valid)
	result.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
	result.Summary = summarizeAcceptanceRun(result.Tests, result.Evidence)
	result.Valid = lifecycle.Valid && result.Summary.Failed == 0 && result.Summary.EvidenceMissing == 0
	for _, item := range result.Evidence {
		if item.Status == "missing" {
			result.Diagnostics = append(result.Diagnostics, acceptance.LifecycleDiagnostic{
				Code:       "required_evidence_runtime_missing",
				Severity:   "error",
				Message:    fmt.Sprintf("required_evidence %q runtime evidence missing: %s", item.ID, item.Path),
				EvidenceID: item.ID,
				Path:       item.Path,
			})
		}
	}
	if !result.Valid {
		result.Status = validationStatusFailed
	}
	if err := writeAcceptanceRunResult(repo, result); err != nil {
		return result, err
	}
	if !result.Valid {
		return result, fmt.Errorf("acceptance run failed: %s", result.ResultPath)
	}
	return result, nil
}

// runAcceptanceTests executes every required test in order without short-circuiting.
func runAcceptanceTests(ctx context.Context, repo, resultDir string, tests []AcceptanceTest) []AcceptanceRunTestResult {
	results := make([]AcceptanceRunTestResult, 0, len(tests))
	for _, test := range tests {
		start := time.Now()
		logRel := filepath.ToSlash(filepath.Join("test-results", "acceptance-run", filepath.Base(resultDir), safeAcceptanceLogName(test.ID)+".log"))
		logAbs := filepath.Join(repo, filepath.FromSlash(logRel))
		var output bytes.Buffer
		cmd := exec.CommandContext(ctx, "bash", "-lc", test.Command)
		cmd.Dir = repo
		cmd.Stdout = &output
		cmd.Stderr = &output
		err := cmd.Run()
		_ = os.WriteFile(logAbs, output.Bytes(), 0o644)
		exitCode := commandExitCode(err)
		status := validationStatusPassed
		if err != nil {
			status = validationStatusFailed
		}
		results = append(results, AcceptanceRunTestResult{
			ID:         test.ID,
			Source:     test.Source,
			Path:       test.Path,
			Command:    test.Command,
			Status:     status,
			ExitCode:   exitCode,
			LogPath:    logRel,
			DurationMS: time.Since(start).Milliseconds(),
		})
	}
	return results
}

// buildAcceptanceRunCoverage exposes the contract coverage that explains each runtime result.
func buildAcceptanceRunCoverage(coverage []Coverage) []AcceptanceRunCoverageResult {
	results := make([]AcceptanceRunCoverageResult, 0, len(coverage))
	for _, item := range coverage {
		results = append(results, AcceptanceRunCoverageResult{
			Spec:     item.Spec,
			Tests:    append([]string(nil), item.Tests...),
			Evidence: append([]string(nil), item.Evidence...),
		})
	}
	return results
}

// buildAcceptanceRunProducers exposes evidence-to-test producer links used by lifecycle validation.
func buildAcceptanceRunProducers(repo string, contract Acceptance, lifecycleValid bool) []AcceptanceRunProducerResult {
	tests := map[string]AcceptanceTest{}
	for _, test := range contract.RequiredTests {
		tests[test.ID] = test
	}
	results := make([]AcceptanceRunProducerResult, 0, len(contract.RequiredEvidence))
	for _, evidence := range contract.RequiredEvidence {
		results = append(results, AcceptanceRunProducerResult{
			EvidenceID: evidence.ID,
			Path:       evidence.Path,
			Tests:      producerTestIDs(evidence.ID, contract.Coverage),
			Verified:   lifecycleValid && acceptance.EvidenceHasProducer(repo, evidence, contract.Coverage, tests),
		})
	}
	return results
}

// producerTestIDs returns the required_tests ids bound to an evidence id by coverage.
func producerTestIDs(evidenceID string, coverage []Coverage) []string {
	seen := map[string]bool{}
	var ids []string
	for _, item := range coverage {
		if !acceptanceRunStringSliceContains(item.Evidence, evidenceID) {
			continue
		}
		for _, testID := range item.Tests {
			if seen[testID] {
				continue
			}
			seen[testID] = true
			ids = append(ids, testID)
		}
	}
	return ids
}

// acceptanceRunStringSliceContains reports whether a list contains a string exactly.
func acceptanceRunStringSliceContains(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}

// checkAcceptanceEvidence reports present only for declared regular files.
func checkAcceptanceEvidence(repo string, evidence []AcceptanceEvidence) []AcceptanceRunEvidenceResult {
	results := make([]AcceptanceRunEvidenceResult, 0, len(evidence))
	for _, item := range evidence {
		status := "missing"
		if fileExists(filepath.Join(repo, filepath.FromSlash(item.Path))) {
			status = "present"
		}
		results = append(results, AcceptanceRunEvidenceResult{ID: item.ID, Kind: item.Kind, Path: item.Path, Status: status})
	}
	return results
}

// summarizeAcceptanceRun builds counts used by CLI exit codes and sealed run gates.
func summarizeAcceptanceRun(tests []AcceptanceRunTestResult, evidence []AcceptanceRunEvidenceResult) AcceptanceRunSummary {
	summary := AcceptanceRunSummary{Total: len(tests), EvidenceTotal: len(evidence)}
	for _, test := range tests {
		if test.Status == validationStatusPassed {
			summary.Passed++
		} else {
			summary.Failed++
		}
	}
	for _, item := range evidence {
		if item.Status == "present" {
			summary.EvidencePresent++
		} else {
			summary.EvidenceMissing++
		}
	}
	return summary
}

// writeAcceptanceRunResult persists the exact JSON object emitted to runner stdout.
func writeAcceptanceRunResult(repo string, result AcceptanceRunResult) error {
	path := filepath.Join(repo, filepath.FromSlash(result.ResultPath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

var unsafeAcceptanceLogChars = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// safeAcceptanceLogName maps arbitrary test ids to result-local log filenames.
func safeAcceptanceLogName(id string) string {
	name := unsafeAcceptanceLogChars.ReplaceAllString(id, "-")
	name = strings.Trim(name, ".-_/\\")
	if name == "" {
		return "required-test"
	}
	if len(name) > 80 {
		return name[:80]
	}
	return name
}

// shouldRunAcceptanceGate limits required_tests execution to implementation stages.
func shouldRunAcceptanceGate(state State) bool {
	stage, err := parseWorkflowStage(state.Stage)
	return err == nil && (stage.isKind(workflowStageExecution) || stage.isKind(workflowStageFix))
}

// runAcceptanceGate runs the same executor used by the public runner command.
func (e *Engine) runAcceptanceGate(ctx context.Context, state *State) (bool, error) {
	if !shouldRunAcceptanceGate(*state) {
		return true, nil
	}
	if state.AcceptanceRun == nil {
		state.AcceptanceRun = map[string]StageValidationState{}
	}
	current := state.AcceptanceRun[state.Stage]
	current.Attempts++
	result, err := runAcceptanceRequiredTests(ctx, e.Repo, state.ChangeName)
	current.Kind = acceptanceRunKind
	current.LastArtifact = result.ResultPath
	current.Status = validationStatusPassed
	current.LastError = ""
	if err != nil {
		current.Status = validationStatusFailed
		current.LastError = err.Error()
	}
	state.AcceptanceRun[state.Stage] = current
	if err == nil {
		clearAcceptanceRunFailure(state)
		return true, nil
	}
	if current.Attempts >= state.Workflow.Validation.MaxAttemptsPerStage {
		state.Status = statusAcceptanceContractBlocked
		state.Stage = statusAcceptanceContractBlocked
		state.Error = fmt.Sprintf("%s: %s", err.Error(), result.ResultPath)
		return false, nil
	}
	if state.Stages == nil {
		state.Stages = map[string]string{}
	}
	state.Stages[state.Stage] = "validation_failed"
	return false, nil
}
