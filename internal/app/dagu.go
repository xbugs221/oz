// Package app keeps legacy Dagu helpers for historical run-node compatibility.
package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type nodeResult struct {
	Status   string `json:"status"`
	RunID    string `json:"run_id"`
	Stage    string `json:"stage,omitempty"`
	Group    string `json:"group,omitempty"`
	Member   string `json:"member,omitempty"`
	Artifact string `json:"artifact,omitempty"`
}

// StartDaguJSON creates a sealed run and delegates node scheduling to Dagu.
func (e *Engine) StartDaguJSON(ctx context.Context, changeName string, stdout io.Writer) error {
	daguPath, err := exec.LookPath("dagu")
	if err != nil {
		return fmt.Errorf("缺少 Dagu CLI：请安装 dagu 后再运行 --engine dagu")
	}
	state, err := e.createRun(changeName)
	if err != nil {
		return err
	}
	if err := writeRunnerState(stdout, state); err != nil {
		return err
	}
	flushWriter(stdout)
	workflowPath, err := e.writeRunDaguWorkflow(state)
	if err != nil {
		return err
	}
	if err := e.runDaguProcess(ctx, daguPath, workflowPath, state.RunID); err != nil {
		latest, loadErr := loadState(e.Repo, state.RunID)
		if loadErr != nil {
			latest = state
		}
		latest = failedState(latest, err)
		_ = saveState(e.Repo, latest)
		_ = writeFailedRunnerState(stdout, latest, err)
		return err
	}
	latest, err := loadState(e.Repo, state.RunID)
	if err != nil {
		return err
	}
	return writeRunnerState(stdout, latest)
}

// RunNode dispatches one Dagu node subcommand.
func (e *Engine) RunNode(ctx context.Context, args []string, stdout io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("用法：wo node run-subagent|fanin|run-stage|gate --run-id <run-id> --json")
	}
	if !hasFlag(args[1:], "--json") {
		return fmt.Errorf("wo node 必须使用 --json")
	}
	runID, err := requireFlagValue(args[1:], "--run-id")
	if err != nil {
		return err
	}
	state, err := loadState(e.Repo, runID)
	if err != nil {
		return err
	}
	if !hasWorkflowConfig(state) {
		return fmt.Errorf("run %s 缺少 workflow_config 快照", runID)
	}
	normalizeWorkflowConfig(&state.Workflow)
	switch args[0] {
	case "run-subagent":
		return e.nodeRunSubagent(ctx, state, args[1:], stdout)
	case "fanin":
		return e.nodeFanin(state, args[1:], stdout)
	case "run-stage":
		return e.nodeRunStage(ctx, state, args[1:], stdout)
	case "gate":
		return e.nodeGate(state, args[1:], stdout)
	default:
		return fmt.Errorf("未知 wo node 子命令 %q", args[0])
	}
}

// writeRunDaguWorkflow stores executable YAML under the run directory.
func (e *Engine) writeRunDaguWorkflow(state State) (string, error) {
	spec := BuildWorkflowSpec(state.ChangeName, state.Workflow)
	path := filepath.Join(runDir(e.Repo, state.RunID), "dagu", "workflow.yaml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(ExportRunWorkflowDaguYAML(spec, state.RunID)), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// runDaguProcess invokes the external Dagu process and persists its logs.
func (e *Engine) runDaguProcess(ctx context.Context, daguPath, workflowPath, runID string) error {
	cmd := commandContext(ctx, daguPath, "start", workflowPath)
	cmd.Dir = e.Repo
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	logPath := filepath.Join(runDir(e.Repo, runID), "dagu", "dagu.log")
	_ = os.MkdirAll(filepath.Dir(logPath), 0o755)
	_ = os.WriteFile(logPath, out.Bytes(), 0o644)
	if err != nil {
		return fmt.Errorf("Dagu 执行失败：%w\n%s", err, limitAgentDiagnostics(out.String()))
	}
	return nil
}

// nodeRunStage runs one activated main stage and validates its artifact.
func (e *Engine) nodeRunStage(ctx context.Context, state State, args []string, stdout io.Writer) error {
	stage, err := requireFlagValue(args, "--stage")
	if err != nil {
		return err
	}
	if state.Status != statusRunning || state.Stage != stage {
		return writeNodeResult(stdout, nodeResult{Status: "skipped", RunID: state.RunID, Stage: stage})
	}
	forceRun := shouldForceStageRerun(state)
	done, err := e.nodeStageDone(state)
	if err != nil && !isStageArtifactGateError(err) {
		return e.failNodeState(state, err)
	}
	if !done || forceRun {
		if err := e.detectManualIntervention(&state); err != nil {
			return err
		}
		if err := e.runStage(ctx, &state); err != nil {
			return e.failNodeState(state, err)
		}
	}
	done, err = e.nodeStageDone(state)
	if err != nil {
		if isStageArtifactGateError(err) {
			if recordErr := recordStageArtifactGateFailure(e.Repo, &state, err); recordErr != nil {
				return recordErr
			}
			if saveErr := saveState(e.Repo, state); saveErr != nil {
				return saveErr
			}
			return err
		}
		return e.failNodeState(state, err)
	}
	if !done {
		return e.failNodeState(state, fmt.Errorf("%s 阶段 artifact 未完成", stage))
	}
	validationPassed, err := e.validateStage(ctx, &state)
	if err != nil {
		return e.failNodeState(state, err)
	}
	if !validationPassed {
		if err := saveState(e.Repo, state); err != nil {
			return err
		}
		return fmt.Errorf("%s validation 未通过", stage)
	}
	if stage == "execution" || strings.HasPrefix(stage, "fix_") || stage == "archive" {
		if err := e.advance(&state); err != nil {
			if isStageArtifactGateError(err) {
				_ = recordStageArtifactGateFailure(e.Repo, &state, err)
				return err
			}
			return e.failNodeState(state, err)
		}
	}
	if err := saveState(e.Repo, state); err != nil {
		return err
	}
	return writeNodeResult(stdout, nodeResult{Status: "completed", RunID: state.RunID, Stage: stage})
}

// nodeStageDone checks stage-local output without consuming Dagu gate decisions.
func (e *Engine) nodeStageDone(state State) (bool, error) {
	base := runDir(e.Repo, state.RunID)
	switch {
	case strings.HasPrefix(state.Stage, "review_"):
		n := strings.TrimPrefix(state.Stage, "review_")
		_, err := ReadReview(filepath.Join(base, "review-"+n+".json"))
		if os.IsNotExist(err) {
			return false, nil
		}
		return err == nil, err
	case strings.HasPrefix(state.Stage, "qa_"):
		n := strings.TrimPrefix(state.Stage, "qa_")
		qa, err := ReadQA(filepath.Join(base, "qa-"+n+".json"))
		if os.IsNotExist(err) {
			return false, nil
		}
		if err != nil {
			return false, newStageArtifactGateError(err)
		}
		acceptance, err := readAcceptanceForState(e.Repo, state)
		if err != nil {
			return false, err
		}
		if err := ValidateQAAgainstAcceptance(qa, acceptance); err != nil {
			return false, newStageArtifactGateError(err)
		}
		return true, nil
	default:
		return e.artifactDone(state)
	}
}

// nodeGate advances durable workflow state after a completed stage.
func (e *Engine) nodeGate(state State, args []string, stdout io.Writer) error {
	stage, err := requireFlagValue(args, "--stage")
	if err != nil {
		return err
	}
	if state.Status != statusRunning || state.Stage != stage {
		return writeNodeResult(stdout, nodeResult{Status: "skipped", RunID: state.RunID, Stage: stage})
	}
	done, err := e.artifactDone(state)
	if err != nil {
		return e.failNodeState(state, err)
	}
	if !done {
		if stage == "archive" {
			return writeNodeResult(stdout, nodeResult{Status: "skipped", RunID: state.RunID, Stage: stage})
		}
		return e.failNodeState(state, fmt.Errorf("%s gate 前 artifact 未完成", stage))
	}
	if err := e.advance(&state); err != nil {
		return e.failNodeState(state, err)
	}
	if err := saveState(e.Repo, state); err != nil {
		return err
	}
	return writeNodeResult(stdout, nodeResult{Status: "completed", RunID: state.RunID, Stage: stage})
}

// nodeFanin combines all member artifacts into the existing parallel schema.
func (e *Engine) nodeFanin(state State, args []string, stdout io.Writer) error {
	groupName, err := requireFlagValue(args, "--group")
	if err != nil {
		return err
	}
	stage, err := requireFlagValue(args, "--stage")
	if err != nil {
		return err
	}
	iteration := nodeIteration(args, stage)
	if state.Status != statusRunning || state.Stage != stage {
		return writeNodeResult(stdout, nodeResult{Status: "skipped", RunID: state.RunID, Stage: stage, Group: groupName})
	}
	configName := configGroupName(groupName)
	group, ok := state.Workflow.Parallel.Groups[configName]
	if !state.Workflow.Parallel.Enabled || !ok {
		return writeNodeResult(stdout, nodeResult{Status: "skipped", RunID: state.RunID, Stage: stage, Group: groupName})
	}
	artifact := ParallelArtifact{Group: configName, Mode: group.Mode, Summary: configName + " fanin completed"}
	for _, member := range group.Members {
		result, err := readMemberArtifact(memberArtifactPath(e.Repo, state.RunID, configName, iteration, member.Name))
		if err != nil {
			return e.failNodeState(state, err)
		}
		result.Required = member.Required
		artifact.Members = append(artifact.Members, result)
	}
	if err := ValidateParallelArtifact(artifact); err != nil {
		return e.failNodeState(state, err)
	}
	if err := ValidateParallelArtifactForGroup(artifact, configName, group); err != nil {
		return e.failNodeState(state, err)
	}
	path := parallelArtifactPath(runDir(e.Repo, state.RunID), configName, iteration)
	if err := writeJSONFile(path, artifact); err != nil {
		return e.failNodeState(state, err)
	}
	return writeNodeResult(stdout, nodeResult{Status: "completed", RunID: state.RunID, Stage: stage, Group: groupName, Artifact: path})
}

// failNodeState records node failures in durable run state.
func (e *Engine) failNodeState(state State, err error) error {
	state = failedState(state, err)
	_ = saveState(e.Repo, state)
	return err
}

func writeJSONFile(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func writeNodeResult(stdout io.Writer, result nodeResult) error {
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return err
	}
	_, err = stdout.Write(append(data, '\n'))
	return err
}
