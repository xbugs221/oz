// Package app tests the durable repair/QA loop through the real Go DAG engine.
package app

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// repairWorkflowRunner writes valid artifacts for a resumed QA-failure repair loop.
type repairWorkflowRunner struct {
	t      *testing.T
	repo   string
	runID  string
	change string
	calls  int
}

// Run simulates agent output while preserving the engine's real stage and session behavior.
func (r *repairWorkflowRunner) Run(_ context.Context, _ string, prompt string, sessionID string, _ StageOptions) (string, error) {
	r.calls++
	base := runDir(r.repo, r.runID)
	switch r.calls {
	case 1:
		if sessionID != "repair-session" {
			return "", fmt.Errorf("first agent session = %q, want repair-session; prompt=%q", sessionID, promptStageExcerpt(prompt))
		}
		repair := cleanReviewForStageDecision()
		repair.Evidence = []string{"go test ./internal/app；runtime DAG resume verified"}
		if err := writeJSONFile(filepath.Join(base, "repair-2.json"), repair); err != nil {
			return "", err
		}
		return "repair-session", nil
	case 2:
		if sessionID != "qa-session" {
			return "", fmt.Errorf("second agent session = %q, want qa-session; prompt=%q", sessionID, promptStageExcerpt(prompt))
		}
		qa := cleanQAForStageDecision()
		qa.Evidence = []string{"runtime go test ./internal/app passed"}
		qa.AcceptanceMatrix = []AcceptanceResult{
			{ID: "repair-dag-contract", Status: "passed", Artifact: "test-results/repair-dag/runtime.log", Evidence: "integration contract passed"},
			{ID: "repair-dag-runtime", Status: "passed", Artifact: "test-results/repair-dag/runtime.log", Evidence: "runtime log exists"},
		}
		if err := writeJSONFile(filepath.Join(base, "qa-2.json"), qa); err != nil {
			return "", err
		}
		return "qa-session", nil
	case 3:
		if err := os.WriteFile(filepath.Join(base, "delivery-summary.md"), []byte("# 交付\n\n## 最终审核\n\n独立 QA 已通过。\n"), 0o644); err != nil {
			return "", err
		}
		archiveRoot := filepath.Join(r.repo, "docs", "changes", "archive")
		if err := os.MkdirAll(archiveRoot, 0o755); err != nil {
			return "", err
		}
		if err := os.Rename(
			filepath.Join(r.repo, "docs", "changes", r.change),
			filepath.Join(archiveRoot, "20260726-"+r.change),
		); err != nil {
			return "", err
		}
		return "archive-session", nil
	default:
		return "", fmt.Errorf("unexpected agent call %d; prompt=%q", r.calls, promptStageExcerpt(prompt))
	}
}

// promptStageExcerpt returns the first line that identifies an artifact target.
func promptStageExcerpt(prompt string) string {
	for _, line := range strings.Split(prompt, "\n") {
		if strings.Contains(line, "repair-") || strings.Contains(line, "qa-") || strings.Contains(line, "delivery-summary") {
			return strings.TrimSpace(line)
		}
	}
	return ""
}

// newRepairEvidenceFixture creates a committed, oz-valid change for engine-level tests.
func newRepairEvidenceFixture(t *testing.T) (string, string, string, string, string) {
	t.Helper()
	repo := gitRepo(t)
	changeName := "1-演示"
	changeDir := filepath.Join(repo, "docs", "changes", changeName)
	if err := os.MkdirAll(changeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"task.md":     "- [x] fixture task\n",
		"brief.md":    "# 优化 DAG 集成测试\n",
		"design.md":   "# 优化 DAG 集成测试\n",
		"proposal.md": "# 优化 DAG 集成测试\n",
		"spec.md":     "### 需求：恢复质量门禁\n\n#### 场景：工作流从持久状态恢复\n\n- **给定** 已封存运行\n- **当** 引擎恢复\n- **则** 必须遵循持久状态\n",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(changeDir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	testPath := filepath.Join(changeDir, "tests", "test_repair_dag.sh")
	if err := os.MkdirAll(filepath.Dir(testPath), 0o755); err != nil {
		t.Fatal(err)
	}
	testBody := "#!/usr/bin/env bash\n# 文件功能目的：为优化 DAG 集成测试生成真实运行证据。\nset -euo pipefail\nmkdir -p test-results/repair-dag\nprintf 'runtime verified\\n' > test-results/repair-dag/runtime.log\n"
	if err := os.WriteFile(testPath, []byte(testBody), 0o755); err != nil {
		t.Fatal(err)
	}
	acceptanceSource := filepath.Join(changeDir, "acceptance.json")
	if err := os.WriteFile(acceptanceSource, []byte(repairDAGAcceptanceJSON()), 0o644); err != nil {
		t.Fatal(err)
	}
	fixture := &workflowFixture{t: t, repo: repo}
	fixture.git("add", ".")
	fixture.git("commit", "-q", "-m", "add repair DAG fixture")
	head, diff, err := gitSnapshot(repo)
	if err != nil {
		t.Fatal(err)
	}
	return repo, changeName, acceptanceSource, head, diff
}

// saveRepairEvidenceState writes one engine-produced state when the contract requests it.
func saveRepairEvidenceState(t *testing.T, path string, state State) {
	t.Helper()
	if path == "" {
		return
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
}

// archiveRepairEvidence writes archive output and moves the active fixture change.
func archiveRepairEvidence(repo, runID, changeName string) error {
	base := runDir(repo, runID)
	if err := os.WriteFile(filepath.Join(base, "delivery-summary.md"), []byte("# 交付\n\n## 最终审核\n\n独立 QA 已通过。\n"), 0o644); err != nil {
		return err
	}
	archiveRoot := filepath.Join(repo, "docs", "changes", "archive")
	if err := os.MkdirAll(archiveRoot, 0o755); err != nil {
		return err
	}
	return os.Rename(
		filepath.Join(repo, "docs", "changes", changeName),
		filepath.Join(archiveRoot, "20260726-"+changeName),
	)
}

// cleanRepairDAGQA returns a QA artifact covering the integration acceptance contract.
func cleanRepairDAGQA() QA {
	qa := cleanQAForStageDecision()
	qa.Evidence = []string{"runtime go test ./internal/app passed"}
	qa.AcceptanceMatrix = []AcceptanceResult{
		{ID: "repair-dag-contract", Status: "passed", Artifact: "test-results/repair-dag/runtime.log", Evidence: "integration contract passed"},
		{ID: "repair-dag-runtime", Status: "passed", Artifact: "test-results/repair-dag/runtime.log", Evidence: "runtime log exists"},
	}
	return qa
}

// TestRepairWorkflowDAGResumeEvidence proves QA failure activates the next repair exactly once.
func TestRepairWorkflowDAGResumeEvidence(t *testing.T) {
	evidencePath := os.Getenv("REPAIR_STATE_EVIDENCE")
	t.Setenv("REPAIR_STATE_EVIDENCE", "")
	repo := gitRepo(t)
	changeName := "1-演示"
	changeDir := filepath.Join(repo, "docs", "changes", changeName)
	if err := os.MkdirAll(changeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(changeDir, "task.md"), []byte("- [x] fixture task\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"brief.md", "design.md", "proposal.md", "spec.md"} {
		body := "# 优化 DAG 集成测试\n"
		if name == "spec.md" {
			body = "### 需求：恢复 QA 回环\n\n#### 场景：QA 失败进入下一 repair\n\n- **给定** 已封存运行\n- **当** QA 返回 needs_fix\n- **则** 必须执行下一 repair\n"
		}
		if err := os.WriteFile(filepath.Join(changeDir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	testPath := filepath.Join(changeDir, "tests", "test_repair_dag.sh")
	if err := os.MkdirAll(filepath.Dir(testPath), 0o755); err != nil {
		t.Fatal(err)
	}
	testBody := "#!/usr/bin/env bash\n# 文件功能目的：为优化 DAG 集成测试生成真实运行证据。\nset -euo pipefail\nmkdir -p test-results/repair-dag\nprintf 'runtime verified\\n' > test-results/repair-dag/runtime.log\n"
	if err := os.WriteFile(testPath, []byte(testBody), 0o755); err != nil {
		t.Fatal(err)
	}
	acceptanceSource := filepath.Join(changeDir, "acceptance.json")
	if err := os.WriteFile(acceptanceSource, []byte(repairDAGAcceptanceJSON()), 0o644); err != nil {
		t.Fatal(err)
	}
	fixture := &workflowFixture{t: t, repo: repo}
	fixture.git("add", ".")
	fixture.git("commit", "-q", "-m", "add repair DAG fixture")

	workflow := DefaultWorkflowConfig()
	workflow.MaxRepairIterations = 2
	workflow.MaxReviewIterations = 0
	head, diff, err := gitSnapshot(repo)
	if err != nil {
		t.Fatal(err)
	}
	runID := newRunID()
	state := State{
		RunID:        runID,
		ChangeName:   changeName,
		Sealed:       true,
		Status:       statusRunning,
		Stage:        "qa_1",
		BaselineHead: head,
		BaselineDiff: diff,
		Workflow:     workflow,
		Sessions: map[string]string{
			sessionStateKey("codex", "repairer"): "repair-session",
			sessionStateKey("codex", "qa"):       "qa-session",
		},
		Stages: map[string]string{
			"execution": "completed",
			"repair_1":  "completed",
			"qa_1":      "completed",
		},
		Paths:    map[string]string{},
		DAGNodes: map[string]DAGNodeState{},
	}
	if err := saveState(repo, state); err != nil {
		t.Fatal(err)
	}
	if err := snapshotRunPrompts(repo, runID); err != nil {
		t.Fatal(err)
	}
	if err := snapshotRunAcceptance(repo, runID, acceptanceSource); err != nil {
		t.Fatal(err)
	}
	qa := cleanQAForStageDecision()
	qa.Decision = "needs_fix"
	qa.Findings = []Finding{blockingFindingForStageDecision()}
	if err := writeJSONFile(filepath.Join(runDir(repo, runID), "qa-1.json"), qa); err != nil {
		t.Fatal(err)
	}

	runner := &repairWorkflowRunner{t: t, repo: repo, runID: runID, change: changeName}
	registry := NewAgentRegistry()
	registry.Register(fakeWorkflowTool{runner: runner})
	engine := NewEngine(repo, registry)
	if done, err := engine.artifactDone(state); err != nil || !done {
		t.Fatalf("seed qa_1 artifact invalid: done=%v err=%v", done, err)
	}
	if err := engine.runGoDAGLocked(context.Background(), state); err != nil {
		t.Fatal(err)
	}

	got, err := loadState(repo, runID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != statusDone || got.Stage != "done" {
		t.Fatalf("final state = %s/%s, want done/done", got.Status, got.Stage)
	}
	if runner.calls != 3 || got.Stages["repair_2"] != "completed" || got.Stages["qa_2"] != "completed" {
		t.Fatalf("repair/QA resume did not complete exactly once: calls=%d stages=%#v", runner.calls, got.Stages)
	}
	if evidencePath != "" {
		data, err := json.MarshalIndent(got, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Dir(evidencePath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(evidencePath, append(data, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// zeroRepairWorkflowRunner produces independent QA and archive artifacts without a repair turn.
type zeroRepairWorkflowRunner struct {
	repo       string
	runID      string
	changeName string
	calls      int
}

// Run writes the zero-repair QA and archive artifacts in engine order.
func (r *zeroRepairWorkflowRunner) Run(_ context.Context, _ string, prompt string, sessionID string, _ StageOptions) (string, error) {
	r.calls++
	switch r.calls {
	case 1:
		if sessionID != "zero-qa-session" {
			return "", fmt.Errorf("zero-repair QA session = %q; prompt=%q", sessionID, promptStageExcerpt(prompt))
		}
		if strings.Contains(prompt, "repair-1.json") || !strings.Contains(prompt, "零轮 repair 模式无 repair 检查点") {
			return "", fmt.Errorf("zero-repair QA prompt has invalid checkpoint context: %q", promptStageExcerpt(prompt))
		}
		if err := writeJSONFile(filepath.Join(runDir(r.repo, r.runID), "qa-1.json"), cleanRepairDAGQA()); err != nil {
			return "", err
		}
		return "zero-qa-session", nil
	case 2:
		if !strings.Contains(prompt, filepath.Join(runDir(r.repo, r.runID), "qa-1.json")) {
			return "", fmt.Errorf("zero-repair archive prompt misses qa-1.json: %q", promptStageExcerpt(prompt))
		}
		if err := archiveRepairEvidence(r.repo, r.runID, r.changeName); err != nil {
			return "", err
		}
		return "zero-archive-session", nil
	default:
		return "", fmt.Errorf("unexpected zero-repair agent call %d", r.calls)
	}
}

// TestZeroRepairWorkflowDAGArchive proves QA clean can archive without a review artifact.
func TestZeroRepairWorkflowDAGArchive(t *testing.T) {
	repo, changeName, acceptanceSource, head, diff := newRepairEvidenceFixture(t)
	workflow := DefaultWorkflowConfig()
	workflow.MaxRepairIterations = 0
	workflow.MaxReviewIterations = 0
	runID := newRunID()
	state := State{
		RunID:        runID,
		ChangeName:   changeName,
		Sealed:       true,
		Status:       statusRunning,
		Stage:        "execution",
		BaselineHead: head,
		BaselineDiff: diff,
		Workflow:     workflow,
		Sessions: map[string]string{
			sessionStateKey("codex", "qa"): "zero-qa-session",
		},
		Stages:   map[string]string{},
		Paths:    map[string]string{},
		DAGNodes: map[string]DAGNodeState{},
	}
	if err := saveState(repo, state); err != nil {
		t.Fatal(err)
	}
	if err := snapshotRunPrompts(repo, runID); err != nil {
		t.Fatal(err)
	}
	if err := snapshotRunAcceptance(repo, runID, acceptanceSource); err != nil {
		t.Fatal(err)
	}
	runner := &zeroRepairWorkflowRunner{repo: repo, runID: runID, changeName: changeName}
	registry := NewAgentRegistry()
	registry.Register(fakeWorkflowTool{runner: runner})
	engine := NewEngine(repo, registry)
	if err := engine.runGoDAGLocked(context.Background(), state); err != nil {
		t.Fatal(err)
	}
	got, err := loadState(repo, runID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != statusDone || got.Stage != "done" || runner.calls != 2 {
		t.Fatalf("zero-repair final state = %s/%s calls=%d", got.Status, got.Stage, runner.calls)
	}
}

// TestRepairLimitBlockedWorkflowEvidence proves the final repair blocks through the real DAG.
func TestRepairLimitBlockedWorkflowEvidence(t *testing.T) {
	evidencePath := os.Getenv("REPAIR_LIMIT_STATE_EVIDENCE")
	t.Setenv("REPAIR_LIMIT_STATE_EVIDENCE", "")
	repo, changeName, acceptanceSource, head, diff := newRepairEvidenceFixture(t)
	workflow := DefaultWorkflowConfig()
	workflow.MaxRepairIterations = 2
	workflow.MaxReviewIterations = 0
	runID := newRunID()
	state := State{
		RunID:        runID,
		ChangeName:   changeName,
		Sealed:       true,
		Status:       statusRunning,
		Stage:        "repair_2",
		BaselineHead: head,
		BaselineDiff: diff,
		Workflow:     workflow,
		Sessions: map[string]string{
			sessionStateKey("codex", "repairer"): "limit-repair-session",
		},
		Stages: map[string]string{
			"execution": "completed",
			"repair_1":  "completed",
		},
		Paths:    map[string]string{},
		DAGNodes: map[string]DAGNodeState{},
	}
	if err := saveState(repo, state); err != nil {
		t.Fatal(err)
	}
	if err := snapshotRunPrompts(repo, runID); err != nil {
		t.Fatal(err)
	}
	if err := snapshotRunAcceptance(repo, runID, acceptanceSource); err != nil {
		t.Fatal(err)
	}
	repair := cleanReviewForStageDecision()
	repair.Decision = "needs_more"
	repair.Findings = []Finding{blockingFindingForStageDecision()}
	if err := writeJSONFile(filepath.Join(runDir(repo, runID), "repair-2.json"), repair); err != nil {
		t.Fatal(err)
	}
	engine := NewEngine(repo, (&workflowFixture{t: t, repo: repo, runner: &fakeWorkflowRunner{}}).fakeToolRegistry())
	if err := engine.runGoDAGLocked(context.Background(), state); err != nil {
		t.Fatal(err)
	}
	got, err := loadState(repo, runID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != statusBlocked || got.Stage != statusBlocked || !strings.Contains(got.Error, "达到上限") {
		t.Fatalf("limit final state = %s/%s error=%q", got.Status, got.Stage, got.Error)
	}
	saveRepairEvidenceState(t, evidencePath, got)
}

// legacyRepairWorkflowRunner completes a restored review/fix sealed snapshot.
type legacyRepairWorkflowRunner struct {
	repo       string
	runID      string
	changeName string
	calls      int
}

// Run writes legacy review, QA, and archive artifacts while checking session continuity.
func (r *legacyRepairWorkflowRunner) Run(_ context.Context, _ string, prompt string, sessionID string, _ StageOptions) (string, error) {
	r.calls++
	switch r.calls {
	case 1:
		if sessionID != "legacy-review-session" {
			return "", fmt.Errorf("legacy review session = %q; prompt=%q", sessionID, promptStageExcerpt(prompt))
		}
		review := cleanReviewForStageDecision()
		review.Evidence = []string{"go test ./internal/app；runtime legacy resume verified"}
		if err := writeJSONFile(filepath.Join(runDir(r.repo, r.runID), "review-2.json"), review); err != nil {
			return "", err
		}
		return "legacy-review-session", nil
	case 2:
		if sessionID != "legacy-qa-session" {
			return "", fmt.Errorf("legacy QA session = %q; prompt=%q", sessionID, promptStageExcerpt(prompt))
		}
		if err := writeJSONFile(filepath.Join(runDir(r.repo, r.runID), "qa-2.json"), cleanRepairDAGQA()); err != nil {
			return "", err
		}
		return "legacy-qa-session", nil
	case 3:
		if err := archiveRepairEvidence(r.repo, r.runID, r.changeName); err != nil {
			return "", err
		}
		return "legacy-archive-session", nil
	default:
		return "", fmt.Errorf("unexpected legacy agent call %d", r.calls)
	}
}

// TestLegacyRepairWorkflowResumeEvidence proves an old review/fix snapshot resumes through the engine.
func TestLegacyRepairWorkflowResumeEvidence(t *testing.T) {
	evidencePath := os.Getenv("REPAIR_LEGACY_STATE_EVIDENCE")
	t.Setenv("REPAIR_LEGACY_STATE_EVIDENCE", "")
	repo, changeName, acceptanceSource, head, diff := newRepairEvidenceFixture(t)
	workflow := DefaultWorkflowConfig()
	workflow.MaxRepairIterations = 0
	workflow.MaxReviewIterations = 2
	byKind := defaultStageOptionsByKind()
	for i := 1; i <= 2; i++ {
		workflow.Stages[fmt.Sprintf("review_%d", i)] = byKind["review"]
		workflow.Stages[fmt.Sprintf("fix_%d", i)] = byKind["fix"]
	}
	runID := newRunID()
	state := State{
		RunID:        runID,
		ChangeName:   changeName,
		Sealed:       true,
		Status:       statusRunning,
		Stage:        "fix_1",
		BaselineHead: head,
		BaselineDiff: diff,
		Workflow:     workflow,
		Sessions: map[string]string{
			sessionStateKey("codex", "fixer"):    "legacy-fix-session",
			sessionStateKey("codex", "reviewer"): "legacy-review-session",
			sessionStateKey("codex", "qa"):       "legacy-qa-session",
		},
		Stages: map[string]string{
			"execution": "completed",
			"review_1":  "completed",
		},
		Paths:    map[string]string{},
		DAGNodes: map[string]DAGNodeState{},
	}
	if err := saveState(repo, state); err != nil {
		t.Fatal(err)
	}
	prompts := defaultPromptSet()
	legacySnapshot := map[string]any{"prompts": map[string]string{
		"planning":  prompts["planning"],
		"execution": prompts["execution"],
		"review":    prompts["review"],
		"qa":        prompts["qa"],
		"fix":       prompts["fix"],
		"archive":   prompts["archive"],
	}}
	if err := writeJSONFile(filepath.Join(runDir(repo, runID), "prompt-snapshot.yaml"), legacySnapshot); err != nil {
		t.Fatal(err)
	}
	if err := snapshotRunAcceptance(repo, runID, acceptanceSource); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runDir(repo, runID), "fix-1-summary.md"), []byte("# 修复完成\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runner := &legacyRepairWorkflowRunner{repo: repo, runID: runID, changeName: changeName}
	registry := NewAgentRegistry()
	registry.Register(fakeWorkflowTool{runner: runner})
	engine := NewEngine(repo, registry)
	if err := engine.runGoDAGLocked(context.Background(), state); err != nil {
		t.Fatal(err)
	}
	got, err := loadState(repo, runID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != statusDone || got.Stage != "done" || got.Stages["review_2"] != "completed" || got.Stages["qa_2"] != "completed" {
		t.Fatalf("legacy final state = %s/%s stages=%#v", got.Status, got.Stage, got.Stages)
	}
	saveRepairEvidenceState(t, evidencePath, got)
}

// repairDAGAcceptanceJSON returns the strict contract exercised by the DAG integration test.
func repairDAGAcceptanceJSON() string {
	return `{
  "summary": "repair DAG integration",
  "coverage": [
    {
      "spec": "需求：QA 失败返回下一 repair / 场景：sealed run 恢复",
      "tests": ["repair-dag-contract"],
      "evidence": ["repair-dag-runtime"],
      "risk": ""
    }
  ],
  "required_tests": [
    {
      "id": "repair-dag-contract",
      "source": "change_contract",
      "path": "docs/changes/1-演示/tests/test_repair_dag.sh",
      "command": "bash docs/changes/1-演示/tests/test_repair_dag.sh",
      "purpose": "prove the resumed repair path executes a real command",
      "assertions": ["the command writes runtime evidence"]
    }
  ],
  "required_evidence": [
    {
      "id": "repair-dag-runtime",
      "kind": "runtime_log",
      "path": "test-results/repair-dag/runtime.log",
      "purpose": "prove the integration command executed"
    }
  ]
}`
}
