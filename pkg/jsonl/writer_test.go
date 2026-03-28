package jsonl

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func makeRecord(t *testing.T, op string, ts int64) MutationRecord {
	t.Helper()
	payload, _ := json.Marshal(map[string]string{"id": "test-001", "title": "Test item"})
	return MutationRecord{
		MsgID:      "msg-" + op,
		CampfireID: "cafecafe" + op,
		Timestamp:  ts,
		Operation:  op,
		Payload:    json.RawMessage(payload),
		Tags:       []string{op},
		Sender:     "agent-abc",
	}
}

func TestWriter_Append_CreatesDirectoryAndFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".ready", MutationsFile)
	w := NewWriter(path)

	rec := makeRecord(t, "work:create", time.Now().UnixNano())
	if err := w.Append(rec); err != nil {
		t.Fatalf("Append failed: %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected file to exist at %s: %v", path, err)
	}
}

func TestWriter_Append_MultipleRecords(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, MutationsFile)
	w := NewWriter(path)

	now := time.Now().UnixNano()
	ops := []string{"work:create", "work:claim", "work:close"}
	for i, op := range ops {
		rec := makeRecord(t, op, now+int64(i))
		if err := w.Append(rec); err != nil {
			t.Fatalf("Append(%s) failed: %v", op, err)
		}
	}

	// Verify raw file has 3 lines.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	lines := splitLines(data)
	if len(lines) != 3 {
		t.Errorf("expected 3 lines, got %d", len(lines))
	}
}

func TestWriter_Append_ConcurrentNonCorruption(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, MutationsFile)
	w := NewWriter(path)

	const goroutines = 20
	var wg sync.WaitGroup
	errs := make(chan error, goroutines)

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			rec := MutationRecord{
				MsgID:     "msg-" + string(rune('a'+n%26)),
				Timestamp: time.Now().UnixNano() + int64(n),
				Operation: "work:create",
				Payload:   json.RawMessage(`{"id":"test"}`),
				Tags:      []string{"work:create"},
				Sender:    "agent",
			}
			if err := w.Append(rec); err != nil {
				errs <- err
			}
		}(i)
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		t.Errorf("concurrent Append error: %v", err)
	}

	// Every line must be valid JSON.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	lines := splitLines(data)
	if len(lines) != goroutines {
		t.Errorf("expected %d lines, got %d", goroutines, len(lines))
	}
	for i, line := range lines {
		var rec MutationRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Errorf("line %d is malformed JSON: %v\nline: %s", i, err, line)
		}
	}
}

// splitLines splits data into non-empty lines (strips trailing \n).
func splitLines(data []byte) []string {
	var lines []string
	start := 0
	for i, b := range data {
		if b == '\n' {
			if i > start {
				lines = append(lines, string(data[start:i]))
			}
			start = i + 1
		}
	}
	if start < len(data) {
		lines = append(lines, string(data[start:]))
	}
	return lines
}
