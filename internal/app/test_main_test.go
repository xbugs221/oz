// Package app tests run with isolated user state so temporary repositories do
// not leak durable runtime files into the developer's real home directory.
package app

import (
	"os"
	"testing"
)

// TestMain installs a package-wide XDG state directory before any test can
// create run or batch state for its temporary repository.
func TestMain(m *testing.M) {
	stateHome, err := os.MkdirTemp("", "wo-app-test-state-*")
	if err != nil {
		panic(err)
	}
	if err := os.Setenv("XDG_STATE_HOME", stateHome); err != nil {
		panic(err)
	}
	code := m.Run()
	_ = os.RemoveAll(stateHome)
	os.Exit(code)
}
