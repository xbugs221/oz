//go:build windows

// Package app configures detached workflow workers on Windows.
package app

import "os/exec"

// configureDetachedCommand keeps the default Windows process attributes.
func configureDetachedCommand(cmd *exec.Cmd) {
	_ = cmd
}
