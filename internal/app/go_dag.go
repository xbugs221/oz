// Package app executes the sealed workflow through the built-in Go DAG scheduler.
package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"
)

var errGoDAGValidationRetry = errors.New("go-dag validation retry")

// StartGoDAGJSON creates a sealed run and executes the WorkflowSpec without external schedulers.
func (e *Engine) StartGoDAGJSON(ctx context.Context, changeName string, stdout io.Writer) error {
	state, err := e.createRun(changeName)
	if err != nil {
		return err
	}
	if err := writeRunnerState(stdout, state); err != nil {
		return err
	}
	flushWriter(stdout)
	if state.Engine != "go-dag" {
		return e.run(ctx, state)
	}
	if err := e.runGoDAG(ctx, state); err != nil {
		latest, loadErr := loadState(e.Repo, state.RunID)
		if loadErr != nil {
			latest = state
		}
		latest = failedState(latest, err)
		saveErr := saveState(e.Repo, latest)
		warnWorkflowWrite("save failed go-dag state", saveErr)
		writeErr := writeFailedRunnerState(stdout, latest, err)
		warnWorkflowWrite("write failed go-dag runner state", writeErr)
		return errors.Join(err, saveErr, writeErr)
	}
	return nil
}

// runGoDAG walks WorkflowSpec in dependency order and records durable node status.
func (e *Engine) runGoDAG(ctx context.Context, state State) error {
	unlock, err := acquireLock(e.Repo, state.RunID)
	if err != nil {
		return err
	}
	defer unlock()
	return e.runGoDAGLocked(ctx, state)
}

// runGoDAGLocked executes ready graph nodes concurrently while the caller owns the run lock.
func (e *Engine) runGoDAGLocked(ctx context.Context, state State) error {
	spec := BuildWorkflowSpec(state.ChangeName, state.Workflow)
	nodes := map[string]WorkflowNode{}
	remainingDeps := map[string]int{}
	outgoing := map[string][]string{}
	for _, node := range spec.Nodes {
		nodes[node.ID] = node
		remainingDeps[node.ID] = 0
	}
	for _, edge := range spec.Edges {
		outgoing[edge.From] = append(outgoing[edge.From], edge.To)
		remainingDeps[edge.To]++
	}
	completed := map[string]bool{}
	retries := map[string]int{}
	maxRetries := state.Workflow.Validation.MaxAttemptsPerStage
	if maxRetries < 1 {
		maxRetries = 3
	}
	for len(completed) < len(nodes) {
		var ready []WorkflowNode
		for _, node := range spec.Nodes {
			if !completed[node.ID] && remainingDeps[node.ID] == 0 {
				ready = append(ready, node)
			}
		}
		if len(ready) == 0 {
			return fmt.Errorf("go-dag workflow has unresolved dependencies")
		}
		results := make(chan goDAGNodeResult, len(ready))
		var wg sync.WaitGroup
		for _, node := range ready {
			wg.Add(1)
			go func(node WorkflowNode) {
				defer wg.Done()
				results <- goDAGNodeResult{nodeID: node.ID, err: e.runGoDAGNode(ctx, state.RunID, node)}
			}(node)
		}
		wg.Wait()
		close(results)
		for result := range results {
			if errors.Is(result.err, errGoDAGValidationRetry) {
				retries[result.nodeID]++
				if retries[result.nodeID] >= maxRetries {
					return fmt.Errorf("go-dag node %s validation retry limit reached", result.nodeID)
				}
				continue
			}
			if result.err != nil {
				return result.err
			}
			completed[result.nodeID] = true
			for _, next := range outgoing[result.nodeID] {
				remainingDeps[next]--
			}
		}
	}
	return nil
}

type goDAGNodeResult struct {
	nodeID string
	err    error
}

// runGoDAGNode executes one graph node by directly invoking the embedded Go helpers.
func (e *Engine) runGoDAGNode(ctx context.Context, runID string, node WorkflowNode) error {
	state, err := loadState(e.Repo, runID)
	if err != nil {
		return err
	}
	if state.Status != statusRunning || state.Stage != node.Stage {
		return nil
	}
	if e.goDAGShouldSkipCompletedExecutionContext(state, node) {
		return nil
	}
	e.recordGoDAGNode(runID, node.ID, DAGNodeState{Status: "running", StartedAt: time.Now().UTC().Format(time.RFC3339Nano)})
	var out bytes.Buffer
	switch node.Type {
	case "subagent":
		err = e.nodeRunSubagent(ctx, state, goDAGNodeArgs(node), &out)
	case "fanin":
		err = e.nodeFanin(state, goDAGNodeArgs(node), &out)
	case "gate":
		err = e.nodeGate(state, goDAGNodeArgs(node), &out)
	default:
		err = e.nodeRunStage(ctx, state, goDAGNodeArgs(node), &out)
	}
	next := DAGNodeState{Status: "success", FinishedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	if artifact := goDAGNodeArtifact(e.Repo, runID, node); artifact != "" && fileExists(artifact) {
		next.Artifact = artifact
	}
	if err != nil {
		next.Status = "failed"
		next.Error = err.Error()
		if terminalStatus, ok := e.goDAGNodeReachedTerminalBlock(runID); ok {
			next.Status = terminalStatus
			e.recordGoDAGNode(runID, node.ID, next)
			return nil
		}
		if e.goDAGShouldRetryNode(runID, node) {
			next.Status = "validation_failed"
			e.recordGoDAGNode(runID, node.ID, next)
			return errGoDAGValidationRetry
		}
		e.recordGoDAGNode(runID, node.ID, next)
		return err
	}
	e.recordGoDAGNode(runID, node.ID, next)
	return nil
}

// goDAGShouldSkipCompletedExecutionContext skips advisory execution helpers when task.md is already complete.
func (e *Engine) goDAGShouldSkipCompletedExecutionContext(state State, node WorkflowNode) bool {
	if node.Stage != "execution" || configGroupName(node.Group) != "implementation_context" {
		return false
	}
	if node.Type != "subagent" && node.Type != "fanin" {
		return false
	}
	done, err := ChangeTasksDone(e.Repo, state.ChangeName)
	return err == nil && done
}

// goDAGNodeReachedTerminalBlock reports non-failed workflow blocks created by node logic.
func (e *Engine) goDAGNodeReachedTerminalBlock(runID string) (string, bool) {
	state, err := loadState(e.Repo, runID)
	if err != nil {
		return "", false
	}
	switch state.Status {
	case statusValidationBlocked, statusAcceptanceContractBlocked:
		return state.Status, true
	default:
		return "", false
	}
}

// goDAGShouldRetryNode preserves runLoop validation semantics for the default scheduler.
func (e *Engine) goDAGShouldRetryNode(runID string, node WorkflowNode) bool {
	state, err := loadState(e.Repo, runID)
	if err != nil {
		return false
	}
	return node.Type == "main_stage" && state.Status == statusRunning && state.Stage == node.Stage && shouldForceStageRerun(state)
}

// recordGoDAGNode updates state.json with node-level progress for status/debugging.
func (e *Engine) recordGoDAGNode(runID string, nodeID string, nodeState DAGNodeState) {
	err := mergeState(e.Repo, runID, func(state *State) {
		if state.DAGNodes == nil {
			state.DAGNodes = map[string]DAGNodeState{}
		}
		current := state.DAGNodes[nodeID]
		if nodeState.StartedAt == "" {
			nodeState.StartedAt = current.StartedAt
		}
		state.DAGNodes[nodeID] = nodeState
	})
	warnWorkflowWrite("record go-dag node", err)
}

func goDAGNodeArgs(node WorkflowNode) []string {
	args := []string{"--stage", node.Stage, "--json"}
	if node.Group != "" {
		args = append(args, "--group", node.Group)
	}
	if node.Member != "" {
		args = append(args, "--member", node.Member)
	}
	if node.Iteration > 0 {
		args = append(args, "--iteration", fmt.Sprint(node.Iteration))
	}
	return args
}

func goDAGNodeArtifact(repo, runID string, node WorkflowNode) string {
	switch node.Type {
	case "subagent":
		return memberArtifactPath(repo, runID, configGroupName(node.Group), node.Iteration, node.Member)
	case "fanin":
		return parallelArtifactPath(runDir(repo, runID), configGroupName(node.Group), node.Iteration)
	default:
		return ""
	}
}

func goDAGOrder(spec WorkflowSpec) []WorkflowNode {
	nodes := map[string]WorkflowNode{}
	incoming := map[string]int{}
	outgoing := map[string][]string{}
	for _, node := range spec.Nodes {
		nodes[node.ID] = node
		incoming[node.ID] = 0
	}
	for _, edge := range spec.Edges {
		outgoing[edge.From] = append(outgoing[edge.From], edge.To)
		incoming[edge.To]++
	}
	var ready []string
	for _, node := range spec.Nodes {
		if incoming[node.ID] == 0 {
			ready = append(ready, node.ID)
		}
	}
	var ordered []WorkflowNode
	for len(ready) > 0 {
		id := ready[0]
		ready = ready[1:]
		ordered = append(ordered, nodes[id])
		for _, next := range outgoing[id] {
			incoming[next]--
			if incoming[next] == 0 {
				ready = append(ready, next)
			}
		}
	}
	return ordered
}
