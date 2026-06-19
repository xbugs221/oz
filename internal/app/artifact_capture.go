// Package app provides a small text capture used by agent backend stream parsers.
package app

import "strings"

// artifactCapture accumulates assistant text observed while draining JSONL output.
type artifactCapture struct {
	builder strings.Builder
}

// Append records non-empty text fragments in stream order.
func (c *artifactCapture) Append(text string) {
	if c == nil || text == "" {
		return
	}
	c.builder.WriteString(text)
}

// String returns the captured assistant text.
func (c *artifactCapture) String() string {
	if c == nil {
		return ""
	}
	return c.builder.String()
}
