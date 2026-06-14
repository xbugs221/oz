// Package app renders compact status view rows for terminal output.
package app

import (
	"fmt"
	"strconv"
	"strings"
)

// compactStatusLines renders the fixed-column human workflow rows for status and watch.
func compactStatusLines(view statusView) []string {
	rows := compactVisibleRows(view.Rows)
	widths := compactColumnWidths(rows)
	var lines []string
	for _, row := range rows {
		prefix := strings.Repeat(" ", row.Indent)
		name := compactHumanRowName(row)
		lines = append(lines, fmt.Sprintf("%s%s %s %s %s",
			prefix,
			padStatusColumn(name, widths.name-row.Indent),
			padStatusColumn(statusText(row.SessionID), widths.session),
			padStatusColumn(statusText(row.Marker), widths.marker),
			padStatusColumn(statusDurationText(row.DurationMinutes), widths.duration),
		))
	}
	return lines
}

type compactColumnWidth struct {
	name     int
	session  int
	marker   int
	duration int
}

// compactVisibleRows removes rows that add no signal to the human compact view.
func compactVisibleRows(rows []statusViewRow) []statusViewRow {
	out := make([]statusViewRow, 0, len(rows))
	for _, row := range rows {
		if row.Kind == "stage" && row.Stage == "planning" && row.SessionID == "" && (row.Marker == "-" || row.Marker == "✓") && row.DurationMinutes == nil {
			continue
		}
		out = append(out, row)
	}
	return out
}

// compactColumnWidths calculates terminal display widths so CJK names align in monospace output.
func compactColumnWidths(rows []statusViewRow) compactColumnWidth {
	widths := compactColumnWidth{}
	for _, row := range rows {
		name := compactHumanRowName(row)
		widths.name = max(widths.name, row.Indent+statusDisplayWidth(name))
		widths.session = max(widths.session, statusDisplayWidth(statusText(row.SessionID)))
		widths.marker = max(widths.marker, statusDisplayWidth(statusText(row.Marker)))
		widths.duration = max(widths.duration, statusDisplayWidth(statusDurationText(row.DurationMinutes)))
	}
	return widths
}

// compactHumanRowName shortens only terminal labels while preserving row names in JSON observability.
func compactHumanRowName(row statusViewRow) string {
	name := row.Name
	if row.Kind == "stage" {
		switch name {
		case "规划阶段":
			return "规划"
		case "执行阶段":
			return "执行"
		case "审核阶段":
			return "审核"
		case "修正阶段":
			return "修正"
		case "测试阶段":
			return "测试"
		case "归档阶段":
			return "归档"
		}
	}
	if row.Kind == "subagent" {
		return compactHumanSubagentName(row.FullName, name)
	}
	if row.Kind == "parallel_group" || row.Kind == "parallel_member" {
		return "- " + name
	}
	return name
}

// compactHumanSubagentName maps common helper roles to toz-flow-cell status labels.
func compactHumanSubagentName(fullName, fallback string) string {
	switch {
	case strings.Contains(fullName, "CLI/API"):
		return "CA"
	case strings.Contains(fullName, "浏览器"):
		return "浏览"
	case strings.Contains(fullName, "回归"):
		return "回归"
	case strings.Contains(fullName, "证据"):
		return "证据"
	case strings.Contains(fullName, "目标"):
		return "目标"
	case strings.Contains(fullName, "代码质量"):
		return "代码"
	case strings.Contains(fullName, "代码"):
		return "代码"
	case strings.Contains(fullName, "外部"):
		return "外部"
	case strings.Contains(fullName, "测试有效"):
		return "测试"
	case strings.Contains(fullName, "风险"):
		return "风险"
	case strings.Contains(fullName, "上下文"):
		return "上下"
	}
	if statusDisplayWidth(fallback) <= 4 {
		return fallback
	}
	runes := []rune(fallback)
	if len(runes) > 2 {
		return string(runes[:2])
	}
	return fallback
}

// statusHeaderText renders the proposal line with overall marker and workflow wall time.
func statusHeaderText(changeName string, view statusView) string {
	if view.WallMinutes == nil {
		return fmt.Sprintf("- %s %s -", changeName, compactOverallMarker(view))
	}
	return fmt.Sprintf("- %s %s %.2f 分钟", changeName, compactOverallMarker(view), *view.WallMinutes)
}

// compactOverallMarker reports a one-cell status indicator for the proposal header.
func compactOverallMarker(view statusView) string {
	switch view.RunStatus {
	case statusDone:
		return "✓"
	case statusFailed, statusBlocked, statusValidationBlocked, statusAcceptanceContractBlocked, statusInterrupted, statusStale:
		return "x"
	case statusRunning:
		if view.Indicator != "" {
			return view.Indicator
		}
		return "→"
	}
	if view.Indicator != "" {
		for _, row := range compactVisibleRows(view.Rows) {
			if strings.Contains(row.Marker, view.Indicator) {
				return view.Indicator
			}
		}
	}
	hasRunning := false
	hasFailed := false
	hasIncomplete := false
	for _, row := range compactVisibleRows(view.Rows) {
		marker := row.Marker
		if strings.Contains(marker, "x") {
			hasFailed = true
		}
		if strings.Contains(marker, "→") {
			hasRunning = true
		}
		if marker == "-" {
			hasIncomplete = true
		}
	}
	switch {
	case hasFailed:
		return "x"
	case hasRunning:
		return "→"
	case !hasIncomplete:
		return "✓"
	default:
		return "-"
	}
}

// statusCountedMarker compacts repeated completed rounds as ✓N before active or failed suffixes.
func statusCountedMarker(completed int, running, failed bool) string {
	var marker strings.Builder
	if completed == 1 {
		marker.WriteString("✓")
	} else if completed > 1 {
		marker.WriteString("✓")
		marker.WriteString(strconv.Itoa(completed))
	}
	if running {
		marker.WriteString("→")
	}
	if failed {
		marker.WriteString("x")
	}
	if marker.Len() == 0 {
		return "-"
	}
	return marker.String()
}

// padStatusColumn adds spaces based on display width instead of byte length.
func padStatusColumn(value string, width int) string {
	padding := width - statusDisplayWidth(value)
	if padding <= 0 {
		return value
	}
	return value + strings.Repeat(" ", padding)
}

// statusDisplayWidth approximates terminal cell width for ASCII and Chinese status text.
func statusDisplayWidth(value string) int {
	width := 0
	for _, r := range value {
		if r <= 127 {
			width++
			continue
		}
		width += 2
	}
	return width
}

// statusText renders an empty field as the required dash column.
func statusText(value string) string {
	if value == "" {
		return "-"
	}
	return value
}

// statusDurationText renders minutes as two decimal places, or a dash when absent.
func statusDurationText(minutes *float64) string {
	if minutes == nil {
		return "-"
	}
	return fmt.Sprintf("%.2f", *minutes)
}

// isASCII reports whether a custom helper name is already a compact ASCII token.
func isASCII(value string) bool {
	for _, r := range value {
		if r > 127 {
			return false
		}
	}
	return true
}
