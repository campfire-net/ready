//go:build !windows

package jsonl

import (
	"os"
	"syscall"
)

// LockFile acquires an exclusive advisory lock on f.
func LockFile(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_EX)
}

// UnlockFile releases an advisory lock on f.
func UnlockFile(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
}
