// Package app builds detached oz flow worker command lines.
package app

// flowWorkerCommandArgs prefixes internal worker commands with the public oz flow group.
func flowWorkerCommandArgs(args ...string) []string {
	return append([]string{"flow"}, args...)
}
