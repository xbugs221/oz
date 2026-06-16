// Package ozcli implements the standalone oz CLI for installing skills and inspecting oz changes.
package ozcli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/xbugs221/oz/internal/acceptance"
)

var (
	version                = "dev"
	activeChangeNumberRe   = regexp.MustCompile(`^([1-9][0-9]*)-.+$`)
	archivedChangeNumberRe = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}-([1-9][0-9]*)-.+$`)
)

type cli struct {
	out io.Writer
	err io.Writer
	now func() time.Time
}

type validationResult struct {
	Valid       bool                             `json:"valid"`
	Change      string                           `json:"change"`
	Errors      []string                         `json:"errors"`
	Warnings    []string                         `json:"warnings"`
	Artifacts   map[string]string                `json:"artifacts"`
	Diagnostics []acceptance.LifecycleDiagnostic `json:"diagnostics,omitempty"`
}

func stateRoot() (string, error) {
	// stateRoot resolves the fixed docs state directory inside the current project.
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return filepath.Join(cwd, "docs"), nil
}

func validateChangeName(name string) error {
	// validateChangeName accepts Chinese descriptions mixed with ASCII words, digits, and hyphens.
	if name == "" {
		return errors.New("change-name 不能为空")
	}
	hasChinese := false
	for i, r := range name {
		if isChinese(r) {
			hasChinese = true
			continue
		}
		valid := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || unicode.IsDigit(r) || r == '-'
		if !valid || (i == 0 && r == '-') {
			return errors.New("change-name 只能包含中文汉字、ASCII 字母、数字和连字符")
		}
	}
	if !hasChinese {
		return errors.New("change-name 至少包含一个中文汉字")
	}
	return nil
}

func validateNumberedChange(change string) error {
	// validateNumberedChange verifies the archived and active change directory naming rule.
	re := regexp.MustCompile(`^[1-9][0-9]*-(.+)$`)
	matches := re.FindStringSubmatch(change)
	if matches == nil {
		return errors.New("变更目录必须符合 <number>-<change-name>")
	}
	return validateChangeName(matches[1])
}

func displayStatus(status string) string {
	// displayStatus localizes human-readable status values while JSON keeps stable machine strings.
	switch status {
	case "present":
		return "已存在"
	case "missing":
		return "缺失"
	case "ready":
		return "可归档"
	case "incomplete":
		return "未完成"
	default:
		return status
	}
}

func looksLikeTestCode(name string) bool {
	// looksLikeTestCode keeps tests/ for executable project tests instead of notes.
	lower := strings.ToLower(name)
	if strings.HasSuffix(lower, ".md") || strings.HasSuffix(lower, ".txt") {
		return false
	}
	return strings.Contains(lower, "test") || strings.HasSuffix(lower, "_test.go") || strings.HasSuffix(lower, ".spec.ts")
}

func isChinese(r rune) bool {
	// isChinese checks the common CJK unified ideograph ranges used by Chinese filenames.
	return (r >= '\u4e00' && r <= '\u9fff') || (r >= '\u3400' && r <= '\u4dbf')
}

func hasArg(args []string, want string) bool {
	// hasArg reports whether a flag-like argument is present.
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}

func hasHelp(args []string) bool {
	// hasHelp reports whether a command-specific help flag was requested.
	return hasArg(args, "--help") || hasArg(args, "-h")
}

func firstPositional(args []string) string {
	// firstPositional skips flags and returns the command target.
	for _, arg := range args {
		if !strings.HasPrefix(arg, "-") {
			return arg
		}
	}
	return ""
}

func unique(values []string) []string {
	// unique returns deterministic diagnostics with duplicates removed.
	seen := map[string]bool{}
	out := []string{}
	for _, value := range values {
		if !seen[value] {
			seen[value] = true
			out = append(out, value)
		}
	}
	sort.Strings(out)
	return out
}

func writeJSON(out io.Writer, payload any) error {
	// writeJSON emits stable indented JSON for scripts and agents.
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(out, string(data))
	return err
}
