// Package app tests repair-loop workflow configuration migration and limits.
package app

import (
	"fmt"
	"strings"
	"testing"
)

// TestRepairLimitConfiguration verifies the documented 0..10 range.
func TestRepairLimitConfiguration(t *testing.T) {
	for _, value := range []int{0, 10} {
		body := []byte(fmt.Sprintf("max_repair_iterations: %d\n", value))
		config, err := workflowConfigFromYAML(body, "test.yaml", nil)
		if err != nil {
			t.Fatalf("max_repair_iterations=%d: %v", value, err)
		}
		if config.MaxRepairIterations != value {
			t.Fatalf("MaxRepairIterations = %d, want %d", config.MaxRepairIterations, value)
		}
		if value == 0 {
			if _, ok := config.Stages["qa_1"]; !ok {
				t.Fatal("max_repair_iterations=0 must retain independent qa_1")
			}
		}
	}
	if _, err := workflowConfigFromYAML([]byte("max_repair_iterations: 11\n"), "test.yaml", nil); err == nil {
		t.Fatal("max_repair_iterations=11 should fail")
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
