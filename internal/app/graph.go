// Package app exports read-only workflow graphs for Dagu, Mermaid, and tests.
package app

import (
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
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

type daguWorkflow struct {
	Name  string     `yaml:"name"`
	Steps []daguStep `yaml:"steps"`
}

type daguStep struct {
	Name        string   `yaml:"name"`
	Command     string   `yaml:"command"`
	Depends     []string `yaml:"depends,omitempty"`
	Description string   `yaml:"description,omitempty"`
}

// runGraph loads effective workflow config and writes one graph representation.
func runGraph(repo string, args []string, stdout io.Writer) error {
	changeName, err := requireFlagValue(args, "--change")
	if err != nil {
		return fmt.Errorf("用法：wo graph --change <change-name> --format json|mermaid|dagu")
	}
	format, err := requireFlagValue(args, "--format")
	if err != nil {
		return fmt.Errorf("用法：wo graph --change <change-name> --format json|mermaid|dagu")
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
		_, err := fmt.Fprint(stdout, ExportWorkflowMermaid(spec))
		return err
	case "dagu":
		_, err := fmt.Fprint(stdout, ExportWorkflowDaguYAML(spec))
		return err
	default:
		return fmt.Errorf("未知 graph format %q，可选 json、mermaid、dagu", format)
	}
}

// BuildWorkflowSpec expands the workflow config into a finite OmO DAG.
func BuildWorkflowSpec(changeName string, workflow WorkflowConfig) WorkflowSpec {
	normalizeWorkflowConfig(&workflow)
	spec := WorkflowSpec{
		ChangeName: changeName,
		Display:    WorkflowDisplay{Title: "wo workflow: " + changeName},
	}
	var beforeExecutionPrereqs []WorkflowEdge
	if planning := addParallelGroup(&spec, workflow, "planning_context", "execution", 0, nil); planning != "" {
		beforeExecutionPrereqs = append(beforeExecutionPrereqs, WorkflowEdge{From: planning})
	}
	spec.addNode(WorkflowNode{ID: "execution", Name: "execution", Type: "main_stage", Stage: "execution"})
	if before := addParallelGroup(&spec, workflow, "before_execution", "execution", 0, beforeExecutionPrereqs); before != "" {
		spec.addEdge(before, "execution", "")
	} else if len(beforeExecutionPrereqs) > 0 {
		spec.addEdge(beforeExecutionPrereqs[0].From, "execution", "")
	}
	previous := "execution"
	for i := 1; i <= workflow.MaxReviewIterations; i++ {
		review := fmt.Sprintf("review_%d", i)
		qa := fmt.Sprintf("qa_%d", i)
		fix := fmt.Sprintf("fix_%d", i)
		spec.addNode(WorkflowNode{ID: review, Name: review, Type: "main_stage", Stage: review, Iteration: i})
		if fanin := addParallelGroup(&spec, workflow, "before_review", review, i, []WorkflowEdge{{From: previous}}); fanin != "" {
			spec.addEdge(fanin, review, "")
		} else {
			spec.addEdge(previous, review, "")
		}
		reviewGate := fmt.Sprintf("gate_review_%d", i)
		spec.addGate(reviewGate, "review gate", review, i)
		spec.addEdge(review, reviewGate, "")
		spec.addNode(WorkflowNode{ID: qa, Name: qa, Type: "main_stage", Stage: qa, Iteration: i})
		if fanin := addParallelGroup(&spec, workflow, "before_qa", qa, i, []WorkflowEdge{{From: reviewGate, Label: "review clean"}}); fanin != "" {
			spec.addEdge(fanin, qa, "")
		} else {
			spec.addEdge(reviewGate, qa, "review clean")
		}
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

// ExportWorkflowDaguYAML renders Dagu YAML that calls stable wo node subcommands.
func ExportWorkflowDaguYAML(spec WorkflowSpec) string {
	return exportWorkflowDaguYAML(spec, "")
}

// ExportRunWorkflowDaguYAML renders executable run-local Dagu YAML.
func ExportRunWorkflowDaguYAML(spec WorkflowSpec, runID string) string {
	return exportWorkflowDaguYAML(spec, runID)
}

func exportWorkflowDaguYAML(spec WorkflowSpec, runID string) string {
	workflow := daguWorkflow{Name: "wo-" + slug(spec.ChangeName)}
	for _, node := range spec.Nodes {
		step := daguStep{Name: node.ID, Command: nodeCommand(spec.ChangeName, runID, node), Description: node.Name}
		for _, edge := range spec.Edges {
			if edge.To == node.ID {
				step.Depends = append(step.Depends, edge.From)
			}
		}
		workflow.Steps = append(workflow.Steps, step)
	}
	data, err := yaml.Marshal(workflow)
	if err != nil {
		return ""
	}
	return string(data)
}

// ExportWorkflowMermaid renders a self-contained user-facing DAG.
func ExportWorkflowMermaid(spec WorkflowSpec) string {
	var out strings.Builder
	out.WriteString("flowchart TD\n")
	for _, node := range spec.Nodes {
		fmt.Fprintf(&out, "  %s[%q]\n", mermaidID(node.ID), node.Name)
	}
	for _, edge := range spec.Edges {
		label := ""
		if edge.Label != "" {
			label = "|" + edge.Label + "|"
		}
		fmt.Fprintf(&out, "  %s -->%s %s\n", mermaidID(edge.From), label, mermaidID(edge.To))
	}
	return out.String()
}

// addParallelGroup expands a configured subagent group into fan-out member nodes and a fan-in node.
func addParallelGroup(spec *WorkflowSpec, workflow WorkflowConfig, visualGroup, stage string, iteration int, prerequisites []WorkflowEdge) string {
	groupName := configGroupName(visualGroup)
	group, ok := workflow.Parallel.Groups[visualGroup]
	if !ok {
		group, ok = workflow.Parallel.Groups[groupName]
	}
	if !workflow.Parallel.Enabled || !ok || len(group.Members) == 0 {
		return ""
	}
	memberIDs := make([]string, 0, len(group.Members))
	for i, member := range group.Members {
		id := fmt.Sprintf("%s_%d", visualGroup, i+1)
		if iteration > 0 {
			id = fmt.Sprintf("%s_%d_%d", visualGroup, iteration, i+1)
		}
		spec.addNode(WorkflowNode{ID: id, Name: groupName + " subagent: " + member.Name, Type: "subagent", Group: visualGroup, Stage: stage, Member: member.Name, Mode: group.Mode, Iteration: iteration})
		for _, prerequisite := range prerequisites {
			spec.addEdge(prerequisite.From, id, prerequisite.Label)
		}
		memberIDs = append(memberIDs, id)
	}
	fanin := visualGroup + "_fanin"
	if iteration > 0 {
		fanin = fmt.Sprintf("%s_%d_fanin", visualGroup, iteration)
	}
	spec.addNode(WorkflowNode{ID: fanin, Name: groupName + " fan-in", Type: "fanin", Group: visualGroup, Stage: stage, Mode: group.Mode, Iteration: iteration})
	for _, id := range memberIDs {
		spec.addEdge(id, fanin, "")
	}
	spec.Artifacts = append(spec.Artifacts, WorkflowArtifact{ID: fanin + "_artifact", Path: graphArtifactPath(visualGroup, iteration), NodeID: fanin})
	return fanin
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

func nodeCommand(changeName, runID string, node WorkflowNode) string {
	target := fmt.Sprintf("--change %s", nodeArg(changeName))
	if runID != "" {
		target = fmt.Sprintf("--run-id %s", nodeArg(runID))
	}
	switch node.Type {
	case "subagent":
		return fmt.Sprintf("wo node run-subagent %s --group %s --member %s --stage %s%s --json", target, nodeArg(node.Group), nodeArg(node.Member), nodeArg(node.Stage), iterationFlag(node.Iteration))
	case "fanin":
		return fmt.Sprintf("wo node fanin %s --group %s --stage %s%s --json", target, nodeArg(node.Group), nodeArg(node.Stage), iterationFlag(node.Iteration))
	case "gate":
		return fmt.Sprintf("wo node gate %s --stage %s%s --json", target, nodeArg(node.Stage), iterationFlag(node.Iteration))
	default:
		return fmt.Sprintf("wo node run-stage %s --stage %s%s --json", target, nodeArg(node.Stage), iterationFlag(node.Iteration))
	}
}

func nodeArg(value string) string {
	if regexp.MustCompile(`^[A-Za-z0-9_./:-]+$`).MatchString(value) {
		return value
	}
	return shellQuote(value)
}

func iterationFlag(iteration int) string {
	if iteration <= 0 {
		return ""
	}
	return fmt.Sprintf(" --iteration %d", iteration)
}

func configGroupName(visualGroup string) string {
	switch visualGroup {
	case "before_execution":
		return "implementation_context"
	case "before_review":
		return "review"
	case "before_qa":
		return "qa"
	default:
		return visualGroup
	}
}

func graphArtifactPath(group string, iteration int) string {
	switch group {
	case "before_execution":
		return "parallel-implementation-context.json"
	case "before_review":
		return fmt.Sprintf("parallel-review-%d.json", iteration)
	case "before_qa":
		return fmt.Sprintf("parallel-qa-%d.json", iteration)
	default:
		return filepath.Join("parallel-" + group + ".json")
	}
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
