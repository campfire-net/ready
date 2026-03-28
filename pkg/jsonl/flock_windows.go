//go:build windows

package jsonl

import "os"

// lockFile is a no-op on Windows. File locking on Windows uses LockFileEx
// which has different semantics. O_APPEND provides sufficient atomicity
// for single-machine use.
// LockFile is a no-op on Windows.
func LockFile(f *os.File) error {
	return nil
}

// UnlockFile is a no-op on Windows.
func UnlockFile(f *os.File) error {
	return nil
}
