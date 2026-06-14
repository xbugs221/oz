// Package app owns per-run lock files and interruption for live sealed workflow runs.
package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"time"
)

type lockStatus string

const (
	lockStatusNone    lockStatus = "none"
	lockStatusActive  lockStatus = "active"
	lockStatusStale   lockStatus = "stale"
	lockStatusUnknown lockStatus = "unknown"
)

// AbortRun marks an unfinished run aborted and interrupts its live worker when possible.
func AbortRun(repo, runID string) error {
	state, err := loadState(repo, runID)
	if err != nil {
		return err
	}
	if err := interruptLockedRun(repo, runID); err != nil {
		return err
	}
	state.Status = "aborted"
	if err := saveState(repo, state); err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(runDir(repo, runID), "lock")); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// interruptLockedRun sends the best available stop signal to the process recorded in a run lock.
func interruptLockedRun(repo, runID string) error {
	data, err := os.ReadFile(filepath.Join(runDir(repo, runID), "lock"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	lock, err := parseLockInfo(data)
	if err != nil {
		return nil
	}
	hostname, _ := os.Hostname()
	if lock.Hostname != "" && hostname != "" && lock.Hostname != hostname {
		return fmt.Errorf("run %s 被主机 %s 上的进程 %d 锁定，无法从主机 %s 中止", runID, lock.Hostname, lock.PID, hostname)
	}
	if lock.PID == os.Getpid() {
		return nil
	}
	return interruptProcessGroup(lock.PID)
}

// ArchiveSupersededRun marks a stale unfinished run as archived after the user starts a replacement.
func ArchiveSupersededRun(repo, runID string) error {
	state, err := loadState(repo, runID)
	if err != nil {
		return err
	}
	state.Status = statusArchived
	state.Error = "superseded by a newer run"
	if err := saveState(repo, state); err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(runDir(repo, runID), "lock")); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// acquireLock creates a process lock file for one run.
func acquireLock(repo, runID string) (func(), error) {
	return acquireLockForGOOS(repo, runID, runtime.GOOS)
}

// acquireLockForGOOS creates a process lock and keeps Windows unknown locks explicit.
func acquireLockForGOOS(repo, runID, goos string) (func(), error) {
	path := filepath.Join(runDir(repo, runID), "lock")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	status, err := lockFileStatus(repo, runID, goos)
	if err != nil {
		return nil, err
	}
	if status == lockStatusActive {
		return nil, newRunLockedError(runID)
	}
	if status == lockStatusUnknown {
		return nil, fmt.Errorf("run %s 的 lock 无法确认，请通过交互菜单恢复或中止", runID)
	}
	hostname, _ := os.Hostname()
	lock := LockInfo{
		PID:       os.Getpid(),
		Hostname:  hostname,
		RunID:     runID,
		StartedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	data, err := json.MarshalIndent(lock, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		return nil, err
	}
	return func() { _ = os.Remove(path) }, nil
}

// lockActive reports whether a lock file points at a live local process.
func lockActive(repo, runID string) (bool, error) {
	status, err := lockFileStatus(repo, runID, runtime.GOOS)
	return status == lockStatusActive, err
}

// lockFileStatus classifies a run lock without killing any external process.
func lockFileStatus(repo, runID, goos string) (lockStatus, error) {
	data, err := os.ReadFile(filepath.Join(runDir(repo, runID), "lock"))
	if os.IsNotExist(err) {
		return lockStatusNone, nil
	}
	if err != nil {
		return lockStatusNone, err
	}
	lock, err := parseLockInfo(data)
	if err != nil || lock.PID <= 0 {
		return lockStatusStale, nil
	}
	hostname, _ := os.Hostname()
	if lock.Hostname != "" && hostname != "" && lock.Hostname != hostname {
		return lockStatusActive, nil
	}
	if goos == "windows" {
		if lock.PID == os.Getpid() {
			return lockStatusActive, nil
		}
		return lockStatusUnknown, nil
	}
	process, err := os.FindProcess(lock.PID)
	if err != nil {
		return lockStatusStale, nil
	}
	if process.Signal(syscall.Signal(0)) == nil {
		return lockStatusActive, nil
	}
	return lockStatusStale, nil
}

// parseLockInfo accepts structured JSON lock metadata.
func parseLockInfo(data []byte) (LockInfo, error) {
	var lock LockInfo
	if err := json.Unmarshal(data, &lock); err != nil {
		return LockInfo{}, err
	}
	if lock.PID <= 0 {
		return LockInfo{}, fmt.Errorf("lock pid 不能为空")
	}
	return lock, nil
}
