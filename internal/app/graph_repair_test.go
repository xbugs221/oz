// Package app tests repair-loop graph exports and executable dependency boundaries.
package app

import (
	"fmt"
	"strings"
	"testing"
)

// TestRepairWorkflowSpecIncludesNeedsMoreDecision verifies business edges do not alter DAG ordering.
func TestRepairWorkflowSpecIncludesNeedsMoreDecision(t *testing.T) {
	workflow := DefaultWorkflowConfig()
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
	workflow := DefaultWorkflowConfig()
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
	workflow := DefaultWorkflowConfig()
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
	workflow := DefaultWorkflowConfig()
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
	workflow := DefaultWorkflowConfig()
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
	workflow := DefaultWorkflowConfig()
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
