// Package app validates QA artifacts from CLI.
package app

import (
	"fmt"
	"io"
	"strings"
)

type qaValidationResult struct {
	Path             string `json:"path"`
	AcceptancePath   string `json:"acceptance_path"`
	Valid            bool   `json:"valid"`
	Decision         string `json:"decision,omitempty"`
	Findings         int    `json:"findings,omitempty"`
	AcceptanceMatrix int    `json:"acceptance_matrix,omitempty"`
	Error            string `json:"error,omitempty"`
	Code             string `json:"code,omitempty"`
}

// runValidateQAArtifact checks one QA artifact against its acceptance contract.
func runValidateQAArtifact(args []string, stdout io.Writer) error {
	if !hasFlag(args, "--artifact") || !hasFlag(args, "--acceptance") {
		return fmt.Errorf("用法：oz flow validate-qa --artifact <artifact-path> --acceptance <acceptance-path> [--json]")
	}
	path, err := requireFlagValue(args, "--artifact")
	if err != nil {
		return err
	}
	acceptancePath, err := requireFlagValue(args, "--acceptance")
	if err != nil {
		return err
	}
	path = strings.TrimSpace(path)
	acceptancePath = strings.TrimSpace(acceptancePath)
	if path == "" {
		return fmt.Errorf("qa artifact 路径不能为空")
	}
	if acceptancePath == "" {
		return fmt.Errorf("acceptance 路径不能为空")
	}

	qa, err := validateQAArtifact(path, acceptancePath)
	if hasFlag(args, "--json") {
		if err != nil {
			result := qaValidationResult{Path: path, AcceptancePath: acceptancePath, Valid: false}
			if ar, ok := err.(ReviewArtifactError); ok {
				result.Error = ar.Reason
				result.Code = ar.Code
			} else {
				result.Error = err.Error()
				result.Code = "readError"
			}
			_ = writeJSON(stdout, result)
			return err
		}
		return writeJSON(stdout, qaValidationResult{
			Path:             path,
			AcceptancePath:   acceptancePath,
			Valid:            true,
			Decision:         qa.Decision,
			Findings:         len(qa.Findings),
			AcceptanceMatrix: len(qa.AcceptanceMatrix),
		})
	}

	if err != nil {
		if ar, ok := err.(ReviewArtifactError); ok {
			return fmt.Errorf("%s: %s", ar.Code, ar.Reason)
		}
		return err
	}
	fmt.Fprintf(stdout, "qa artifact 合法: %s (decision=%s, findings=%d, acceptance_matrix=%d)\n", path, qa.Decision, len(qa.Findings), len(qa.AcceptanceMatrix))
	return nil
}

// validateQAArtifact loads QA and acceptance files, then enforces their shared contract.
func validateQAArtifact(path, acceptancePath string) (QA, error) {
	qa, err := ReadQA(path)
	if err != nil {
		return QA{}, err
	}
	acceptance, err := ReadAcceptance(acceptancePath)
	if err != nil {
		if ar, ok := err.(ReviewArtifactError); ok {
			return QA{}, ar
		}
		return QA{}, ReviewArtifactError{Path: acceptancePath, Code: reviewArtifactParseError, Reason: err.Error()}
	}
	if err := ValidateQAAgainstAcceptance(qa, acceptance); err != nil {
		return QA{}, ReviewArtifactError{Path: path, Code: reviewArtifactValidationError, Reason: err.Error()}
	}
	return qa, nil
}
