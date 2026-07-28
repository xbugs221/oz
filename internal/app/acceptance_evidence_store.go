// Package app seals workflow acceptance evidence and promotes the final proof into the repository archive.
package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/xbugs221/oz/internal/acceptance"
)

const acceptanceEvidenceDirectory = "evidence"

// acceptanceRunSealedDir resolves the immutable state directory for one workflow attempt.
func acceptanceRunSealedDir(repo string, binding acceptanceRunBinding) (string, error) {
	if !safeAcceptancePathSegment(binding.RunID) || !safeAcceptancePathSegment(binding.Stage) || binding.Attempt < 1 {
		return "", fmt.Errorf("acceptance evidence binding 非法：run=%q stage=%q attempt=%d", binding.RunID, binding.Stage, binding.Attempt)
	}
	return filepath.Join(
		runDir(repo, binding.RunID),
		acceptanceEvidenceDirectory,
		binding.Stage,
		fmt.Sprintf("attempt-%d", binding.Attempt),
	), nil
}

// sealAcceptanceRunArtifacts snapshots temporary logs and required evidence before later stages can overwrite them.
func sealAcceptanceRunArtifacts(repo string, binding acceptanceRunBinding, result *AcceptanceRunResult) error {
	if result == nil {
		return fmt.Errorf("acceptance evidence result 不能为空")
	}
	sealedDir, err := acceptanceRunSealedDir(repo, binding)
	if err != nil {
		return err
	}
	result.ResultPath = filepath.Join(sealedDir, "result.json")
	var sealErrors []error
	for index := range result.Tests {
		item := &result.Tests[index]
		data, readErr := readAcceptanceArtifactFile(repo, item.LogPath)
		if readErr != nil {
			sealErrors = append(sealErrors, fmt.Errorf("读取 required_test %q 临时日志失败: %w", item.ID, readErr))
			continue
		}
		target := filepath.Join(sealedDir, "tests", acceptanceLogName(index, item.ID))
		if writeErr := writeImmutableEvidenceFile(target, data, 0o600); writeErr != nil {
			sealErrors = append(sealErrors, fmt.Errorf("封存 required_test %q 日志失败: %w", item.ID, writeErr))
			continue
		}
		item.SealedLogPath = target
	}
	for index := range result.Evidence {
		item := &result.Evidence[index]
		if item.Status != "present" {
			continue
		}
		data, readErr := readRepositoryEvidence(repo, item.Path)
		if readErr != nil {
			sealErrors = append(sealErrors, fmt.Errorf("读取 required_evidence %q 失败: %w", item.ID, readErr))
			continue
		}
		target := filepath.Join(sealedDir, "required", acceptanceEvidenceName(index, item.ID, item.Path))
		if writeErr := writeImmutableEvidenceFile(target, data, 0o600); writeErr != nil {
			sealErrors = append(sealErrors, fmt.Errorf("封存 required_evidence %q 失败: %w", item.ID, writeErr))
			continue
		}
		rawHash, progressHash, _, hashErr := qualityHashEvidenceFilePair(target, item.Kind)
		if hashErr != nil {
			sealErrors = append(sealErrors, fmt.Errorf("校验 required_evidence %q 封存副本失败: %w", item.ID, hashErr))
			continue
		}
		item.SealedPath = target
		item.ContentHash = rawHash
		item.ProgressHash = progressHash
	}
	return errors.Join(sealErrors...)
}

// acceptanceEvidenceName gives each required artifact a stable collision-free filename.
func acceptanceEvidenceName(index int, id, sourcePath string) string {
	base := safeAcceptanceLogName(filepath.Base(filepath.FromSlash(sourcePath)))
	sum := qualityHashStrings(id, sourcePath)
	if len(sum) > 12 {
		sum = sum[:12]
	}
	return fmt.Sprintf("%03d-%s-%s", index+1, safeAcceptanceLogName(id), base+"-"+sum)
}

// readRepositoryEvidence rechecks repository containment and file identity while taking the snapshot.
func readRepositoryEvidence(repo, relative string) ([]byte, error) {
	clean := filepath.Clean(filepath.FromSlash(strings.TrimSpace(relative)))
	if clean == "." || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("evidence 路径非法：%q", relative)
	}
	resolved, safe, err := qualityResolveEvidencePath(repo, filepath.Join(repo, clean))
	if err != nil {
		return nil, err
	}
	if !safe {
		return nil, fmt.Errorf("evidence 逃逸仓库：%q", relative)
	}
	return readRegularEvidenceFile(resolved)
}

// readRegularEvidenceFile reads one non-linked regular file after an identity check.
func readRegularEvidenceFile(path string) ([]byte, error) {
	file, _, err := qualityOpenEvidenceFile(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return io.ReadAll(file)
}

// writeImmutableEvidenceFile creates an artifact once and only accepts byte-identical retries.
func writeImmutableEvidenceFile(path string, data []byte, mode os.FileMode) error {
	if err := ensureRealEvidenceDirectory(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if os.IsExist(err) {
		existing, readErr := readRegularEvidenceFile(path)
		if readErr != nil {
			return readErr
		}
		if !bytes.Equal(existing, data) {
			return fmt.Errorf("immutable acceptance evidence 已存在且内容不同：%s", path)
		}
		return nil
	}
	if err != nil {
		return err
	}
	if _, err = file.Write(data); err != nil {
		file.Close()
		return err
	}
	return file.Close()
}

// ensureRealEvidenceDirectory creates a directory path without accepting linked components.
func ensureRealEvidenceDirectory(path string, mode os.FileMode) error {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	volume := filepath.VolumeName(absolute)
	current := string(filepath.Separator)
	remainder := strings.TrimPrefix(absolute, volume+string(filepath.Separator))
	if volume != "" {
		current = volume + string(filepath.Separator)
	}
	for _, segment := range strings.Split(remainder, string(filepath.Separator)) {
		if segment == "" {
			continue
		}
		current = filepath.Join(current, segment)
		info, statErr := os.Lstat(current)
		if os.IsNotExist(statErr) {
			if mkdirErr := os.Mkdir(current, mode); mkdirErr != nil && !os.IsExist(mkdirErr) {
				return mkdirErr
			}
			info, statErr = os.Lstat(current)
		}
		if statErr != nil {
			return statErr
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("acceptance evidence 目录必须是真实目录：%s", current)
		}
	}
	return nil
}

// writeSealedAcceptanceRunResult commits the result metadata once after every evidence path is bound.
func writeSealedAcceptanceRunResult(result AcceptanceRunResult) error {
	if !filepath.IsAbs(result.ResultPath) {
		return fmt.Errorf("sealed acceptance result 路径必须是绝对路径：%s", result.ResultPath)
	}
	data, err := marshalAcceptanceRunResult(result)
	if err != nil {
		return err
	}
	return writeImmutableEvidenceFile(result.ResultPath, data, 0o600)
}

// readSealedAcceptanceArtifact reads a state artifact without following a final link.
func readSealedAcceptanceArtifact(path string) ([]byte, error) {
	if !filepath.IsAbs(path) {
		return nil, fmt.Errorf("sealed acceptance artifact 路径必须是绝对路径：%s", path)
	}
	return readRegularEvidenceFile(path)
}

// acceptanceEvidenceManifest records portable provenance for one tracked proposal evidence package.
type acceptanceEvidenceManifest struct {
	Version      int                               `json:"version"`
	Change       string                            `json:"change"`
	RunID        string                            `json:"run_id"`
	Stage        string                            `json:"stage"`
	Attempt      int                               `json:"attempt"`
	ContractHash string                            `json:"contract_hash"`
	TestsHash    string                            `json:"tests_hash"`
	EvidenceHash string                            `json:"evidence_hash"`
	Evidence     []acceptanceEvidenceManifestEntry `json:"evidence"`
}

// acceptanceEvidenceManifestEntry binds a declared temporary source to its sealed and tracked hashes.
type acceptanceEvidenceManifestEntry struct {
	EvidenceID  string `json:"evidence_id"`
	SourcePath  string `json:"source_path"`
	ArchivePath string `json:"archive_path"`
	ContentHash string `json:"content_hash"`
}

// promoteQualityAcceptanceEvidence copies mapped final evidence from the sealed checkpoint into the tracked archive bundle.
func promoteQualityAcceptanceEvidence(repo string, state State, checkpoint string) error {
	result, err := verifyQualityAcceptanceCheckpoint(repo, state, checkpoint)
	if err != nil {
		return err
	}
	contract, _, err := readVerifiedRunAcceptance(repo, state)
	if err != nil {
		return err
	}
	if contract.SubmissionEvidence == nil {
		return fmt.Errorf("提案 %q 归档前必须声明 submission_evidence", state.ChangeName)
	}
	targetRelative := filepath.ToSlash(filepath.Join("tests", "evidence", "proposals", state.ChangeName))
	targetDir, err := ensureAcceptanceArtifactDirectory(repo, targetRelative)
	if err != nil {
		return err
	}
	copies := map[string]string{
		result.ResultPath: filepath.Join(targetDir, "result.json"),
	}
	for _, item := range result.Tests {
		copies[item.SealedLogPath] = filepath.Join(targetDir, "tests", filepath.Base(item.SealedLogPath))
	}
	evidenceByID := make(map[string]AcceptanceRunEvidenceResult, len(result.Evidence))
	for _, item := range result.Evidence {
		evidenceByID[item.ID] = item
	}
	manifest := acceptanceEvidenceManifest{
		Version:      1,
		Change:       state.ChangeName,
		RunID:        result.RunID,
		Stage:        result.Stage,
		Attempt:      result.Attempt,
		ContractHash: result.ContractHash,
		TestsHash:    result.TestsHash,
		EvidenceHash: result.EvidenceHash,
		Evidence:     make([]acceptanceEvidenceManifestEntry, 0, len(contract.SubmissionEvidence)),
	}
	for _, mapping := range contract.SubmissionEvidence {
		item, ok := evidenceByID[mapping.EvidenceID]
		if !ok || item.Path != mapping.SourcePath || item.SealedPath == "" || item.Status != "present" {
			return fmt.Errorf("submission_evidence %q 缺少匹配的封存副本", mapping.EvidenceID)
		}
		target, pathErr := acceptanceArtifactFilePath(repo, mapping.ArchivePath, true)
		if pathErr != nil {
			return pathErr
		}
		copies[item.SealedPath] = target
		manifest.Evidence = append(manifest.Evidence, acceptanceEvidenceManifestEntry{
			EvidenceID:  mapping.EvidenceID,
			SourcePath:  mapping.SourcePath,
			ArchivePath: mapping.ArchivePath,
			ContentHash: item.ContentHash,
		})
	}
	for source, target := range copies {
		if strings.TrimSpace(source) == "" {
			return fmt.Errorf("acceptance archive bundle 缺少封存源路径")
		}
		data, readErr := readSealedAcceptanceArtifact(source)
		if readErr != nil {
			return readErr
		}
		if writeErr := writePromotedAcceptanceFile(repo, target, data); writeErr != nil {
			return writeErr
		}
	}
	manifestData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	if err := writePromotedAcceptanceFile(repo, filepath.Join(targetDir, "manifest.json"), append(manifestData, '\n')); err != nil {
		return err
	}
	readme := fmt.Sprintf(
		"# %s 验收证据\n\n本目录由 Oz 从运行态封存副本提升生成；`manifest.json` 记录 run/stage/attempt 与内容哈希，`result.json` 和 `tests/` 保留最终通过检查点。\n",
		state.ChangeName,
	)
	if err := writePromotedAcceptanceFile(repo, filepath.Join(targetDir, "README.md"), []byte(readme)); err != nil {
		return err
	}
	return acceptance.ValidateSubmissionEvidenceForChange(repo, contract, state.ChangeName)
}

// writePromotedAcceptanceFile atomically refreshes the uncommitted final package from immutable state evidence.
func writePromotedAcceptanceFile(repo, target string, data []byte) error {
	relative, err := filepath.Rel(repo, target)
	if err != nil {
		return err
	}
	return writeAcceptanceArtifactFile(repo, filepath.ToSlash(relative), data)
}
