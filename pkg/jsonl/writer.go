package jsonl

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

const (
	// MutationsFile is the filename for the mutations log.
	MutationsFile = "mutations.jsonl"
	// ReadyDir is the directory name under the project root.
	ReadyDir = ".ready"
)

// Writer appends MutationRecords to a JSONL file atomically.
// Each Append call acquires an advisory flock, writes the record, fsyncs, and
// releases the lock. The file is created (along with its parent directory) on
// first write.
type Writer struct {
	path string
}

// NewWriter creates a Writer for the given JSONL file path.
// The file and its parent directory are created on first Append, not here.
func NewWriter(path string) *Writer {
	return &Writer{path: path}
}

// WriterForProject returns a Writer rooted at the project directory.
// The project directory is located via the same .campfire/root walk-up used
// by send.go. Returns an error if no project root is found.
func WriterForProject() (*Writer, error) {
	dir, err := findProjectRoot()
	if err != nil {
		return nil, err
	}
	return NewWriter(filepath.Join(dir, ReadyDir, MutationsFile)), nil
}

// Append marshals r as a single JSON line and appends it to the JSONL file.
// The append is protected by an advisory flock and followed by an fsync.
func (w *Writer) Append(r MutationRecord) error {
	line, err := json.Marshal(r)
	if err != nil {
		return fmt.Errorf("jsonl: marshal record: %w", err)
	}
	line = append(line, '\n')

	// Ensure directory exists.
	if err := os.MkdirAll(filepath.Dir(w.path), 0755); err != nil {
		return fmt.Errorf("jsonl: mkdir %s: %w", filepath.Dir(w.path), err)
	}

	// Open or create the file for appending.
	f, err := os.OpenFile(w.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("jsonl: open %s: %w", w.path, err)
	}
	defer f.Close()

	// Acquire advisory exclusive lock.
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("jsonl: flock %s: %w", w.path, err)
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN) //nolint:errcheck

	// Write and fsync.
	if _, err := f.Write(line); err != nil {
		return fmt.Errorf("jsonl: write %s: %w", w.path, err)
	}
	if err := f.Sync(); err != nil {
		return fmt.Errorf("jsonl: fsync %s: %w", w.path, err)
	}
	return nil
}

// Path returns the absolute path of the mutations file.
func (w *Writer) Path() string {
	return w.path
}

// findProjectRoot walks up from cwd looking for a .campfire/root file,
// matching the logic in cmd/rd/send.go:projectRoot().
func findProjectRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("jsonl: getwd: %w", err)
	}
	for {
		rootFile := filepath.Join(dir, ".campfire", "root")
		data, err := os.ReadFile(rootFile)
		if err == nil {
			id := strings.TrimSpace(string(data))
			if len(id) == 64 {
				return dir, nil
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", fmt.Errorf("jsonl: no .campfire/root found — run 'rd init' first")
}
