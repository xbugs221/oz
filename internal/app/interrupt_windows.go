//go:build windows

// Package app interrupts detached workflow workers on Windows.
package app

import (
	"errors"
	"os/exec"
	"strconv"
)

// interruptProcessGroup asks Windows to terminate the worker process tree.
func interruptProcessGroup(pid int) error {
	if pid <= 0 {
		return nil
	}
	cmd := exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(pid))
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil
		}
		return err
	}
	return nil
}
