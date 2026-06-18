// Package app wraps Pi CLI JSONL execution for sealed workflow stages.
package app

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// PiTool adapts Pi Coding Agent CLI to the generic agent backend contract.
type PiTool struct{}

// Name returns the config name for the Pi backend.
func (PiTool) Name() string { return "pi" }

// Resolve verifies Pi is available on PATH.
func (PiTool) Resolve() error {
	_, err := resolveCommand("pi")
	return err
}

// PlanningCommand starts Pi interactively with the rendered planning prompt.
func (PiTool) PlanningCommand(ctx context.Context, _ string, prompt string, stdin io.Reader, options StageOptions) (*exec.Cmd, error) {
	path, err := resolveCommand("pi")
	if err != nil {
		return nil, err
	}
	cmd := commandContext(ctx, path, piPlanningArgs(prompt, options)...)
	cmd.Stdin = stdin
	return cmd, nil
}

// NewRunner returns a Pi sealed-run runner.
func (PiTool) NewRunner() AgentRunner {
	path, err := resolveCommand("pi")
	return &PiCLI{Path: path, ResolveErr: err}
}

// PiCLI invokes the real pi executable.
type PiCLI struct {
	Path       string
	ResolveErr error
	Progress   io.Writer
	Artifact   *artifactCapture
}

// SetProgress redirects concise process progress for callers that own the UI.
func (p *PiCLI) SetProgress(progress io.Writer) {
	p.Progress = progress
}

// SetArtifactCapture records assistant text for read-only subagent artifact materialization.
func (p *PiCLI) SetArtifactCapture(capture *artifactCapture) {
	p.Artifact = capture
}

// Run executes pi in JSON mode, extracts session metadata, and waits for process exit.
func (p PiCLI) Run(ctx context.Context, repo, prompt, sessionID string, options StageOptions) (string, error) {
	if p.ResolveErr != nil {
		return "", p.ResolveErr
	}
	if p.Path == "" {
		return "", fmt.Errorf("找不到 pi 可执行文件")
	}
	args := piRunArgs(prompt, sessionID, options)
	cmd := commandContext(ctx, p.Path, args...)
	cmd.Dir = repo
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return "", err
	}
	printAgentProcessStarted(p.Progress, "pi", cmd.Process.Pid)
	observed, drainErr, waitErr := p.waitPiJSONLCommand(ctx, cmd, stdout)
	if drainErr != nil {
		return observed, drainErr
	}
	if waitErr != nil {
		stderrText := strings.TrimSpace(limitAgentDiagnostics(stderr.String()))
		if stderrText == "" {
			return observed, waitErr
		}
		return observed, fmt.Errorf("%w；stderr：%s", waitErr, stderrText)
	}
	return observed, nil
}

// waitPiJSONLCommand drains Pi JSONL with an output-idle watchdog so stuck backends release the workflow.
func (p PiCLI) waitPiJSONLCommand(ctx context.Context, cmd *exec.Cmd, stdout io.Reader) (string, error, error) {
	type drainResult struct {
		sessionID string
		err       error
	}
	touch := make(chan struct{}, 1)
	drained := make(chan drainResult, 1)
	waited := make(chan error, 1)
	var observedMu sync.Mutex
	observed := ""
	setObserved := func(id string) {
		observedMu.Lock()
		observed = id
		observedMu.Unlock()
	}
	getObserved := func() string {
		observedMu.Lock()
		defer observedMu.Unlock()
		return observed
	}
	go func() {
		sessionID, err := drainPiJSONLWithCaptureNotify(stdout, p.Progress, p.Artifact, func() {
			select {
			case touch <- struct{}{}:
			default:
			}
		}, setObserved)
		drained <- drainResult{sessionID: sessionID, err: err}
	}()
	go func() {
		waited <- cmd.Wait()
	}()
	timer := time.NewTimer(agentOutputIdleTimeout)
	defer timer.Stop()
	drainDone := false
	waitDone := false
	var drainErr error
	var waitErr error
	for !drainDone || !waitDone {
		select {
		case <-ctx.Done():
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
			return getObserved(), ctx.Err(), nil
		case <-touch:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(agentOutputIdleTimeout)
		case result := <-drained:
			drainDone = true
			if result.sessionID != "" {
				setObserved(result.sessionID)
			}
			drainErr = result.err
		case err := <-waited:
			waitDone = true
			waitErr = err
			exitCode := -1
			if cmd.ProcessState != nil {
				exitCode = cmd.ProcessState.ExitCode()
			}
			printAgentProcessExited(p.Progress, "pi", cmd.Process.Pid, exitCode)
		case <-timer.C:
			if cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
			return getObserved(), fmt.Errorf("%w: pi %s 内没有新输出，已终止本次进程并准备续跑", errGoDAGRetryableNode, agentOutputIdleTimeout), nil
		}
	}
	return getObserved(), drainErr, waitErr
}

// piPlanningArgs builds interactive planning arguments using Pi option names.
func piPlanningArgs(prompt string, options StageOptions) []string {
	args := piCommonArgs(options)
	return append(args, agentPromptText(prompt))
}

// piRunArgs builds shell-free sealed-run arguments for Pi JSON mode.
func piRunArgs(prompt, sessionID string, options StageOptions) []string {
	args := []string{"--mode", "json"}
	args = append(args, piCommonArgs(options)...)
	if sessionID != "" {
		args = append(args, "--session", sessionID)
	}
	return append(args, agentPromptText(prompt))
}

// piCommonArgs maps shared stage options to Pi-supported CLI flags.
func piCommonArgs(options StageOptions) []string {
	var args []string
	if options.Model != "" {
		args = append(args, "--model", options.Model)
	}
	if options.Reasoning != "" {
		args = append(args, "--thinking", options.Reasoning)
	}
	switch options.Permissions {
	case "danger-full-access":
		args = append(args, "--tools", "read,bash,edit,write,grep,find,ls")
	case "sandbox":
		args = append(args, "--tools", "read,grep,find,ls")
	}
	return args
}

// piSessionEvent is the Pi JSON session header needed for workflow control.
type piSessionEvent struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

// drainPiJSONL reads stdout while best-effort extracting Pi session metadata.
func drainPiJSONL(stdout io.Reader, progress io.Writer) (sessionID string, err error) {
	return drainPiJSONLWithCapture(stdout, progress, nil)
}

// drainPiJSONLWithCapture reads stdout while best-effort extracting Pi session metadata and final text.
func drainPiJSONLWithCapture(stdout io.Reader, progress io.Writer, capture *artifactCapture) (sessionID string, err error) {
	return drainPiJSONLWithCaptureNotify(stdout, progress, capture, nil, nil)
}

// drainPiJSONLWithCaptureNotify reports each output line and session id to the caller.
func drainPiJSONLWithCaptureNotify(stdout io.Reader, progress io.Writer, capture *artifactCapture, touch func(), session func(string)) (sessionID string, err error) {
	reader := bufio.NewReaderSize(stdout, 64*1024)
	for {
		line, readErr := reader.ReadBytes('\n')
		if len(line) > 0 {
			if touch != nil {
				touch()
			}
			if id := piSessionIDFromLine(line, progress); id != "" {
				sessionID = id
				if session != nil {
					session(id)
				}
			}
			capturePiText(line, capture)
		}
		if readErr == nil {
			continue
		}
		if readErr != io.EOF && err == nil {
			err = readErr
		}
		return sessionID, err
	}
}

// capturePiText stores assistant text while ignoring tool read results.
func capturePiText(line []byte, capture *artifactCapture) {
	if capture == nil {
		return
	}
	var event map[string]interface{}
	if err := json.Unmarshal(line, &event); err != nil {
		return
	}
	if message, ok := event["message"].(map[string]interface{}); ok {
		if role, _ := message["role"].(string); role != "" && role != "assistant" {
			return
		}
		appendPiAssistantContent(capture, message["content"])
		return
	}
	if role, _ := event["role"].(string); role != "" && role != "assistant" {
		return
	}
	for _, field := range []string{"content", "text", "delta", "message"} {
		appendPiTextValue(capture, event[field])
	}
}

func appendPiAssistantContent(capture *artifactCapture, value interface{}) {
	switch v := value.(type) {
	case string:
		capture.Append(v)
	case []interface{}:
		for _, item := range v {
			obj, ok := item.(map[string]interface{})
			if !ok {
				appendPiAssistantContent(capture, item)
				continue
			}
			if typ, _ := obj["type"].(string); typ != "" && typ != "text" {
				continue
			}
			appendPiTextValue(capture, obj["text"])
		}
	default:
		appendPiTextValue(capture, value)
	}
}

func appendPiTextValue(capture *artifactCapture, value interface{}) {
	switch v := value.(type) {
	case string:
		capture.Append(v)
	case []interface{}:
		for _, item := range v {
			appendPiTextValue(capture, item)
		}
	case map[string]interface{}:
		for _, field := range []string{"text", "content", "delta"} {
			appendPiTextValue(capture, v[field])
		}
	}
}

// piSessionIDFromLine parses Pi session header events without depending on other event shapes.
func piSessionIDFromLine(line []byte, progress io.Writer) string {
	var event piSessionEvent
	if err := json.Unmarshal(line, &event); err != nil {
		return ""
	}
	if strings.Contains(event.Type, "error") || strings.Contains(event.Type, "failed") {
		printAgentSessionFailed(progress, "pi")
	}
	if event.Type != "session" || event.ID == "" {
		return ""
	}
	printAgentSessionStarted(progress, "pi", event.ID)
	return event.ID
}
