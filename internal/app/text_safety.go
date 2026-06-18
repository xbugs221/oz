// Package app centralizes text sanitization before data enters agent prompts or durable diagnostics.
package app

import (
	"strings"
	"unicode/utf8"
)

const invalidUTF8Replacement = "\uFFFD"

// agentPromptText returns text that is safe to send to agent CLIs expecting UTF-8.
func agentPromptText(text string) string {
	return strings.ToValidUTF8(text, invalidUTF8Replacement)
}

// utf8SafeLimit trims text by byte budget without cutting through a UTF-8 rune.
func utf8SafeLimit(text string, limit int) string {
	text = agentPromptText(text)
	if limit <= 0 {
		return ""
	}
	if len(text) <= limit {
		return text
	}
	cut := limit
	for cut > 0 && !utf8.ValidString(text[:cut]) {
		cut--
	}
	return text[:cut]
}

// limitUTF8Text returns a bounded UTF-8 string and appends suffix only when truncated.
func limitUTF8Text(text string, limit int, suffix string) string {
	text = agentPromptText(text)
	if len(text) <= limit {
		return text
	}
	return utf8SafeLimit(text, limit) + suffix
}
