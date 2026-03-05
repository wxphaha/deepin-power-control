package utils

import (
	"deepin-power-control/internal/global"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// FileLock represents an individual file-based mutex
type FileLock struct {
	path    string
	timeout time.Duration
	filePtr *os.File
}

// NewFileLock creates a new lock instance with default settings
func NewFileLock(path string, timeout time.Duration) *FileLock {
	return &FileLock{
		path:    path,
		timeout: timeout,
	}
}

// SetPath updates the lock file path
func (l *FileLock) SetPath(path string) {
	l.path = path
}

// SetTimeout updates the wait timeout
func (l *FileLock) SetTimeout(timeout time.Duration) {
	l.timeout = timeout
}

// Lock acquires the exclusive lock.
// It handles directory creation, file opening, and the flock retry logic.
func (l *FileLock) Lock() error {
	if l.filePtr != nil {
		return nil // Already locked by this instance
	}

	// 1. Prepare directory
	lockDir := filepath.Dir(l.path)
	if err := os.MkdirAll(lockDir, 0755); err != nil {
		return fmt.Errorf("failed to prepare lock directory %s: %v", lockDir, err)
	}

	// 2. Open file
	f, err := os.OpenFile(l.path, os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		if os.IsPermission(err) {
			return fmt.Errorf("permission denied: %s", l.path)
		}
		return fmt.Errorf("failed to open lock file: %v", err)
	}

	// 3. Flock retry logic
	start := time.Now()
	for {
		err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			l.filePtr = f
			return nil
		}

		if err != syscall.EWOULDBLOCK {
			f.Close()
			return fmt.Errorf("unexpected flock error: %v", err)
		}

		if l.timeout > 0 && time.Since(start) > l.timeout {
			f.Close()
			return fmt.Errorf("timeout: lock %s is held by another process", l.path)
		}

		global.Debug("Waiting for lock %s... (%.1fs)", filepath.Base(l.path), time.Since(start).Seconds())
		time.Sleep(500 * time.Millisecond)
	}
}

// Unlock releases the lock and cleans up the file handle
func (l *FileLock) Unlock() error {
	if l.filePtr == nil {
		return nil
	}

	// Ensure closure even if Flock fails
	defer func() {
		l.filePtr.Close()
		l.filePtr = nil
	}()

	if err := syscall.Flock(int(l.filePtr.Fd()), syscall.LOCK_UN); err != nil {
		return fmt.Errorf("failed to release flock: %v", err)
	}

	return nil
}

// --- Global Pre-defined Locks ---
// You can still provide easy-to-use instances for standard tasks

var tlpLock = NewFileLock("/run/deepin-power-control.lock", 3*time.Second)

func LockTlp() error   { return tlpLock.Lock() }
func UnlockTlp() error { return tlpLock.Unlock() }
