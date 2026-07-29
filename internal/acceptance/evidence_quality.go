// Package acceptance rejects renamed text, placeholders, and unreadable artifacts from final evidence.
package acceptance

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const (
	minimumReviewableVideoBytes  = 512
	minimumReviewableImageWidth  = 64
	minimumReviewableImageHeight = 36
	maxTextEvidenceProbeBytes    = 1 << 20
)

// validateReviewableEvidenceFile checks that an artifact's bytes match its declared review purpose.
func validateReviewableEvidenceFile(path string, evidence Evidence) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	switch evidence.Kind {
	case "demo_video":
		return validateReviewableVideo(path, info.Size())
	case "screenshot":
		return validateReviewableScreenshot(path)
	case "trace":
		return validateReviewableTrace(path)
	case "runtime_log", "console":
		return validateReviewableText(path)
	case "network", "state_snapshot":
		return validateReviewableJSON(path)
	default:
		if info.Size() < 64 {
			return fmt.Errorf("证据内容过少，审核人员无法据此判断用户结果")
		}
		return nil
	}
}

// validateReviewableVideo rejects obvious placeholders without imposing a video container format.
func validateReviewableVideo(path string, size int64) error {
	if size < minimumReviewableVideoBytes {
		return fmt.Errorf("demo_video 小于 %d 字节，不是可复核的真实视频", minimumReviewableVideoBytes)
	}
	probe, err := readEvidenceContainerProbe(path, size)
	if err != nil {
		return err
	}
	if utf8.Valid(probe) && !bytes.ContainsRune(probe, '\x00') && strings.TrimSpace(string(probe)) != "" {
		return fmt.Errorf("demo_video 只是文本内容，不能代替审核人员可直接打开的视频")
	}
	return nil
}

// readEvidenceContainerProbe reads the beginning and end of a media file without loading an arbitrarily large artifact.
func readEvidenceContainerProbe(path string, size int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	const window int64 = 4 << 20
	if size <= window*2 {
		return io.ReadAll(file)
	}
	head := make([]byte, window)
	if _, err := io.ReadFull(file, head); err != nil {
		return nil, err
	}
	tail := make([]byte, window)
	if _, err := file.ReadAt(tail, size-window); err != nil && err != io.EOF {
		return nil, err
	}
	return append(head, tail...), nil
}

// validateReviewableScreenshot decodes image metadata and rejects tiny placeholders.
func validateReviewableScreenshot(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	config, _, err := image.DecodeConfig(file)
	if err != nil {
		return fmt.Errorf("screenshot 无法解码，不能用文本改后缀充当截图：%w", err)
	}
	if config.Width < minimumReviewableImageWidth || config.Height < minimumReviewableImageHeight {
		return fmt.Errorf(
			"screenshot 尺寸 %dx%d 过小，无法展示用户可见结果",
			config.Width,
			config.Height,
		)
	}
	return nil
}

// validateReviewableTrace requires a real, nonempty ZIP trace archive.
func validateReviewableTrace(path string) error {
	trace, err := zip.OpenReader(path)
	if err != nil {
		return fmt.Errorf("trace 不是可打开的 ZIP 归档：%w", err)
	}
	defer trace.Close()
	if len(trace.File) == 0 {
		return fmt.Errorf("trace 归档为空")
	}
	return nil
}

// validateReviewableText accepts explanatory runtime output but rejects command-only or key=value-only records.
func validateReviewableText(path string) error {
	data, err := readTextEvidence(path)
	if err != nil {
		return err
	}
	lines := nonemptyEvidenceLines(string(data))
	if len(lines) < 3 {
		return fmt.Errorf("文本证据至少需要三行场景、操作和用户可见结果")
	}
	allMetadata := true
	for _, line := range lines {
		if !keyValueOnlyLine.MatchString(line) && !commandOnlyText.MatchString(line) && !weakReviewerText.MatchString(line) {
			allMetadata = false
			break
		}
	}
	if allMetadata {
		return fmt.Errorf("文本证据只有命令、状态或 key=value，审核人员无法理解用户结果")
	}
	return nil
}

// validateReviewableJSON requires structured runtime facts instead of a renamed text file.
func validateReviewableJSON(path string) error {
	data, err := readTextEvidence(path)
	if err != nil {
		return err
	}
	if len(bytes.TrimSpace(data)) < 64 || !json.Valid(data) {
		return fmt.Errorf("结构化证据必须是包含实际业务字段的有效 JSON")
	}
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	switch typed := value.(type) {
	case map[string]any:
		if len(typed) == 0 {
			return fmt.Errorf("结构化证据不能是空对象")
		}
	case []any:
		if len(typed) == 0 {
			return fmt.Errorf("结构化证据不能是空数组")
		}
	default:
		return fmt.Errorf("结构化证据必须是对象或数组")
	}
	return nil
}

// readTextEvidence reads bounded UTF-8 evidence for human-reviewability checks.
func readTextEvidence(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxTextEvidenceProbeBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxTextEvidenceProbeBytes {
		return nil, fmt.Errorf("文本证据超过 1 MiB，请筛选出审核人员需要的关键事实")
	}
	if !utf8.Valid(data) {
		return nil, fmt.Errorf("文本证据必须是审核人员可直接阅读的 UTF-8")
	}
	return data, nil
}

// nonemptyEvidenceLines returns trimmed, nonempty lines from one human-readable artifact.
func nonemptyEvidenceLines(text string) []string {
	var lines []string
	for _, line := range strings.Split(text, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			lines = append(lines, trimmed)
		}
	}
	return lines
}

// validateDeliveryComparisonArtifacts proves repair before/after references are different artifact bytes.
func validateDeliveryComparisonArtifacts(projectRoot string, contract Contract) error {
	if contract.DeliveryReport == nil {
		return nil
	}
	archiveByID := submissionArchivePaths(contract.SubmissionEvidence)
	for i, scenario := range contract.DeliveryReport.Scenarios {
		if scenario.Comparison == nil {
			continue
		}
		beforePath := filepath.Join(projectRoot, filepath.FromSlash(archiveByID[scenario.Comparison.BeforeEvidenceID]))
		afterPath := filepath.Join(projectRoot, filepath.FromSlash(archiveByID[scenario.Comparison.AfterEvidenceID]))
		beforeHash, err := evidenceFileHash(beforePath)
		if err != nil {
			return fmt.Errorf("delivery_report.scenarios[%d] 读取修复前证据失败：%w", i, err)
		}
		afterHash, err := evidenceFileHash(afterPath)
		if err != nil {
			return fmt.Errorf("delivery_report.scenarios[%d] 读取修复后证据失败：%w", i, err)
		}
		if beforeHash == afterHash {
			return fmt.Errorf("delivery_report.scenarios[%d] 的修复前后证据内容完全相同", i)
		}
	}
	return nil
}

// evidenceFileHash hashes one regular evidence file for before/after comparison.
func evidenceFileHash(path string) ([sha256.Size]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return [sha256.Size]byte{}, err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return [sha256.Size]byte{}, err
	}
	var result [sha256.Size]byte
	copy(result[:], hash.Sum(nil))
	return result, nil
}
