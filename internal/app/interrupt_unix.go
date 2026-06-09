//go:build !windows

// Package app interrupts detached workflow workers on Unix-like systems.
package app

import "syscall"

// interruptProcessGroup asks the detached worker session to stop as a group.
func interruptProcessGroup(pid int) error {
	if pid <= 0 {
		return nil
	}
	if err := syscall.Kill(-pid, syscall.SIGTERM); err != nil && err != syscall.ESRCH {
		return err
	}
	return nil
}
