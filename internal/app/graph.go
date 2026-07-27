// Package app exports read-only workflow graphs for JSON, Mermaid, and tests.
package app

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"
)

// WorkflowSpec is the stable graph representation behind every graph exporter.
type WorkflowSpec struct {
	ChangeName   string             `json:"change_name" yaml:"change_name"`
	RunID        string             `json:"run_id,omitempty" yaml:"run_id,omitempty"`
	RunStatus    string             `json:"run_status,omitempty" yaml:"run_status,omitempty"`
	CurrentStage string             `json:"current_stage,omitempty" yaml:"current_stage,omitempty"`
	Nodes        []WorkflowNode     `json:"nodes" yaml:"nodes"`
	Edges        []WorkflowEdge     `json:"edges" yaml:"edges"`
	Artifacts    []WorkflowArtifact `json:"artifacts" yaml:"artifacts"`
	Gates        []WorkflowGate     `json:"gates" yaml:"gates"`
	Display      WorkflowDisplay    `json:"display" yaml:"display"`
}

// WorkflowNode describes one user-visible graph step.
type WorkflowNode struct {
	ID           string `json:"id" yaml:"id"`
	Name         string `json:"name" yaml:"name"`
	Type         string `json:"type" yaml:"type"`
	Group        string `json:"group,omitempty" yaml:"group,omitempty"`
	Stage        string `json:"stage,omitempty" yaml:"stage,omitempty"`
	RunStage     string `json:"run_stage,omitempty" yaml:"run_stage,omitempty"`
	Member       string `json:"member,omitempty" yaml:"member,omitempty"`
	Mode         string `json:"mode,omitempty" yaml:"mode,omitempty"`
	Status       string `json:"status,omitempty" yaml:"status,omitempty"`
	Iteration    int    `json:"iteration,omitempty" yaml:"iteration,omitempty"`
	DecisionOnly bool   `json:"decision_only,omitempty" yaml:"decision_only,omitempty"`
}

// WorkflowEdge records execution or decision ordering between graph nodes.
type WorkflowEdge struct {
	From         string `json:"from" yaml:"from"`
	To           string `json:"to" yaml:"to"`
	Label        string `json:"label,omitempty" yaml:"label,omitempty"`
	DecisionOnly bool   `json:"decision_only,omitempty" yaml:"decision_only,omitempty"`
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

// runGraph prefers a sealed run snapshot and otherwise writes the active workflow graph.
func runGraph(repo string, args []string, stdout io.Writer) error {
	changeName, err := requireFlagValue(args, "--change")
	if err != nil {
		return fmt.Errorf("用法：oz flow graph --change <change-name> --format json|mermaid")
	}
	format, err := requireFlagValue(args, "--format")
	if err != nil {
		return fmt.Errorf("用法：oz flow graph --change <change-name> --format json|mermaid")
	}
	state, err := latestSealedGraphState(repo, changeName)
	if err != nil {
		return err
	}
	var workflow WorkflowConfig
	if state != nil {
		workflow = state.Workflow
	} else {
		workflow, err = LoadWorkflowConfig(repo)
		if err != nil {
			return err
		}
	}
	spec := BuildWorkflowSpec(changeName, workflow)
	if state != nil {
		spec.RunID = state.RunID
		spec.RunStatus = state.Status
		spec.CurrentStage = state.Stage
		if usesQualityLoop(state.Workflow) {
			spec = appendQualityLoopGraphInstances(spec, *state)
		}
	}
	switch format {
	case "json":
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(spec)
	case "mermaid":
		graph := buildCompactMermaid(changeName, workflow)
		if state != nil && usesQualityLoop(state.Workflow) {
			graph = appendQualityLoopMermaidInstances(graph, *state)
		}
		_, err := fmt.Fprint(stdout, graph)
		return err
	default:
		return fmt.Errorf("未知 graph format %q，可选 json、mermaid", format)
	}
}

// BuildWorkflowSpec expands legacy finite generations or the dynamic quality-loop template.
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
	spec.addNode(WorkflowNode{
		ID: "execution", Name: humanWorkflowStageName("execution"), Type: "main_stage", Stage: "execution",
	})
	previous := "execution"
	if workflow.Generation == qualityLoopWorkflowGeneration {
		return buildQualityLoopWorkflowSpec(spec)
	}
	if usesRepairWorkflow(workflow) {
		for i := 1; i <= workflow.MaxRepairIterations; i++ {
			repair := fmt.Sprintf("repair_%d", i)
			qa := fmt.Sprintf("qa_%d", i)
			spec.addNode(WorkflowNode{ID: repair, Name: repair, Type: "main_stage", Stage: repair, Iteration: i})
			if i == 1 {
				spec.addEdge("execution", repair, "")
			}
			repairGate := fmt.Sprintf("gate_repair_%d", i)
			spec.addGate(repairGate, "repair gate", repair, i)
			spec.addEdge(repair, repairGate, "")
			spec.addNode(WorkflowNode{
				ID: qa, Name: humanWorkflowStageName(qa), Type: "main_stage", Stage: qa, Iteration: i,
			})
			spec.addEdge(repairGate, qa, "repair confirmation clean")
			qaGate := fmt.Sprintf("gate_qa_%d", i)
			spec.addGate(qaGate, "QA gate", qa, i)
			spec.addEdge(qa, qaGate, "")
			if i < workflow.MaxRepairIterations {
				spec.addDecisionEdge(repairGate, fmt.Sprintf("repair_%d", i+1), "repair needs_more / first clean")
				spec.addEdge(qaGate, fmt.Sprintf("repair_%d", i+1), "QA needs_fix")
				spec.addDecisionEdge(qaGate, "gate_archive", "QA clean")
			} else {
				spec.addDecisionEdge(repairGate, statusBlocked, "repair needs_more / first clean")
				spec.addDecisionEdge(qaGate, statusBlocked, "QA needs_fix")
			}
		}
		if workflow.MaxRepairIterations > 0 {
			spec.addNode(WorkflowNode{
				ID: statusBlocked, Name: "blocked", Type: "terminal", Stage: statusBlocked, DecisionOnly: true,
			})
		}
		spec.addNode(WorkflowNode{
			ID: "archive", Name: humanWorkflowStageName("archive"), Type: "main_stage", Stage: "archive",
		})
		archiveGate := "gate_archive"
		spec.addGate(archiveGate, "archive gate", "archive", 0)
		if workflow.MaxRepairIterations == 0 {
			qa := "qa_1"
			spec.addNode(WorkflowNode{
				ID: qa, Name: humanWorkflowStageName(qa), Type: "main_stage", Stage: qa, Iteration: 1,
			})
			spec.addEdge("execution", qa, "")
			qaGate := "gate_qa_1"
			spec.addGate(qaGate, "QA gate", qa, 1)
			spec.addEdge(qa, qaGate, "")
			spec.addNode(WorkflowNode{
				ID: statusBlocked, Name: "blocked", Type: "terminal", Stage: statusBlocked, DecisionOnly: true,
			})
			spec.addDecisionEdge(qaGate, statusBlocked, "QA needs_fix")
			spec.addEdge(qaGate, archiveGate, "QA clean")
		} else {
			spec.addEdge(fmt.Sprintf("gate_qa_%d", workflow.MaxRepairIterations), archiveGate, "QA clean")
		}
		spec.addEdge(archiveGate, "archive", "")
		return spec
	}
	for i := 1; i <= workflow.MaxReviewIterations; i++ {
		review := fmt.Sprintf("review_%d", i)
		qa := fmt.Sprintf("qa_%d", i)
		fix := fmt.Sprintf("fix_%d", i)
		spec.addNode(WorkflowNode{ID: review, Name: review, Type: "main_stage", Stage: review, Iteration: i})
		spec.addEdge(previous, review, "")
		reviewGate := fmt.Sprintf("gate_review_%d", i)
		spec.addGate(reviewGate, "review gate", review, i)
		spec.addEdge(review, reviewGate, "")
		spec.addNode(WorkflowNode{
			ID: qa, Name: humanWorkflowStageName(qa), Type: "main_stage", Stage: qa, Iteration: i,
		})
		spec.addEdge(reviewGate, qa, "review clean")
		qaGate := fmt.Sprintf("gate_qa_%d", i)
		spec.addGate(qaGate, "QA gate", qa, i)
		spec.addEdge(qa, qaGate, "")
		spec.addNode(WorkflowNode{ID: fix, Name: fix, Type: "main_stage", Stage: fix, Iteration: i})
		spec.addEdge(reviewGate, fix, "review needs_fix")
		spec.addEdge(qaGate, fix, "QA needs_fix")
		previous = fix
	}
	spec.addNode(WorkflowNode{
		ID: "archive", Name: humanWorkflowStageName("archive"), Type: "main_stage", Stage: "archive",
	})
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

// buildQualityLoopWorkflowSpec adds the unbounded audit, QA, and targeted-repair template.
func buildQualityLoopWorkflowSpec(spec WorkflowSpec) WorkflowSpec {
	spec.addNode(WorkflowNode{
		ID: "audit_N", Name: humanWorkflowStageName("audit_N"), Type: "loop_template", Stage: "audit_N",
		Mode: "pre_qa_audit",
	})
	spec.addEdge("execution", "audit_N", "")
	spec.addGate("gate_audit_N", "audit gate", "audit_N", 0)
	spec.addEdge("audit_N", "gate_audit_N", "")
	spec.addDecisionEdge("gate_audit_N", "audit_N", "needs_more / self-tests failed")

	spec.addNode(WorkflowNode{
		ID: "qa_N", Name: humanWorkflowStageName("qa_N"), Type: "loop_template", Stage: "qa_N",
	})
	spec.addEdge("gate_audit_N", "qa_N", "clean + self-tests passed")
	spec.addGate("gate_qa_N", "QA gate", "qa_N", 0)
	spec.addEdge("qa_N", "gate_qa_N", "")

	spec.addNode(WorkflowNode{
		ID: "targeted_repair_N", Name: humanWorkflowStageName("targeted_repair_N"), Type: "loop_template", Stage: "targeted_repair_N",
		Mode: "qa_targeted_repair",
	})
	spec.addEdge("gate_qa_N", "targeted_repair_N", "QA needs_fix")
	spec.addGate("gate_targeted_repair_N", "targeted repair gate", "targeted_repair_N", 0)
	spec.addEdge("targeted_repair_N", "gate_targeted_repair_N", "")
	spec.addDecisionEdge("gate_targeted_repair_N", "targeted_repair_N", "self-tests failed")
	spec.addDecisionEdge("gate_targeted_repair_N", "qa_N", "failed tests + required tests passed")

	for _, blocked := range []WorkflowNode{
		{ID: statusBlockedEnvironment, Name: statusBlockedEnvironment, Type: "pause", Stage: statusBlockedEnvironment, DecisionOnly: true},
		{ID: statusBlockedStalled, Name: statusBlockedStalled, Type: "pause", Stage: statusBlockedStalled, DecisionOnly: true},
	} {
		spec.addNode(blocked)
	}
	for _, from := range []string{"gate_audit_N", "gate_targeted_repair_N"} {
		spec.addDecisionEdge(from, statusBlockedEnvironment, "environment missing")
		spec.addDecisionEdge(from, statusBlockedStalled, "no proven progress")
	}
	for _, blocked := range []string{statusBlockedEnvironment, statusBlockedStalled} {
		spec.addDecisionEdge(blocked, "audit_N", "resume/restart → blocked_from_stage")
		spec.addDecisionEdge(blocked, "targeted_repair_N", "resume/restart → blocked_from_stage")
	}

	spec.addNode(WorkflowNode{
		ID: "archive", Name: humanWorkflowStageName("archive"), Type: "main_stage", Stage: "archive",
	})
	spec.addGate("gate_archive", "archive gate", "archive", 0)
	spec.addDecisionEdge("gate_qa_N", "gate_archive", "QA clean")
	spec.addEdge("gate_archive", "archive", "")
	return spec
}

// buildCompactMermaid renders a compact Chinese state-machine graph for human review.
func buildCompactMermaid(changeName string, workflow WorkflowConfig) string {
	var out strings.Builder
	out.WriteString("flowchart TD\n")
	out.WriteString("  execution[执行]\n")

	if workflow.Generation == qualityLoopWorkflowGeneration {
		out.WriteString("  audit[自查]\n")
		out.WriteString("  qa[测试]\n")
		out.WriteString("  targeted_repair[修复]\n")
		out.WriteString("  blocked_environment[环境阻塞]\n")
		out.WriteString("  blocked_stalled[停滞阻塞]\n")
		out.WriteString("  archive[归档]\n")
		out.WriteString("  execution --> audit\n")
		out.WriteString("  audit -->|needs_more 或自测失败| audit\n")
		out.WriteString("  audit -->|clean 且自测通过| qa\n")
		out.WriteString("  qa -->|needs_fix| targeted_repair\n")
		out.WriteString("  targeted_repair -->|失败测试与 required tests 通过| qa\n")
		out.WriteString("  targeted_repair -->|自测失败| targeted_repair\n")
		out.WriteString("  audit -->|缺少环境| blocked_environment\n")
		out.WriteString("  targeted_repair -->|缺少环境| blocked_environment\n")
		out.WriteString("  audit -->|无可证明进展| blocked_stalled\n")
		out.WriteString("  targeted_repair -->|无可证明进展| blocked_stalled\n")
		out.WriteString("  blocked_environment -.->|resume/restart 回原阶段| audit\n")
		out.WriteString("  blocked_environment -.->|resume/restart 回原阶段| targeted_repair\n")
		out.WriteString("  blocked_stalled -.->|resume/restart 回原阶段| audit\n")
		out.WriteString("  blocked_stalled -.->|resume/restart 回原阶段| targeted_repair\n")
		out.WriteString("  qa -->|clean| archive\n")
		return out.String()
	}
	if usesRepairWorkflow(workflow) {
		if workflow.MaxRepairIterations == 0 {
			out.WriteString("  qa[测试]\n")
			out.WriteString("  archive[归档]\n")
			out.WriteString("  blocked[阻塞]\n")
			out.WriteString("  execution --> qa\n")
			out.WriteString("  qa -->|clean| archive\n")
			out.WriteString("  qa -->|needs_fix，无修正轮次| blocked\n")
			return out.String()
		}
		out.WriteString("  repair[优化]\n")
		out.WriteString("  qa[测试]\n")
		out.WriteString("  archive[归档]\n")
		out.WriteString("  blocked[阻塞]\n")
		out.WriteString("  execution --> repair\n")
		out.WriteString("  repair -->|确认 clean| qa\n")
		fmt.Fprintf(&out, "  repair -->|needs_more 或首次 clean，未达第%d轮| repair\n", workflow.MaxRepairIterations)
		fmt.Fprintf(&out, "  repair -->|needs_more 或首次 clean，第%d轮| blocked\n", workflow.MaxRepairIterations)
		fmt.Fprintf(&out, "  qa -->|needs_fix，未达第%d轮| repair\n", workflow.MaxRepairIterations)
		fmt.Fprintf(&out, "  qa -->|needs_fix，第%d轮| blocked\n", workflow.MaxRepairIterations)
		out.WriteString("  qa -->|clean| archive\n")
		return out.String()
	}
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

// latestSealedGraphState returns the newest readable sealed run for one change.
func latestSealedGraphState(repo, changeName string) (*State, error) {
	root, err := runsRoot(repo)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	for index := len(entries) - 1; index >= 0; index-- {
		entry := entries[index]
		if !entry.IsDir() {
			continue
		}
		state, loadErr := loadState(repo, entry.Name())
		if loadErr != nil || !state.Sealed || state.ChangeName != changeName {
			continue
		}
		return &state, nil
	}
	return nil, nil
}

// appendQualityLoopGraphInstances adds durable dynamic stages without replacing the loop template.
func appendQualityLoopGraphInstances(spec WorkflowSpec, state State) WorkflowSpec {
	spec.RunID = state.RunID
	spec.RunStatus = state.Status
	spec.CurrentStage = state.Stage
	for index := range spec.Nodes {
		if spec.Nodes[index].ID == "execution" {
			spec.Nodes[index].Status = qualityLoopGraphStageStatus(state, "execution")
			break
		}
	}
	stages := qualityLoopGraphInstanceStages(state)
	if len(stages) == 0 {
		return spec
	}
	for _, stageName := range stages {
		stage, err := parseWorkflowStage(stageName)
		if err != nil {
			continue
		}
		mode := ""
		switch stage.Kind {
		case workflowStageAudit:
			mode = "pre_qa_audit"
		case workflowStageTargetedRepair:
			mode = "qa_targeted_repair"
		}
		spec.addNode(WorkflowNode{
			ID: stageName, Name: humanWorkflowStageName(stageName), Type: "loop_instance", Stage: stageName,
			Mode: mode, Status: qualityLoopGraphStageStatus(state, stageName), Iteration: stage.Iteration,
		})
	}
	previous := "execution"
	for _, stageName := range stages {
		spec.addDecisionEdge(previous, stageName, "durable instance")
		previous = stageName
	}
	if isQualityLoopBlockedState(state) && state.QualityLoop.BlockedFromStage != "" {
		spec.addDecisionEdge(state.Stage, state.QualityLoop.BlockedFromStage, "resume/restart → blocked_from_stage")
	}
	return spec
}

// qualityLoopGraphInstanceStages returns persisted instances in business execution order.
func qualityLoopGraphInstanceStages(state State) []string {
	seen := map[string]bool{}
	add := func(stage string) {
		if isDynamicQualityLoopStage(stage) {
			seen[stage] = true
		}
	}
	add(state.Stage)
	for stage := range state.Stages {
		add(stage)
	}
	for stage := range state.StageTimings {
		add(stage)
	}
	for stage := range state.DAGNodes {
		add(stage)
	}
	stages := make([]string, 0, len(seen))
	for stage := range seen {
		stages = append(stages, stage)
	}
	sort.Slice(stages, func(left, right int) bool {
		leftStarted, leftTimed := qualityLoopGraphStageStartedAt(state, stages[left])
		rightStarted, rightTimed := qualityLoopGraphStageStartedAt(state, stages[right])
		if leftTimed != rightTimed {
			return leftTimed
		}
		if leftTimed && !leftStarted.Equal(rightStarted) {
			return leftStarted.Before(rightStarted)
		}
		return qualityLoopGraphStageLess(stages[left], stages[right])
	})
	return stages
}

// qualityLoopGraphStageStartedAt returns the durable start time for one dynamic stage.
func qualityLoopGraphStageStartedAt(state State, stage string) (time.Time, bool) {
	timing, ok := state.StageTimings[stage]
	if !ok || timing.StartedAt == "" {
		return time.Time{}, false
	}
	startedAt, err := time.Parse(time.RFC3339Nano, timing.StartedAt)
	return startedAt, err == nil
}

// qualityLoopGraphStageLess provides deterministic legacy ordering when timing data is absent.
func qualityLoopGraphStageLess(leftName, rightName string) bool {
	left, leftErr := parseWorkflowStage(leftName)
	right, rightErr := parseWorkflowStage(rightName)
	if leftErr != nil || rightErr != nil {
		return leftName < rightName
	}
	if left.Kind == workflowStageAudit || right.Kind == workflowStageAudit {
		if left.Kind != right.Kind {
			return left.Kind == workflowStageAudit
		}
		return left.Iteration < right.Iteration
	}
	if left.Iteration != right.Iteration {
		return left.Iteration < right.Iteration
	}
	if left.Kind != right.Kind {
		return left.Kind == workflowStageQA
	}
	return leftName < rightName
}

// qualityLoopGraphStageStatus resolves one instance from scheduler, stage, and timing records.
func qualityLoopGraphStageStatus(state State, stage string) string {
	if status := state.Stages[stage]; status != "" {
		return status
	}
	if node, ok := state.DAGNodes[stage]; ok && node.Status != "" {
		return node.Status
	}
	if timing, ok := state.StageTimings[stage]; ok && timing.StartedAt != "" {
		if timing.FinishedAt != "" {
			return "completed"
		}
		return statusRunning
	}
	if state.Stage == stage {
		return state.Status
	}
	return ""
}

// appendQualityLoopMermaidInstances adds the active run timeline and exact resume target.
func appendQualityLoopMermaidInstances(graph string, state State) string {
	stages := qualityLoopGraphInstanceStages(state)
	if len(stages) == 0 {
		return graph
	}
	var out strings.Builder
	out.WriteString(strings.TrimSuffix(graph, "\n"))
	out.WriteString("\n  subgraph current_instances[当前运行实例]\n")
	for _, stage := range stages {
		status := qualityLoopGraphStageStatus(state, stage)
		parsed, err := parseWorkflowStage(stage)
		if err != nil {
			continue
		}
		fmt.Fprintf(&out, "    %s[%s %d · %s]\n",
			mermaidID(stage), humanWorkflowStageName(stage), parsed.Iteration, nonEmpty(status, "unknown"))
	}
	out.WriteString("  end\n")
	fmt.Fprintf(&out, "  execution -.-> %s\n", mermaidID(stages[0]))
	for index := 1; index < len(stages); index++ {
		fmt.Fprintf(&out, "  %s -.-> %s\n", mermaidID(stages[index-1]), mermaidID(stages[index]))
	}
	if isQualityLoopBlockedState(state) && state.QualityLoop.BlockedFromStage != "" {
		fmt.Fprintf(&out, "  %s -.->|resume/restart 回 blocked_from_stage| %s\n",
			mermaidID(state.Stage), mermaidID(state.QualityLoop.BlockedFromStage))
	}
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

// addDecisionEdge records a business transition that must not become an extra DAG dependency.
func (spec *WorkflowSpec) addDecisionEdge(from, to, label string) {
	spec.Edges = append(spec.Edges, WorkflowEdge{From: from, To: to, Label: label, DecisionOnly: true})
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
