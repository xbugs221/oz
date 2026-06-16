// Package ozcli implements standalone oz change validation.
package ozcli

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
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
		visibleEntries := 0
		for _, entry := range entries {
			path := filepath.Join(testsDir, entry.Name())
			if isGitIgnored(filepath.Dir(root), path) {
				continue
			}
			visibleEntries++
			if entry.IsDir() || !looksLikeTestCode(entry.Name()) {
				result.Errors = append(result.Errors, "tests 包含非测试代码："+entry.Name())
			}
		}
		if visibleEntries == 0 {
			result.Errors = append(result.Errors, "tests 必须包含至少一个测试文件")
		}
	}
	if data, err := os.ReadFile(filepath.Join(changeDir, "spec.md")); err == nil {
		result.Errors = append(result.Errors, validateSpecText(string(data))...)
	}
	if data, err := os.ReadFile(filepath.Join(changeDir, "task.md")); err == nil && !strings.Contains(string(data), "- [") {
		result.Errors = append(result.Errors, "task.md 必须包含任务项")
	}
	acceptancePath := filepath.Join(changeDir, "acceptance.json")
	diagnostics, acceptanceErrors := validateAcceptanceFiles(filepath.Dir(root), acceptancePath)
	result.Diagnostics = append(result.Diagnostics, diagnostics...)
	result.Errors = append(result.Errors, acceptanceErrors...)
	result.Errors = append(result.Errors, validateRuntimeArtifactPolicy(filepath.Dir(root), changeDir)...)
	result.Errors = unique(result.Errors)
	result.Valid = len(result.Errors) == 0
	return result
}

func validateAcceptanceFiles(projectRoot, acceptancePath string) ([]acceptance.LifecycleDiagnostic, []string) {
	// validateAcceptanceFiles preserves the ozcli validation boundary while delegating lifecycle checks.
	contract, err := acceptance.Read(acceptancePath)
	if err != nil {
		return nil, []string{"acceptance.json 无效：" + err.Error()}
	}
	lifecycle := acceptance.ValidateLifecycle(projectRoot, contract)
	errs := make([]string, 0, len(lifecycle.Diagnostics))
	for _, diagnostic := range lifecycle.Diagnostics {
		errs = append(errs, diagnostic.Message)
	}
	return lifecycle.Diagnostics, errs
}

func isGitIgnored(projectRoot, path string) bool {
	// isGitIgnored asks git whether a generated test artifact should be invisible to validation.
	rel, err := filepath.Rel(projectRoot, path)
	if err != nil || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return false
	}
	cmd := exec.Command("git", "-C", projectRoot, "check-ignore", "--quiet", "--", filepath.ToSlash(rel))
	return cmd.Run() == nil
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
