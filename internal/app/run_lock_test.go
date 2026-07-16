// Package app tests exclusive workflow lease ownership and safe release.
package app

import (
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
)

// TestAcquireLockAllowsOnlyOneConcurrentOwner verifies check-and-create is one atomic operation.
func TestAcquireLockAllowsOnlyOneConcurrentOwner(t *testing.T) {
	repo := gitRepo(t)
	const runID = "concurrent-lock-run"
	const contenders = 100
	start := make(chan struct{})
	unlocks := make(chan func(), contenders)
	errs := make(chan error, contenders)
	var acquired atomic.Int32
	var wg sync.WaitGroup
	for range contenders {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			unlock, err := acquireLock(repo, runID)
			if err != nil {
				if !isRunLockedError(err) {
					errs <- err
				}
				return
			}
			acquired.Add(1)
			unlocks <- unlock
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("unexpected acquire error: %v", err)
	}
	if got := acquired.Load(); got != 1 {
		t.Fatalf("acquired owners = %d, want 1", got)
	}
	close(unlocks)
	for unlock := range unlocks {
		unlock()
	}
}

// TestOwnedUnlockDoesNotRemoveReplacementLease protects a newer generation from an old defer.
func TestOwnedUnlockDoesNotRemoveReplacementLease(t *testing.T) {
	repo := gitRepo(t)
	const runID = "replacement-lock-run"
	unlock, err := acquireLock(repo, runID)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(runDir(repo, runID), "lock")
	replacement, err := newLockInfo(runID)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeLockInfo(path, replacement); err != nil {
		t.Fatal(err)
	}
	unlock()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("replacement lock was removed: %v", err)
	}
	current, err := parseLockInfo(data)
	if err != nil {
		t.Fatal(err)
	}
	if current.OwnerToken != replacement.OwnerToken {
		t.Fatalf("owner token = %q, want replacement %q", current.OwnerToken, replacement.OwnerToken)
	}
}

// TestLockStatusRejectsWrongRunID prevents a live process from owning the wrong run path.
func TestLockStatusRejectsWrongRunID(t *testing.T) {
	repo := gitRepo(t)
	const runID = "expected-run"
	lock, err := newLockInfo("different-run")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(runDir(repo, runID), "lock")
	if err := writeLockInfo(path, lock); err != nil {
		t.Fatal(err)
	}
	status, err := lockFileStatus(repo, runID, runtime.GOOS)
	if err != nil {
		t.Fatal(err)
	}
	if status != lockStatusStale {
		t.Fatalf("lock status = %s, want stale", status)
	}
}
