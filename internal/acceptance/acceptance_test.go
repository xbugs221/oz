// Package acceptance tests shared validation and evidence producer tracing.
package acceptance

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEvidenceHasProducerFromMetadata(t *testing.T) {
	// TestEvidenceHasProducerFromMetadata verifies command, purpose, and assertions can name evidence output.
	evidence := Evidence{ID: "metadata-log", Path: "test-results/acceptance/metadata.log"}
	test := Test{
		ID:         "metadata-test",
		Path:       "tests/metadata_test.sh",
		Command:    "bash tests/metadata_test.sh | tee test-results/acceptance/metadata.log",
		Purpose:    "collects metadata-log runtime evidence",
		Assertions: []string{"writes test-results/acceptance/metadata.log"},
	}

	if !EvidenceHasProducer(t.TempDir(), evidence, coverageFor(evidence.ID, test.ID), map[string]Test{test.ID: test}) {
		t.Fatalf("expected metadata fields to trace evidence producer")
	}
}

func TestEvidenceHasProducerFromDeclaredTestFile(t *testing.T) {
	// TestEvidenceHasProducerFromDeclaredTestFile verifies the declared test file itself can produce evidence.
	root := t.TempDir()
	writeFile(t, root, "tests/producer_test.sh", "go test ./cmd/oz | tee test-results/acceptance/producer.log\n")
	evidence := Evidence{ID: "producer-log", Path: "test-results/acceptance/producer.log"}
	test := Test{
		ID:      "producer-test",
		Path:    "tests/producer_test.sh",
		Command: "bash tests/producer_test.sh",
		Purpose: "runs producer script",
	}

	if !EvidenceHasProducer(root, evidence, coverageFor(evidence.ID, test.ID), map[string]Test{test.ID: test}) {
		t.Fatalf("expected declared test file to trace evidence producer")
	}
}

func TestEvidenceHasProducerFromSiblingShellWrapper(t *testing.T) {
	// TestEvidenceHasProducerFromSiblingShellWrapper verifies a sibling .sh wrapper can produce evidence for a declared test.
	root := t.TempDir()
	writeFile(t, root, "tests/producer_go_test.go", "package tests\n")
	writeFile(t, root, "tests/run_producer.sh", "go test ./tests/producer_go_test.go | tee test-results/acceptance/wrapper.log\n")
	evidence := Evidence{ID: "wrapper-log", Path: "test-results/acceptance/wrapper.log"}
	test := Test{
		ID:      "wrapper-test",
		Path:    "tests/producer_go_test.go",
		Command: "go test ./tests/producer_go_test.go",
		Purpose: "runs producer Go test through a wrapper",
	}

	if !EvidenceHasProducer(root, evidence, coverageFor(evidence.ID, test.ID), map[string]Test{test.ID: test}) {
		t.Fatalf("expected sibling shell wrapper to trace evidence producer")
	}
}

func TestEvidenceHasProducerRejectsMissingProducer(t *testing.T) {
	// TestEvidenceHasProducerRejectsMissingProducer verifies coverage alone is not enough without a concrete producer.
	root := t.TempDir()
	writeFile(t, root, "tests/unrelated_test.sh", "echo no runtime artifact here\n")
	evidence := Evidence{ID: "missing-log", Path: "test-results/acceptance/missing.log"}
	test := Test{
		ID:      "unrelated-test",
		Path:    "tests/unrelated_test.sh",
		Command: "bash tests/unrelated_test.sh",
		Purpose: "runs unrelated test",
	}

	if EvidenceHasProducer(root, evidence, coverageFor(evidence.ID, test.ID), map[string]Test{test.ID: test}) {
		t.Fatalf("expected missing producer to be rejected")
	}
}

func TestValidateLifecycleReportsProducerDiagnostics(t *testing.T) {
	// TestValidateLifecycleReportsProducerDiagnostics verifies producer tracing failures become structured diagnostics.
	root := t.TempDir()
	writeFile(t, root, "tests/no_producer_test.sh", "echo no runtime artifact here\n")
	contract := Contract{
		Summary: "lifecycle contract",
		Coverage: []Coverage{{
			Spec:     "需求：lifecycle / 场景：producer",
			Tests:    []string{"no-producer"},
			Evidence: []string{"runtime-log"},
			Risk:     "fixture",
		}},
		RequiredTests: []Test{{
			ID:         "no-producer",
			Source:     "change_contract",
			Path:       "tests/no_producer_test.sh",
			Command:    "bash tests/no_producer_test.sh",
			Purpose:    "runs without evidence output",
			Assertions: []string{"business acceptance executes without producing the declared runtime log"},
		}},
		RequiredEvidence: []Evidence{{
			ID:      "runtime-log",
			Kind:    "runtime_log",
			Path:    "test-results/lifecycle/runtime.log",
			Purpose: "declared runtime evidence",
		}},
	}

	result := ValidateLifecycle(root, contract)
	if result.Valid {
		t.Fatalf("expected missing producer to fail lifecycle")
	}
	if len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != "required_evidence_producer_missing" {
		t.Fatalf("unexpected diagnostics: %#v", result.Diagnostics)
	}
}

func TestValidateLifecycleAcceptsProducerAndExposesRequiredItems(t *testing.T) {
	// TestValidateLifecycleAcceptsProducerAndExposesRequiredItems verifies the positive lifecycle path and QA item set.
	root := t.TempDir()
	writeFile(t, root, "tests/producer_test.sh", "mkdir -p test-results/lifecycle\nprintf ok > test-results/lifecycle/runtime.log\n")
	contract := Contract{
		Summary: "lifecycle contract",
		Coverage: []Coverage{{
			Spec:     "需求：lifecycle / 场景：producer",
			Tests:    []string{"producer"},
			Evidence: []string{"runtime-log"},
			Risk:     "fixture",
		}},
		RequiredTests: []Test{{
			ID:         "producer",
			Source:     "change_contract",
			Path:       "tests/producer_test.sh",
			Command:    "bash tests/producer_test.sh",
			Purpose:    "runs producer script",
			Assertions: []string{"business acceptance writes the declared runtime log"},
		}},
		RequiredEvidence: []Evidence{{
			ID:      "runtime-log",
			Kind:    "runtime_log",
			Path:    "test-results/lifecycle/runtime.log",
			Purpose: "declared runtime evidence",
		}},
	}

	result := ValidateLifecycle(root, contract)
	if !result.Valid || len(result.Diagnostics) != 0 {
		t.Fatalf("expected lifecycle to pass, got %#v", result)
	}
	if result.Required.Tests["producer"] == "" || result.Required.Evidence["runtime-log"] == "" {
		t.Fatalf("required item set missing ids: %#v", result.Required)
	}
}

func coverageFor(evidenceID, testID string) []Coverage {
	// coverageFor builds the minimal contract link needed by producer tracing.
	return []Coverage{{Spec: "producer tracing", Tests: []string{testID}, Evidence: []string{evidenceID}}}
}

func writeFile(t *testing.T, root, relPath, body string) {
	// writeFile creates a repo-relative fixture file for producer tracing tests.
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relPath))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create fixture dir: %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write fixture file: %v", err)
	}
}
