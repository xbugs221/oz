// Package app calculates status view durations for human and JSON output.
package app

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

type stageDurationItem struct {
	label   string
	stage   string
	minutes float64
}

// stageDurationSummaryLines formats human duration totals for executed stages only.
func stageDurationSummaryLines(state State, now time.Time) []string {
	items := stageDurationItems(state, now)
	if len(items) == 0 {
		return nil
	}
	parts := make([]string, 0, len(items))
	total := 0.0
	for _, item := range items {
		parts = append(parts, formatMinutes(item.minutes))
		total += item.minutes
	}
	lines := []string{fmt.Sprintf("- 耗时 %s分钟=%s", formatMinutes(total), strings.Join(parts, "+"))}
	for _, item := range items {
		lines = append(lines, fmt.Sprintf("  - %s %s %s分钟", item.label, item.stage, formatMinutes(item.minutes)))
	}
	return lines
}

type stageDurationBucket struct {
	label     string
	stage     string
	minutes   float64
	stageName string
	count     int
}

// stageDurationItems collects valid timing records in workflow display order.
func stageDurationItems(state State, now time.Time) []stageDurationItem {
	if len(state.StageTimings) == 0 {
		return nil
	}
	var order []string
	buckets := map[string]*stageDurationBucket{}
	for _, stage := range workflowStagesForState(state) {
		timing, ok := state.StageTimings[stage]
		if !ok || timing.StartedAt == "" {
			continue
		}
		startedAt, err := time.Parse(time.RFC3339Nano, timing.StartedAt)
		if err != nil {
			continue
		}
		finishedAt := now
		if timing.FinishedAt != "" {
			finishedAt, err = time.Parse(time.RFC3339Nano, timing.FinishedAt)
			if err != nil {
				continue
			}
		} else if state.Stage != stage || state.Status != statusRunning {
			continue
		}
		duration := finishedAt.Sub(startedAt)
		if duration < 0 {
			continue
		}
		role, err := roleForStage(stage)
		if err != nil {
			continue
		}
		key := role.Name
		bucket, ok := buckets[key]
		if !ok {
			bucket = &stageDurationBucket{label: role.Label, stage: role.Name, stageName: stage}
			buckets[key] = bucket
			order = append(order, key)
		}
		bucket.minutes += duration.Minutes()
		bucket.count++
	}
	items := make([]stageDurationItem, 0, len(order))
	for _, key := range order {
		bucket := buckets[key]
		stage := bucket.stage
		if bucket.count == 1 {
			stage = bucket.stageName
		}
		items = append(items, stageDurationItem{
			label:   bucket.label,
			stage:   stage,
			minutes: bucket.minutes,
		})
	}
	return items
}

// formatMinutes renders minutes with at most two decimal places for status text.
func formatMinutes(minutes float64) string {
	text := strconv.FormatFloat(minutes, 'f', 2, 64)
	text = strings.TrimRight(text, "0")
	text = strings.TrimRight(text, ".")
	if text == "" || text == "-0" {
		return "0"
	}
	return text
}

// statusNodeDuration returns a helper node wall-time duration once the node has started.
func statusNodeDuration(node DAGNodeState, now time.Time) (float64, bool) {
	if node.StartedAt == "" {
		return 0, false
	}
	startedAt, err := time.Parse(time.RFC3339Nano, node.StartedAt)
	if err != nil {
		return 0, false
	}
	finishedAt := now
	if node.FinishedAt != "" {
		finishedAt, err = time.Parse(time.RFC3339Nano, node.FinishedAt)
		if err != nil {
			return 0, false
		}
	} else if node.Status != statusRunning {
		return 0, false
	}
	if finishedAt.Before(startedAt) {
		return 0, false
	}
	return finishedAt.Sub(startedAt).Minutes(), true
}

// statusWorkflowWallDuration measures elapsed workflow wall time from run creation to latest known activity.
func statusWorkflowWallDuration(state State, now time.Time) *float64 {
	startedAt, ok := statusWorkflowStartedAt(state)
	if !ok {
		return nil
	}
	finishedAt := now
	if state.Status != statusRunning {
		latest, found := statusLatestWorkflowActivity(state)
		if !found {
			return nil
		}
		finishedAt = latest
	}
	if finishedAt.Before(startedAt) {
		return nil
	}
	minutes := finishedAt.Sub(startedAt).Minutes()
	return &minutes
}

// statusWorkflowStartedAt finds the durable run start, preferring the sortable run id timestamp.
func statusWorkflowStartedAt(state State) (time.Time, bool) {
	if startedAt, err := time.Parse("20060102T150405.000000000Z", state.RunID); err == nil {
		return startedAt, true
	}
	return statusEarliestWorkflowActivity(state)
}

// statusEarliestWorkflowActivity returns the first persisted stage or DAG node timestamp.
func statusEarliestWorkflowActivity(state State) (time.Time, bool) {
	var earliest time.Time
	found := false
	visit := func(value string) {
		if value == "" {
			return
		}
		timestamp, err := time.Parse(time.RFC3339Nano, value)
		if err != nil {
			return
		}
		if !found || timestamp.Before(earliest) {
			earliest = timestamp
			found = true
		}
	}
	for _, timing := range state.StageTimings {
		visit(timing.StartedAt)
	}
	for _, node := range state.DAGNodes {
		visit(node.StartedAt)
	}
	return earliest, found
}

// statusLatestWorkflowActivity returns the last persisted stage or DAG node timestamp.
func statusLatestWorkflowActivity(state State) (time.Time, bool) {
	var latest time.Time
	found := false
	visit := func(value string) {
		if value == "" {
			return
		}
		timestamp, err := time.Parse(time.RFC3339Nano, value)
		if err != nil {
			return
		}
		if !found || timestamp.After(latest) {
			latest = timestamp
			found = true
		}
	}
	for _, timing := range state.StageTimings {
		visit(timing.FinishedAt)
		visit(timing.StartedAt)
	}
	for _, node := range state.DAGNodes {
		visit(node.FinishedAt)
		visit(node.StartedAt)
	}
	return latest, found
}
