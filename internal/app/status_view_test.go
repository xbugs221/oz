// Package app tests the compact status view shared by human status/watch output and JSON observability.
package app

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestBuildStatusViewAggregatesFixStages verifies fix_N rounds are first-class compact rows.
func TestBuildStatusViewAggregatesFixStages(t *testing.T) {
	repo := gitRepo(t)
	state := State{
		RunID:      "run-fix-view",
		ChangeName: "7-统一-状态视图",
		Status:     statusDone,
		Stage:      "done",
		Sessions:   map[string]string{"codex:fixer": "fixer-session"},
		Stages: map[string]string{
			"execution": "completed",
			"review_1":  "completed",
			"fix_1":     "completed",
			"review_2":  "completed",
			"fix_2":     "completed",
			"qa_2":      "completed",
		},
		StageTimings: map[string]StageTiming{
			"fix_1": {StartedAt: "2026-06-09T00:00:00Z", FinishedAt: "2026-06-09T00:03:00Z"},
			"fix_2": {StartedAt: "2026-06-09T00:03:00Z", FinishedAt: "2026-06-09T00:08:00Z"},
		},
		Workflow: DefaultWorkflowConfig(),
	}

	view := buildStatusView(repo, state, "w1", "→")
	row := statusViewRowByName(t, view, "修正阶段")
	if row.Marker != "✓✓" {
		t.Fatalf("fix marker = %q, want two completed rounds", row.Marker)
	}
	if row.SessionID != "fixer-session" {
		t.Fatalf("fix session = %q, want fixer-session", row.SessionID)
	}
	if row.DurationMinutes == nil || *row.DurationMinutes != 8 {
		t.Fatalf("fix duration = %v, want 8 minutes", row.DurationMinutes)
	}
	wantArtifact := filepath.Join(runDir(repo, state.RunID), "fix-2-summary.md")
	if row.Artifacts["stage_artifact"] != wantArtifact {
		t.Fatalf("fix artifact = %q, want %q", row.Artifacts["stage_artifact"], wantArtifact)
	}
	if !strings.Contains(strings.Join(compactStatusLines(view), "\n"), "修正阶段 fixer-session ✓✓ 8.00") {
		t.Fatalf("human status missing compact fix row:\n%s", strings.Join(compactStatusLines(view), "\n"))
	}
}

// TestBuildStatusViewInfersCompletedStagesFromDAGAndTimings verifies resumed sealed runs keep prior progress visible.
func TestBuildStatusViewInfersCompletedStagesFromDAGAndTimings(t *testing.T) {
	repo := gitRepo(t)
	state := State{
		RunID:      "run-resumed-dag-view",
		ChangeName: "7-统一-状态视图",
		Status:     statusRunning,
		Stage:      "qa_1",
		Sessions: map[string]string{
			"codex:executor": "executor-session",
			"codex:reviewer": "reviewer-session",
			"codex:qa":       "qa-session",
		},
		Stages: map[string]string{},
		StageTimings: map[string]StageTiming{
			"review_1": {StartedAt: "2026-06-09T00:04:00Z", FinishedAt: "2026-06-09T00:06:00Z"},
			"qa_1":     {StartedAt: "2026-06-09T00:06:00Z"},
		},
		DAGNodes: map[string]DAGNodeState{
			"execution": {Status: "success", StartedAt: "2026-06-09T00:00:00Z", FinishedAt: "2026-06-09T00:04:00Z"},
			"review_1":  {Status: "success", StartedAt: "2026-06-09T00:04:00Z", FinishedAt: "2026-06-09T00:06:00Z"},
			"qa_1":      {Status: statusRunning, StartedAt: "2026-06-09T00:06:00Z"},
		},
		Workflow: DefaultWorkflowConfig(),
	}

	view := buildHumanStatusView(repo, state, "w1", "|")
	execution := statusViewRowByName(t, view, "执行阶段")
	if execution.Marker != "✓" {
		t.Fatalf("execution marker = %q, want completed from DAG node", execution.Marker)
	}
	review := statusViewRowByName(t, view, "审核阶段")
	if review.Marker != "✓" {
		t.Fatalf("review marker = %q, want completed from DAG node or finished timing", review.Marker)
	}
	qa := statusViewRowByName(t, view, "测试阶段")
	if qa.Marker != "|" {
		t.Fatalf("qa marker = %q, want spinner for running DAG node", qa.Marker)
	}
}

// TestBuildStatusViewReadsReviewAndQADAGSubagentNodes verifies status uses graph.go visual node ids.
func TestBuildStatusViewReadsReviewAndQADAGSubagentNodes(t *testing.T) {
	repo := gitRepo(t)
	workflow := DefaultWorkflowConfig()
	workflow.Parallel.Enabled = true
	workflow.Parallel.Groups = map[string]ParallelGroupConfig{
		"review": {
			Mode: "gate_input",
			Members: []ParallelMemberConfig{
				{Name: "目标核对审核员", Purpose: "核对目标"},
				{Name: "安全风险审核员", Purpose: "检查风险"},
			},
		},
		"qa": {
			Mode: "gate_input",
			Members: []ParallelMemberConfig{
				{Name: "CLI/API 测试员", Purpose: "跑 CLI"},
			},
		},
	}
	state := State{
		RunID:      "run-subagent-view",
		ChangeName: "7-统一-状态视图",
		Status:     statusRunning,
		Stage:      "qa_1",
		Sessions: map[string]string{
			"pi:subagent:review:目标核对审核员:1": "review-target-session",
			"pi:subagent:qa:CLI/API 测试员:1": "qa-cli-session",
		},
		Stages: map[string]string{"execution": "completed", "review_1": "completed"},
		DAGNodes: map[string]DAGNodeState{
			"before_review_1_1": {
				Status:     "success",
				StartedAt:  "2026-06-09T00:00:00Z",
				FinishedAt: "2026-06-09T00:02:00Z",
			},
			"before_review_1_2": {
				Status: "running",
			},
			"before_qa_1_1": {
				Status: "running",
			},
		},
		Workflow: workflow,
	}

	view := buildStatusView(repo, state, "w1", "→")
	target := statusViewRowByFullName(t, view, "目标核对审核员")
	if target.Marker != "✓" || target.DurationMinutes == nil || *target.DurationMinutes != 2 {
		t.Fatalf("review target row = %#v, want completed row with DAG duration", target)
	}
	risk := statusViewRowByFullName(t, view, "安全风险审核员")
	if risk.Marker != "→" {
		t.Fatalf("review risk marker = %q, want running from before_review node", risk.Marker)
	}
	cli := statusViewRowByFullName(t, view, "CLI/API 测试员")
	if cli.Marker != "→" || cli.SessionID != "qa-cli-session" {
		t.Fatalf("qa CLI row = %#v, want running row with session", cli)
	}
}

// TestBuildHumanStatusViewReplacesRunningMarkersInsideRows verifies watch spinners update every active indicator.
func TestBuildHumanStatusViewReplacesRunningMarkersInsideRows(t *testing.T) {
	repo := gitRepo(t)
	workflow := DefaultWorkflowConfig()
	workflow.Parallel.Enabled = true
	workflow.Parallel.Groups = map[string]ParallelGroupConfig{
		"review": {
			Mode: "gate_input",
			Members: []ParallelMemberConfig{
				{Name: "目标核对审核员", Purpose: "核对目标"},
			},
		},
	}
	state := State{
		RunID:      "run-review-spin",
		ChangeName: "7-统一-状态视图",
		Status:     statusRunning,
		Stage:      "review_2",
		Sessions: map[string]string{
			"codex:reviewer": "reviewer-session",
			"pi:subagent:review:目标核对审核员:2": "review-target-session",
		},
		Stages: map[string]string{"execution": "completed", "review_1": "completed"},
		DAGNodes: map[string]DAGNodeState{
			"review_2":          {Status: statusRunning},
			"before_review_2_1": {Status: statusRunning},
		},
		Workflow: workflow,
	}

	view := buildHumanStatusView(repo, state, "w1", "|")
	review := statusViewRowByName(t, view, "审核阶段")
	if review.Marker != "✓|" {
		t.Fatalf("review marker = %q, want spinner inside multi-round marker", review.Marker)
	}
	target := statusViewRowByFullName(t, view, "目标核对审核员")
	if target.Marker != "|" {
		t.Fatalf("subagent marker = %q, want spinner", target.Marker)
	}
	if strings.Contains(strings.Join(compactStatusLines(view), "\n"), "→") {
		t.Fatalf("watch compact view should not contain static arrow:\n%s", strings.Join(compactStatusLines(view), "\n"))
	}
}

// TestBuildStatusViewMarksRunningIteratedStages verifies active rounds are not hidden by prior completions.
func TestBuildStatusViewMarksRunningIteratedStages(t *testing.T) {
	repo := gitRepo(t)
	cases := []struct {
		name         string
		stage        string
		rowName      string
		sessionKey   string
		sessionValue string
		previous     string
		artifact     string
	}{
		{
			name:         "review",
			stage:        "review_2",
			rowName:      "审核阶段",
			sessionKey:   "codex:reviewer",
			sessionValue: "reviewer-session",
			previous:     "review_1",
			artifact:     "review-2.json",
		},
		{
			name:         "qa",
			stage:        "qa_2",
			rowName:      "测试阶段",
			sessionKey:   "codex:qa",
			sessionValue: "qa-session",
			previous:     "qa_1",
			artifact:     "qa-2.json",
		},
		{
			name:         "fix",
			stage:        "fix_2",
			rowName:      "修正阶段",
			sessionKey:   "codex:fixer",
			sessionValue: "fixer-session",
			previous:     "fix_1",
			artifact:     "fix-2-summary.md",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			state := State{
				RunID:      "run-" + tc.name + "-running",
				ChangeName: "7-统一-状态视图",
				Status:     statusRunning,
				Stage:      tc.stage,
				Sessions:   map[string]string{tc.sessionKey: tc.sessionValue},
				Stages: map[string]string{
					"execution": "completed",
					tc.previous: "completed",
					tc.stage:    "running",
				},
				StageTimings: map[string]StageTiming{
					tc.previous: {StartedAt: "2026-06-09T00:00:00Z", FinishedAt: "2026-06-09T00:02:00Z"},
					tc.stage:    {StartedAt: "2026-06-09T00:02:00Z"},
				},
				Workflow: DefaultWorkflowConfig(),
			}

			view := buildStatusView(repo, state, "w1", "→")
			row := statusViewRowByName(t, view, tc.rowName)
			if row.Marker != "✓→" {
				t.Fatalf("%s marker = %q, want historical completion plus running marker", tc.rowName, row.Marker)
			}
			if row.SessionID != tc.sessionValue {
				t.Fatalf("%s session = %q, want %q", tc.rowName, row.SessionID, tc.sessionValue)
			}
			if !strings.HasSuffix(row.Artifacts["stage_artifact"], tc.artifact) {
				t.Fatalf("%s artifact = %q, want suffix %q", tc.rowName, row.Artifacts["stage_artifact"], tc.artifact)
			}
			if !strings.Contains(strings.Join(compactStatusLines(view), "\n"), tc.rowName+" "+tc.sessionValue+" ✓→") {
				t.Fatalf("human status missing running %s row:\n%s", tc.rowName, strings.Join(compactStatusLines(view), "\n"))
			}
		})
	}
}

// TestBuildStatusViewMarksFailedIteratedStages verifies failed rounds are not hidden by prior completions.
func TestBuildStatusViewMarksFailedIteratedStages(t *testing.T) {
	repo := gitRepo(t)
	cases := []struct {
		name         string
		stage        string
		rowName      string
		sessionKey   string
		sessionValue string
		previous     string
		artifact     string
	}{
		{
			name:         "review",
			stage:        "review_2",
			rowName:      "审核阶段",
			sessionKey:   "codex:reviewer",
			sessionValue: "reviewer-session",
			previous:     "review_1",
			artifact:     "review-2.json",
		},
		{
			name:         "qa",
			stage:        "qa_2",
			rowName:      "测试阶段",
			sessionKey:   "codex:qa",
			sessionValue: "qa-session",
			previous:     "qa_1",
			artifact:     "qa-2.json",
		},
		{
			name:         "fix",
			stage:        "fix_2",
			rowName:      "修正阶段",
			sessionKey:   "codex:fixer",
			sessionValue: "fixer-session",
			previous:     "fix_1",
			artifact:     "fix-2-summary.md",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			state := State{
				RunID:      "run-" + tc.name + "-failed",
				ChangeName: "7-统一-状态视图",
				Status:     statusFailed,
				Stage:      tc.stage,
				Sessions:   map[string]string{tc.sessionKey: tc.sessionValue},
				Stages: map[string]string{
					"execution": "completed",
					tc.previous: "completed",
				},
				StageTimings: map[string]StageTiming{
					tc.previous: {StartedAt: "2026-06-09T00:00:00Z", FinishedAt: "2026-06-09T00:02:00Z"},
					tc.stage:    {StartedAt: "2026-06-09T00:02:00Z", FinishedAt: "2026-06-09T00:03:00Z"},
				},
				Workflow: DefaultWorkflowConfig(),
			}

			view := buildStatusView(repo, state, "w1", "→")
			row := statusViewRowByName(t, view, tc.rowName)
			if row.Marker != "✓x" {
				t.Fatalf("%s marker = %q, want historical completion plus failed marker", tc.rowName, row.Marker)
			}
			if row.SessionID != tc.sessionValue {
				t.Fatalf("%s session = %q, want %q", tc.rowName, row.SessionID, tc.sessionValue)
			}
			if !strings.HasSuffix(row.Artifacts["stage_artifact"], tc.artifact) {
				t.Fatalf("%s artifact = %q, want suffix %q", tc.rowName, row.Artifacts["stage_artifact"], tc.artifact)
			}
			if !strings.Contains(strings.Join(compactStatusLines(view), "\n"), tc.rowName+" "+tc.sessionValue+" ✓x") {
				t.Fatalf("human status missing failed %s row:\n%s", tc.rowName, strings.Join(compactStatusLines(view), "\n"))
			}
		})
	}
}

// statusViewRowByName finds a compact row by visible stage name.
func statusViewRowByName(t *testing.T, view statusView, name string) statusViewRow {
	t.Helper()
	for _, row := range view.Rows {
		if row.Name == name {
			return row
		}
	}
	t.Fatalf("row %q not found in %#v", name, view.Rows)
	return statusViewRow{}
}

// statusViewRowByFullName finds a compact row by configured stage or helper name.
func statusViewRowByFullName(t *testing.T, view statusView, fullName string) statusViewRow {
	t.Helper()
	for _, row := range view.Rows {
		if row.FullName == fullName {
			return row
		}
	}
	t.Fatalf("row %q not found in %#v", fullName, view.Rows)
	return statusViewRow{}
}
