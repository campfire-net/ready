package jsonl

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
)

// Reader reads MutationRecords from a JSONL file.
// Records are returned sorted by Timestamp ascending.
// Malformed lines are skipped silently (resilience, matching state.go pattern).
type Reader struct {
	path string
}

// NewReader creates a Reader for the given JSONL file path.
func NewReader(path string) *Reader {
	return &Reader{path: path}
}

// ReaderForProject returns a Reader rooted at the project directory.
// Returns an error if no project root is found.
func ReaderForProject() (*Reader, error) {
	dir, err := findProjectRoot()
	if err != nil {
		return nil, err
	}
	return NewReader(filepath.Join(dir, ReadyDir, MutationsFile)), nil
}

// ReadAll returns all records from the file sorted by Timestamp ascending.
// Returns an empty slice if the file does not exist.
// Malformed lines are skipped.
func (r *Reader) ReadAll() ([]MutationRecord, error) {
	return r.readSince(0)
}

// ReadSince returns all records with Timestamp > after, sorted ascending.
// after is nanoseconds since Unix epoch (matches MutationRecord.Timestamp).
// Returns an empty slice if the file does not exist or has no matching records.
func (r *Reader) ReadSince(after int64) ([]MutationRecord, error) {
	return r.readSince(after)
}

// Path returns the absolute path of the mutations file this Reader targets.
func (r *Reader) Path() string {
	return r.path
}

// readSince reads all records with Timestamp > after from the JSONL file.
func (r *Reader) readSince(after int64) ([]MutationRecord, error) {
	f, err := os.Open(r.path)
	if err != nil {
		if os.IsNotExist(err) {
			return []MutationRecord{}, nil
		}
		return nil, fmt.Errorf("jsonl: open %s: %w", r.path, err)
	}
	defer f.Close()

	var records []MutationRecord
	scanner := bufio.NewScanner(f)
	// Increase buffer size to handle large payloads (e.g. context fields).
	scanner.Buffer(make([]byte, 1024*1024), 10*1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var rec MutationRecord
		if err := json.Unmarshal(line, &rec); err != nil {
			// Skip malformed lines — resilience, matching state.go pattern.
			continue
		}
		if rec.Timestamp > after {
			records = append(records, rec)
		}
	}
	if err := scanner.Err(); err != nil && err != io.EOF {
		return nil, fmt.Errorf("jsonl: read %s: %w", r.path, err)
	}

	// Sort by timestamp ascending (file order is usually already sorted,
	// but concurrent appends could theoretically interleave).
	sort.Slice(records, func(i, j int) bool {
		return records[i].Timestamp < records[j].Timestamp
	})

	return records, nil
}
