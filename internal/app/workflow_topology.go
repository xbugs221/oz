// Package app centralizes workflow stage, group, and artifact topology.
package app

import (
	"fmt"
	"path/filepath"
	"strings"
)

const (
	parallelGroupPlanning       = "planning_context"
	parallelGroupImplementation = "implementation_context"
	parallelGroupReview         = "review"
	parallelGroupQA             = "qa"

	parallelAnchorPlanning  = "planning"
	parallelAnchorBeforeRun = "before_execution"
	parallelAnchorReview    = "before_review"
	parallelAnchorQA        = "before_qa"
)

type stageParallelTopology struct {
	Stage  string
	Group  string
	Anchor string
	Mode   string
}

func stageParallelTopologies() []stageParallelTopology {
	return []stageParallelTopology{
		{Stage: "planning", Group: parallelGroupPlanning, Anchor: parallelAnchorPlanning, Mode: "advisory"},
		{Stage: "execution", Group: parallelGroupImplementation, Anchor: parallelAnchorBeforeRun, Mode: "advisory"},
		{Stage: "review", Group: parallelGroupReview, Anchor: parallelAnchorReview, Mode: "gate_input"},
		{Stage: "qa", Group: parallelGroupQA, Anchor: parallelAnchorQA, Mode: "gate_input"},
	}
}

func parallelTopologyForStage(stage string) (stageParallelTopology, bool) {
	for _, topology := range stageParallelTopologies() {
		if topology.Stage == stage {
			return topology, true
		}
	}
	return stageParallelTopology{}, false
}

func parallelGroupForCompactStage(stage string) []string {
	if stage == "planning" {
		return []string{parallelGroupPlanning}
	}
	if stage == "execution" {
		return []string{parallelGroupImplementation}
	}
	if strings.HasPrefix(stage, "review_") {
		return []string{parallelGroupReview}
	}
	if strings.HasPrefix(stage, "qa_") {
		return []string{parallelGroupQA}
	}
	return nil
}

func visualGroupForConfigGroup(groupName string) string {
	if groupName == parallelGroupReview {
		return parallelAnchorReview
	}
	if groupName == parallelGroupQA {
		return parallelAnchorQA
	}
	return groupName
}

func configGroupNameForVisualGroup(visualGroup string) string {
	if visualGroup == parallelAnchorBeforeRun {
		return parallelGroupImplementation
	}
	if visualGroup == parallelAnchorReview {
		return parallelGroupReview
	}
	if visualGroup == parallelAnchorQA {
		return parallelGroupQA
	}
	return visualGroup
}

// configGroupName preserves existing runner call sites while delegating to topology.
func configGroupName(visualGroup string) string {
	return configGroupNameForVisualGroup(visualGroup)
}

func parallelArtifactName(group string, iteration int) string {
	if group == parallelAnchorBeforeRun || group == parallelGroupImplementation {
		return "parallel-implementation-context.json"
	}
	if group == parallelAnchorReview {
		return fmt.Sprintf("parallel-review-%d.json", iteration)
	}
	if group == parallelAnchorQA {
		return fmt.Sprintf("parallel-qa-%d.json", iteration)
	}
	return filepath.Join("parallel-" + group + ".json")
}

func allowedStagesForParallelGroup(groupName string) (map[string]bool, bool) {
	for _, topology := range stageParallelTopologies() {
		if topology.Group == groupName {
			return map[string]bool{topology.Anchor: true, topology.Stage: true}, true
		}
	}
	return nil, false
}

func stageAtLeastKind(kind string, minimum string) bool {
	order := map[string]int{
		"planning":  1,
		"execution": 2,
		"review":    3,
		"qa":        4,
		"archive":   5,
		"done":      6,
	}
	return order[kind] >= order[minimum]
}
