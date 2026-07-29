// Package acceptance validates structured oz and oz flow acceptance contracts.
package acceptance

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

var weakAssertionPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)^\s*http\s*200\s*$`),
	regexp.MustCompile(`(?i)^\s*(status\s*)?(code\s*)?200\s*$`),
	regexp.MustCompile(`(?i)^\s*2xx\s*$`),
	regexp.MustCompile(`^\s*元素存在\s*$`),
	regexp.MustCompile(`^\s*组件渲染成功\s*$`),
	regexp.MustCompile(`^\s*页面能打开\s*$`),
}

const maxInlineSubmissionEvidenceBytes int64 = 20 << 20

// Contract is the JSON contract produced before implementation starts.
type Contract struct {
	Summary            string               `json:"summary"`
	Coverage           []Coverage           `json:"coverage,omitempty"`
	RequiredTests      []Test               `json:"required_tests"`
	RequiredEvidence   []Evidence           `json:"required_evidence"`
	SubmissionEvidence []SubmissionEvidence `json:"submission_evidence,omitempty"`
	DeliveryReport     *DeliveryReport      `json:"delivery_report,omitempty"`
}

// Coverage links spec scenarios to concrete tests and QA evidence.
type Coverage struct {
	Spec     string   `json:"spec"`
	Tests    []string `json:"tests"`
	Evidence []string `json:"evidence"`
	Risk     string   `json:"risk"`
}

// Test records one executable test command that later stages must pass.
type Test struct {
	ID                     string   `json:"id"`
	Source                 string   `json:"source"`
	Path                   string   `json:"path"`
	Command                string   `json:"command"`
	Purpose                string   `json:"purpose"`
	Assertions             []string `json:"assertions,omitempty"`
	ExpectedInitialFailure string   `json:"expected_initial_failure,omitempty"`
}

// Evidence records one runtime artifact that QA must collect.
type Evidence struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	Path    string `json:"path"`
	Purpose string `json:"purpose"`
}

// SubmissionEvidence maps one temporary required-evidence source to its committed proposal archive.
// It represents final proposal demonstrations, not repair-only before/after diagnostics.
type SubmissionEvidence struct {
	EvidenceID  string `json:"evidence_id"`
	SourcePath  string `json:"source_path"`
	ArchivePath string `json:"archive_path"`
}

// DeliveryReport defines the user-facing review guide generated beside final evidence.
type DeliveryReport struct {
	UserBenefits  []string           `json:"user_benefits"`
	Prerequisites []string           `json:"prerequisites,omitempty"`
	Scenarios     []DeliveryScenario `json:"scenarios"`
	KnownLimits   []string           `json:"known_limits,omitempty"`
}

// DeliveryScenario explains one user-visible capability and how a reviewer can verify it.
type DeliveryScenario struct {
	ID          string              `json:"id"`
	Title       string              `json:"title"`
	UserValue   string              `json:"user_value"`
	Steps       []DeliveryStep      `json:"steps"`
	EvidenceIDs []string            `json:"evidence_ids"`
	Comparison  *DeliveryComparison `json:"comparison,omitempty"`
}

// DeliveryStep pairs a reviewer action with the result visible to the user.
type DeliveryStep struct {
	Action   string `json:"action"`
	Expected string `json:"expected"`
}

// DeliveryComparison binds a repair claim to distinct before and after evidence.
type DeliveryComparison struct {
	Before           string `json:"before"`
	After            string `json:"after"`
	BeforeEvidenceID string `json:"before_evidence_id"`
	AfterEvidenceID  string `json:"after_evidence_id"`
}

// Read loads and validates the acceptance JSON file.
func Read(path string) (Contract, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Contract{}, err
	}
	return Parse(data)
}

// Parse strictly decodes and validates an acceptance JSON document.
func Parse(data []byte) (Contract, error) {
	var contract Contract
	cleaned := bytes.TrimSpace(data)
	cleaned = bytes.TrimPrefix(cleaned, []byte{0xef, 0xbb, 0xbf})
	if len(cleaned) == 0 {
		return Contract{}, fmt.Errorf("artifact is empty")
	}
	dec := json.NewDecoder(bytes.NewReader(cleaned))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&contract); err != nil {
		return Contract{}, err
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return Contract{}, fmt.Errorf("artifact contains trailing content; output must be a single JSON object")
	}
	if err := Validate(contract); err != nil {
		return Contract{}, err
	}
	return contract, nil
}

// Validate enforces the pre-implementation acceptance contract shape.
func Validate(contract Contract) error {
	if strings.TrimSpace(contract.Summary) == "" {
		return fmt.Errorf("acceptance summary 不能为空")
	}
	if len(contract.RequiredTests) == 0 {
		return fmt.Errorf("acceptance required_tests 至少包含一个测试")
	}
	testIDs := map[string]bool{}
	for i, test := range contract.RequiredTests {
		if strings.TrimSpace(test.ID) == "" || strings.TrimSpace(test.Path) == "" || strings.TrimSpace(test.Command) == "" || strings.TrimSpace(test.Purpose) == "" {
			return fmt.Errorf("required_tests[%d] 不完整", i)
		}
		if !validTestSource(test.Source) {
			return fmt.Errorf("required_tests[%d].source 无效：%q", i, test.Source)
		}
		if testIDs[test.ID] {
			return fmt.Errorf("required_tests[%d].id 重复：%q", i, test.ID)
		}
		if len(test.Assertions) == 0 {
			return fmt.Errorf("required_tests[%d].assertions 至少包含一个业务级断言", i)
		}
		for j, assertion := range test.Assertions {
			if strings.TrimSpace(assertion) == "" {
				return fmt.Errorf("required_tests[%d].assertions[%d] 不能为空", i, j)
			}
			if weakAssertion(assertion) {
				return fmt.Errorf("required_tests[%d].assertions[%d] 是弱验收断言：%q", i, j, assertion)
			}
		}
		testIDs[test.ID] = true
	}
	evidenceByID := map[string]Evidence{}
	for i, evidence := range contract.RequiredEvidence {
		if strings.TrimSpace(evidence.ID) == "" || strings.TrimSpace(evidence.Kind) == "" || strings.TrimSpace(evidence.Path) == "" || strings.TrimSpace(evidence.Purpose) == "" {
			return fmt.Errorf("required_evidence[%d] 不完整", i)
		}
		if !validEvidenceKind(evidence.Kind) {
			return fmt.Errorf("required_evidence[%d].kind 无效：%q", i, evidence.Kind)
		}
		if _, exists := evidenceByID[evidence.ID]; exists {
			return fmt.Errorf("required_evidence[%d].id 重复：%q", i, evidence.ID)
		}
		evidenceByID[evidence.ID] = evidence
	}
	if err := validateSubmissionEvidence(contract.SubmissionEvidence, evidenceByID); err != nil {
		return err
	}
	if contract.DeliveryReport != nil {
		if err := validateDeliveryReportContract(*contract.DeliveryReport, evidenceByID); err != nil {
			return err
		}
	}
	for i, coverage := range contract.Coverage {
		if strings.TrimSpace(coverage.Spec) == "" {
			return fmt.Errorf("coverage[%d].spec 不能为空", i)
		}
		if len(coverage.Tests) == 0 {
			return fmt.Errorf("coverage[%d].tests 至少引用一个 required_tests id", i)
		}
		for j, id := range coverage.Tests {
			if strings.TrimSpace(id) == "" {
				return fmt.Errorf("coverage[%d].tests[%d] 不能为空", i, j)
			}
			if !testIDs[id] {
				return fmt.Errorf("coverage[%d].tests[%d] 引用未知 required_tests id：%q", i, j, id)
			}
		}
		for j, id := range coverage.Evidence {
			if strings.TrimSpace(id) == "" {
				return fmt.Errorf("coverage[%d].evidence[%d] 不能为空", i, j)
			}
			if _, exists := evidenceByID[id]; !exists {
				return fmt.Errorf("coverage[%d].evidence[%d] 引用未知 required_evidence id：%q", i, j, id)
			}
		}
		if len(coverage.Evidence) == 0 && strings.TrimSpace(coverage.Risk) == "" {
			return fmt.Errorf("coverage[%d].risk 必须说明无证据覆盖的剩余风险", i)
		}
	}
	return nil
}

// ValidateSubmissionEvidenceContractForChange rejects contracts that cannot produce one complete proposal package.
func ValidateSubmissionEvidenceContractForChange(contract Contract, changeName string) error {
	if err := Validate(contract); err != nil {
		return err
	}
	if contract.SubmissionEvidence == nil {
		return fmt.Errorf("提案 %q 归档前必须声明 submission_evidence", changeName)
	}
	if contract.DeliveryReport == nil {
		return fmt.Errorf("提案 %q 归档前必须声明面向审核人员的 delivery_report", changeName)
	}
	if !validSubmissionChangeName(changeName) {
		return fmt.Errorf("submission_evidence 的 changeName 无效：%q", changeName)
	}
	for i, item := range contract.SubmissionEvidence {
		archiveRelative, ok := submissionPathUnder(item.ArchivePath, "tests/evidence/proposals")
		if !ok {
			return fmt.Errorf("submission_evidence[%d].archive_path 必须位于 tests/evidence/proposals/<change>/：%s", i, item.ArchivePath)
		}
		archiveParts := strings.Split(archiveRelative, "/")
		if len(archiveParts) < 2 || archiveParts[0] != changeName {
			return fmt.Errorf("submission_evidence[%d].archive_path 的提案目录必须等于当前 change %q：%s", i, changeName, item.ArchivePath)
		}
	}
	return nil
}

// ValidateSubmissionEvidenceForChange enforces the archive-time evidence gate for one proposal.
func ValidateSubmissionEvidenceForChange(projectRoot string, contract Contract, changeName string) error {
	if err := ValidateSubmissionEvidenceContractForChange(contract, changeName); err != nil {
		return err
	}
	evidenceByID := make(map[string]Evidence, len(contract.RequiredEvidence))
	for _, evidence := range contract.RequiredEvidence {
		evidenceByID[evidence.ID] = evidence
	}
	for i, item := range contract.SubmissionEvidence {
		archivePath := filepath.Join(projectRoot, filepath.FromSlash(item.ArchivePath))
		info, err := os.Lstat(archivePath)
		if err != nil {
			return fmt.Errorf("submission_evidence[%d].archive_path 不存在：%s: %w", i, item.ArchivePath, err)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("submission_evidence[%d].archive_path 必须是非符号链接的普通文件：%s", i, item.ArchivePath)
		}
		if info.Size() == 0 {
			return fmt.Errorf("submission_evidence[%d].archive_path 不能为空文件：%s", i, item.ArchivePath)
		}
		ignored, err := submissionPathGitIgnored(projectRoot, item.ArchivePath)
		if err != nil {
			return fmt.Errorf("submission_evidence[%d].archive_path 无法检查 Git ignore：%s: %w", i, item.ArchivePath, err)
		}
		if ignored {
			return fmt.Errorf("submission_evidence[%d].archive_path 不得被 Git ignore：%s", i, item.ArchivePath)
		}
		if info.Size() > maxInlineSubmissionEvidenceBytes {
			usesLFS, err := submissionPathUsesGitLFS(projectRoot, item.ArchivePath)
			if err != nil {
				return fmt.Errorf("submission_evidence[%d].archive_path 无法检查 Git LFS：%s: %w", i, item.ArchivePath, err)
			}
			if !usesLFS {
				return fmt.Errorf(
					"submission_evidence[%d].archive_path 超过 20 MiB，必须通过 Git LFS 跟踪：%s",
					i,
					item.ArchivePath,
				)
			}
		}
		if err := validateReviewableEvidenceFile(archivePath, evidenceByID[item.EvidenceID]); err != nil {
			return fmt.Errorf("submission_evidence[%d] 不是审核人员可理解的真实证据：%s: %w", i, item.ArchivePath, err)
		}
	}
	packageRoot := filepath.ToSlash(filepath.Join("tests", "evidence", "proposals", changeName))
	for _, name := range []string{"README.md", "DELIVERY.md", "manifest.json"} {
		relativePath := filepath.ToSlash(filepath.Join(packageRoot, name))
		if err := validateSubmissionArchiveFile(projectRoot, relativePath); err != nil {
			return fmt.Errorf("提交级证据包缺少有效的 %s：%w", name, err)
		}
	}
	if err := ValidateDeliveryReportFileForChange(projectRoot, contract, changeName); err != nil {
		return fmt.Errorf("面向审核人员的交付报告无效：%w", err)
	}
	if err := validateDeliveryComparisonArtifacts(projectRoot, contract); err != nil {
		return err
	}
	return nil
}

// validateSubmissionArchiveFile enforces a reviewable, Git-visible package artifact.
func validateSubmissionArchiveFile(projectRoot, relativePath string) error {
	archivePath := filepath.Join(projectRoot, filepath.FromSlash(relativePath))
	info, err := os.Lstat(archivePath)
	if err != nil {
		return fmt.Errorf("%s 不存在: %w", relativePath, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s 必须是非符号链接的普通文件", relativePath)
	}
	if info.Size() == 0 {
		return fmt.Errorf("%s 不能为空文件", relativePath)
	}
	ignored, err := submissionPathGitIgnored(projectRoot, relativePath)
	if err != nil {
		return fmt.Errorf("%s 无法检查 Git ignore: %w", relativePath, err)
	}
	if ignored {
		return fmt.Errorf("%s 不得被 Git ignore", relativePath)
	}
	return nil
}

// validateSubmissionEvidence enforces the final proposal evidence package while preserving contracts
// created before submission_evidence existed.
func validateSubmissionEvidence(items []SubmissionEvidence, evidenceByID map[string]Evidence) error {
	if items == nil {
		return nil
	}
	if len(items) == 0 {
		return fmt.Errorf("submission_evidence 声明后至少包含一个最终演示证据")
	}

	seenEvidence := map[string]bool{}
	seenArchives := map[string]bool{}
	proposalDir := ""
	hasDemoVideo := false
	for i, item := range items {
		if strings.TrimSpace(item.EvidenceID) == "" || strings.TrimSpace(item.SourcePath) == "" || strings.TrimSpace(item.ArchivePath) == "" {
			return fmt.Errorf("submission_evidence[%d] 不完整", i)
		}
		evidence, exists := evidenceByID[item.EvidenceID]
		if !exists {
			return fmt.Errorf("submission_evidence[%d].evidence_id 引用未知 required_evidence id：%q", i, item.EvidenceID)
		}
		if seenEvidence[item.EvidenceID] {
			return fmt.Errorf("submission_evidence[%d].evidence_id 重复：%q", i, item.EvidenceID)
		}
		seenEvidence[item.EvidenceID] = true
		if item.SourcePath != evidence.Path {
			return fmt.Errorf("submission_evidence[%d].source_path 必须等于 required_evidence %q 的 path：%s", i, item.EvidenceID, evidence.Path)
		}
		if _, ok := submissionPathUnder(item.SourcePath, "test-results"); !ok {
			return fmt.Errorf("submission_evidence[%d].source_path 必须是 test-results/ 下的规范相对文件路径：%s", i, item.SourcePath)
		}
		archiveRelative, ok := submissionPathUnder(item.ArchivePath, "tests/evidence/proposals")
		if !ok {
			return fmt.Errorf("submission_evidence[%d].archive_path 必须位于 tests/evidence/proposals/<change>/：%s", i, item.ArchivePath)
		}
		archiveParts := strings.Split(archiveRelative, "/")
		if len(archiveParts) < 2 {
			return fmt.Errorf("submission_evidence[%d].archive_path 必须包含提案目录和证据文件：%s", i, item.ArchivePath)
		}
		if proposalDir == "" {
			proposalDir = archiveParts[0]
		} else if proposalDir != archiveParts[0] {
			return fmt.Errorf("submission_evidence[%d].archive_path 必须与证据包使用同一提案目录：%s", i, item.ArchivePath)
		}
		if seenArchives[item.ArchivePath] {
			return fmt.Errorf("submission_evidence[%d].archive_path 重复：%q", i, item.ArchivePath)
		}
		seenArchives[item.ArchivePath] = true
		if evidence.Kind == "demo_video" {
			hasDemoVideo = true
		}
	}
	for evidenceID := range evidenceByID {
		if !seenEvidence[evidenceID] {
			return fmt.Errorf("submission_evidence 必须覆盖全部 required_evidence，缺少：%q", evidenceID)
		}
	}
	if !hasDemoVideo {
		return fmt.Errorf("submission_evidence 至少引用一个 kind=demo_video 的 required_evidence；修复前后对比不能替代最终演示视频")
	}
	return nil
}

// validSubmissionChangeName reports whether a proposal name is one safe path segment.
func validSubmissionChangeName(changeName string) bool {
	trimmed := strings.TrimSpace(changeName)
	cleaned := filepath.Clean(filepath.FromSlash(trimmed))
	return trimmed != "" && trimmed == changeName && cleaned == trimmed && cleaned != "." && cleaned != ".." && filepath.Base(cleaned) == cleaned
}

// submissionPathUnder returns a normalized path relative to an allowed submission evidence root.
func submissionPathUnder(rawPath, rawRoot string) (string, bool) {
	trimmed := strings.TrimSpace(rawPath)
	if trimmed == "" || strings.Contains(trimmed, `\`) || filepath.IsAbs(filepath.FromSlash(trimmed)) {
		return "", false
	}
	cleaned := filepath.ToSlash(filepath.Clean(filepath.FromSlash(trimmed)))
	if cleaned != trimmed || cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", false
	}
	root := filepath.ToSlash(filepath.Clean(filepath.FromSlash(rawRoot)))
	relative, err := filepath.Rel(filepath.FromSlash(root), filepath.FromSlash(cleaned))
	if err != nil {
		return "", false
	}
	relative = filepath.ToSlash(relative)
	if relative == "." || relative == ".." || strings.HasPrefix(relative, "../") {
		return "", false
	}
	return relative, true
}

// submissionPathGitIgnored asks Git whether a committed evidence target is hidden by ignore rules.
func submissionPathGitIgnored(projectRoot, relativePath string) (bool, error) {
	cmd := exec.Command("git", "-C", projectRoot, "check-ignore", "--no-index", "--quiet", "--", relativePath)
	err := cmd.Run()
	if err == nil {
		return true, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return false, nil
	}
	return false, err
}

// submissionPathUsesGitLFS reports whether repository attributes route a large artifact through LFS.
func submissionPathUsesGitLFS(projectRoot, relativePath string) (bool, error) {
	cmd := exec.Command("git", "-C", projectRoot, "check-attr", "filter", "--", relativePath)
	output, err := cmd.Output()
	if err != nil {
		return false, err
	}
	fields := strings.Split(strings.TrimSpace(string(output)), ":")
	return len(fields) >= 3 && strings.TrimSpace(fields[len(fields)-1]) == "lfs", nil
}

// EvidenceHasProducer reports whether an evidence artifact is tied to a required test producer.
func EvidenceHasProducer(projectRoot string, evidence Evidence, coverage []Coverage, tests map[string]Test) bool {
	for _, item := range coverage {
		if !stringSliceContains(item.Evidence, evidence.ID) {
			continue
		}
		for _, testID := range item.Tests {
			test, ok := tests[testID]
			if ok && (testMentionsEvidence(test, evidence) || testScriptProducesEvidence(projectRoot, test, evidence)) {
				return true
			}
		}
	}
	return false
}

func testMentionsEvidence(test Test, evidence Evidence) bool {
	// testMentionsEvidence conservatively traces a runtime artifact to required_test metadata.
	needles := evidenceNeedles(evidence)
	haystacks := []string{test.ID, test.Path, test.Command, test.Purpose}
	haystacks = append(haystacks, test.Assertions...)
	for _, needle := range needles {
		if needle == "" {
			continue
		}
		for _, haystack := range haystacks {
			if strings.Contains(haystack, needle) {
				return true
			}
		}
	}
	return false
}

func testScriptProducesEvidence(projectRoot string, test Test, evidence Evidence) bool {
	// testScriptProducesEvidence inspects the declared test and nearby shell wrappers.
	if strings.TrimSpace(projectRoot) == "" {
		return false
	}
	needles := evidenceNeedles(evidence)
	for _, relPath := range producerCandidatePaths(projectRoot, test) {
		body, ok := readRelativeFile(projectRoot, relPath)
		if !ok || !textMentionsAny(body, needles) {
			continue
		}
		if producerScriptMentionsTest(body, test) || relPath == test.Path {
			return true
		}
	}
	return false
}

func producerCandidatePaths(projectRoot string, test Test) []string {
	// producerCandidatePaths returns declared test files and sibling shell wrappers.
	seen := map[string]bool{}
	paths := []string{}
	add := func(path string) {
		path = strings.Trim(strings.TrimSpace(path), `"'`)
		if path == "" || strings.HasPrefix(path, "-") || filepath.IsAbs(path) {
			return
		}
		path = filepath.ToSlash(filepath.Clean(path))
		if path == "." || seen[path] {
			return
		}
		seen[path] = true
		paths = append(paths, path)
	}
	add(test.Path)
	for _, field := range strings.Fields(test.Command) {
		if strings.Contains(field, "/") {
			add(field)
		}
	}
	if test.Path == "" {
		return paths
	}
	dir := filepath.Dir(filepath.FromSlash(test.Path))
	entries, err := os.ReadDir(filepath.Join(projectRoot, dir))
	if err != nil {
		return paths
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sh") {
			continue
		}
		add(filepath.ToSlash(filepath.Join(dir, entry.Name())))
	}
	return paths
}

func producerScriptMentionsTest(body string, test Test) bool {
	// producerScriptMentionsTest keeps sibling wrappers tied to the declared required_test.
	if test.Path != "" && strings.Contains(body, test.Path) {
		return true
	}
	base := filepath.Base(filepath.FromSlash(test.Path))
	if base != "." && base != "" && strings.Contains(body, base) {
		return true
	}
	for _, field := range strings.Fields(test.Command) {
		field = strings.Trim(strings.TrimSpace(field), `"'`)
		if field != "" && strings.Contains(field, "/") && strings.Contains(body, field) {
			return true
		}
	}
	return false
}

func evidenceNeedles(evidence Evidence) []string {
	// evidenceNeedles includes stable identifiers and artifact names.
	needles := []string{}
	for _, needle := range []string{evidence.ID, evidence.Path} {
		needle = strings.TrimSpace(needle)
		if needle != "" {
			needles = append(needles, needle)
		}
	}
	if evidence.Path != "" {
		base := filepath.Base(filepath.FromSlash(evidence.Path))
		if base != "." && base != "" && base != evidence.Path {
			needles = append(needles, base)
		}
	}
	return needles
}

func textMentionsAny(body string, needles []string) bool {
	// textMentionsAny checks whether local producer content names evidence output.
	for _, needle := range needles {
		if needle != "" && strings.Contains(body, needle) {
			return true
		}
	}
	return false
}

func readRelativeFile(projectRoot, relPath string) (string, bool) {
	// readRelativeFile reads only paths under the validated project.
	relPath = filepath.Clean(filepath.FromSlash(relPath))
	if relPath == "." || strings.HasPrefix(relPath, ".."+string(filepath.Separator)) || filepath.IsAbs(relPath) {
		return "", false
	}
	body, err := os.ReadFile(filepath.Join(projectRoot, relPath))
	if err != nil {
		return "", false
	}
	return string(body), true
}

func stringSliceContains(values []string, want string) bool {
	// stringSliceContains keeps contract id matching exact and case-sensitive.
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func weakAssertion(assertion string) bool {
	// weakAssertion rejects clear surface checks that do not describe business behavior.
	trimmed := strings.TrimSpace(assertion)
	for _, pattern := range weakAssertionPatterns {
		if pattern.MatchString(trimmed) {
			return true
		}
	}
	return false
}

func validTestSource(source string) bool {
	// validTestSource matches the existing oz flow sealed-run schema.
	switch source {
	case "change_contract", "root_e2e", "existing_regression", "new_regression":
		return true
	default:
		return false
	}
}

func validEvidenceKind(kind string) bool {
	// validEvidenceKind matches the existing oz flow sealed-run schema.
	switch kind {
	case "screenshot", "trace", "network", "console", "runtime_log", "state_snapshot", "demo_video", "other":
		return true
	default:
		return false
	}
}
