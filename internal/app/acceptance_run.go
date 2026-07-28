// Package app executes active change acceptance contracts for runner gates.
package app

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/xbugs221/oz/internal/acceptance"
)

const acceptanceRunKind = "acceptance_run"

// AcceptanceRunResult is the stable JSON result consumed by runners and QA reviewers.
type AcceptanceRunResult struct {
	Change               string                           `json:"change"`
	RunID                string                           `json:"run_id,omitempty"`
	Stage                string                           `json:"stage,omitempty"`
	Attempt              int                              `json:"attempt,omitempty"`
	DiffHash             string                           `json:"diff_hash,omitempty"`
	TestsHash            string                           `json:"tests_hash,omitempty"`
	EvidenceHash         string                           `json:"evidence_hash,omitempty"`
	TestsProgressHash    string                           `json:"tests_progress_hash,omitempty"`
	EvidenceProgressHash string                           `json:"evidence_progress_hash,omitempty"`
	ContractHash         string                           `json:"contract_hash,omitempty"`
	Valid                bool                             `json:"valid"`
	Status               string                           `json:"status"`
	ResultPath           string                           `json:"result_path"`
	StartedAt            string                           `json:"started_at"`
	FinishedAt           string                           `json:"finished_at"`
	Summary              AcceptanceRunSummary             `json:"summary"`
	Tests                []AcceptanceRunTestResult        `json:"tests"`
	Evidence             []AcceptanceRunEvidenceResult    `json:"evidence"`
	Coverage             []AcceptanceRunCoverageResult    `json:"coverage,omitempty"`
	Producers            []AcceptanceRunProducerResult    `json:"producers,omitempty"`
	Diagnostics          []acceptance.LifecycleDiagnostic `json:"diagnostics"`
}

// acceptanceRunBinding namespaces durable workflow results and binds them to sealed inputs.
type acceptanceRunBinding struct {
	RunID        string
	Stage        string
	Attempt      int
	DiffHash     string
	ContractHash string
}

// qualityAcceptanceCheckpointDriftError reports valid acceptance content that changed after review.
type qualityAcceptanceCheckpointDriftError struct {
	Stage     string
	Component string
	Result    string
	Current   string
	State     string
}

// Error preserves the existing operator-facing hash mismatch diagnostic.
func (e *qualityAcceptanceCheckpointDriftError) Error() string {
	return fmt.Sprintf(
		"阶段 %s acceptance %s hash 不一致：result=%s current=%s state=%s",
		e.Stage,
		e.Component,
		e.Result,
		e.Current,
		e.State,
	)
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
	ID              string `json:"id"`
	Source          string `json:"source"`
	Path            string `json:"path"`
	Command         string `json:"command"`
	Status          string `json:"status"`
	ExitCode        int    `json:"exit_code"`
	LogPath         string `json:"log_path"`
	LogHash         string `json:"log_hash,omitempty"`
	LogProgressHash string `json:"log_progress_hash,omitempty"`
	DurationMS      int64  `json:"duration_ms"`
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
	return runAcceptanceContract(ctx, repo, changeName, contract, acceptanceRunBinding{})
}

// runAcceptanceRequiredTestsForState executes the immutable contract sealed with a run.
func runAcceptanceRequiredTestsForState(ctx context.Context, repo string, state State, attempt int) (AcceptanceRunResult, error) {
	if err := validateChangeNameForPath(state.ChangeName); err != nil {
		return AcceptanceRunResult{}, err
	}
	if !usesQualityLoop(state.Workflow) {
		contract, err := readAcceptanceForState(repo, state)
		if err != nil {
			return AcceptanceRunResult{}, err
		}
		return runAcceptanceContract(ctx, repo, state.ChangeName, contract, acceptanceRunBinding{})
	}
	contract, contractHash, err := readVerifiedRunAcceptance(repo, state)
	if err != nil {
		return AcceptanceRunResult{}, err
	}
	return runAcceptanceContract(ctx, repo, state.ChangeName, contract, acceptanceRunBinding{
		RunID:        state.RunID,
		Stage:        state.Stage,
		Attempt:      attempt,
		DiffHash:     state.QualityLoop.DiffHash,
		ContractHash: contractHash,
	})
}

// runAcceptanceContract executes one already-resolved acceptance contract.
func runAcceptanceContract(ctx context.Context, repo, changeName string, contract Acceptance, binding acceptanceRunBinding) (AcceptanceRunResult, error) {
	resultDirRel, err := acceptanceRunResultDir(changeName, binding)
	if err != nil {
		return AcceptanceRunResult{}, err
	}
	resultDir, err := ensureAcceptanceArtifactDirectory(repo, resultDirRel)
	if err != nil {
		return AcceptanceRunResult{}, err
	}
	result := AcceptanceRunResult{
		Change:       changeName,
		RunID:        binding.RunID,
		Stage:        binding.Stage,
		Attempt:      binding.Attempt,
		DiffHash:     binding.DiffHash,
		ContractHash: binding.ContractHash,
		Status:       validationStatusPassed,
		ResultPath:   filepath.ToSlash(filepath.Join(resultDirRel, "result.json")),
		StartedAt:    time.Now().UTC().Format(time.RFC3339Nano),
	}
	var logDiagnostics []acceptance.LifecycleDiagnostic
	result.Tests, logDiagnostics = runAcceptanceTests(ctx, repo, resultDir, resultDirRel, binding.RunID != "", contract.RequiredTests)
	result.Evidence = checkAcceptanceEvidence(repo, contract.RequiredEvidence)
	if binding.RunID != "" {
		result.TestsHash, result.EvidenceHash, result.TestsProgressHash, result.EvidenceProgressHash =
			qualityAcceptanceSnapshotHashes(repo, result)
	}
	lifecycle := acceptance.ValidateLifecycle(repo, contract)
	result.Diagnostics = append(result.Diagnostics, lifecycle.Diagnostics...)
	result.Diagnostics = append(result.Diagnostics, logDiagnostics...)
	result.Coverage = buildAcceptanceRunCoverage(contract.Coverage)
	result.Producers = buildAcceptanceRunProducers(repo, contract, lifecycle.Valid)
	result.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
	result.Summary = summarizeAcceptanceRun(result.Tests, result.Evidence)
	result.Valid = lifecycle.Valid && result.Summary.Failed == 0 && result.Summary.EvidenceMissing == 0
	for _, item := range result.Evidence {
		diagnostic, ok := acceptanceEvidenceDiagnostic(item)
		if ok {
			result.Diagnostics = append(result.Diagnostics, diagnostic)
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

// acceptanceRunResultDir keeps the public command stable while isolating sealed workflow attempts.
func acceptanceRunResultDir(changeName string, binding acceptanceRunBinding) (string, error) {
	base := filepath.ToSlash(filepath.Join("test-results", "acceptance-run", changeName))
	if binding.RunID == "" {
		return base, nil
	}
	if !safeAcceptancePathSegment(binding.RunID) || !safeAcceptancePathSegment(binding.Stage) || binding.Attempt < 1 {
		return "", fmt.Errorf("acceptance run binding 非法：run=%q stage=%q attempt=%d", binding.RunID, binding.Stage, binding.Attempt)
	}
	return filepath.ToSlash(filepath.Join(base, binding.RunID, binding.Stage, fmt.Sprintf("attempt-%d", binding.Attempt))), nil
}

// safeAcceptancePathSegment prevents persisted state from escaping the result namespace.
func safeAcceptancePathSegment(value string) bool {
	clean := filepath.Clean(strings.TrimSpace(value))
	return clean != "" && clean != "." && clean == value && !filepath.IsAbs(clean) && filepath.Base(clean) == clean
}

// runAcceptanceTests executes every required test and reports output persistence failures.
func runAcceptanceTests(ctx context.Context, repo, resultDir, resultDirRel string, durableNames bool, tests []AcceptanceTest) ([]AcceptanceRunTestResult, []acceptance.LifecycleDiagnostic) {
	results := make([]AcceptanceRunTestResult, 0, len(tests))
	diagnostics := make([]acceptance.LifecycleDiagnostic, 0)
	for index, test := range tests {
		start := time.Now()
		logName := safeAcceptanceLogName(test.ID) + ".log"
		if durableNames {
			logName = acceptanceLogName(index, test.ID)
		}
		logRel := filepath.ToSlash(filepath.Join(resultDirRel, logName))
		var output bytes.Buffer
		cmd := exec.CommandContext(ctx, "bash", "-lc", test.Command)
		cmd.Dir = repo
		cmd.Stdout = &output
		cmd.Stderr = &output
		err := cmd.Run()
		safeOutput := redactQualityEnvironmentMarkers(output.String())
		logHash := acceptanceLogContentHash(safeOutput)
		logErr := writeAcceptanceArtifactFile(repo, logRel, []byte(safeOutput))
		exitCode := commandExitCode(err)
		status := validationStatusPassed
		if err != nil || logErr != nil {
			status = validationStatusFailed
		}
		if logErr != nil {
			diagnostics = append(diagnostics, acceptance.LifecycleDiagnostic{
				Code:     "required_test_log_write_failed",
				Severity: "error",
				Message:  fmt.Sprintf("required_test %q 日志写入失败: %s: %v", test.ID, logRel, logErr),
				TestID:   test.ID,
				Path:     logRel,
			})
		}
		results = append(results, AcceptanceRunTestResult{
			ID:              test.ID,
			Source:          test.Source,
			Path:            test.Path,
			Command:         test.Command,
			Status:          status,
			ExitCode:        exitCode,
			LogPath:         logRel,
			LogHash:         logHash,
			LogProgressHash: acceptanceLogProgressHash(safeOutput),
			DurationMS:      time.Since(start).Milliseconds(),
		})
	}
	return results, diagnostics
}

// acceptanceLogContentHash binds progress checks to the persisted, redacted command output.
func acceptanceLogContentHash(output string) string {
	sum := sha256.Sum256([]byte(output))
	return fmt.Sprintf("%x", sum[:])
}

// acceptanceLogProgressHash hashes normalized diagnostics for stalled-progress decisions.
func acceptanceLogProgressHash(output string) string {
	return acceptanceLogContentHash(qualityStableDiagnosticText(output))
}

// acceptanceLogName combines order, readable ID, and digest so sanitized IDs cannot collide.
func acceptanceLogName(index int, id string) string {
	sum := sha256.Sum256([]byte(id))
	return fmt.Sprintf("%03d-%s-%x.log", index+1, safeAcceptanceLogName(id), sum[:6])
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

// checkAcceptanceEvidence reports present only for readable repository-contained regular files.
func checkAcceptanceEvidence(repo string, evidence []AcceptanceEvidence) []AcceptanceRunEvidenceResult {
	results := make([]AcceptanceRunEvidenceResult, 0, len(evidence))
	for _, item := range evidence {
		status := acceptanceEvidenceRuntimeStatus(repo, item.Path)
		results = append(results, AcceptanceRunEvidenceResult{ID: item.ID, Kind: item.Kind, Path: item.Path, Status: status})
	}
	return results
}

// acceptanceEvidenceRuntimeStatus rejects traversal, repository escape, and non-regular artifacts.
func acceptanceEvidenceRuntimeStatus(repo, path string) string {
	clean := filepath.Clean(filepath.FromSlash(strings.TrimSpace(path)))
	if clean == "." || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "unsafe"
	}
	resolved, safe, err := qualityResolveEvidencePath(repo, filepath.Join(repo, clean))
	if !safe {
		return "unsafe"
	}
	if err != nil {
		return "missing"
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "unreadable"
	}
	if !info.Mode().IsRegular() {
		return "unsafe"
	}
	file, err := os.Open(resolved)
	if err != nil {
		return "unreadable"
	}
	if err := file.Close(); err != nil {
		return "unreadable"
	}
	return "present"
}

// acceptanceEvidenceDiagnostic maps every fail-closed evidence state to a durable diagnostic.
func acceptanceEvidenceDiagnostic(item AcceptanceRunEvidenceResult) (acceptance.LifecycleDiagnostic, bool) {
	if item.Status == "present" {
		return acceptance.LifecycleDiagnostic{}, false
	}
	code := "required_evidence_runtime_missing"
	reason := "missing"
	if item.Status == "unsafe" {
		code = "required_evidence_runtime_unsafe"
		reason = "unsafe or non-regular"
	} else if item.Status == "unreadable" {
		code = "required_evidence_runtime_unreadable"
		reason = "unreadable"
	}
	return acceptance.LifecycleDiagnostic{
		Code:       code,
		Severity:   "error",
		Message:    fmt.Sprintf("required_evidence %q runtime evidence %s: %s", item.ID, reason, item.Path),
		EvidenceID: item.ID,
		Path:       item.Path,
	}, true
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

// writeAcceptanceRunResult atomically persists the exact JSON object emitted to runner stdout.
func writeAcceptanceRunResult(repo string, result AcceptanceRunResult) error {
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	return writeAcceptanceArtifactFile(repo, result.ResultPath, append(data, '\n'))
}

// ensureAcceptanceArtifactDirectory creates a repository-contained directory without following links.
func ensureAcceptanceArtifactDirectory(repo, relative string) (string, error) {
	return resolveAcceptanceArtifactDirectory(repo, relative, true)
}

// resolveAcceptanceArtifactDirectory resolves each directory component and rejects symbolic links.
func resolveAcceptanceArtifactDirectory(repo, relative string, create bool) (string, error) {
	clean, err := cleanAcceptanceArtifactRelativePath(relative)
	if err != nil {
		return "", err
	}
	root, err := filepath.Abs(repo)
	if err != nil {
		return "", err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	current := root
	for _, segment := range strings.Split(clean, string(filepath.Separator)) {
		current = filepath.Join(current, segment)
		info, statErr := os.Lstat(current)
		if os.IsNotExist(statErr) && create {
			if mkdirErr := os.Mkdir(current, 0o755); mkdirErr != nil && !os.IsExist(mkdirErr) {
				return "", mkdirErr
			}
			info, statErr = os.Lstat(current)
		}
		if statErr != nil {
			return "", statErr
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return "", fmt.Errorf("acceptance artifact 目录必须是仓库内真实目录：%s", relative)
		}
	}
	return current, nil
}

// cleanAcceptanceArtifactRelativePath rejects absolute paths and repository traversal.
func cleanAcceptanceArtifactRelativePath(relative string) (string, error) {
	clean := filepath.Clean(filepath.FromSlash(strings.TrimSpace(relative)))
	if clean == "" || clean == "." || filepath.IsAbs(clean) || clean == ".." ||
		strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("acceptance artifact 路径非法：%q", relative)
	}
	return clean, nil
}

// acceptanceArtifactFilePath resolves a file beneath verified real repository directories.
func acceptanceArtifactFilePath(repo, relative string, createParent bool) (string, error) {
	clean, err := cleanAcceptanceArtifactRelativePath(relative)
	if err != nil {
		return "", err
	}
	dir, err := resolveAcceptanceArtifactDirectory(repo, filepath.Dir(clean), createParent)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, filepath.Base(clean)), nil
}

// writeAcceptanceArtifactFile replaces links atomically and verifies the final artifact is regular.
func writeAcceptanceArtifactFile(repo, relative string, data []byte) error {
	path, err := acceptanceArtifactFilePath(repo, relative, true)
	if err != nil {
		return err
	}
	if err := atomicWriteFile(path, data, 0o644); err != nil {
		return err
	}
	persisted, err := readAcceptanceArtifactFile(repo, relative)
	if err != nil {
		return err
	}
	if !bytes.Equal(persisted, data) {
		return fmt.Errorf("acceptance artifact 原子写入后内容不一致：%s", relative)
	}
	return nil
}

// readAcceptanceArtifactFile reads only regular files beneath verified real repository directories.
func readAcceptanceArtifactFile(repo, relative string) ([]byte, error) {
	path, err := acceptanceArtifactFilePath(repo, relative, false)
	if err != nil {
		return nil, err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("acceptance artifact 必须是普通文件：%s", relative)
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !opened.Mode().IsRegular() {
		return nil, fmt.Errorf("acceptance artifact 打开后不是普通文件：%s", relative)
	}
	if !os.SameFile(info, opened) {
		return nil, fmt.Errorf("acceptance artifact 检查期间被替换：%s", relative)
	}
	return io.ReadAll(file)
}

// verifyQualityAcceptanceCheckpoint replays trust checks for the latest passed stage result.
func verifyQualityAcceptanceCheckpoint(repo string, state State, stage string) (AcceptanceRunResult, error) {
	checkpoint, ok := state.AcceptanceRun[stage]
	if !ok || checkpoint.Kind != acceptanceRunKind || checkpoint.Status != validationStatusPassed ||
		checkpoint.Attempts < 1 || strings.TrimSpace(checkpoint.LastArtifact) == "" {
		return AcceptanceRunResult{}, fmt.Errorf("阶段 %s 缺少最后通过的 acceptance checkpoint", stage)
	}
	resultDir, err := acceptanceRunResultDir(state.ChangeName, acceptanceRunBinding{
		RunID: state.RunID, Stage: stage, Attempt: checkpoint.Attempts,
	})
	if err != nil {
		return AcceptanceRunResult{}, err
	}
	expectedPath := filepath.ToSlash(filepath.Join(resultDir, "result.json"))
	if checkpoint.LastArtifact != expectedPath {
		return AcceptanceRunResult{}, fmt.Errorf(
			"阶段 %s acceptance checkpoint 路径不一致：state=%s expected=%s",
			stage,
			checkpoint.LastArtifact,
			expectedPath,
		)
	}
	data, err := readAcceptanceArtifactFile(repo, expectedPath)
	if err != nil {
		return AcceptanceRunResult{}, fmt.Errorf("读取 acceptance checkpoint 失败：%w", err)
	}
	var result AcceptanceRunResult
	if err := decodeStrictArtifactJSON(data, &result); err != nil {
		return AcceptanceRunResult{}, fmt.Errorf("解析 acceptance checkpoint 失败：%w", err)
	}
	contract, contractHash, err := readVerifiedRunAcceptance(repo, state)
	if err != nil {
		return AcceptanceRunResult{}, err
	}
	if err := validateQualityAcceptanceCheckpointIdentity(state, stage, checkpoint, expectedPath, contractHash, result); err != nil {
		return AcceptanceRunResult{}, err
	}
	if err := validateQualityAcceptanceCheckpointTests(repo, resultDir, contract, result); err != nil {
		return AcceptanceRunResult{}, err
	}
	currentEvidence := checkAcceptanceEvidence(repo, contract.RequiredEvidence)
	if err := validateQualityAcceptanceCheckpointEvidence(contract, result.Evidence, currentEvidence); err != nil {
		return AcceptanceRunResult{}, err
	}
	if result.Summary != summarizeAcceptanceRun(result.Tests, result.Evidence) {
		return AcceptanceRunResult{}, fmt.Errorf("阶段 %s acceptance summary 与明细不一致", stage)
	}
	testsHash, evidenceHash := qualityAcceptanceProgressHashes(repo, result)
	if result.TestsHash != testsHash || result.TestsHash != state.QualityLoop.TestsHash {
		return AcceptanceRunResult{}, &qualityAcceptanceCheckpointDriftError{
			Stage: stage, Component: "tests", Result: result.TestsHash,
			Current: testsHash, State: state.QualityLoop.TestsHash,
		}
	}
	if result.EvidenceHash != evidenceHash || result.EvidenceHash != state.QualityLoop.EvidenceHash {
		return AcceptanceRunResult{}, &qualityAcceptanceCheckpointDriftError{
			Stage: stage, Component: "evidence", Result: result.EvidenceHash,
			Current: evidenceHash, State: state.QualityLoop.EvidenceHash,
		}
	}
	return result, nil
}

// validateQualityAcceptanceCheckpointIdentity binds persisted metadata to engine-owned state.
func validateQualityAcceptanceCheckpointIdentity(
	state State,
	stage string,
	checkpoint StageValidationState,
	expectedPath string,
	contractHash string,
	result AcceptanceRunResult,
) error {
	switch {
	case result.Change != state.ChangeName:
		return fmt.Errorf("阶段 %s acceptance change 绑定不一致", stage)
	case result.RunID != state.RunID || result.Stage != stage || result.Attempt != checkpoint.Attempts:
		return fmt.Errorf("阶段 %s acceptance run/stage/attempt 绑定不一致", stage)
	case checkpoint.DiffHash == "" || result.DiffHash != checkpoint.DiffHash || result.DiffHash != state.QualityLoop.DiffHash:
		return fmt.Errorf("阶段 %s acceptance diff 绑定不一致", stage)
	case result.ContractHash != contractHash || result.ContractHash != state.AcceptanceHash:
		return fmt.Errorf("阶段 %s acceptance contract 绑定不一致", stage)
	case result.ResultPath != expectedPath:
		return fmt.Errorf("阶段 %s acceptance result_path 绑定不一致", stage)
	case !result.Valid || result.Status != validationStatusPassed:
		return fmt.Errorf("阶段 %s acceptance checkpoint 不是有效通过结果", stage)
	default:
		return nil
	}
}

// validateQualityAcceptanceCheckpointTests binds test metadata and every persisted log to the sealed contract.
func validateQualityAcceptanceCheckpointTests(repo, resultDir string, contract Acceptance, result AcceptanceRunResult) error {
	if len(result.Tests) != len(contract.RequiredTests) {
		return fmt.Errorf("acceptance checkpoint required_tests 数量不一致")
	}
	for index, expected := range contract.RequiredTests {
		actual := result.Tests[index]
		if actual.ID != expected.ID || actual.Source != expected.Source || actual.Path != expected.Path ||
			actual.Command != expected.Command || actual.Status != validationStatusPassed || actual.ExitCode != 0 {
			return fmt.Errorf("acceptance checkpoint required_test %q 结果或合同绑定不一致", expected.ID)
		}
		expectedLog := filepath.ToSlash(filepath.Join(resultDir, acceptanceLogName(index, expected.ID)))
		if actual.LogPath != expectedLog {
			return fmt.Errorf("acceptance checkpoint required_test %q 日志路径不一致", expected.ID)
		}
		data, err := readAcceptanceArtifactFile(repo, actual.LogPath)
		if err != nil {
			return fmt.Errorf("读取 acceptance checkpoint required_test %q 日志失败：%w", expected.ID, err)
		}
		if actual.LogHash == "" || acceptanceLogContentHash(string(data)) != actual.LogHash {
			return fmt.Errorf("acceptance checkpoint required_test %q 日志哈希不一致", expected.ID)
		}
		progressHash := acceptanceLogProgressHash(string(data))
		if actual.LogProgressHash != "" && progressHash != actual.LogProgressHash {
			return fmt.Errorf("acceptance checkpoint required_test %q 进展日志哈希不一致", expected.ID)
		}
	}
	return nil
}

// validateQualityAcceptanceCheckpointEvidence requires persisted and current evidence to match the sealed contract.
func validateQualityAcceptanceCheckpointEvidence(
	contract Acceptance,
	persisted []AcceptanceRunEvidenceResult,
	current []AcceptanceRunEvidenceResult,
) error {
	if len(persisted) != len(contract.RequiredEvidence) || len(current) != len(contract.RequiredEvidence) {
		return fmt.Errorf("acceptance checkpoint required_evidence 数量不一致")
	}
	for index, expected := range contract.RequiredEvidence {
		recorded := persisted[index]
		observed := current[index]
		if recorded.ID != expected.ID || recorded.Kind != expected.Kind || recorded.Path != expected.Path ||
			recorded.Status != "present" || observed.ID != recorded.ID || observed.Kind != recorded.Kind ||
			observed.Path != recorded.Path || observed.Status != recorded.Status {
			return fmt.Errorf("acceptance checkpoint required_evidence %q 已漂移或合同绑定不一致", expected.ID)
		}
	}
	return nil
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
	return err == nil && (stage.isKind(workflowStageExecution) || stage.isKind(workflowStageRepair) || stage.isKind(workflowStageAudit) || stage.isKind(workflowStageTargetedRepair) || stage.isKind(workflowStageFix))
}

// runAcceptanceGate runs the same executor used by the public runner command.
func (e *Engine) runAcceptanceGate(ctx context.Context, state *State) (bool, error) {
	if !shouldRunAcceptanceGate(*state) {
		return true, nil
	}
	current, err := e.reserveAcceptanceRunAttempt(state)
	if err != nil {
		return false, err
	}
	result, err := runAcceptanceRequiredTestsForState(ctx, e.Repo, *state, current.Attempts)
	if usesQualityLoop(state.Workflow) {
		state.QualityLoop.TestsHash = result.TestsHash
		state.QualityLoop.EvidenceHash = result.EvidenceHash
		state.QualityLoop.TestsProgressHash = result.TestsProgressHash
		state.QualityLoop.EvidenceProgressHash = result.EvidenceProgressHash
		if state.QualityLoop.TestsProgressHash == "" || state.QualityLoop.EvidenceProgressHash == "" {
			state.QualityLoop.TestsProgressHash, state.QualityLoop.EvidenceProgressHash =
				qualityAcceptanceOutcomeHashes(e.Repo, result)
		}
	}
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
	if usesQualityLoop(state.Workflow) {
		if names := qualityEnvironmentNamesFromAcceptanceResult(e.Repo, result); len(names) > 0 {
			if blockErr := blockQualityEnvironment(e.Repo, state, names); blockErr != nil {
				return false, blockErr
			}
			return false, nil
		}
	}
	if recordQualityGateFailure(state, acceptanceRunKind, acceptanceGateFailureKey(result, err)) {
		return false, nil
	}
	if isQualityLoopRepairStage(*state) {
		state.Stages[state.Stage] = "validation_failed"
		return false, nil
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

// reserveAcceptanceRunAttempt persists a quality-loop attempt before commands can write its result.
func (e *Engine) reserveAcceptanceRunAttempt(state *State) (StageValidationState, error) {
	if state.AcceptanceRun == nil {
		state.AcceptanceRun = map[string]StageValidationState{}
	}
	current := state.AcceptanceRun[state.Stage]
	current.Attempts++
	if !usesQualityLoop(state.Workflow) {
		return current, nil
	}
	current.Kind = acceptanceRunKind
	current.Status = statusRunning
	current.LastError = ""
	state.AcceptanceRun[state.Stage] = current
	if err := saveState(e.Repo, *state); err != nil {
		return StageValidationState{}, err
	}
	return current, nil
}

// acceptanceGateFailureKey removes attempt-specific paths from stalled-failure detection.
func acceptanceGateFailureKey(result AcceptanceRunResult, runErr error) string {
	parts := make([]string, 0, len(result.Tests)+len(result.Evidence)+len(result.Diagnostics)+1)
	for _, item := range result.Tests {
		if item.Status == validationStatusPassed {
			continue
		}
		parts = append(parts, fmt.Sprintf(
			"test\x00%s\x00%s\x00%d\x00%s",
			item.ID,
			item.Status,
			item.ExitCode,
			item.LogProgressHash,
		))
	}
	for _, item := range result.Evidence {
		if item.Status != "missing" {
			continue
		}
		parts = append(parts, fmt.Sprintf("evidence\x00%s\x00%s", item.ID, item.Status))
	}
	for _, item := range result.Diagnostics {
		if strings.TrimSpace(item.Code) == "" {
			continue
		}
		parts = append(parts, fmt.Sprintf("diagnostic\x00%s\x00%s", item.Code, item.Severity))
	}
	if len(parts) == 0 && runErr != nil {
		message := runErr.Error()
		if result.ResultPath != "" {
			message = strings.ReplaceAll(message, result.ResultPath, "<result>")
		}
		parts = append(parts, "error\x00"+message)
	}
	sort.Strings(parts)
	return strings.Join(parts, "\n")
}

// verifyQualityLoopActiveAcceptance prevents repair stages from changing the delivery contract.
func (e *Engine) verifyQualityLoopActiveAcceptance(state State) error {
	if !usesQualityLoop(state.Workflow) {
		return nil
	}
	return verifyAcceptanceMatchesSealed(acceptancePath(e.Repo, state.ChangeName), state.AcceptanceHash)
}

// verifyQualityLoopArchivedAcceptance ensures archive moved the exact sealed contract.
func (e *Engine) verifyQualityLoopArchivedAcceptance(state State) error {
	if !usesQualityLoop(state.Workflow) {
		return nil
	}
	path, err := archivedAcceptancePath(e.Repo, state.ChangeName)
	if err != nil {
		return err
	}
	return verifyArchivedAcceptanceMatchesSealed(e.Repo, filepath.Dir(path), state.ChangeName, state.AcceptanceHash)
}

// verifyArchivedAcceptanceMatchesSealed permits only deterministic path rewrites made by oz archive.
func verifyArchivedAcceptanceMatchesSealed(repo, archivedDir, changeName, expected string) error {
	if !validAcceptanceHash(strings.TrimSpace(expected)) {
		return fmt.Errorf("delivery acceptance 缺少有效的封存完整性哈希")
	}
	path := filepath.Join(archivedDir, "acceptance.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("读取 delivery acceptance %s 失败: %w", path, err)
	}
	normalized, err := normalizeQualityLoopArchivedReferences(repo, archivedDir, archivedDir, changeName, data)
	if err != nil {
		return err
	}
	actual := acceptanceContentHash(normalized)
	if actual != strings.TrimSpace(expected) {
		return fmt.Errorf("delivery acceptance 与封存合同不一致: path=%s expected=%s actual=%s", path, expected, actual)
	}
	return nil
}

// verifyAcceptanceMatchesSealed compares one delivery contract with the engine-owned run digest.
func verifyAcceptanceMatchesSealed(path, expected string) error {
	if !validAcceptanceHash(strings.TrimSpace(expected)) {
		return fmt.Errorf("delivery acceptance 缺少有效的封存完整性哈希")
	}
	actual, err := acceptanceFileHash(path)
	if err != nil {
		return fmt.Errorf("读取 delivery acceptance %s 失败: %w", path, err)
	}
	if actual != strings.TrimSpace(expected) {
		return fmt.Errorf("delivery acceptance 与封存合同不一致: path=%s expected=%s actual=%s", path, expected, actual)
	}
	return nil
}

// qualityEnvironmentNamesFromAcceptanceResult scans failed-test logs for explicit safe block markers.
func qualityEnvironmentNamesFromAcceptanceResult(repo string, result AcceptanceRunResult) []string {
	var output strings.Builder
	for _, test := range result.Tests {
		if test.Status == validationStatusPassed || strings.TrimSpace(test.LogPath) == "" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(repo, filepath.FromSlash(test.LogPath)))
		if err != nil {
			continue
		}
		output.Write(data)
		output.WriteByte('\n')
	}
	return qualityEnvironmentNamesFromText(output.String())
}

// qualityAcceptanceProgressHashes records semantic test results and required evidence content.
func qualityAcceptanceProgressHashes(repo string, result AcceptanceRunResult) (string, string) {
	tests, evidence, _, _ := qualityAcceptanceSnapshotHashes(repo, result)
	return tests, evidence
}

// qualityAcceptanceOutcomeHashes records stable gate outcomes and evidence content for progress decisions.
func qualityAcceptanceOutcomeHashes(repo string, result AcceptanceRunResult) (string, string) {
	_, _, tests, evidence := qualityAcceptanceSnapshotHashes(repo, result)
	return tests, evidence
}

// qualityAcceptanceSnapshotHashes derives raw and stable evidence hashes from one file observation.
func qualityAcceptanceSnapshotHashes(repo string, result AcceptanceRunResult) (string, string, string, string) {
	testParts := make([]string, 0, len(result.Tests))
	testProgressParts := make([]string, 0, len(result.Tests))
	for _, item := range result.Tests {
		testParts = append(testParts, fmt.Sprintf("%s\x00%s\x00%s\x00%d\x00%s", item.ID, item.Command, item.Status, item.ExitCode, item.LogHash))
		testProgressParts = append(testProgressParts, fmt.Sprintf(
			"%s\x00%s\x00%s\x00%d\x00%s",
			item.ID,
			item.Command,
			item.Status,
			item.ExitCode,
			item.LogProgressHash,
		))
	}
	evidenceParts := make([]string, 0, len(result.Evidence))
	evidenceProgressParts := make([]string, 0, len(result.Evidence))
	for _, item := range result.Evidence {
		rawContent, stableContent := item.Status, item.Status
		if item.Status == "present" {
			rawContent, stableContent, _ = qualityEvidencePathHashes(repo, item.Path, item.Kind)
		}
		evidenceParts = append(evidenceParts, fmt.Sprintf(
			"%s\x00%s\x00%s\x00%s",
			item.ID,
			item.Path,
			item.Status,
			rawContent,
		))
		evidenceProgressParts = append(evidenceProgressParts, fmt.Sprintf(
			"%s\x00%s\x00%s\x00%s",
			item.ID,
			item.Path,
			item.Status,
			stableContent,
		))
	}
	return qualityHashStrings(testParts...),
		qualityHashStrings(evidenceParts...),
		qualityHashStrings(testProgressParts...),
		qualityHashStrings(evidenceProgressParts...)
}

// qualityCurrentEvidenceHash re-reads sealed required evidence for stalled-run recovery.
func qualityCurrentEvidenceHash(repo string, state State) (string, error) {
	observation, err := qualityCurrentEvidenceObservationForState(repo, state)
	return observation.RawHash, err
}

// qualityCurrentEvidenceHashes returns raw integrity and semantic progress hashes together.
func qualityCurrentEvidenceHashes(repo string, state State) (string, string, error) {
	observation, err := qualityCurrentEvidenceObservationForState(repo, state)
	return observation.RawHash, observation.ProgressHash, err
}

// qualityCurrentEvidenceObservation records whether every required file can safely prove progress.
type qualityCurrentEvidenceObservation struct {
	RawHash          string
	ProgressHash     string
	ProgressEligible bool
}

// qualityCurrentEvidenceObservationForState reads sealed evidence into one recovery observation.
func qualityCurrentEvidenceObservationForState(repo string, state State) (qualityCurrentEvidenceObservation, error) {
	contract, err := readAcceptanceForState(repo, state)
	if err != nil {
		return qualityCurrentEvidenceObservation{}, err
	}
	result := AcceptanceRunResult{Evidence: checkAcceptanceEvidence(repo, contract.RequiredEvidence)}
	rawParts := make([]string, 0, len(result.Evidence))
	semanticParts := make([]string, 0, len(result.Evidence))
	eligible := true
	for _, item := range result.Evidence {
		rawContent, semanticContent := item.Status, item.Status
		if item.Status == "present" {
			var itemEligible bool
			rawContent, semanticContent, itemEligible = qualityEvidencePathHashes(repo, item.Path, item.Kind)
			eligible = eligible && itemEligible
		} else {
			eligible = false
		}
		rawParts = append(rawParts, fmt.Sprintf("%s\x00%s\x00%s\x00%s",
			item.ID, item.Path, item.Status, rawContent))
		semanticParts = append(semanticParts, fmt.Sprintf("%s\x00%s\x00%s\x00%s",
			item.ID, item.Path, item.Status, semanticContent))
	}
	return qualityCurrentEvidenceObservation{
		RawHash:          qualityHashStrings(rawParts...),
		ProgressHash:     qualityHashStrings(semanticParts...),
		ProgressEligible: eligible,
	}, nil
}

// qualityEvidenceContentHash hashes only safe repository-relative evidence files.
func qualityEvidenceContentHash(repo, path string) string {
	return qualityEvidencePathHash(repo, path, false, "")
}

// qualityEvidenceProgressHash normalizes known text noise while preserving substantive evidence changes.
func qualityEvidenceProgressHash(repo, path, kind string) string {
	return qualityEvidencePathHash(repo, path, true, kind)
}

// qualityEvidencePathHashes reads one regular evidence file once for raw and semantic recovery hashes.
func qualityEvidencePathHashes(repo, path, kind string) (string, string, bool) {
	clean := filepath.Clean(filepath.FromSlash(strings.TrimSpace(path)))
	if clean == "." || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "unsafe", "unsafe", false
	}
	resolved, safe, err := qualityResolveEvidencePath(repo, filepath.Join(repo, clean))
	if !safe {
		return "unsafe", "unsafe", false
	}
	if err != nil {
		return "unavailable", "unavailable", false
	}
	raw, semantic, eligible, err := qualityHashEvidenceFilePair(resolved, kind)
	if err != nil {
		return "unavailable", "unavailable", false
	}
	return raw, semantic, eligible
}

// qualityEvidencePathHash hashes one safe evidence file or directory in raw or semantic mode.
func qualityEvidencePathHash(repo, path string, stable bool, kind string) string {
	clean := filepath.Clean(filepath.FromSlash(strings.TrimSpace(path)))
	if clean == "." || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "unsafe"
	}
	fullPath := filepath.Join(repo, clean)
	resolvedPath, safe, err := qualityResolveEvidencePath(repo, fullPath)
	if !safe {
		return "unsafe"
	}
	if err != nil {
		return "unavailable"
	}
	info, err := os.Stat(resolvedPath)
	if err != nil {
		return "unavailable"
	}
	hash := sha256.New()
	if !info.IsDir() {
		if err := qualityHashEvidenceFile(hash, resolvedPath, stable, kind); err != nil {
			return "unavailable"
		}
		return fmt.Sprintf("%x", hash.Sum(nil))
	}
	if err := filepath.WalkDir(resolvedPath, func(current string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(resolvedPath, current)
		if err != nil {
			return err
		}
		fmt.Fprintf(hash, "%s\x00", filepath.ToSlash(relative))
		if entry.Type().IsRegular() {
			return qualityHashEvidenceFile(hash, current, stable, kind)
		}
		return nil
	}); err != nil {
		return "unavailable"
	}
	return fmt.Sprintf("%x", hash.Sum(nil))
}

// qualityResolveEvidencePath resolves symlinks and rejects evidence whose real path escapes the repository.
func qualityResolveEvidencePath(repo, fullPath string) (string, bool, error) {
	repoRoot, err := filepath.EvalSymlinks(repo)
	if err != nil {
		return "", true, err
	}
	repoRoot, err = filepath.Abs(repoRoot)
	if err != nil {
		return "", true, err
	}
	resolvedPath, err := filepath.EvalSymlinks(fullPath)
	if err != nil {
		return "", true, err
	}
	resolvedPath, err = filepath.Abs(resolvedPath)
	if err != nil {
		return "", true, err
	}
	relative, err := filepath.Rel(repoRoot, resolvedPath)
	if err != nil {
		return "", false, nil
	}
	if relative == ".." || filepath.IsAbs(relative) || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", false, nil
	}
	return resolvedPath, true, nil
}

const qualityEvidenceTextLimit = int64(4 << 20)
const qualityOversizedStateSnapshot = "<oversized-state-snapshot>"

var (
	qualityEvidenceAttemptDir            = regexp.MustCompile(`([/\\]attempt-)\d+([/\\])`)
	qualityEvidenceValidationAttemptFile = regexp.MustCompile(`(validation-[^/\\]+-)\d+(\.json)\b`)
	qualityGoTestDurationSuffix          = regexp.MustCompile(` \(\d+(?:\.\d+)?s\)(\r?\n)?$`)
	qualityGoPackageDurationSuffix       = regexp.MustCompile(`[ \t]+(?:\d+(?:\.\d+)?s|\(cached\))(\r?\n)?$`)
	qualityGoPackageLinePrefix           = regexp.MustCompile(`^(?:ok|FAIL|\?)[ \t]+\S`)
)

// qualityHashEvidenceFile writes raw bytes or normalized UTF-8 evidence into a caller-owned digest.
func qualityHashEvidenceFile(hash io.Writer, path string, stable bool, kind string) error {
	file, info, err := qualityOpenEvidenceFile(path)
	if err != nil {
		return err
	}
	defer file.Close()
	if stable && kind == "runtime_log" {
		return qualityStreamStableRuntimeLog(hash, file)
	}
	if !stable || kind != "state_snapshot" {
		_, err = io.Copy(hash, file)
		return err
	}
	if info.Size() > qualityEvidenceTextLimit {
		_, err = io.WriteString(hash, qualityOversizedStateSnapshot)
		return err
	}
	data, err := io.ReadAll(io.LimitReader(file, qualityEvidenceTextLimit+1))
	if err != nil {
		return err
	}
	if int64(len(data)) > qualityEvidenceTextLimit {
		_, err = io.WriteString(hash, qualityOversizedStateSnapshot)
		return err
	}
	if utf8.Valid(data) && !bytes.ContainsRune(data, '\x00') {
		data = []byte(qualityStableEvidenceText(string(data), kind))
	}
	_, err = hash.Write(data)
	return err
}

// qualityHashEvidenceFilePair derives integrity and stable hashes from the same opened file snapshot.
func qualityHashEvidenceFilePair(path, kind string) (string, string, bool, error) {
	file, info, err := qualityOpenEvidenceFile(path)
	if err != nil {
		return "", "", false, err
	}
	defer file.Close()
	rawHash := sha256.New()
	semanticHash := sha256.New()
	progressEligible := true
	if kind == "runtime_log" {
		err = qualityStreamRuntimeLogHashes(rawHash, semanticHash, file)
	} else if kind != "state_snapshot" {
		_, err = io.Copy(io.MultiWriter(rawHash, semanticHash), file)
	} else if info.Size() > qualityEvidenceTextLimit {
		progressEligible = false
		semanticHash.Write([]byte(qualityOversizedStateSnapshot))
		_, err = io.Copy(rawHash, file)
	} else {
		var data []byte
		data, err = io.ReadAll(io.LimitReader(file, qualityEvidenceTextLimit+1))
		if err == nil {
			rawHash.Write(data)
			if int64(len(data)) > qualityEvidenceTextLimit {
				progressEligible = false
				semanticHash.Write([]byte(qualityOversizedStateSnapshot))
				_, err = io.Copy(rawHash, file)
			} else {
				stable := data
				if utf8.Valid(data) && !bytes.ContainsRune(data, '\x00') {
					stable = []byte(qualityStableEvidenceText(string(data), kind))
				}
				semanticHash.Write(stable)
			}
		}
	}
	if err != nil {
		return "", "", false, err
	}
	return fmt.Sprintf("%x", rawHash.Sum(nil)), fmt.Sprintf("%x", semanticHash.Sum(nil)), progressEligible, nil
}

// qualityStreamEvidenceFile hashes evidence without buffering the entire artifact in memory.
func qualityStreamEvidenceFile(hash io.Writer, path string) error {
	file, _, err := qualityOpenEvidenceFile(path)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = io.Copy(hash, file)
	return err
}

// qualityOpenEvidenceFile rejects special files and replacement races before any content read.
func qualityOpenEvidenceFile(path string) (*os.File, os.FileInfo, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, nil, fmt.Errorf("evidence 必须是普通文件：%s", path)
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	opened, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, nil, err
	}
	if !opened.Mode().IsRegular() || !os.SameFile(info, opened) {
		file.Close()
		return nil, nil, fmt.Errorf("evidence 检查期间被替换：%s", path)
	}
	return file, opened, nil
}

// qualityStreamStableRuntimeLog normalizes volatile test durations without a whole-file size limit.
func qualityStreamStableRuntimeLog(hash io.Writer, file *os.File) error {
	return qualityStreamRuntimeLogHashes(io.Discard, hash, file)
}

// qualityStreamRuntimeLogHashes derives raw and normalized log hashes in one bounded pass.
func qualityStreamRuntimeLogHashes(raw, stable io.Writer, file *os.File) error {
	reader := bufio.NewReaderSize(file, 64<<10)
	const tailLimit = 512
	var tail []byte
	lineKind := ""
	for {
		part, err := reader.ReadSlice('\n')
		if _, writeErr := raw.Write(part); writeErr != nil {
			return writeErr
		}
		if len(tail) == 0 {
			lineKind = qualityRuntimeLogLineKind(part)
		}
		tail = append(tail, part...)
		if len(tail) > tailLimit {
			flush := len(tail) - tailLimit
			if _, writeErr := stable.Write(tail[:flush]); writeErr != nil {
				return writeErr
			}
			tail = append(tail[:0], tail[flush:]...)
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		normalized := qualityNormalizeRuntimeLogTail(tail, lineKind)
		if _, writeErr := stable.Write(normalized); writeErr != nil {
			return writeErr
		}
		tail = tail[:0]
		lineKind = ""
		switch {
		case errors.Is(err, io.EOF):
			return nil
		case err != nil:
			return err
		}
	}
}

// qualityRuntimeLogLineKind recognizes duration-bearing Go test and package lines from their prefix.
func qualityRuntimeLogLineKind(line []byte) string {
	switch {
	case bytes.HasPrefix(line, []byte("--- PASS: ")),
		bytes.HasPrefix(line, []byte("--- FAIL: ")),
		bytes.HasPrefix(line, []byte("--- SKIP: ")):
		return "test"
	case qualityGoPackageLinePrefix.Match(line):
		return "package"
	default:
		return ""
	}
}

// qualityNormalizeRuntimeLogTail replaces only a recognized line's volatile duration suffix.
func qualityNormalizeRuntimeLogTail(tail []byte, kind string) []byte {
	if !utf8.Valid(tail) || bytes.ContainsRune(tail, '\x00') {
		return tail
	}
	switch kind {
	case "test":
		return qualityGoTestDurationSuffix.ReplaceAll(tail, []byte(` (<duration>)$1`))
	case "package":
		return qualityGoPackageDurationSuffix.ReplaceAll(tail, []byte(` <duration>$1`))
	default:
		return tail
	}
}

// qualityStableEvidenceText normalizes only producer-specific metadata for the declared evidence kind.
func qualityStableEvidenceText(content, kind string) string {
	switch kind {
	case "runtime_log":
		stable := qualityGoTestCaseDuration.ReplaceAllString(content, `$1 (<duration>)`)
		return qualityGoPackageDuration.ReplaceAllString(stable, `$1 <duration>`)
	case "state_snapshot":
		var value any
		decoder := json.NewDecoder(strings.NewReader(content))
		decoder.UseNumber()
		if err := decoder.Decode(&value); err != nil {
			return content
		}
		if err := decoder.Decode(&struct{}{}); err != io.EOF {
			return content
		}
		qualityNormalizeStateEvidenceJSON(value, false)
		data, err := json.Marshal(value)
		if err != nil {
			return content
		}
		return string(data)
	default:
		return content
	}
}

// qualityNormalizeStateEvidenceJSON removes runtime fields only inside recognized engine records.
func qualityNormalizeStateEvidenceJSON(value any, engineOwned bool) {
	qualityNormalizeStateEvidenceJSONRecord(value, engineOwned, true)
}

// qualityNormalizeStateEvidenceJSONRecord prevents nested business objects from spoofing a root engine record.
func qualityNormalizeStateEvidenceJSONRecord(value any, engineOwned, allowRecognition bool) {
	switch typed := value.(type) {
	case []any:
		for _, item := range typed {
			qualityNormalizeStateEvidenceJSONRecord(item, engineOwned, allowRecognition)
		}
	case map[string]any:
		recognized := allowRecognition && qualityStateEvidenceEngineRecord(typed)
		normalizeCurrent := engineOwned || recognized
		workerRecord := qualityStateEvidenceWorkerRecord(typed)
		_, isCommand := typed["command"]
		_, hasExitCode := typed["exit_code"]
		for key, item := range typed {
			if normalizeCurrent {
				switch key {
				case "started_at", "finished_at", "last_heartbeat_at":
					typed[key] = "<timestamp>"
				case "run_id", "batch_id":
					typed[key] = "<run-id>"
				case "attempt", "attempts":
					typed[key] = json.Number("0")
				case "pid":
					if workerRecord {
						typed[key] = json.Number("0")
					}
				case "hostname":
					if workerRecord {
						typed[key] = "<hostname>"
					}
				case "duration_ms":
					if isCommand && hasExitCode {
						typed[key] = json.Number("0")
					}
				case "last_artifact", "log_path", "result_path":
					if text, ok := item.(string); ok {
						typed[key] = qualityNormalizeStateEvidenceArtifactPath(text)
					}
				}
			}
			childOwned := engineOwned || (recognized && qualityStateEvidenceOwnedChild(key))
			qualityNormalizeStateEvidenceJSONRecord(typed[key], childOwned, false)
		}
	}
}

// qualityNormalizeStateEvidenceArtifactPath removes run and retry identities from engine paths.
func qualityNormalizeStateEvidenceArtifactPath(path string) string {
	stable := qualityRunTimestamp.ReplaceAllString(path, "<run-id>")
	stable = qualityEvidenceAttemptDir.ReplaceAllString(stable, `${1}<attempt>${2}`)
	return qualityEvidenceValidationAttemptFile.ReplaceAllString(stable, `${1}<attempt>${2}`)
}

// qualityStateEvidenceWorkerRecord recognizes the persisted worker runtime metadata subtree.
func qualityStateEvidenceWorkerRecord(value map[string]any) bool {
	_, hasPID := value["pid"]
	_, hasHostname := value["hostname"]
	_, hasHeartbeat := value["last_heartbeat_at"]
	return hasPID && hasHostname && hasHeartbeat
}

// qualityStateEvidenceOwnedChild limits recursive normalization to known engine subtrees.
func qualityStateEvidenceOwnedChild(key string) bool {
	switch key {
	case "stage_timings", "dag_nodes", "validation", "artifact_gates", "acceptance_run", "worker", "commands", "tests":
		return true
	default:
		return false
	}
}

// qualityStateEvidenceEngineRecord recognizes persisted state, validation, and acceptance records.
func qualityStateEvidenceEngineRecord(value map[string]any) bool {
	_, hasStage := value["stage"]
	_, hasChangeName := value["change_name"]
	_, hasWorkflow := value["workflow_config"]
	if hasChangeName && hasStage && hasWorkflow {
		return true
	}
	_, hasAttempt := value["attempt"]
	_, hasCommands := value["commands"]
	if hasStage && hasAttempt && hasCommands {
		return true
	}
	_, hasChange := value["change"]
	_, hasRunID := value["run_id"]
	_, hasTests := value["tests"]
	_, hasEvidence := value["evidence"]
	return hasChange && hasRunID && hasStage && hasTests && hasEvidence
}
