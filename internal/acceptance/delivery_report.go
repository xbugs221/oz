// Package acceptance renders and validates reviewer-facing delivery reports.
package acceptance

import (
	"bytes"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

var (
	weakReviewerText = regexp.MustCompile(`(?i)^\s*(ok|pass(?:ed)?|success|successful|verified|done|todo|tbd|n/?a|测试通过|验证通过|命令成功|运行成功|功能正常|没有问题|无问题|见日志|见截图|最终演示|占位)\s*[.!。]?\s*$`)
	keyValueOnlyLine = regexp.MustCompile(`^[A-Za-z0-9_.-]+=[^\s]+$`)
	commandOnlyText  = regexp.MustCompile(`(?i)^\s*(go\s+test|pytest|pnpm|npm|npx|cargo\s+test|git\s+|curl\s+|make\s+test)(?:\s+.*)?$`)
)

var reviewerInternalTerms = []string{
	"run_id",
	"content_hash",
	"contract_hash",
	"required_evidence",
	"submission_evidence",
	"delivery_base_head",
	"exit code",
	"runtime",
	"artifact",
	"acceptance",
	"content hash",
	"运行时",
	"验收合同",
	"门禁",
	"检查点",
	"测试结果",
}

// DeliveryObservation records what final independent QA actually saw for one user scenario.
type DeliveryObservation struct {
	ScenarioID string
	Observed   string
}

// ValidateUserFacingText rejects placeholders and implementation-only prose that cannot guide a reviewer.
func ValidateUserFacingText(label, text string) error {
	trimmed := strings.TrimSpace(text)
	if utf8.RuneCountInString(trimmed) < 6 {
		return fmt.Errorf("%s 必须具体说明用户能看到的行为", label)
	}
	if strings.ContainsAny(trimmed, "\r\n") {
		return fmt.Errorf("%s 必须是一条清晰、可直接阅读的陈述", label)
	}
	if weakReviewerText.MatchString(trimmed) || commandOnlyText.MatchString(trimmed) {
		return fmt.Errorf("%s 不能只写测试结果、命令或占位语：%q", label, text)
	}
	lower := strings.ToLower(trimmed)
	for _, term := range reviewerInternalTerms {
		if strings.Contains(lower, term) {
			return fmt.Errorf("%s 不得用内部字段代替用户可见结果：%s", label, term)
		}
	}
	return nil
}

// validateDeliveryReportContract requires benefits, review steps, evidence, and paired repair comparisons.
func validateDeliveryReportContract(report DeliveryReport, evidenceByID map[string]Evidence) error {
	if len(report.UserBenefits) == 0 {
		return fmt.Errorf("delivery_report.user_benefits 至少说明一项用户获得的能力或改善")
	}
	for i, benefit := range report.UserBenefits {
		if err := ValidateUserFacingText(fmt.Sprintf("delivery_report.user_benefits[%d]", i), benefit); err != nil {
			return err
		}
	}
	for i, prerequisite := range report.Prerequisites {
		if err := ValidateUserFacingText(fmt.Sprintf("delivery_report.prerequisites[%d]", i), prerequisite); err != nil {
			return err
		}
	}
	for i, limit := range report.KnownLimits {
		if err := ValidateUserFacingText(fmt.Sprintf("delivery_report.known_limits[%d]", i), limit); err != nil {
			return err
		}
	}
	if len(report.Scenarios) == 0 {
		return fmt.Errorf("delivery_report.scenarios 至少包含一个用户验收场景")
	}
	seenScenario := map[string]bool{}
	hasDemo := false
	for i, scenario := range report.Scenarios {
		prefix := fmt.Sprintf("delivery_report.scenarios[%d]", i)
		if strings.TrimSpace(scenario.ID) == "" || strings.TrimSpace(scenario.Title) == "" {
			return fmt.Errorf("%s.id 和 title 不能为空", prefix)
		}
		if seenScenario[scenario.ID] {
			return fmt.Errorf("%s.id 重复：%q", prefix, scenario.ID)
		}
		seenScenario[scenario.ID] = true
		if err := ValidateUserFacingText(prefix+".user_value", scenario.UserValue); err != nil {
			return err
		}
		if len(scenario.Steps) == 0 {
			return fmt.Errorf("%s.steps 至少包含一个审核人员可执行的步骤", prefix)
		}
		for j, step := range scenario.Steps {
			if err := ValidateUserFacingText(fmt.Sprintf("%s.steps[%d].action", prefix, j), step.Action); err != nil {
				return err
			}
			if err := ValidateUserFacingText(fmt.Sprintf("%s.steps[%d].expected", prefix, j), step.Expected); err != nil {
				return err
			}
		}
		evidenceIDs, err := validateDeliveryEvidenceIDs(prefix+".evidence_ids", scenario.EvidenceIDs, evidenceByID)
		if err != nil {
			return err
		}
		for id := range evidenceIDs {
			if err := ValidateUserFacingText(prefix+".evidence_purpose", evidenceByID[id].Purpose); err != nil {
				return err
			}
			if evidenceByID[id].Kind == "demo_video" {
				hasDemo = true
			}
		}
		if scenario.Comparison != nil {
			if err := validateDeliveryComparison(prefix+".comparison", *scenario.Comparison, evidenceIDs, evidenceByID); err != nil {
				return err
			}
		}
	}
	if !hasDemo {
		return fmt.Errorf("delivery_report 至少一个验收场景必须直接引用 demo_video")
	}
	return nil
}

// validateDeliveryEvidenceIDs binds a reviewer scenario to declared runtime evidence.
func validateDeliveryEvidenceIDs(label string, ids []string, evidenceByID map[string]Evidence) (map[string]bool, error) {
	if len(ids) == 0 {
		return nil, fmt.Errorf("%s 至少引用一份审核人员可直接查看的证据", label)
	}
	seen := map[string]bool{}
	for i, id := range ids {
		if _, ok := evidenceByID[id]; !ok {
			return nil, fmt.Errorf("%s[%d] 引用未知 required_evidence：%q", label, i, id)
		}
		if seen[id] {
			return nil, fmt.Errorf("%s[%d] 重复：%q", label, i, id)
		}
		seen[id] = true
	}
	return seen, nil
}

// validateDeliveryComparison requires distinct, understandable before and after proof for a repair claim.
func validateDeliveryComparison(label string, comparison DeliveryComparison, scenarioEvidence map[string]bool, evidenceByID map[string]Evidence) error {
	if err := ValidateUserFacingText(label+".before", comparison.Before); err != nil {
		return err
	}
	if err := ValidateUserFacingText(label+".after", comparison.After); err != nil {
		return err
	}
	if normalizedReviewerClaim(comparison.Before) == normalizedReviewerClaim(comparison.After) {
		return fmt.Errorf("%s 的 before 与 after 必须描述不同的用户表现", label)
	}
	if comparison.BeforeEvidenceID == "" || comparison.AfterEvidenceID == "" {
		return fmt.Errorf("%s 必须同时提供 before_evidence_id 与 after_evidence_id", label)
	}
	if comparison.BeforeEvidenceID == comparison.AfterEvidenceID {
		return fmt.Errorf("%s 的前后证据不能是同一个文件", label)
	}
	for field, id := range map[string]string{
		"before_evidence_id": comparison.BeforeEvidenceID,
		"after_evidence_id":  comparison.AfterEvidenceID,
	} {
		if _, ok := evidenceByID[id]; !ok {
			return fmt.Errorf("%s.%s 引用未知 required_evidence：%q", label, field, id)
		}
		if !scenarioEvidence[id] {
			return fmt.Errorf("%s.%s 必须同时列入场景 evidence_ids：%q", label, field, id)
		}
	}
	return nil
}

// RenderDeliveryReport creates the canonical human review entry from the sealed contract and final QA observations.
func RenderDeliveryReport(contract Contract, changeName string, observations []DeliveryObservation) ([]byte, error) {
	if err := ValidateSubmissionEvidenceContractForChange(contract, changeName); err != nil {
		return nil, err
	}
	observedByScenario := map[string]string{}
	scenarios := map[string]bool{}
	for _, scenario := range contract.DeliveryReport.Scenarios {
		scenarios[scenario.ID] = true
	}
	for _, item := range observations {
		if !scenarios[item.ScenarioID] {
			return nil, fmt.Errorf("最终 QA 引用了未知交付场景：%s", item.ScenarioID)
		}
		if _, duplicate := observedByScenario[item.ScenarioID]; duplicate {
			return nil, fmt.Errorf("交付报告实测场景重复：%s", item.ScenarioID)
		}
		if err := ValidateUserFacingText("交付报告实测结果", item.Observed); err != nil {
			return nil, err
		}
		observedByScenario[item.ScenarioID] = strings.TrimSpace(item.Observed)
	}
	report := *contract.DeliveryReport
	archiveByID := submissionArchivePaths(contract.SubmissionEvidence)
	evidenceIndex := evidenceByID(contract.RequiredEvidence)
	var out strings.Builder
	fmt.Fprintf(&out, "# %s 交付报告\n\n", changeName)
	out.WriteString("## 这次给用户带来了什么\n\n")
	for _, benefit := range report.UserBenefits {
		fmt.Fprintf(&out, "- %s\n", strings.TrimSpace(benefit))
	}
	out.WriteString("\n## 如何验收\n\n### 验收前准备\n\n")
	if len(report.Prerequisites) == 0 {
		out.WriteString("- 无需特殊准备，使用正常可访问该能力的账号和数据即可。\n")
	} else {
		for _, prerequisite := range report.Prerequisites {
			fmt.Fprintf(&out, "- %s\n", strings.TrimSpace(prerequisite))
		}
	}
	for index, scenario := range report.Scenarios {
		observed, ok := observedByScenario[scenario.ID]
		if !ok {
			return nil, fmt.Errorf("最终 QA 缺少交付场景 %q 的实际观察", scenario.ID)
		}
		fmt.Fprintf(&out, "\n### %d. %s\n\n", index+1, strings.TrimSpace(scenario.Title))
		fmt.Fprintf(&out, "- 用户价值：%s\n", strings.TrimSpace(scenario.UserValue))
		for stepIndex, step := range scenario.Steps {
			fmt.Fprintf(&out, "- 步骤 %d：%s\n", stepIndex+1, strings.TrimSpace(step.Action))
			fmt.Fprintf(&out, "  - 应看到：%s\n", strings.TrimSpace(step.Expected))
		}
		fmt.Fprintf(&out, "- 实测：%s\n", observed)
		fmt.Fprintf(&out, "- 直接证据：%s\n", deliveryEvidenceLinks(scenario.EvidenceIDs, archiveByID, evidenceIndex))
	}
	out.WriteString("\n## 修复前后对比\n")
	hasComparison := false
	for _, scenario := range report.Scenarios {
		if scenario.Comparison == nil {
			continue
		}
		hasComparison = true
		fmt.Fprintf(&out, "\n### %s\n\n", strings.TrimSpace(scenario.Title))
		fmt.Fprintf(&out, "- 修复前：%s（%s）\n", strings.TrimSpace(scenario.Comparison.Before), deliveryEvidenceLink(scenario.Comparison.BeforeEvidenceID, "查看修复前证据", archiveByID))
		fmt.Fprintf(&out, "- 修复后：%s（%s）\n", strings.TrimSpace(scenario.Comparison.After), deliveryEvidenceLink(scenario.Comparison.AfterEvidenceID, "查看修复后证据", archiveByID))
	}
	if !hasComparison {
		out.WriteString("\n本次是新增能力，验收重点是上述用户路径和最终可见结果。\n")
	}
	out.WriteString("\n## 可直接查看的证据\n\n")
	for _, item := range contract.SubmissionEvidence {
		purpose := strings.TrimSpace(evidenceIndex[item.EvidenceID].Purpose)
		fmt.Fprintf(&out, "- %s\n", deliveryEvidenceLink(item.EvidenceID, purpose, archiveByID))
	}
	out.WriteString("\n## 已知限制\n\n")
	if len(report.KnownLimits) == 0 {
		out.WriteString("- 暂无额外限制。\n")
	} else {
		for _, limit := range report.KnownLimits {
			fmt.Fprintf(&out, "- %s\n", strings.TrimSpace(limit))
		}
	}
	return []byte(out.String()), nil
}

// normalizedReviewerClaim removes cosmetic differences before comparing before/after descriptions.
func normalizedReviewerClaim(text string) string {
	var out strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(text)) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			out.WriteRune(r)
		}
	}
	return out.String()
}

// ValidateDeliveryReportFileForChange ensures the tracked report contains the promised user guidance and evidence links.
func ValidateDeliveryReportFileForChange(projectRoot string, contract Contract, changeName string) error {
	relative := filepath.ToSlash(filepath.Join("tests", "evidence", "proposals", changeName, "DELIVERY.md"))
	if err := validateSubmissionArchiveFile(projectRoot, relative); err != nil {
		return err
	}
	data, err := os.ReadFile(filepath.Join(projectRoot, filepath.FromSlash(relative)))
	if err != nil {
		return err
	}
	if !utf8.Valid(data) || len(bytes.TrimSpace(data)) < 256 {
		return fmt.Errorf("%s 必须是完整、可阅读的 UTF-8 交付报告", relative)
	}
	text := string(data)
	for _, heading := range []string{
		"## 这次给用户带来了什么",
		"## 如何验收",
		"## 修复前后对比",
		"## 可直接查看的证据",
		"## 已知限制",
	} {
		if !strings.Contains(text, heading) {
			return fmt.Errorf("%s 缺少审核章节：%s", relative, heading)
		}
	}
	report := *contract.DeliveryReport
	for _, benefit := range report.UserBenefits {
		if !strings.Contains(text, strings.TrimSpace(benefit)) {
			return fmt.Errorf("%s 缺少用户收益：%s", relative, benefit)
		}
	}
	for _, scenario := range report.Scenarios {
		for label, expected := range map[string]string{
			"title":      scenario.Title,
			"user_value": scenario.UserValue,
		} {
			if !strings.Contains(text, strings.TrimSpace(expected)) {
				return fmt.Errorf("%s 缺少场景 %s：%s", relative, label, expected)
			}
		}
		for _, step := range scenario.Steps {
			if !strings.Contains(text, strings.TrimSpace(step.Action)) || !strings.Contains(text, strings.TrimSpace(step.Expected)) {
				return fmt.Errorf("%s 没有完整写出场景 %q 的操作和可见结果", relative, scenario.Title)
			}
		}
	}
	for _, item := range contract.SubmissionEvidence {
		link := deliveryEvidenceRelativePath(item.ArchivePath)
		if !strings.Contains(text, link) && !strings.Contains(text, deliveryURLPath(link)) {
			return fmt.Errorf("%s 没有链接证据 %q", relative, item.EvidenceID)
		}
	}
	observed := regexp.MustCompile(`(?m)^- 实测：(.+)$`).FindAllStringSubmatch(text, -1)
	if len(observed) != len(report.Scenarios) {
		return fmt.Errorf("%s 必须为每个用户验收场景写一条实际观察", relative)
	}
	for i, match := range observed {
		if err := ValidateUserFacingText(fmt.Sprintf("%s 实测结果[%d]", relative, i), match[1]); err != nil {
			return err
		}
	}
	return nil
}

// submissionArchivePaths indexes committed evidence by contract ID.
func submissionArchivePaths(items []SubmissionEvidence) map[string]string {
	paths := make(map[string]string, len(items))
	for _, item := range items {
		paths[item.EvidenceID] = item.ArchivePath
	}
	return paths
}

// deliveryEvidenceLinks renders multiple directly clickable evidence files.
func deliveryEvidenceLinks(ids []string, archiveByID map[string]string, evidenceByID map[string]Evidence) string {
	links := make([]string, 0, len(ids))
	for _, id := range ids {
		links = append(links, deliveryEvidenceLink(id, evidenceByID[id].Purpose, archiveByID))
	}
	return strings.Join(links, "、")
}

// deliveryEvidenceLink renders one artifact using a reviewer-facing label instead of an internal ID.
func deliveryEvidenceLink(id, label string, archiveByID map[string]string) string {
	relative := deliveryEvidenceRelativePath(archiveByID[id])
	return fmt.Sprintf("[%s](%s)", strings.TrimSpace(label), deliveryURLPath(relative))
}

// deliveryEvidenceRelativePath removes the stable proposal package prefix from an archive path.
func deliveryEvidenceRelativePath(archivePath string) string {
	parts := strings.Split(filepath.ToSlash(archivePath), "/")
	if len(parts) < 5 {
		return filepath.Base(filepath.FromSlash(archivePath))
	}
	return strings.Join(parts[4:], "/")
}

// deliveryURLPath escapes each path segment without obscuring the evidence filename.
func deliveryURLPath(relative string) string {
	parts := strings.Split(filepath.ToSlash(relative), "/")
	for i, part := range parts {
		parts[i] = url.PathEscape(part)
	}
	return strings.Join(parts, "/")
}

// evidenceByID indexes required evidence for report rendering.
func evidenceByID(items []Evidence) map[string]Evidence {
	index := make(map[string]Evidence, len(items))
	for _, item := range items {
		index[item.ID] = item
	}
	return index
}
