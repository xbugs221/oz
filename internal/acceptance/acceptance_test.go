// Package acceptance tests shared validation and evidence producer tracing.
package acceptance

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

func TestValidateSubmissionEvidenceAcceptsFinalDemoPackage(t *testing.T) {
	// TestValidateSubmissionEvidenceAcceptsFinalDemoPackage verifies temporary evidence is mapped to a committed proposal bundle.
	contract := submissionEvidenceContract()

	if err := Validate(contract); err != nil {
		t.Fatalf("expected final demo package to pass: %v", err)
	}
	data, err := json.Marshal(contract)
	if err != nil {
		t.Fatalf("marshal submission evidence contract: %v", err)
	}
	parsed, err := Parse(data)
	if err != nil {
		t.Fatalf("parse submission evidence contract: %v", err)
	}
	if len(parsed.SubmissionEvidence) != 2 {
		t.Fatalf("submission evidence count = %d, want 2", len(parsed.SubmissionEvidence))
	}
}

func TestValidateSubmissionEvidenceKeepsLegacyContractCompatible(t *testing.T) {
	// TestValidateSubmissionEvidenceKeepsLegacyContractCompatible verifies old contracts remain valid without the new optional field.
	contract := submissionEvidenceContract()
	contract.SubmissionEvidence = nil

	if err := Validate(contract); err != nil {
		t.Fatalf("legacy contract without submission_evidence should pass: %v", err)
	}
}

func TestValidateSubmissionEvidenceRequiresDemoVideo(t *testing.T) {
	// TestValidateSubmissionEvidenceRequiresDemoVideo verifies repair comparisons or screenshots cannot replace the final demo video.
	contract := submissionEvidenceContract()
	contract.RequiredEvidence = contract.RequiredEvidence[1:]
	contract.SubmissionEvidence = contract.SubmissionEvidence[1:]

	err := Validate(contract)
	if err == nil || !strings.Contains(err.Error(), "demo_video") {
		t.Fatalf("expected missing demo video error, got %v", err)
	}
}

func TestValidateSubmissionEvidenceRejectsUnsafeOrUnboundPaths(t *testing.T) {
	// TestValidateSubmissionEvidenceRejectsUnsafeOrUnboundPaths verifies source and archive paths cannot escape their lifecycle roots.
	tests := []struct {
		name    string
		mutate  func(*Contract)
		wantErr string
	}{
		{
			name: "empty declared package",
			mutate: func(contract *Contract) {
				contract.SubmissionEvidence = []SubmissionEvidence{}
			},
			wantErr: "至少包含一个",
		},
		{
			name: "unknown required evidence",
			mutate: func(contract *Contract) {
				contract.SubmissionEvidence[0].EvidenceID = "unknown-video"
			},
			wantErr: "未知 required_evidence",
		},
		{
			name: "package omits required evidence",
			mutate: func(contract *Contract) {
				contract.SubmissionEvidence = contract.SubmissionEvidence[:1]
			},
			wantErr: "覆盖全部 required_evidence",
		},
		{
			name: "source differs from required evidence",
			mutate: func(contract *Contract) {
				contract.SubmissionEvidence[0].SourcePath = "test-results/demo/other.webm"
			},
			wantErr: "必须等于 required_evidence",
		},
		{
			name: "source escapes temporary results",
			mutate: func(contract *Contract) {
				contract.RequiredEvidence[0].Path = "artifacts/demo.webm"
				contract.SubmissionEvidence[0].SourcePath = "artifacts/demo.webm"
			},
			wantErr: "test-results/",
		},
		{
			name: "archive escapes proposal evidence root",
			mutate: func(contract *Contract) {
				contract.SubmissionEvidence[0].ArchivePath = "docs/changes/1-demo/demo.webm"
			},
			wantErr: "tests/evidence/proposals/<change>/",
		},
		{
			name: "archive omits proposal directory",
			mutate: func(contract *Contract) {
				contract.SubmissionEvidence[0].ArchivePath = "tests/evidence/proposals/demo.webm"
			},
			wantErr: "提案目录和证据文件",
		},
		{
			name: "archive mixes proposal directories",
			mutate: func(contract *Contract) {
				contract.SubmissionEvidence[1].ArchivePath = "tests/evidence/proposals/2-other/final.png"
			},
			wantErr: "同一提案目录",
		},
		{
			name: "demo video has non-video extension",
			mutate: func(contract *Contract) {
				contract.RequiredEvidence[0].Path = "test-results/demo/final-demo.txt"
				contract.SubmissionEvidence[0].SourcePath = "test-results/demo/final-demo.txt"
				contract.SubmissionEvidence[0].ArchivePath = "tests/evidence/proposals/1-demo/final-demo.txt"
			},
			wantErr: "demo_video",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			contract := submissionEvidenceContract()
			test.mutate(&contract)
			err := Validate(contract)
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("Validate() error = %v, want substring %q", err, test.wantErr)
			}
		})
	}
}

func TestValidateSubmissionEvidenceForChangeRequiresCommittedPackage(t *testing.T) {
	// TestValidateSubmissionEvidenceForChangeRequiresCommittedPackage verifies archive-time identity, file, and Git visibility gates.
	root := t.TempDir()
	initGitRepository(t, root)
	contract := submissionEvidenceContract()
	writeFile(t, root, contract.SubmissionEvidence[0].ArchivePath, "final demo video")
	writeFile(t, root, contract.SubmissionEvidence[1].ArchivePath, "final screenshot")
	writeFile(t, root, "tests/evidence/proposals/1-demo/README.md", "review instructions")
	writeFile(t, root, "tests/evidence/proposals/1-demo/manifest.json", `{"version":1,"change":"1-demo"}`)

	if err := ValidateSubmissionEvidenceForChange(root, contract, "1-demo"); err != nil {
		t.Fatalf("valid committed proposal evidence should pass: %v", err)
	}

	t.Run("legacy contract", func(t *testing.T) {
		legacy := submissionEvidenceContract()
		legacy.SubmissionEvidence = nil
		err := ValidateSubmissionEvidenceForChange(root, legacy, "1-demo")
		if err == nil || !strings.Contains(err.Error(), "必须声明 submission_evidence") {
			t.Fatalf("expected legacy archive rejection, got %v", err)
		}
	})

	t.Run("wrong change directory", func(t *testing.T) {
		wrongChange := submissionEvidenceContract()
		wrongChange.SubmissionEvidence[0].ArchivePath = "tests/evidence/proposals/2-other/final-demo.webm"
		wrongChange.SubmissionEvidence[1].ArchivePath = "tests/evidence/proposals/2-other/final.png"
		err := ValidateSubmissionEvidenceForChange(root, wrongChange, "1-demo")
		if err == nil || !strings.Contains(err.Error(), "当前 change") {
			t.Fatalf("expected change directory rejection, got %v", err)
		}
	})

	t.Run("missing archive file", func(t *testing.T) {
		missing := submissionEvidenceContract()
		missing.SubmissionEvidence[0].ArchivePath = "tests/evidence/proposals/1-demo/missing.webm"
		err := ValidateSubmissionEvidenceForChange(root, missing, "1-demo")
		if err == nil || !strings.Contains(err.Error(), "不存在") {
			t.Fatalf("expected missing archive rejection, got %v", err)
		}
	})

	t.Run("empty archive file", func(t *testing.T) {
		empty := submissionEvidenceContract()
		empty.SubmissionEvidence[0].ArchivePath = "tests/evidence/proposals/1-demo/empty.webm"
		writeFile(t, root, empty.SubmissionEvidence[0].ArchivePath, "")
		err := ValidateSubmissionEvidenceForChange(root, empty, "1-demo")
		if err == nil || !strings.Contains(err.Error(), "不能为空文件") {
			t.Fatalf("expected empty archive rejection, got %v", err)
		}
	})

	t.Run("archive file is symlink", func(t *testing.T) {
		symlink := submissionEvidenceContract()
		symlink.SubmissionEvidence[0].ArchivePath = "tests/evidence/proposals/1-demo/link.webm"
		linkPath := filepath.Join(root, filepath.FromSlash(symlink.SubmissionEvidence[0].ArchivePath))
		if err := os.Symlink(filepath.Join(root, filepath.FromSlash(contract.SubmissionEvidence[0].ArchivePath)), linkPath); err != nil {
			t.Fatal(err)
		}
		err := ValidateSubmissionEvidenceForChange(root, symlink, "1-demo")
		if err == nil || !strings.Contains(err.Error(), "普通文件") {
			t.Fatalf("expected symlink archive rejection, got %v", err)
		}
	})

	t.Run("ignored archive file", func(t *testing.T) {
		ignored := submissionEvidenceContract()
		writeFile(t, root, ".gitignore", "tests/evidence/proposals/1-demo/final-demo.webm\n")
		err := ValidateSubmissionEvidenceForChange(root, ignored, "1-demo")
		if err == nil || !strings.Contains(err.Error(), "不得被 Git ignore") {
			t.Fatalf("expected ignored archive rejection, got %v", err)
		}
	})
}

func TestValidateSubmissionEvidenceContractForChangeRejectsLegacyAndWrongDirectory(t *testing.T) {
	// TestValidateSubmissionEvidenceContractForChange keeps new runs from sealing an undeliverable contract.
	legacy := submissionEvidenceContract()
	legacy.SubmissionEvidence = nil
	if err := ValidateSubmissionEvidenceContractForChange(legacy, "1-demo"); err == nil ||
		!strings.Contains(err.Error(), "必须声明 submission_evidence") {
		t.Fatalf("expected legacy contract rejection, got %v", err)
	}

	wrong := submissionEvidenceContract()
	if err := ValidateSubmissionEvidenceContractForChange(wrong, "2-other"); err == nil ||
		!strings.Contains(err.Error(), "当前 change") {
		t.Fatalf("expected change-bound archive path rejection, got %v", err)
	}
}

func TestValidateSubmissionEvidenceForChangeRequiresLFSForLargeArtifacts(t *testing.T) {
	// TestValidateSubmissionEvidenceForChangeRequiresLFSForLargeArtifacts keeps videos reviewable without bloating normal Git objects.
	root := t.TempDir()
	initGitRepository(t, root)
	contract := submissionEvidenceContract()
	for _, item := range contract.SubmissionEvidence {
		writeFile(t, root, item.ArchivePath, "final evidence")
	}
	writeFile(t, root, "tests/evidence/proposals/1-demo/README.md", "review instructions")
	writeFile(t, root, "tests/evidence/proposals/1-demo/manifest.json", `{"version":1,"change":"1-demo"}`)
	videoPath := filepath.Join(root, filepath.FromSlash(contract.SubmissionEvidence[0].ArchivePath))
	if err := os.Truncate(videoPath, maxInlineSubmissionEvidenceBytes+1); err != nil {
		t.Fatal(err)
	}

	err := ValidateSubmissionEvidenceForChange(root, contract, "1-demo")
	if err == nil || !strings.Contains(err.Error(), "Git LFS") {
		t.Fatalf("expected large artifact LFS rejection, got %v", err)
	}

	writeFile(t, root, ".gitattributes", "tests/evidence/proposals/**/*.webm filter=lfs diff=lfs merge=lfs -text\n")
	if err := ValidateSubmissionEvidenceForChange(root, contract, "1-demo"); err != nil {
		t.Fatalf("large LFS-routed evidence should pass: %v", err)
	}
}

func submissionEvidenceContract() Contract {
	// submissionEvidenceContract builds a valid new-format contract with one video and one supporting screenshot.
	return Contract{
		Summary: "final proposal demonstration",
		RequiredTests: []Test{{
			ID:         "final-demo",
			Source:     "change_contract",
			Path:       "tests/final_demo_test.sh",
			Command:    "bash tests/final_demo_test.sh",
			Purpose:    "records the final proposal behavior",
			Assertions: []string{"the completed proposal is demonstrated through its real user workflow"},
		}},
		RequiredEvidence: []Evidence{
			{
				ID:      "final-demo-video",
				Kind:    "demo_video",
				Path:    "test-results/demo/final-demo.webm",
				Purpose: "shows the final proposal behavior from start to finish",
			},
			{
				ID:      "final-demo-screenshot",
				Kind:    "screenshot",
				Path:    "test-results/demo/final.png",
				Purpose: "shows the final proposal result",
			},
		},
		SubmissionEvidence: []SubmissionEvidence{
			{
				EvidenceID:  "final-demo-video",
				SourcePath:  "test-results/demo/final-demo.webm",
				ArchivePath: "tests/evidence/proposals/1-demo/final-demo.webm",
			},
			{
				EvidenceID:  "final-demo-screenshot",
				SourcePath:  "test-results/demo/final.png",
				ArchivePath: "tests/evidence/proposals/1-demo/final.png",
			},
		},
	}
}

func initGitRepository(t *testing.T, root string) {
	// initGitRepository creates the minimum repository needed for Git ignore validation.
	t.Helper()
	cmd := exec.Command("git", "-C", root, "init", "--quiet")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("initialize git repository: %v: %s", err, output)
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
