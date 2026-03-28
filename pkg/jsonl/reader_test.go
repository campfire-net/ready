package jsonl

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeRecords(t *testing.T, path string, records []MutationRecord) {
	t.Helper()
	w := NewWriter(path)
	for _, r := range records {
		if err := w.Append(r); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}
}

func TestReader_ReadAll_EmptyFileReturnsEmptySlice(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, MutationsFile)

	// File does not exist yet.
	r := NewReader(path)
	records, err := r.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll on missing file: %v", err)
	}
	if len(records) != 0 {
		t.Errorf("expected empty slice, got %d records", len(records))
	}

	// Now create an empty file.
	f, _ := os.Create(path)
	f.Close()
	records, err = r.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll on empty file: %v", err)
	}
	if len(records) != 0 {
		t.Errorf("expected empty slice for empty file, got %d records", len(records))
	}
}

func TestReader_ReadAll_ReturnsRecordsInOrder(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, MutationsFile)

	base := time.Now().UnixNano()
	want := []MutationRecord{
		makeRecord(t, "work:create", base+0),
		makeRecord(t, "work:claim", base+1),
		makeRecord(t, "work:close", base+2),
	}
	writeRecords(t, path, want)

	r := NewReader(path)
	got, err := r.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("expected %d records, got %d", len(want), len(got))
	}
	for i := range want {
		if got[i].Operation != want[i].Operation {
			t.Errorf("[%d] operation: want %q, got %q", i, want[i].Operation, got[i].Operation)
		}
		if got[i].Timestamp != want[i].Timestamp {
			t.Errorf("[%d] timestamp: want %d, got %d", i, want[i].Timestamp, got[i].Timestamp)
		}
		if got[i].MsgID != want[i].MsgID {
			t.Errorf("[%d] msg_id: want %q, got %q", i, want[i].MsgID, got[i].MsgID)
		}
	}
}

func TestReader_ReadSince_FiltersCorrectly(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, MutationsFile)

	base := int64(1_000_000_000)
	records := []MutationRecord{
		makeRecord(t, "work:create", base+0),
		makeRecord(t, "work:claim", base+100),
		makeRecord(t, "work:update", base+200),
		makeRecord(t, "work:close", base+300),
	}
	writeRecords(t, path, records)

	r := NewReader(path)

	// ReadSince(base+100) should return records with Timestamp > base+100.
	got, err := r.ReadSince(base + 100)
	if err != nil {
		t.Fatalf("ReadSince: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 records after ts=%d, got %d", base+100, len(got))
	}
	if got[0].Operation != "work:update" {
		t.Errorf("expected work:update, got %q", got[0].Operation)
	}
	if got[1].Operation != "work:close" {
		t.Errorf("expected work:close, got %q", got[1].Operation)
	}

	// ReadSince(0) should return all records.
	got, err = r.ReadSince(0)
	if err != nil {
		t.Fatalf("ReadSince(0): %v", err)
	}
	if len(got) != 4 {
		t.Errorf("ReadSince(0) expected 4, got %d", len(got))
	}

	// ReadSince(base+300) should return no records (strict >).
	got, err = r.ReadSince(base + 300)
	if err != nil {
		t.Fatalf("ReadSince(base+300): %v", err)
	}
	if len(got) != 0 {
		t.Errorf("ReadSince(base+300) expected 0, got %d", len(got))
	}
}

func TestReader_ReadAll_MalformedLinesSkipped(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, MutationsFile)

	// Write one valid record, one malformed line, one valid record.
	base := time.Now().UnixNano()
	rec0 := makeRecord(t, "work:create", base+0)
	rec1 := makeRecord(t, "work:close", base+2)

	line0, _ := json.Marshal(rec0)
	line1, _ := json.Marshal(rec1)
	content := string(line0) + "\n{invalid json\n" + string(line1) + "\n"

	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	r := NewReader(path)
	got, err := r.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 records (malformed line skipped), got %d", len(got))
	}
	if got[0].Operation != "work:create" {
		t.Errorf("expected work:create, got %q", got[0].Operation)
	}
	if got[1].Operation != "work:close" {
		t.Errorf("expected work:close, got %q", got[1].Operation)
	}
}

func TestReader_ReadAll_ContentIntegrity(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, MutationsFile)

	payload, _ := json.Marshal(map[string]string{"id": "proj-001", "title": "Test item with unicode: 日本語"})
	rec := MutationRecord{
		MsgID:       "msg-abc123",
		CampfireID:  "cafecafe123",
		Timestamp:   time.Now().UnixNano(),
		Operation:   "work:create",
		Payload:     json.RawMessage(payload),
		Tags:        []string{"work:create", "extra-tag"},
		Sender:      "agent-xyz",
		Antecedents: []string{"msg-parent"},
	}

	w := NewWriter(path)
	if err := w.Append(rec); err != nil {
		t.Fatalf("Append: %v", err)
	}

	r := NewReader(path)
	got, err := r.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 record, got %d", len(got))
	}

	g := got[0]
	if g.MsgID != rec.MsgID {
		t.Errorf("MsgID: want %q, got %q", rec.MsgID, g.MsgID)
	}
	if g.CampfireID != rec.CampfireID {
		t.Errorf("CampfireID: want %q, got %q", rec.CampfireID, g.CampfireID)
	}
	if g.Timestamp != rec.Timestamp {
		t.Errorf("Timestamp: want %d, got %d", rec.Timestamp, g.Timestamp)
	}
	if g.Operation != rec.Operation {
		t.Errorf("Operation: want %q, got %q", rec.Operation, g.Operation)
	}
	if g.Sender != rec.Sender {
		t.Errorf("Sender: want %q, got %q", rec.Sender, g.Sender)
	}
	if len(g.Antecedents) != 1 || g.Antecedents[0] != "msg-parent" {
		t.Errorf("Antecedents: want [msg-parent], got %v", g.Antecedents)
	}
	if len(g.Tags) != 2 {
		t.Errorf("Tags: want 2, got %d", len(g.Tags))
	}
}

func TestMutationRecord_ToMessageRecord_RoundTrip(t *testing.T) {
	payload, _ := json.Marshal(map[string]string{"id": "test-001"})
	rec := MutationRecord{
		MsgID:       "msg-roundtrip",
		CampfireID:  "cafecafe",
		Timestamp:   time.Now().UnixNano(),
		Operation:   "work:create",
		Payload:     json.RawMessage(payload),
		Tags:        []string{"work:create"},
		Sender:      "agent-123",
		Antecedents: []string{"msg-prev"},
	}

	mr := rec.ToMessageRecord()
	if mr.ID != rec.MsgID {
		t.Errorf("ID: want %q, got %q", rec.MsgID, mr.ID)
	}
	if mr.CampfireID != rec.CampfireID {
		t.Errorf("CampfireID: want %q, got %q", rec.CampfireID, mr.CampfireID)
	}
	if mr.Timestamp != rec.Timestamp {
		t.Errorf("Timestamp: want %d, got %d", rec.Timestamp, mr.Timestamp)
	}
	if string(mr.Payload) != string(rec.Payload) {
		t.Errorf("Payload: want %s, got %s", rec.Payload, mr.Payload)
	}
	if len(mr.Tags) != 1 || mr.Tags[0] != "work:create" {
		t.Errorf("Tags: want [work:create], got %v", mr.Tags)
	}
	if mr.Sender != rec.Sender {
		t.Errorf("Sender: want %q, got %q", rec.Sender, mr.Sender)
	}
}
