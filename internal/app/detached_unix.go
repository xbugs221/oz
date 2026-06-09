//go:build !windows

// Package app configures detached workflow workers on Unix-like systems.
package app

import (
	"os/exec"
	"syscall"
)

// configureDetachedCommand starts the worker in a new session so it survives terminal exit.
func configureDetachedCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}
