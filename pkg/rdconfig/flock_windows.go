//go:build windows

package rdconfig

// FlockExclusive is a no-op on Windows. File locking on Windows uses
// LockFileEx which has different semantics. The lock file approach
// provides sufficient protection for single-machine use.
func FlockExclusive(fd int) error {
	return nil
}

// FlockUnlock is a no-op on Windows.
func FlockUnlock(fd int) error {
	return nil
}
