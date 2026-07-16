// Package app owns process leases and interruption for live workflow workers.
package app

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
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
	lockPath := filepath.Join(runDir(repo, runID), "lock")
	guardUnlock, err := flockLock(lockPath + ".guard")
	if err != nil {
		return err
	}
	defer func() { _ = guardUnlock() }()
	state, err := loadState(repo, runID)
	if err != nil {
		return err
	}
	interrupted, hasOwner, err := interruptLockedRun(repo, runID)
	if err != nil {
		return err
	}
	state.Status = "aborted"
	if err := saveState(repo, state); err != nil {
		return err
	}
	if hasOwner {
		return removeOwnedLockGuarded(lockPath, interrupted)
	}
	return nil
}

// interruptLockedRun sends the best available stop signal to the process recorded in a run lock.
func interruptLockedRun(repo, runID string) (LockInfo, bool, error) {
	data, err := os.ReadFile(filepath.Join(runDir(repo, runID), "lock"))
	if os.IsNotExist(err) {
		return LockInfo{}, false, nil
	}
	if err != nil {
		return LockInfo{}, false, err
	}
	lock, err := parseLockInfo(data)
	if err != nil {
		return LockInfo{}, false, nil
	}
	if lock.RunID != runID {
		return LockInfo{}, false, fmt.Errorf("run %s 的 lock 记录了错误资源 %s，拒绝中止", runID, lock.RunID)
	}
	hostname, _ := os.Hostname()
	if lock.Hostname != "" && hostname != "" && lock.Hostname != hostname {
		return LockInfo{}, false, fmt.Errorf("run %s 被主机 %s 上的进程 %d 锁定，无法从主机 %s 中止", runID, lock.Hostname, lock.PID, hostname)
	}
	if lock.PID == os.Getpid() {
		return lock, true, nil
	}
	if err := interruptProcessGroup(lock.PID); err != nil {
		return LockInfo{}, false, err
	}
	return lock, true, nil
}

// ArchiveSupersededRun marks a stale unfinished run as archived after the user starts a replacement.
func ArchiveSupersededRun(repo, runID string) error {
	state, err := loadState(repo, runID)
	if err != nil {
		return err
	}
	status, err := clearInactiveRunLock(repo, runID, runtime.GOOS, false)
	if err != nil {
		return err
	}
	if status == lockStatusActive {
		return newRunLockedError(runID)
	}
	if status == lockStatusUnknown {
		return fmt.Errorf("run %s 存在无法确认的 lock，不能归档", runID)
	}
	state.Status = statusArchived
	state.Error = "superseded by a newer run"
	if err := saveState(repo, state); err != nil {
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
	unlock, acquired, status, err := acquireOwnedLock(path, runID, goos)
	if err != nil {
		return nil, err
	}
	if !acquired && status == lockStatusActive {
		return nil, newRunLockedError(runID)
	}
	if !acquired && status == lockStatusUnknown {
		return nil, fmt.Errorf("run %s 的 lock 无法确认，请通过交互菜单恢复或中止", runID)
	}
	if !acquired {
		return nil, fmt.Errorf("run %s 获取 lock 失败", runID)
	}
	return unlock, nil
}

// acquireOwnedLock atomically claims a JSON process lease behind a cross-process guard.
func acquireOwnedLock(path, resourceID, goos string) (func(), bool, lockStatus, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, false, lockStatusNone, err
	}
	guardUnlock, err := flockLock(path + ".guard")
	if err != nil {
		return nil, false, lockStatusNone, err
	}
	defer func() { _ = guardUnlock() }()
	status, err := lockInfoFileStatusForResource(path, goos, resourceID)
	if err != nil {
		return nil, false, status, err
	}
	if status == lockStatusActive || status == lockStatusUnknown {
		return nil, false, status, nil
	}
	if status == lockStatusStale {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return nil, false, status, err
		}
	}
	lock, err := newLockInfo(resourceID)
	if err != nil {
		return nil, false, status, err
	}
	if err := writeLockInfo(path, lock); err != nil {
		return nil, false, status, err
	}
	var once sync.Once
	return func() {
		once.Do(func() { _ = releaseOwnedLock(path, lock) })
	}, true, status, nil
}

// newLockInfo captures one unique process lease generation.
func newLockInfo(resourceID string) (LockInfo, error) {
	token := make([]byte, 16)
	if _, err := rand.Read(token); err != nil {
		return LockInfo{}, fmt.Errorf("生成 lock owner token 失败：%w", err)
	}
	hostname, _ := os.Hostname()
	return LockInfo{
		PID:        os.Getpid(),
		Hostname:   hostname,
		RunID:      resourceID,
		StartedAt:  time.Now().UTC().Format(time.RFC3339Nano),
		OwnerToken: hex.EncodeToString(token),
	}, nil
}

// writeLockInfo atomically publishes a complete process lease.
func writeLockInfo(path string, lock LockInfo) error {
	data, err := json.MarshalIndent(lock, "", "  ")
	if err != nil {
		return err
	}
	return atomicWriteFile(path, append(data, '\n'), 0o644)
}

// releaseOwnedLock removes path only when it still belongs to the same lease generation.
func releaseOwnedLock(path string, owner LockInfo) error {
	guardUnlock, err := flockLock(path + ".guard")
	if err != nil {
		return err
	}
	defer func() { _ = guardUnlock() }()
	return removeOwnedLockGuarded(path, owner)
}

// removeOwnedLockGuarded conditionally removes a lease while the caller holds its guard.
func removeOwnedLockGuarded(path string, owner LockInfo) error {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	current, err := parseLockInfo(data)
	if err != nil || !sameLockOwner(current, owner) {
		return nil
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// sameLockOwner compares the immutable identity of one lease generation.
func sameLockOwner(current, expected LockInfo) bool {
	return current.OwnerToken == expected.OwnerToken &&
		current.RunID == expected.RunID &&
		current.PID == expected.PID &&
		current.Hostname == expected.Hostname &&
		current.StartedAt == expected.StartedAt
}

// clearInactiveRunLock removes stale or explicitly accepted unknown leases under the acquisition guard.
func clearInactiveRunLock(repo, runID, goos string, allowUnknown bool) (lockStatus, error) {
	path := filepath.Join(runDir(repo, runID), "lock")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return lockStatusNone, err
	}
	guardUnlock, err := flockLock(path + ".guard")
	if err != nil {
		return lockStatusNone, err
	}
	defer func() { _ = guardUnlock() }()
	status, err := lockInfoFileStatusForResource(path, goos, runID)
	if err != nil {
		return status, err
	}
	if status == lockStatusStale || (status == lockStatusUnknown && allowUnknown) {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return status, err
		}
	}
	return status, nil
}

// lockActive reports whether a lock file points at a live local process.
func lockActive(repo, runID string) (bool, error) {
	status, err := lockFileStatus(repo, runID, runtime.GOOS)
	return status == lockStatusActive, err
}

// lockFileStatus classifies a run lock without killing any external process.
func lockFileStatus(repo, runID, goos string) (lockStatus, error) {
	return lockInfoFileStatusForResource(filepath.Join(runDir(repo, runID), "lock"), goos, runID)
}

// lockInfoFileStatus classifies a JSON process lock without killing any external process.
func lockInfoFileStatus(path, goos string) (lockStatus, error) {
	return lockInfoFileStatusForResource(path, goos, "")
}

// lockInfoFileStatusForResource rejects leases stored under the wrong resource path.
func lockInfoFileStatusForResource(path, goos, resourceID string) (lockStatus, error) {
	data, err := os.ReadFile(path)
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
	if resourceID != "" && lock.RunID != resourceID {
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
