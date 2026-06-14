// Package app exposes the machine-readable contract used by oz flow runners.
package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
)

// Version is the oz flow CLI version injected from the release git tag.
var Version = "dev"

// runnerCapabilities lists the JSON subcommands promised by the runner contract.
var runnerCapabilities = []string{"list-changes", "run", "resume", "restart", "status", "abort"}

// RunnerState is the JSON DTO consumed by workflow runners.
type RunnerState struct {
	RunID         string            `json:"run_id"`
	ChangeName    string            `json:"change_name"`
	Status        string            `json:"status"`
	Stage         string            `json:"stage"`
	Stages        map[string]string `json:"stages"`
	Paths         map[string]string `json:"paths"`
	Sessions      map[string]string `json:"sessions"`
	Processes     []ProcessState    `json:"processes,omitempty"`
	Error         string            `json:"error"`
	Observability *statusView       `json:"observability,omitempty"`
}

// runnerContract is the capability discovery payload for oz flow.
type runnerContract struct {
	JSON         bool     `json:"json"`
	Version      string   `json:"version"`
	Capabilities []string `json:"capabilities"`
}

// changeList is the JSON payload returned by list-changes.
type changeList struct {
	Changes []changeSummary `json:"changes"`
}

// changeSummary is the minimal machine-readable description of an active change.
type changeSummary struct {
	Name   string `json:"name"`
	Title  string `json:"title"`
	Status string `json:"status"`
}

// runnerStateFromState converts durable state into the runner-facing DTO.
func runnerStateFromState(state State) RunnerState {
	normalizeStateMaps(&state)
	refreshStateProcesses(&state)
	return RunnerState{
		RunID:      state.RunID,
		ChangeName: state.ChangeName,
		Status:     state.Status,
		Stage:      state.Stage,
		Stages:     state.Stages,
		Paths:      state.Paths,
		Sessions:   state.Sessions,
		Processes:  state.Processes,
		Error:      state.Error,
	}
}

// runnerStateFromStatusView keeps the legacy runner fields and attaches compact observability.
func runnerStateFromStatusView(repo string, state State, displayID string) RunnerState {
	dto := runnerStateFromState(state)
	view := buildStatusView(repo, state, displayID, "")
	dto.Observability = &view
	return dto
}

// normalizeStateMaps ensures JSON maps are encoded as empty objects, not null.
func normalizeStateMaps(state *State) {
	if state.Sessions == nil {
		state.Sessions = map[string]string{}
	}
	if state.Stages == nil {
		state.Stages = map[string]string{}
	}
	if state.Paths == nil {
		state.Paths = map[string]string{}
	}
	if state.ArtifactGates == nil {
		state.ArtifactGates = map[string]StageValidationState{}
	}
}

// writeRunnerContract writes the JSON capability contract to stdout.
func writeRunnerContract(stdout io.Writer) error {
	return writeJSON(stdout, runnerContract{JSON: true, Version: resolvedVersion(), Capabilities: runnerCapabilities})
}

// resolvedVersion reports the release tag when oz flow was installed or run from the source repository.
func resolvedVersion() string {
	if Version != "" && Version != "dev" {
		return Version
	}
	if tag, err := sourceGitTag(); err == nil && tag != "" {
		return tag
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return Version
}

// sourceGitTag reads git describe only when the current source tree is oz itself.
func sourceGitTag() (string, error) {
	root, err := sourceRoot()
	if err != nil {
		return "", err
	}
	describeCmd := exec.Command("git", "-C", root, "describe", "--tags", "--always", "--dirty")
	describeOut, err := describeCmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(describeOut)), nil
}

// sourceRoot locates the oz checkout from compiled source paths before consulting the current directory.
func sourceRoot() (string, error) {
	if _, file, _, ok := runtime.Caller(0); ok {
		if root, err := ozModuleRoot(filepath.Dir(file)); err == nil {
			return root, nil
		}
	}
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return ozModuleRoot(wd)
}

// ozModuleRoot walks upward until it finds the oz module root.
func ozModuleRoot(start string) (string, error) {
	for dir := start; ; dir = filepath.Dir(dir) {
		data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
		if err == nil && strings.Contains(string(data), "module github.com/xbugs221/oz\n") {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("oz module root not found")
		}
	}
}

// writeRunnerState writes one runner DTO line to stdout.
func writeRunnerState(stdout io.Writer, state State) error {
	return writeJSON(stdout, runnerStateFromState(state))
}

// writeFailedRunnerState writes a best-effort failed DTO when a run id is known.
func writeFailedRunnerState(stdout io.Writer, state State, err error) error {
	return writeRunnerState(stdout, failedState(state, err))
}

// writeFailedRunnerError writes a failed DTO when only the run id and error are available.
func writeFailedRunnerError(stdout io.Writer, runID string, err error) error {
	state := State{RunID: runID, Status: statusFailed}
	if err != nil {
		state.Error = err.Error()
	}
	return writeRunnerState(stdout, state)
}

// failedState records a terminal workflow error without inventing log artifacts.
func failedState(state State, err error) State {
	state.Status = statusFailed
	if state.Error == "" && err != nil {
		state.Error = err.Error()
	}
	return state
}

// writeChangeList writes active changes in the runner JSON shape.
func writeChangeList(stdout io.Writer, changes []Change) error {
	out := changeList{Changes: make([]changeSummary, 0, len(changes))}
	for _, change := range changes {
		out.Changes = append(out.Changes, changeSummary{
			Name:   change.Name,
			Title:  titleFromChangeName(change.Name),
			Status: "active",
		})
	}
	return writeJSON(stdout, out)
}

// writeJSON encodes one compact JSON object followed by a newline.
func writeJSON(stdout io.Writer, value any) error {
	enc := json.NewEncoder(stdout)
	return enc.Encode(value)
}

// flushWriter flushes stdout when the writer supports it.
func flushWriter(stdout io.Writer) {
	if flusher, ok := stdout.(interface{ Flush() error }); ok {
		_ = flusher.Flush()
	}
}

// titleFromChangeName creates a readable title without relying on proposal parsing.
func titleFromChangeName(name string) string {
	words := strings.Fields(strings.ReplaceAll(name, "-", " "))
	for i, word := range words {
		if word == "" {
			continue
		}
		runes := []rune(word)
		if len(runes) > 0 && runes[0] >= 'a' && runes[0] <= 'z' {
			runes[0] -= 'a' - 'A'
		}
		words[i] = string(runes)
	}
	return strings.Join(words, " ")
}

// requireFlagValue extracts a required subcommand flag value.
func requireFlagValue(args []string, flag string) (string, error) {
	for i := 0; i < len(args); i++ {
		if args[i] == flag {
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "-") {
				return "", fmt.Errorf("缺少 %s 的值", flag)
			}
			return args[i+1], nil
		}
	}
	return "", fmt.Errorf("缺少必需参数 %s", flag)
}

// optionalFlagValue extracts a flag value when the flag is present.
func optionalFlagValue(args []string, flag string) (string, error) {
	for i := 0; i < len(args); i++ {
		if args[i] == flag {
			if i+1 >= len(args) || strings.HasPrefix(args[i+1], "-") {
				return "", fmt.Errorf("缺少 %s 的值", flag)
			}
			return args[i+1], nil
		}
	}
	return "", nil
}

// hasFlag reports whether a subcommand flag is present.
func hasFlag(args []string, flag string) bool {
	for _, arg := range args {
		if arg == flag {
			return true
		}
	}
	return false
}
