// Package ozcli implements standalone oz change validation.
package ozcli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/xbugs221/oz/internal/acceptance"
)

func (c *cli) validateCmd(args []string) error {
	// validateCmd validates a fixed-format oz change and optionally emits stable JSON.
	if hasHelp(args) {
		fmt.Fprintln(c.out, "用法：oz validate <change> [--json]")
		return nil
	}
	jsonOut := hasArg(args, "--json")
	change := firstPositional(args)
	if change == "" {
		return errors.New("用法：oz validate <change> [--json]")
	}
	root, err := stateRoot()
	if err != nil {
		return err
	}
	result := validateChange(root, change)
	if jsonOut {
		_ = writeJSON(c.out, result)
	} else if result.Valid {
		fmt.Fprintf(c.out, "%s 校验通过\n", change)
	} else {
		for _, e := range result.Errors {
			fmt.Fprintln(c.err, e)
		}
	}
	if !result.Valid {
		return errors.New("校验失败")
	}
	return nil
}

func validateChange(root, change string) validationResult {
	// validateChange checks naming, required artifacts, spec semantics, and test directory purpose.
	result := validationResult{
		Valid:     true,
		Change:    change,
		Errors:    []string{},
		Warnings:  []string{},
		Artifacts: map[string]string{},
	}
	if err := validateNumberedChange(change); err != nil {
		result.Errors = append(result.Errors, err.Error())
	}
	changeDir := filepath.Join(root, "changes", change)
	for _, name := range []string{"brief.md", "proposal.md", "design.md", "spec.md", "task.md", "acceptance.json"} {
		path := filepath.Join(changeDir, name)
		result.Artifacts[name] = path
		if info, err := os.Stat(path); err != nil || info.IsDir() {
			result.Errors = append(result.Errors, "缺少 "+name)
		}
	}
	testsDir := filepath.Join(changeDir, "tests")
	result.Artifacts["tests"] = testsDir
	if info, err := os.Stat(testsDir); err != nil || !info.IsDir() {
		result.Errors = append(result.Errors, "缺少 tests")
	} else if entries, err := os.ReadDir(testsDir); err != nil {
		result.Errors = append(result.Errors, "无法读取 tests："+err.Error())
	} else {
		if len(entries) == 0 {
			result.Errors = append(result.Errors, "tests 必须包含至少一个测试文件")
		}
		for _, entry := range entries {
			if entry.IsDir() || !looksLikeTestCode(entry.Name()) {
				result.Errors = append(result.Errors, "tests 包含非测试代码："+entry.Name())
			}
		}
	}
	if data, err := os.ReadFile(filepath.Join(changeDir, "spec.md")); err == nil {
		result.Errors = append(result.Errors, validateSpecText(string(data))...)
	}
	if data, err := os.ReadFile(filepath.Join(changeDir, "task.md")); err == nil && !strings.Contains(string(data), "- [") {
		result.Errors = append(result.Errors, "task.md 必须包含任务项")
	}
	acceptancePath := filepath.Join(changeDir, "acceptance.json")
	if contract, err := acceptance.Read(acceptancePath); err != nil {
		result.Errors = append(result.Errors, "acceptance.json 无效："+err.Error())
	} else {
		result.Errors = append(result.Errors, validateAcceptanceFiles(root, contract)...)
	}
	result.Errors = append(result.Errors, validateRuntimeArtifactPolicy(filepath.Dir(root), changeDir)...)
	result.Errors = unique(result.Errors)
	result.Valid = len(result.Errors) == 0
	return result
}

func validateAcceptanceFiles(root string, contract acceptance.Contract) []string {
	// validateAcceptanceFiles binds acceptance.json entries to real tests and evidence references.
	errs := []string{}
	projectRoot := filepath.Dir(root)
	tests := map[string]acceptance.Test{}
	for i, test := range contract.RequiredTests {
		tests[test.ID] = test
		if filepath.IsAbs(test.Path) || strings.TrimSpace(test.Path) == "." {
			errs = append(errs, fmt.Sprintf("required_tests[%d].path 必须是相对测试路径：%s", i, test.Path))
			continue
		}
		testPath := filepath.Join(projectRoot, filepath.Clean(test.Path))
		if info, err := os.Stat(testPath); err != nil || info.IsDir() {
			errs = append(errs, fmt.Sprintf("required_tests[%d].path 指向的测试不存在：%s", i, test.Path))
		}
		if !strings.Contains(test.Command, test.Path) {
			errs = append(errs, fmt.Sprintf("required_tests[%d].command 必须引用 path：%s", i, test.Path))
		}
	}
	for i, evidence := range contract.RequiredEvidence {
		if filepath.IsAbs(evidence.Path) || strings.TrimSpace(evidence.Path) == "." {
			errs = append(errs, fmt.Sprintf("required_evidence[%d].path 必须是相对产物路径：%s", i, evidence.Path))
		}
		if !acceptance.EvidenceHasProducer(projectRoot, evidence, contract.Coverage, tests) {
			errs = append(errs, fmt.Sprintf("required_evidence[%d] %q 无法追溯到 required_tests producer：必须在 coverage 绑定的 required_tests 的 command/purpose/assertions、测试文件或同目录 .sh wrapper 中明确产出 evidence id/path", i, evidence.ID))
		}
	}
	return errs
}

func validateRuntimeArtifactPolicy(projectRoot, changeDir string) []string {
	// validateRuntimeArtifactPolicy keeps runtime output out of version-control contracts.
	errs := []string{}
	testsDir := filepath.Join(changeDir, "tests")
	if entries, err := os.ReadDir(testsDir); err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			path := filepath.Join(testsDir, entry.Name())
			data, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			text := string(data)
			if strings.Contains(text, "test-results") && strings.Contains(text, "git ls-files") && strings.Contains(text, "--error-unmatch") {
				rel, _ := filepath.Rel(projectRoot, path)
				errs = append(errs, fmt.Sprintf("%s 不得要求 test-results 通过 git ls-files 被版本控制；测试结果是运行产物，只能校验存在性和内容", rel))
			}
		}
	}
	if data, err := os.ReadFile(filepath.Join(projectRoot, ".gitignore")); err == nil {
		for lineNo, line := range strings.Split(string(data), "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "!test-results") || strings.HasPrefix(trimmed, "!/test-results") {
				errs = append(errs, fmt.Sprintf(".gitignore:%d 不得为 test-results 添加跟踪例外；测试结果应保持忽略", lineNo+1))
			}
		}
	}
	return errs
}

func validateSpecText(text string) []string {
	// validateSpecText recognizes the minimum Chinese requirement, normative word, and scenario form.
	lines := strings.Split(text, "\n")
	hasReq, hasNorm, hasScenario := false, false, false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "### 需求：") {
			hasReq = true
		}
		if strings.Contains(trimmed, "必须") || strings.Contains(trimmed, "应当") || strings.Contains(trimmed, "不得") {
			hasNorm = true
		}
		if strings.HasPrefix(trimmed, "#### 场景：") {
			hasScenario = true
		}
	}
	errs := []string{}
	if !hasReq {
		errs = append(errs, "spec.md 缺少需求")
	}
	if !hasNorm {
		errs = append(errs, "spec.md 缺少规范词")
	}
	if !hasScenario {
		errs = append(errs, "spec.md 缺少场景")
	}
	return errs
}
