// Package app detects stale status runs for human display without mutating state.
package app

import "time"

// humanDisplayState marks unowned running runs as stale without mutating durable state.
func humanDisplayState(repo string, state State) State {
	if isStaleRunningRun(repo, state) {
		state.Status = statusStale
	}
	return state
}

// isStaleRunningRun reports running state whose owner lock is no longer live.
func isStaleRunningRun(repo string, state State) bool {
	stopped, _ := runningRunCannotAdvance(repo, state, time.Now())
	return stopped
}
