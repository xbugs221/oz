// Package app tests repair-loop graph exports and executable dependency boundaries.
package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRepairWorkflowSpecIncludesNeedsMoreDecision verifies business edges do not alter DAG ordering.
func TestRepairWorkflowSpecIncludesNeedsMoreDecision(t *testing.T) {
	workflow := legacyRepairGraphWorkflow()
	workflow.MaxRepairIterations = 2
	workflow.MaxReviewIterations = 0
	spec := BuildWorkflowSpec("1-演示", workflow)

	found := false
	for _, edge := range spec.Edges {
		if edge.From == "gate_repair_1" && edge.To == "repair_2" && edge.Label == "repair needs_more / first clean" {
			found = true
			if !edge.DecisionOnly {
				t.Fatal("repair needs_more must be descriptive without becoming a scheduler dependency")
			}
		}
	}
	if !found {
		t.Fatal("workflow graph is missing repair needs_more → next repair")
	}

	index := map[string]int{}
	for i, node := range goDAGOrder(spec) {
		index[node.ID] = i
	}
	if index["gate_qa_1"] < index["repair_2"] {
		return
	}
	t.Fatalf("repair_2 order=%d must follow gate_qa_1 order=%d", index["repair_2"], index["gate_qa_1"])
}

// TestRepairWorkflowSpecIncludesFinalBlockedDecisions verifies both last-round failures terminate visibly.
func TestRepairWorkflowSpecIncludesFinalBlockedDecisions(t *testing.T) {
	workflow := legacyRepairGraphWorkflow()
	workflow.MaxRepairIterations = 2
	workflow.MaxReviewIterations = 0
	spec := BuildWorkflowSpec("1-演示", workflow)

	blockedNode := false
	wants := map[string]bool{
		"gate_repair_2|repair needs_more / first clean": false,
		"gate_qa_2|QA needs_fix":                        false,
	}
	for _, node := range spec.Nodes {
		if node.ID == statusBlocked && node.DecisionOnly {
			blockedNode = true
		}
	}
	for _, edge := range spec.Edges {
		key := edge.From + "|" + edge.Label
		if _, ok := wants[key]; ok && edge.To == statusBlocked && edge.DecisionOnly {
			wants[key] = true
		}
	}
	if !blockedNode {
		t.Fatal("positive repair graph is missing its display-only blocked terminal")
	}
	for transition, found := range wants {
		if !found {
			t.Fatalf("positive repair graph is missing final blocked transition %s", transition)
		}
	}
	for _, node := range goDAGOrder(spec) {
		if node.ID == statusBlocked {
			t.Fatal("display-only blocked terminal must not enter executable DAG order")
		}
	}
}

// TestRepairWorkflowSpecIncludesEarlyQACleanDecisions verifies every successful QA can archive immediately.
func TestRepairWorkflowSpecIncludesEarlyQACleanDecisions(t *testing.T) {
	workflow := legacyRepairGraphWorkflow()
	workflow.MaxRepairIterations = 3
	workflow.MaxReviewIterations = 0
	spec := BuildWorkflowSpec("1-演示", workflow)

	for iteration := 1; iteration < workflow.MaxRepairIterations; iteration++ {
		from := fmt.Sprintf("gate_qa_%d", iteration)
		found := false
		for _, edge := range spec.Edges {
			if edge.From == from && edge.To == "gate_archive" && edge.Label == "QA clean" && edge.DecisionOnly {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("positive repair graph is missing early QA clean transition from %s", from)
		}
	}
}

// TestZeroRepairWorkflowSpecIncludesQABlockedDecision verifies failed independent QA terminates visibly.
func TestZeroRepairWorkflowSpecIncludesQABlockedDecision(t *testing.T) {
	workflow := legacyRepairGraphWorkflow()
	workflow.MaxRepairIterations = 0
	workflow.MaxReviewIterations = 0
	spec := BuildWorkflowSpec("1-演示", workflow)

	blockedNode := false
	blockedEdge := false
	for _, node := range spec.Nodes {
		if node.ID == statusBlocked && node.DecisionOnly {
			blockedNode = true
		}
	}
	for _, edge := range spec.Edges {
		if edge.From == "gate_qa_1" && edge.To == statusBlocked && edge.Label == "QA needs_fix" && edge.DecisionOnly {
			blockedEdge = true
		}
	}
	if !blockedNode || !blockedEdge {
		t.Fatalf("zero-repair graph must expose QA needs_fix block: node=%t edge=%t", blockedNode, blockedEdge)
	}
}

// TestPositiveRepairCompactMermaidShowsFinalBlock verifies compact output separates retries from exhaustion.
func TestPositiveRepairCompactMermaidShowsFinalBlock(t *testing.T) {
	workflow := legacyRepairGraphWorkflow()
	workflow.MaxRepairIterations = 2
	workflow.MaxReviewIterations = 0
	graph := buildCompactMermaid("1-演示", workflow)

	for _, want := range []string{
		"blocked[阻塞]",
		"repair -->|确认 clean| qa",
		"repair -->|needs_more 或首次 clean，未达第2轮| repair",
		"repair -->|needs_more 或首次 clean，第2轮| blocked",
		"qa -->|needs_fix，未达第2轮| repair",
		"qa -->|needs_fix，第2轮| blocked",
	} {
		if !strings.Contains(graph, want) {
			t.Fatalf("positive repair graph missing %q:\n%s", want, graph)
		}
	}
}

// TestZeroRepairCompactMermaidMatchesStateMachine verifies zero repair goes directly to independent QA.
func TestZeroRepairCompactMermaidMatchesStateMachine(t *testing.T) {
	workflow := legacyRepairGraphWorkflow()
	workflow.MaxRepairIterations = 0
	workflow.MaxReviewIterations = 0
	graph := buildCompactMermaid("1-演示", workflow)

	for _, want := range []string{"execution --> qa", "qa -->|clean| archive", "qa -->|needs_fix，无修正轮次| blocked"} {
		if !strings.Contains(graph, want) {
			t.Fatalf("zero-repair graph missing %q:\n%s", want, graph)
		}
	}
	if strings.Contains(graph, "repair[") || strings.Contains(graph, "execution --> repair") {
		t.Fatalf("zero-repair graph must not render repair stage:\n%s", graph)
	}
}

// TestQualityLoopWorkflowSpecShowsUnboundedTemplate verifies graph JSON has no fixed repair budget.
func TestQualityLoopWorkflowSpecShowsUnboundedTemplate(t *testing.T) {
	workflow := DefaultWorkflowConfig()
	workflow.Generation = qualityLoopWorkflowGeneration
	workflow.MaxRepairIterations = 2
	spec := BuildWorkflowSpec("45-质量闭环", workflow)

	nodes := map[string]WorkflowNode{}
	for _, node := range spec.Nodes {
		nodes[node.ID] = node
	}
	for id, mode := range map[string]string{
		"audit_N":           "pre_qa_audit",
		"targeted_repair_N": "qa_targeted_repair",
	} {
		node, ok := nodes[id]
		if !ok || node.Type != "loop_template" || node.Mode != mode {
			t.Fatalf("quality-loop node %s = %#v, want loop template mode %s", id, node, mode)
		}
	}
	for _, id := range []string{"qa_N", "blocked_environment", "blocked_stalled", "archive"} {
		if _, ok := nodes[id]; !ok {
			t.Fatalf("quality-loop graph missing node %s", id)
		}
	}
	for _, id := range []string{statusBlockedEnvironment, statusBlockedStalled} {
		if nodes[id].Type != "pause" {
			t.Fatalf("recoverable block %s type = %q, want pause", id, nodes[id].Type)
		}
		for _, target := range []string{"audit_N", "targeted_repair_N"} {
			if !workflowSpecHasEdge(spec, id, target, "resume/restart → blocked_from_stage") {
				t.Fatalf("recoverable block %s missing resume edge to %s", id, target)
			}
		}
	}
	for _, forbidden := range []string{"repair_1", "repair_2", statusBlocked} {
		if _, ok := nodes[forbidden]; ok {
			t.Fatalf("quality-loop graph must not contain finite legacy node %s", forbidden)
		}
	}
}

// TestQualityLoopCompactMermaidShowsQualityDecisions verifies human graph explains both loops and pause states.
func TestQualityLoopCompactMermaidShowsQualityDecisions(t *testing.T) {
	workflow := DefaultWorkflowConfig()
	workflow.Generation = qualityLoopWorkflowGeneration
	graph := buildCompactMermaid("45-质量闭环", workflow)

	for _, want := range []string{
		"audit[全量自查 audit_N]",
		"targeted_repair[定向修复 targeted_repair_N]",
		"audit -->|clean 且自测通过| qa",
		"qa -->|needs_fix| targeted_repair",
		"targeted_repair -->|失败测试与 required tests 通过| qa",
		"blocked_environment[环境阻塞]",
		"blocked_stalled[停滞阻塞]",
		"blocked_environment -.->|resume/restart 回原阶段| audit",
		"blocked_stalled -.->|resume/restart 回原阶段| targeted_repair",
	} {
		if !strings.Contains(graph, want) {
			t.Fatalf("quality-loop graph missing %q:\n%s", want, graph)
		}
	}
	if strings.Contains(graph, "最多") || strings.Contains(graph, "第5轮") {
		t.Fatalf("quality-loop graph must not expose a fixed iteration limit:\n%s", graph)
	}
}

// TestQualityLoopGraphAppendsDurableInstances verifies graph output remains useful after twelve repairs.
func TestQualityLoopGraphAppendsDurableInstances(t *testing.T) {
	workflow := DefaultWorkflowConfig()
	workflow.Generation = qualityLoopWorkflowGeneration
	state := State{
		RunID:      "20260727T120000.000000000Z",
		ChangeName: "45-质量闭环",
		Sealed:     true,
		Status:     statusRunning,
		Stage:      "qa_13",
		Workflow:   workflow,
		Stages:     map[string]string{"execution": "completed", "audit_1": "completed"},
		DAGNodes:   map[string]DAGNodeState{},
	}
	for iteration := 1; iteration <= 12; iteration++ {
		state.Stages[fmt.Sprintf("qa_%d", iteration)] = "completed"
		state.Stages[fmt.Sprintf("targeted_repair_%d", iteration)] = "completed"
	}
	state.Stages["qa_13"] = statusRunning

	spec := appendQualityLoopGraphInstances(BuildWorkflowSpec(state.ChangeName, workflow), state)
	if spec.RunID != state.RunID || spec.CurrentStage != "qa_13" {
		t.Fatalf("instance metadata = run %q stage %q", spec.RunID, spec.CurrentStage)
	}
	nodes := map[string]WorkflowNode{}
	for _, node := range spec.Nodes {
		nodes[node.ID] = node
	}
	for _, id := range []string{"audit_N", "qa_N", "targeted_repair_N", "audit_1", "qa_13", "targeted_repair_12"} {
		if _, ok := nodes[id]; !ok {
			t.Fatalf("quality-loop graph missing template or durable instance %s", id)
		}
	}
	if nodes["qa_13"].Type != "loop_instance" || nodes["qa_13"].Status != statusRunning {
		t.Fatalf("qa_13 node = %#v", nodes["qa_13"])
	}
	if !workflowSpecHasEdge(spec, "qa_12", "targeted_repair_12", "durable instance") ||
		!workflowSpecHasEdge(spec, "targeted_repair_12", "qa_13", "durable instance") {
		t.Fatalf("dynamic graph does not preserve QA → targeted repair → QA order")
	}
	graph := appendQualityLoopMermaidInstances(buildCompactMermaid(state.ChangeName, workflow), state)
	for _, want := range []string{"current_instances[当前运行实例]", "execution -.-> audit_1", "targeted_repair_12", "qa_13"} {
		if !strings.Contains(graph, want) {
			t.Fatalf("dynamic Mermaid graph missing %q:\n%s", want, graph)
		}
	}
}

// TestQualityLoopGraphExecutionOnlyCarriesRunMetadata verifies a sealed run is visible before audit starts.
func TestQualityLoopGraphExecutionOnlyCarriesRunMetadata(t *testing.T) {
	workflow := DefaultWorkflowConfig()
	state := State{
		RunID:      "20260727T120000.000000000Z",
		ChangeName: "45-质量闭环",
		Sealed:     true,
		Status:     statusRunning,
		Stage:      "execution",
		Workflow:   workflow,
		Stages:     map[string]string{"execution": statusRunning},
	}

	spec := appendQualityLoopGraphInstances(BuildWorkflowSpec(state.ChangeName, workflow), state)
	if spec.RunID != state.RunID || spec.RunStatus != statusRunning || spec.CurrentStage != "execution" {
		t.Fatalf("execution-only metadata = %#v", spec)
	}
	for _, node := range spec.Nodes {
		if node.ID == "execution" && node.Status == statusRunning {
			return
		}
	}
	t.Fatalf("execution-only graph does not mark execution running: %#v", spec.Nodes)
}

// TestQualityLoopGraphResumesExactBlockedInstance verifies a pause points back to BlockedFromStage.
func TestQualityLoopGraphResumesExactBlockedInstance(t *testing.T) {
	workflow := DefaultWorkflowConfig()
	workflow.Generation = qualityLoopWorkflowGeneration
	state := State{
		RunID:      "20260727T120001.000000000Z",
		ChangeName: "45-质量闭环",
		Sealed:     true,
		Status:     statusBlockedEnvironment,
		Stage:      statusBlockedEnvironment,
		Workflow:   workflow,
		Stages: map[string]string{
			"audit_1":           "completed",
			"qa_1":              "completed",
			"targeted_repair_1": statusBlockedEnvironment,
		},
		QualityLoop: QualityLoopState{BlockedFromStage: "targeted_repair_1"},
	}

	spec := appendQualityLoopGraphInstances(BuildWorkflowSpec(state.ChangeName, workflow), state)
	if !workflowSpecHasEdge(spec, statusBlockedEnvironment, "targeted_repair_1", "resume/restart → blocked_from_stage") {
		t.Fatal("blocked graph missing exact resume target")
	}
	graph := appendQualityLoopMermaidInstances(buildCompactMermaid(state.ChangeName, workflow), state)
	if !strings.Contains(graph, "blocked_environment -.->|resume/restart 回 blocked_from_stage| targeted_repair_1") {
		t.Fatalf("blocked Mermaid graph missing exact resume edge:\n%s", graph)
	}
}

// TestQualityLoopGraphUsesDurableStartOrder preserves the real QA-to-audit execution sequence.
func TestQualityLoopGraphUsesDurableStartOrder(t *testing.T) {
	workflow := DefaultWorkflowConfig()
	workflow.Generation = qualityLoopWorkflowGeneration
	state := State{
		ChangeName: "45-质量闭环",
		Status:     statusRunning,
		Stage:      "audit_2",
		Workflow:   workflow,
		Stages: map[string]string{
			"audit_1": "completed",
			"qa_1":    "completed",
			"audit_2": statusRunning,
		},
		StageTimings: map[string]StageTiming{
			"audit_1": {StartedAt: "2026-07-27T09:00:00Z", FinishedAt: "2026-07-27T09:01:00Z"},
			"qa_1":    {StartedAt: "2026-07-27T09:02:00Z", FinishedAt: "2026-07-27T09:03:00Z"},
			"audit_2": {StartedAt: "2026-07-27T09:04:00Z"},
		},
	}

	spec := appendQualityLoopGraphInstances(BuildWorkflowSpec(state.ChangeName, workflow), state)
	if !workflowSpecHasEdge(spec, "audit_1", "qa_1", "durable instance") ||
		!workflowSpecHasEdge(spec, "qa_1", "audit_2", "durable instance") ||
		workflowSpecHasEdge(spec, "audit_2", "qa_1", "durable instance") {
		t.Fatalf("JSON graph did not preserve durable stage timing: %#v", spec.Edges)
	}
	graph := appendQualityLoopMermaidInstances(buildCompactMermaid(state.ChangeName, workflow), state)
	if !strings.Contains(graph, "audit_1 -.-> qa_1\n  qa_1 -.-> audit_2") {
		t.Fatalf("Mermaid graph did not preserve durable stage timing:\n%s", graph)
	}
}

// TestLatestSealedGraphStateSelectsMatchingChange verifies unrelated and unsealed runs are ignored.
func TestLatestSealedGraphStateSelectsMatchingChange(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	repo := t.TempDir()
	workflow := DefaultWorkflowConfig()
	for _, state := range []State{
		{RunID: "20260727T120000.000000000Z", ChangeName: "45-质量闭环", Sealed: true, Workflow: workflow},
		{RunID: "20260727T120001.000000000Z", ChangeName: "other", Sealed: true, Workflow: workflow},
		{RunID: "20260727T120002.000000000Z", ChangeName: "45-质量闭环", Sealed: false, Workflow: workflow},
	} {
		if err := saveState(repo, state); err != nil {
			t.Fatal(err)
		}
	}
	state, err := latestSealedGraphState(repo, "45-质量闭环")
	if err != nil {
		t.Fatal(err)
	}
	if state == nil || state.RunID != "20260727T120000.000000000Z" {
		t.Fatalf("latest sealed matching state = %#v", state)
	}
}

// TestRunGraphUsesLegacySealedWorkflow verifies current config cannot migrate an old run's graph.
func TestRunGraphUsesLegacySealedWorkflow(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
	repo := t.TempDir()
	workflow := legacyRepairGraphWorkflow()
	workflow.MaxRepairIterations = 2
	state := State{
		RunID:      "20260727T120000.000000000Z",
		ChangeName: "45-质量闭环",
		Sealed:     true,
		Status:     statusRunning,
		Stage:      "repair_2",
		Workflow:   workflow,
		Stages:     map[string]string{"execution": "completed", "repair_1": "completed", "repair_2": statusRunning},
	}
	if err := saveState(repo, state); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "oz-flow.yaml"), []byte("unknown_active_config: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout strings.Builder
	if err := runGraph(repo, []string{"--change", state.ChangeName, "--format", "json"}, &stdout); err != nil {
		t.Fatal(err)
	}
	var spec WorkflowSpec
	if err := json.Unmarshal([]byte(stdout.String()), &spec); err != nil {
		t.Fatal(err)
	}
	nodes := map[string]bool{}
	for _, node := range spec.Nodes {
		nodes[node.ID] = true
	}
	if spec.RunID != state.RunID || spec.CurrentStage != "repair_2" || !nodes["repair_2"] {
		t.Fatalf("legacy sealed graph = %#v", spec)
	}
	if nodes["audit_N"] || nodes["targeted_repair_N"] {
		t.Fatalf("legacy sealed graph was migrated to current quality loop: %#v", spec.Nodes)
	}

	stdout.Reset()
	if err := runGraph(repo, []string{"--change", state.ChangeName, "--format", "mermaid"}, &stdout); err != nil {
		t.Fatal(err)
	}
	if graph := stdout.String(); !strings.Contains(graph, "repair[优化]") || strings.Contains(graph, "audit[全量自查") {
		t.Fatalf("legacy sealed Mermaid used active config:\n%s", graph)
	}
}

// workflowSpecHasEdge reports whether a graph includes one exact labeled transition.
func workflowSpecHasEdge(spec WorkflowSpec, from, to, label string) bool {
	for _, edge := range spec.Edges {
		if edge.From == from && edge.To == to && edge.Label == label {
			return true
		}
	}
	return false
}

// legacyRepairGraphWorkflow returns an explicit sealed repair-v1 snapshot for compatibility tests.
func legacyRepairGraphWorkflow() WorkflowConfig {
	workflow := DefaultWorkflowConfig()
	workflow.Generation = repairWorkflowGeneration
	return workflow
}
