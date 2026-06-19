// Package app exports read-only workflow graphs for JSON, Mermaid, and tests.
package app

import (
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"
)

// WorkflowSpec is the stable graph representation behind every graph exporter.
type WorkflowSpec struct {
	ChangeName string             `json:"change_name" yaml:"change_name"`
	Nodes      []WorkflowNode     `json:"nodes" yaml:"nodes"`
	Edges      []WorkflowEdge     `json:"edges" yaml:"edges"`
	Artifacts  []WorkflowArtifact `json:"artifacts" yaml:"artifacts"`
	Gates      []WorkflowGate     `json:"gates" yaml:"gates"`
	Display    WorkflowDisplay    `json:"display" yaml:"display"`
}

// WorkflowNode describes one user-visible graph step.
type WorkflowNode struct {
	ID        string `json:"id" yaml:"id"`
	Name      string `json:"name" yaml:"name"`
	Type      string `json:"type" yaml:"type"`
	Group     string `json:"group,omitempty" yaml:"group,omitempty"`
	Stage     string `json:"stage,omitempty" yaml:"stage,omitempty"`
	RunStage  string `json:"run_stage,omitempty" yaml:"run_stage,omitempty"`
	Member    string `json:"member,omitempty" yaml:"member,omitempty"`
	Mode      string `json:"mode,omitempty" yaml:"mode,omitempty"`
	Iteration int    `json:"iteration,omitempty" yaml:"iteration,omitempty"`
}

// WorkflowEdge records execution or decision ordering between graph nodes.
type WorkflowEdge struct {
	From  string `json:"from" yaml:"from"`
	To    string `json:"to" yaml:"to"`
	Label string `json:"label,omitempty" yaml:"label,omitempty"`
}

// WorkflowArtifact records files produced by fan-in steps.
type WorkflowArtifact struct {
	ID     string `json:"id" yaml:"id"`
	Path   string `json:"path" yaml:"path"`
	NodeID string `json:"node_id" yaml:"node_id"`
}

// WorkflowGate documents business gates represented in the graph.
type WorkflowGate struct {
	ID        string `json:"id" yaml:"id"`
	Name      string `json:"name" yaml:"name"`
	Stage     string `json:"stage,omitempty" yaml:"stage,omitempty"`
	Iteration int    `json:"iteration,omitempty" yaml:"iteration,omitempty"`
}

// WorkflowDisplay carries human-facing graph metadata.
type WorkflowDisplay struct {
	Title string `json:"title" yaml:"title"`
}

// runGraph loads effective workflow config and writes one graph representation.
func runGraph(repo string, args []string, stdout io.Writer) error {
	changeName, err := requireFlagValue(args, "--change")
	if err != nil {
		return fmt.Errorf("用法：oz flow graph --change <change-name> --format json|mermaid")
	}
	format, err := requireFlagValue(args, "--format")
	if err != nil {
		return fmt.Errorf("用法：oz flow graph --change <change-name> --format json|mermaid")
	}
	workflow, err := LoadWorkflowConfig(repo)
	if err != nil {
		return err
	}
	spec := BuildWorkflowSpec(changeName, workflow)
	switch format {
	case "json":
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(spec)
	case "mermaid":
		_, err := fmt.Fprint(stdout, buildCompactMermaid(changeName, workflow))
		return err
	default:
		return fmt.Errorf("未知 graph format %q，可选 json、mermaid", format)
	}
}

// BuildWorkflowSpec expands the workflow config into a finite OmO DAG.
func BuildWorkflowSpec(changeName string, workflow WorkflowConfig) WorkflowSpec {
	normalizeWorkflowConfig(&workflow)
	spec := WorkflowSpec{
		ChangeName: changeName,
		Nodes:      []WorkflowNode{},
		Edges:      []WorkflowEdge{},
		Artifacts:  []WorkflowArtifact{},
		Gates:      []WorkflowGate{},
		Display:    WorkflowDisplay{Title: "oz flow workflow: " + changeName},
	}
	spec.addNode(WorkflowNode{ID: "execution", Name: "execution", Type: "main_stage", Stage: "execution"})
	previous := "execution"
	for i := 1; i <= workflow.MaxReviewIterations; i++ {
		review := fmt.Sprintf("review_%d", i)
		qa := fmt.Sprintf("qa_%d", i)
		fix := fmt.Sprintf("fix_%d", i)
		spec.addNode(WorkflowNode{ID: review, Name: review, Type: "main_stage", Stage: review, Iteration: i})
		spec.addEdge(previous, review, "")
		reviewGate := fmt.Sprintf("gate_review_%d", i)
		spec.addGate(reviewGate, "review gate", review, i)
		spec.addEdge(review, reviewGate, "")
		spec.addNode(WorkflowNode{ID: qa, Name: qa, Type: "main_stage", Stage: qa, Iteration: i})
		spec.addEdge(reviewGate, qa, "review clean")
		qaGate := fmt.Sprintf("gate_qa_%d", i)
		spec.addGate(qaGate, "QA gate", qa, i)
		spec.addEdge(qa, qaGate, "")
		spec.addNode(WorkflowNode{ID: fix, Name: fix, Type: "main_stage", Stage: fix, Iteration: i})
		spec.addEdge(reviewGate, fix, "review needs_fix")
		spec.addEdge(qaGate, fix, "QA needs_fix")
		previous = fix
	}
	spec.addNode(WorkflowNode{ID: "archive", Name: "archive", Type: "main_stage", Stage: "archive"})
	archiveGate := "gate_archive"
	spec.addGate(archiveGate, "archive gate", "archive", 0)
	if workflow.MaxReviewIterations == 0 {
		spec.addEdge("execution", archiveGate, "")
	} else {
		spec.addEdge(fmt.Sprintf("gate_qa_%d", workflow.MaxReviewIterations), archiveGate, "QA clean")
	}
	spec.addEdge(archiveGate, "archive", "")
	return spec
}

// buildCompactMermaid renders a compact Chinese state-machine graph for human review.
func buildCompactMermaid(changeName string, workflow WorkflowConfig) string {
	var out strings.Builder
	out.WriteString("flowchart TD\n")
	out.WriteString("  execution[执行]\n")

	out.WriteString("  review[审核]\n")
	out.WriteString("  qa[测试]\n")
	out.WriteString("  fix[修复]\n")
	out.WriteString("  archive[归档]\n")

	out.WriteString("  execution --> review\n")
	out.WriteString("  review --> qa\n")
	out.WriteString("  qa --> fix\n")
	fmt.Fprintf(&out, "  fix -->|最多%d轮| review\n", workflow.MaxReviewIterations)
	out.WriteString("  qa --> archive\n")

	return out.String()
}

func (spec *WorkflowSpec) addGate(id, name, stage string, iteration int) {
	spec.addNode(WorkflowNode{ID: id, Name: name, Type: "gate", Stage: stage, Iteration: iteration})
	spec.Gates = append(spec.Gates, WorkflowGate{ID: id, Name: name, Stage: stage, Iteration: iteration})
}

func (spec *WorkflowSpec) addNode(node WorkflowNode) {
	spec.Nodes = append(spec.Nodes, node)
}

func (spec *WorkflowSpec) addEdge(from, to, label string) {
	spec.Edges = append(spec.Edges, WorkflowEdge{From: from, To: to, Label: label})
}

func slug(text string) string {
	slug := regexp.MustCompile(`[^A-Za-z0-9_-]+`).ReplaceAllString(text, "-")
	slug = strings.Trim(slug, "-")
	if slug == "" {
		return "workflow"
	}
	return slug
}

func mermaidID(id string) string {
	return regexp.MustCompile(`[^A-Za-z0-9_]`).ReplaceAllString(id, "_")
}
