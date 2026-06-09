// Package app defines workflow error categories shared by runners and CLI glue.
package app

import (
	"errors"
	"fmt"
)

type runLockedError struct {
	runID string
}

// Error returns the user-facing active lock message for a running workflow.
func (e runLockedError) Error() string {
	return fmt.Sprintf("run %s 正被存活进程锁定", e.runID)
}

// newRunLockedError creates a typed error for live run-lock conflicts.
func newRunLockedError(runID string) error {
	return runLockedError{runID: runID}
}

// isRunLockedError reports whether an error is only a live lock conflict.
func isRunLockedError(err error) bool {
	var locked runLockedError
	return errors.As(err, &locked)
}
