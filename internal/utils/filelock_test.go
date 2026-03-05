package utils

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFileLockClass(t *testing.T) {
	tmpDir, _ := ioutil.TempDir("", "lock_test")
	defer os.RemoveAll(tmpDir)

	lockPath := filepath.Join(tmpDir, "business.lock")
	l1 := NewFileLock(lockPath, 1*time.Second)
	l2 := NewFileLock(lockPath, 1*time.Second)

	t.Run("Mutual Exclusion", func(t *testing.T) {
		// First instance acquires lock
		if err := l1.Lock(); err != nil {
			t.Fatalf("l1 failed to lock: %v", err)
		}

		// Second instance should fail due to timeout
		err := l2.Lock()
		if err == nil {
			t.Error("l2 should have failed to acquire the same lock")
		}

		// First instance unlocks
		if err := l1.Unlock(); err != nil {
			t.Fatalf("l1 failed to unlock: %v", err)
		}

		// Second instance should now succeed
		if err := l2.Lock(); err != nil {
			t.Fatalf("l2 should succeed after l1 unlocks, but got: %v", err)
		}
		l2.Unlock()
	})

	t.Run("Re-entrant Safety", func(t *testing.T) {
		l := NewFileLock(lockPath, 1*time.Second)
		l.Lock()
		err := l.Lock() // Re-lock same instance
		if err != nil {
			t.Error("Re-entrant lock on same instance should not fail")
		}
		l.Unlock()
	})
}
