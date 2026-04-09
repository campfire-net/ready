//go:build !windows

package rdconfig

import "syscall"

// FlockExclusive acquires an exclusive file lock.
func FlockExclusive(fd int) error {
	return syscall.Flock(fd, syscall.LOCK_EX)
}

// FlockUnlock releases a file lock.
func FlockUnlock(fd int) error {
	return syscall.Flock(fd, syscall.LOCK_UN)
}
