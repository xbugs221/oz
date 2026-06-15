#!/usr/bin/env bash
# Sources: 25-限定子智能体artifact写入目录
# 文件功能目的：验证 subagent member artifact 使用独立目录、文件交付、自校验 CLI，并且 helper 交付失败不直接阻断主流程。
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
RESULT_DIR="$ROOT/test-results/subagent-artifact-directory"
TEST_FILE="$ROOT/internal/app/subagent_artifact_directory_contract_test.go"
LOG="$RESULT_DIR/subagent-artifact-directory.log"

mkdir -p "$RESULT_DIR"
: >"$LOG"

cleanup() {
  rm -f "$TEST_FILE"
}
trap cleanup EXIT

note() {
  # note 记录合同测试步骤，便于失败时区分实现问题和测试环境问题。
  printf '%s\n' "$*" | tee -a "$LOG"
}

cd "$ROOT"

note "写入 internal/app 包级合同测试，覆盖 artifact 目录、prompt 和 CLI 校验"
cat >"$TEST_FILE" <<'GO'
package app

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSubagentArtifactPathUsesDedicatedDirectory 证明每个 member 只能被引导写入自己的 artifact 目录。
func TestSubagentArtifactPathUsesDedicatedDirectory(t *testing.T) {
	repo := t.TempDir()
	memberA := "浏览器路径测试员"
	memberB := "CLI/API 路径测试员"

	pathA := memberArtifactPath(repo, "run-25", "qa", 1, memberA)
	pathB := memberArtifactPath(repo, "run-25", "qa", 1, memberB)

	if filepath.Base(pathA) != "member.json" {
		t.Fatalf("member artifact basename = %q, want member.json; path=%s", filepath.Base(pathA), pathA)
	}
	if !strings.HasSuffix(filepath.Base(filepath.Dir(pathA)), ".artifact") {
		t.Fatalf("member artifact parent dir = %q, want *.artifact; path=%s", filepath.Base(filepath.Dir(pathA)), pathA)
	}
	if filepath.Dir(pathA) == filepath.Dir(pathB) {
		t.Fatalf("different members share artifact dir: %s", filepath.Dir(pathA))
	}
	if !strings.Contains(pathA, filepath.Join("parallel-members", "qa", "1")) {
		t.Fatalf("member artifact path %q does not contain group/iteration segment", pathA)
	}
}

// TestSubagentPromptRequiresArtifactFileAndValidationCommand 证明 prompt 把文件写入和自校验作为交付合同。
func TestSubagentPromptRequiresArtifactFileAndValidationCommand(t *testing.T) {
	artifactPath := filepath.Join(t.TempDir(), "浏览器路径测试员.artifact", "member.json")
	member := ParallelMemberConfig{Name: "浏览器路径测试员", Purpose: "执行页面真实用户路径"}
	context := subagentContext{
		ChangeName:     "25-限定子智能体artifact写入目录",
		StatePath:      "/tmp/run/state.json",
		ChangePath:     "docs/changes/25-限定子智能体artifact写入目录",
		AcceptancePath: "docs/changes/25-限定子智能体artifact写入目录/acceptance.json",
	}

	prompt := subagentPrompt("qa", member, artifactPath, context)
	for _, required := range []string{
		"ARTIFACT_DIR=",
		"ARTIFACT_PATH=" + artifactPath,
		"oz flow validate-member-artifact",
		"--artifact \"$ARTIFACT_PATH\"",
		"--group qa",
		"--member 浏览器路径测试员",
		"--change 25-限定子智能体artifact写入目录",
	} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("subagent prompt missing %q\nprompt:\n%s", required, prompt)
		}
	}
	if strings.Contains(prompt, "最终只输出一个 JSON object") || strings.Contains(prompt, "最终只输出裸 JSON object") {
		t.Fatalf("subagent prompt still relies on final bare JSON instead of file artifact\nprompt:\n%s", prompt)
	}
}

// TestValidateMemberArtifactCommandReportsHelpfulErrors 证明 CLI 可被 subagent 用来快速修正 artifact。
func TestValidateMemberArtifactCommandReportsHelpfulErrors(t *testing.T) {
	dir := t.TempDir()
	validPath := filepath.Join(dir, "member.json")
	valid := `{"name":"浏览器路径测试员","change_name":"25-限定子智能体artifact写入目录","purpose":"执行页面真实用户路径","status":"skipped","relevant":false,"irrelevant_reason":"本提案没有浏览器页面路径","summary":"","evidence":[],"findings":[]}` + "\n"
	if err := os.WriteFile(validPath, []byte(valid), 0o644); err != nil {
		t.Fatalf("write valid artifact: %v", err)
	}

	var stdout bytes.Buffer
	err := Run([]string{
		"validate-member-artifact",
		"--artifact", validPath,
		"--group", "qa",
		"--member", "浏览器路径测试员",
		"--change", "25-限定子智能体artifact写入目录",
	}, strings.NewReader(""), &stdout, io.Discard)
	if err != nil {
		t.Fatalf("valid member artifact should pass, err=%v stdout=%s", err, stdout.String())
	}
	if !strings.Contains(stdout.String(), "member artifact 合法") {
		t.Fatalf("success output should name valid member artifact, got %q", stdout.String())
	}

	invalidPath := filepath.Join(dir, "invalid-member.json")
	invalid := `{"name":"浏览器路径测试员","change_name":"25-限定子智能体artifact写入目录","purpose":"执行页面真实用户路径","status":"skipped","relevant":false,"irrelevant_reason":"本提案没有浏览器页面路径","summary":"","evidence":{"log":"bad"},"findings":[]}` + "\n"
	if err := os.WriteFile(invalidPath, []byte(invalid), 0o644); err != nil {
		t.Fatalf("write invalid artifact: %v", err)
	}

	stdout.Reset()
	err = Run([]string{
		"validate-member-artifact",
		"--artifact", invalidPath,
		"--group", "qa",
		"--member", "浏览器路径测试员",
		"--change", "25-限定子智能体artifact写入目录",
	}, strings.NewReader(""), &stdout, io.Discard)
	if err == nil {
		t.Fatalf("invalid member artifact should fail")
	}
	message := err.Error() + "\n" + stdout.String()
	for _, required := range []string{"field=evidence", "expected=array<string>", "修复建议"} {
		if !strings.Contains(message, required) {
			t.Fatalf("invalid artifact error missing %q\nmessage:\n%s", required, message)
		}
	}
}
GO

note "运行 subagent artifact directory 合同测试"
go test ./internal/app -run 'TestSubagentArtifactPathUsesDedicatedDirectory|TestSubagentPromptRequiresArtifactFileAndValidationCommand|TestValidateMemberArtifactCommandReportsHelpfulErrors|TestSubagentMalformedArtifactBecomesAdvisoryInput|TestSubagentBoundaryRevertsSiblingRunArtifact|TestSubagentBoundaryRevertsSourceWrite|TestSubagentBoundaryAllowsSessionProgressStateWrite' -count=1 2>&1 | tee -a "$LOG"
note "PASS"
