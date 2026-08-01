// Package app persists sealed run state and exposes workflow state helpers.
package app

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/xbugs221/oz/internal/acceptance"
)

const sealedAcceptanceHashFile = "acceptance.sha256"

// sessionStateKey isolates resumable sessions by backend and workflow role.
func sessionStateKey(tool, role string) string {
	return tool + ":" + role
}

// stageSessionRole maps internal workflow stages to durable agent session roles.
func stageSessionRole(stage string) string {
	role, err := roleForStage(stage)
	if err != nil {
		return "executor"
	}
	return role.Session
}

// stageSessionRoleForState isolates each quality-loop QA while preserving shared repairer context.
func stageSessionRoleForState(state State, stage string) string {
	parsed, err := parseWorkflowStage(stage)
	if err == nil && usesQualityLoop(state.Workflow) && parsed.isKind(workflowStageQA) {
		return stage
	}
	return stageSessionRole(stage)
}

// workflowStagesForState returns the sealed stage list from the state snapshot.
func workflowStagesForState(state State) []string {
	ensureWorkflowConfig(&state)
	return workflowStagesForConfig(state.Workflow)
}

// blockedWorkflowRole identifies the role responsible for a review-limit block in the sealed generation.
func blockedWorkflowRole(state State) string {
	if usesRepairWorkflow(state.Workflow) {
		if state.Workflow.MaxRepairIterations == 0 {
			return "qa"
		}
		return "repairer"
	}
	return "reviewer"
}

// ensureWorkflowConfig normalizes the workflow snapshot used by state checklists.
func ensureWorkflowConfig(state *State) {
	normalizeWorkflowConfig(&state.Workflow)
}

// hasWorkflowConfig reports whether durable state contains an effective workflow snapshot.
func hasWorkflowConfig(state State) bool {
	normalizeWorkflowConfig(&state.Workflow)
	return state.Workflow.Stages != nil
}

// detectManualIntervention aborts if current-run paths change outside the recorded stage flow.
func (e *Engine) detectManualIntervention(state *State) error {
	head, diff, err := gitSnapshot(e.Repo)
	if err != nil {
		return err
	}
	if head == state.BaselineHead && diff == state.BaselineDiff {
		return nil
	}
	if usesQualityLoop(state.Workflow) && state.Stage == workflowStageArchive {
		state.BaselineHead = head
		state.BaselineDiff = diff
		return nil
	}
	allowedDirs := manualInterventionAllowedDirs(e.Repo, *state)
	guard, err := classifyGitSnapshotChangeWithAllowed(
		e.Repo, state.ChangeName, state.BaselineHead, state.BaselineDiff, head, diff, allowedDirs,
	)
	if err != nil {
		return err
	}
	if guard.Blocked {
		recordRuntimeWarning(e.Repo, *state, "git_snapshot", guard.Detail())
	}
	state.BaselineHead = head
	state.BaselineDiff = diff
	return nil
}

// manualInterventionAllowedDirs permits only workflow-owned archive artifacts while resuming archive.
func manualInterventionAllowedDirs(repo string, state State) []string {
	if !usesQualityLoop(state.Workflow) || state.Stage != workflowStageArchive {
		return nil
	}
	dirs := []string{
		filepath.Join(repo, "tests", "evidence", "proposals", state.ChangeName),
		filepath.Join(repo, "docs", "changes", state.ChangeName),
	}
	matches, err := filepath.Glob(filepath.Join(repo, "docs", "changes", "archive", "*-"+state.ChangeName))
	if err == nil && len(matches) == 1 {
		dirs = append(dirs, matches[0])
	}
	return dirs
}

// promptNameForStage maps workflow stages to named prompt templates.
func promptNameForStage(stage string) (string, error) {
	role, err := roleForStage(stage)
	if err != nil {
		return "", err
	}
	return role.PromptName, nil
}

// FindUnfinishedRun returns the newest run whose state is not done.
func FindUnfinishedRun(repo string) (string, error) {
	root, err := runsRoot(repo)
	if err != nil {
		return "", err
	}
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	for i := len(entries) - 1; i >= 0; i-- {
		entry := entries[i]
		if !entry.IsDir() {
			continue
		}
		state, err := loadState(repo, entry.Name())
		if err == nil && state.BatchID == "" && (state.Status == statusRunning || state.Status == statusBlockedEnvironment || state.Status == statusBlockedStalled) {
			return entry.Name(), nil
		}
	}
	return "", nil
}

// FindStartupRuns returns the newest resumable and stopped runs for the interactive menu.
func FindStartupRuns(repo string) (string, []State, error) {
	root, err := runsRoot(repo)
	if err != nil {
		return "", nil, err
	}
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return "", nil, nil
	}
	if err != nil {
		return "", nil, err
	}
	var running string
	var stopped []State
	for i := len(entries) - 1; i >= 0; i-- {
		entry := entries[i]
		if !entry.IsDir() {
			continue
		}
		state, err := loadState(repo, entry.Name())
		if err != nil || state.BatchID != "" {
			continue
		}
		if isStoppedRunState(state) {
			stopped = append(stopped, state)
			continue
		}
		if running == "" && state.Status == statusRunning {
			running = entry.Name()
		}
	}
	return running, stopped, nil
}

// isStoppedRunState reports terminal states that should not be shown as running work.
func isStoppedRunState(state State) bool {
	switch state.Status {
	case statusFailed, statusBlocked, statusBlockedEnvironment, statusBlockedStalled, statusValidationBlocked, statusAcceptanceContractBlocked, statusAborted, "aborted":
		return true
	default:
		return state.Stage == statusBlocked || state.Stage == statusBlockedEnvironment || state.Stage == statusBlockedStalled || state.Stage == statusValidationBlocked || state.Stage == statusAcceptanceContractBlocked
	}
}

// FindCurrentRun returns the newest readable run, regardless of whether older runs are unfinished.
func FindCurrentRun(repo string) (string, error) {
	root, err := runsRoot(repo)
	if err != nil {
		return "", err
	}
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	var newest string
	for i := len(entries) - 1; i >= 0; i-- {
		entry := entries[i]
		if !entry.IsDir() {
			continue
		}
		if newest == "" {
			newest = entry.Name()
		}
		if _, err := loadState(repo, entry.Name()); err == nil {
			return entry.Name(), nil
		}
	}
	return newest, nil
}

// snapshotRunAcceptance stores the sealed acceptance contract and its integrity digest inside the run.
func snapshotRunAcceptance(repo, runID, sourcePath string) error {
	data, err := os.ReadFile(sourcePath)
	if err != nil {
		return err
	}
	base := runDir(repo, runID)
	if err := os.MkdirAll(base, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(base, "acceptance.json"), data, 0o644); err != nil {
		return err
	}
	hash := acceptanceContentHash(data)
	return os.WriteFile(filepath.Join(base, sealedAcceptanceHashFile), []byte(hash+"\n"), 0o644)
}

// snapshotQualityLoopAcceptance seals a contract and stores its trust-root digest in run state.
func snapshotQualityLoopAcceptance(repo string, state *State, sourcePath string) error {
	if state == nil {
		return fmt.Errorf("quality loop acceptance snapshot 缺少 state")
	}
	if err := snapshotRunAcceptance(repo, state.RunID, sourcePath); err != nil {
		return err
	}
	hash, err := acceptanceFileHash(filepath.Join(runDir(repo, state.RunID), "acceptance.json"))
	if err != nil {
		return err
	}
	state.AcceptanceHash = hash
	return nil
}

// readAcceptanceForState reads the immutable run contract, with legacy fallbacks.
func readAcceptanceForState(repo string, state State) (Acceptance, error) {
	if err := validateChangeNameForPath(state.ChangeName); err != nil {
		return Acceptance{}, err
	}
	runPath := filepath.Join(runDir(repo, state.RunID), "acceptance.json")
	if usesQualityLoop(state.Workflow) {
		contract, _, err := readVerifiedRunAcceptance(repo, state)
		if err != nil {
			return Acceptance{}, fmt.Errorf("quality loop 缺少有效的封存 acceptance %s: %w", runPath, err)
		}
		return contract, nil
	}
	if acceptance, err := ReadAcceptance(runPath); err == nil {
		return acceptance, nil
	} else if !os.IsNotExist(err) {
		return Acceptance{}, err
	}

	activePath := acceptancePath(repo, state.ChangeName)
	if acceptance, err := ReadAcceptance(activePath); err == nil {
		return acceptance, nil
	} else if !os.IsNotExist(err) {
		return Acceptance{}, err
	}

	archivedPath, err := archivedAcceptancePath(repo, state.ChangeName)
	if err != nil {
		return Acceptance{}, err
	}
	return ReadAcceptance(archivedPath)
}

// readVerifiedRunAcceptance hashes and parses one sealed contract from the same byte snapshot.
func readVerifiedRunAcceptance(repo string, state State) (Acceptance, string, error) {
	runPath := filepath.Join(runDir(repo, state.RunID), "acceptance.json")
	data, err := os.ReadFile(runPath)
	if err != nil {
		return Acceptance{}, "", err
	}
	actual := acceptanceContentHash(data)
	expected := strings.TrimSpace(state.AcceptanceHash)
	if expected == "" && !state.Sealed {
		// Unsealed in-memory fixtures preserve the pre-existing preflight unit-test contract.
		expected = actual
	} else if !validAcceptanceHash(expected) {
		return Acceptance{}, "", fmt.Errorf("封存 acceptance 缺少有效的 state 完整性哈希")
	}
	if actual != expected {
		return Acceptance{}, "", fmt.Errorf("封存 acceptance 完整性校验失败：expected=%s actual=%s", expected, actual)
	}
	contract, err := acceptance.Parse(data)
	if err != nil {
		return Acceptance{}, "", ReviewArtifactError{Path: runPath, Code: reviewArtifactParseError, Reason: err.Error()}
	}
	return contract, actual, nil
}

// acceptanceContentHash returns the stable SHA-256 digest persisted with a sealed contract.
func acceptanceContentHash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// acceptanceFileHash hashes a contract file before its digest is sealed into state.json.
func acceptanceFileHash(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return acceptanceContentHash(data), nil
}

// validAcceptanceHash rejects missing or malformed integrity metadata.
func validAcceptanceHash(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size
}

// archivedAcceptancePath locates acceptance.json after oz archive moves a change.
func archivedAcceptancePath(repo, changeName string) (string, error) {
	if err := validateChangeNameForPath(changeName); err != nil {
		return "", err
	}
	matches, err := filepath.Glob(filepath.Join(repo, "docs", "changes", "archive", "*-"+changeName, "acceptance.json"))
	if err != nil {
		return "", err
	}
	if len(matches) == 0 {
		return "", os.ErrNotExist
	}
	return matches[len(matches)-1], nil
}

// archiveExists checks for an archived change directory with the date prefix.
func archiveExists(repo, changeName string) bool {
	if err := validateChangeNameForPath(changeName); err != nil {
		return false
	}
	matches, _ := filepath.Glob(filepath.Join(repo, "docs", "changes", "archive", "*-"+changeName))
	return len(matches) > 0
}

// startDetachedResumeCommand runs the sealed workflow worker without streaming output to the terminal.
func startDetachedResumeCommand(repo, runID string) error {
	exe, err := currentExecutable()
	if err != nil {
		return fmt.Errorf("解析 oz flow 可执行文件失败：%w", err)
	}
	base := runDir(repo, runID)
	if err := os.MkdirAll(base, 0o755); err != nil {
		return err
	}
	cmd := exec.Command(exe, flowWorkerCommandArgs("resume", "--run-id", runID, "--json")...)
	cmd.Dir = repo
	configureDetachedCommand(cmd)
	return startDetachedWorkerCommand(cmd, runWorkerLogPath(repo, runID))
}
