//go:build windows

package app

import (
	"os"

	"golang.org/x/sys/windows"
)

// flockLock creates an exclusive advisory lock on a file using Windows LockFileEx.
func flockLock(path string) (func() error, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	handle := windows.Handle(f.Fd())
	var overlapped windows.Overlapped
	err = windows.LockFileEx(handle, windows.LOCKFILE_EXCLUSIVE_LOCK, 0, 0xFFFFFFFF, 0xFFFFFFFF, &overlapped)
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	return func() error {
		_ = windows.UnlockFileEx(handle, 0, 0xFFFFFFFF, 0xFFFFFFFF, &overlapped)
		return f.Close()
	}, nil
}
