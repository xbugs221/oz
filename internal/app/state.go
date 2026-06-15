// Package app persists sealed run state and exposes workflow state helpers.
package app

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

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

// workflowStagesForState returns the sealed stage list from the state snapshot.
func workflowStagesForState(state State) []string {
	ensureWorkflowConfig(&state)
	return workflowStagesForConfig(state.Workflow)
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
	guard, err := classifyGitSnapshotChange(e.Repo, state.ChangeName, state.BaselineHead, state.BaselineDiff, head, diff)
	if err != nil {
		return err
	}
	if guard.Blocked {
		state.Status = statusAborted
		if err := saveState(e.Repo, *state); err != nil {
			return err
		}
		return fmt.Errorf("在 %s 阶段前检测到当前 run 相关路径或源码变化：%s", state.Stage, guard.Detail())
	}
	state.BaselineHead = head
	state.BaselineDiff = diff
	return nil
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
		if err == nil && state.BatchID == "" && state.Status == statusRunning {
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
	case statusFailed, statusBlocked, statusValidationBlocked, statusAcceptanceContractBlocked, statusAborted, "aborted":
		return true
	default:
		return state.Stage == statusBlocked || state.Stage == statusValidationBlocked || state.Stage == statusAcceptanceContractBlocked
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

// snapshotRunAcceptance stores the sealed acceptance contract inside the run.
func snapshotRunAcceptance(repo, runID, sourcePath string) error {
	data, err := os.ReadFile(sourcePath)
	if err != nil {
		return err
	}
	base := runDir(repo, runID)
	if err := os.MkdirAll(base, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(base, "acceptance.json"), data, 0o644)
}

// readAcceptanceForState reads the immutable run contract, with legacy fallbacks.
func readAcceptanceForState(repo string, state State) (Acceptance, error) {
	if err := validateChangeNameForPath(state.ChangeName); err != nil {
		return Acceptance{}, err
	}
	runPath := filepath.Join(runDir(repo, state.RunID), "acceptance.json")
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
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}
