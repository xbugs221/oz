// Package app tests quality-loop configuration migration and sealed repair compatibility.
package app

import (
	"fmt"
	"strings"
	"testing"
)

// TestRepairLimitConfiguration verifies the old limit remains diagnostic without bounding new runs.
func TestRepairLimitConfiguration(t *testing.T) {
	for _, value := range []int{0, 12} {
		body := []byte(fmt.Sprintf("max_repair_iterations: %d\n", value))
		config, err := workflowConfigFromYAML(body, "test.yaml", nil)
		if err != nil {
			t.Fatalf("max_repair_iterations=%d: %v", value, err)
		}
		if config.MaxRepairIterations != value {
			t.Fatalf("MaxRepairIterations = %d, want %d", config.MaxRepairIterations, value)
		}
		if config.Generation != qualityLoopWorkflowGeneration {
			t.Fatalf("generation = %q, want %q", config.Generation, qualityLoopWorkflowGeneration)
		}
		for _, stage := range []string{"audit_1", "qa_1", "targeted_repair_1"} {
			if _, ok := config.Stages[stage]; !ok {
				t.Fatalf("quality loop must retain %s template", stage)
			}
		}
	}
	if _, err := workflowConfigFromYAML([]byte("max_repair_iterations: -1\n"), "test.yaml", nil); err == nil {
		t.Fatal("negative max_repair_iterations should fail")
	}
}

// TestQualityLoopDynamicStageOptions verifies later stages inherit sealed first-stage templates.
func TestQualityLoopDynamicStageOptions(t *testing.T) {
	config, err := workflowConfigFromYAML([]byte("stages:\n  repair:\n    reasoning: high\n"), "test.yaml", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, stage := range []string{"audit_14", "targeted_repair_13", "qa_14"} {
		if _, err := config.StageOption(stage); err != nil {
			t.Fatalf("StageOption(%q): %v", stage, err)
		}
	}
	if got, _ := config.StageOption("targeted_repair_13"); got.Reasoning != "high" {
		t.Fatalf("targeted repair reasoning = %q, want high", got.Reasoning)
	}
}

// TestQualityLoopPartialConfigInheritsRepairTemplate preserves profile defaults on partial overrides.
func TestQualityLoopPartialConfigInheritsRepairTemplate(t *testing.T) {
	base := DefaultWorkflowConfig()
	config, err := workflowConfigFromYAML([]byte("validation:\n  limit: 4\n"), "partial.yaml", &base)
	if err != nil {
		t.Fatal(err)
	}
	for _, stage := range []string{"audit_1", "targeted_repair_1"} {
		option, err := config.StageOption(stage)
		if err != nil {
			t.Fatal(err)
		}
		if option.Reasoning != "high" {
			t.Fatalf("%s reasoning = %q, want inherited high", stage, option.Reasoning)
		}
	}
}

// TestSealedRepairV1Compatibility verifies old finite snapshots keep expanded stages and limits.
func TestSealedRepairV1Compatibility(t *testing.T) {
	config := WorkflowConfig{
		Generation:          repairWorkflowGeneration,
		MaxRepairIterations: 2,
		Stages: map[string]StageOptions{
			"execution": {},
			"repair_1":  {},
			"qa_1":      {},
			"repair_2":  {},
			"qa_2":      {},
			"archive":   {},
		},
	}
	got := workflowStagesForConfig(config)
	want := []string{"execution", "repair_1", "qa_1", "repair_2", "qa_2", "archive"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("sealed repair-v1 stages = %v, want %v", got, want)
	}
}

// TestLegacyRepairPromptMigration verifies explicit old prompts override inherited defaults.
func TestLegacyRepairPromptMigration(t *testing.T) {
	for _, key := range []string{"fix", "review"} {
		base := DefaultWorkflowConfig()
		body := []byte(fmt.Sprintf("prompts:\n  %s: |\n    LEGACY_%s_MARKER\n", key, strings.ToUpper(key)))
		config, err := workflowConfigFromYAML(body, "legacy.yaml", &base)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(config.Prompts["repair"], "LEGACY_") {
			t.Fatalf("prompts.%s did not migrate to prompts.repair", key)
		}
		if len(config.Warnings) == 0 {
			t.Fatalf("prompts.%s migration must emit a deprecation warning", key)
		}
	}
}

// TestLegacyRepairConfiguration verifies the old limit migrates and ambiguous input is rejected.
func TestLegacyRepairConfiguration(t *testing.T) {
	config, err := workflowConfigFromYAML([]byte("max_review_iterations: 3\n"), "legacy.yaml", nil)
	if err != nil {
		t.Fatal(err)
	}
	if config.MaxRepairIterations != 3 || config.MaxReviewIterations != 0 {
		t.Fatalf("legacy limit did not migrate: %#v", config)
	}
	if _, err := workflowConfigFromYAML([]byte("max_repair_iterations: 3\nmax_review_iterations: 3\n"), "conflict.yaml", nil); err == nil {
		t.Fatal("new and legacy limits together should fail")
	}
}
