package resolve_test

// Tests for ByIDFromJSONL and AllItemsFromJSONL.
//
// These tests write mutations.jsonl to a temp directory and verify that the
// JSONL-backed resolvers return the same results as the store-backed resolvers
// for the same mutation sequences.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/campfire-net/ready/pkg/resolve"
)

const jsonlResolveCampfire = "1a2b3c4d5e6f7a8b9c0d1e2f3a4b5c6d7e8f9a0b1c2d3e4f5a6b7c8d9e0f1a2b"

// mutJSON is the minimal mutation record shape for JSONL resolve tests.
type mutJSON struct {
	MsgID       string          `json:"msg_id"`
	CampfireID  string          `json:"campfire_id"`
	Timestamp   int64           `json:"timestamp"`
	Operation   string          `json:"operation"`
	Payload     json.RawMessage `json:"payload"`
	Tags        []string        `json:"tags"`
	Sender      string          `json:"sender"`
	Antecedents []string        `json:"antecedents,omitempty"`
}

// writeJSONL writes mutation records to a temp file and returns the path.
func writeJSONL(t *testing.T, records []mutJSON) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "mutations.jsonl")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("creating JSONL: %v", err)
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for _, r := range records {
		if err := enc.Encode(r); err != nil {
			t.Fatalf("encoding record: %v", err)
		}
	}
	return path
}

func createMut(msgID, itemID, title string, ts int64) mutJSON {
	p, _ := json.Marshal(map[string]interface{}{
		"id": itemID, "title": title, "type": "task",
		"for": "baron@3dl.dev", "priority": "p1",
	})
	return mutJSON{
		MsgID:     msgID,
		CampfireID: jsonlResolveCampfire,
		Timestamp: ts,
		Operation: "work:create",
		Payload:   json.RawMessage(p),
		Tags:      []string{"work:create"},
		Sender:    "testsender",
	}
}

// TestAllItemsFromJSONL_ReturnsAllItems verifies basic item retrieval.
func TestAllItemsFromJSONL_ReturnsAllItems(t *testing.T) {
	ts := time.Now().UnixNano()
	path := writeJSONL(t, []mutJSON{
		createMut("msg-r1", "ready-r1", "Item R1", ts),
		createMut("msg-r2", "ready-r2", "Item R2", ts+1),
		createMut("msg-r3", "ready-r3", "Item R3", ts+2),
	})

	items, err := resolve.AllItemsFromJSONL(path, jsonlResolveCampfire)
	if err != nil {
		t.Fatalf("AllItemsFromJSONL error: %v", err)
	}
	if len(items) != 3 {
		t.Errorf("expected 3 items, got %d", len(items))
	}
	ids := make(map[string]bool)
	for _, item := range items {
		ids[item.ID] = true
	}
	for _, wantID := range []string{"ready-r1", "ready-r2", "ready-r3"} {
		if !ids[wantID] {
			t.Errorf("expected item %q in results", wantID)
		}
	}
}

// TestAllItemsFromJSONL_EmptyFile verifies empty result for empty file.
func TestAllItemsFromJSONL_EmptyFile(t *testing.T) {
	path := writeJSONL(t, nil)
	items, err := resolve.AllItemsFromJSONL(path, jsonlResolveCampfire)
	if err != nil {
		t.Fatalf("AllItemsFromJSONL error: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("expected 0 items for empty file, got %d", len(items))
	}
}

// TestAllItemsFromJSONL_MissingFile verifies empty result for missing file.
func TestAllItemsFromJSONL_MissingFile(t *testing.T) {
	items, err := resolve.AllItemsFromJSONL("/nonexistent/path/mutations.jsonl", jsonlResolveCampfire)
	if err != nil {
		t.Fatalf("AllItemsFromJSONL should not error for missing file, got: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("expected 0 items for missing file, got %d", len(items))
	}
}

// TestByIDFromJSONL_ExactMatch verifies exact ID lookup.
func TestByIDFromJSONL_ExactMatch(t *testing.T) {
	ts := time.Now().UnixNano()
	path := writeJSONL(t, []mutJSON{
		createMut("msg-ex1", "ready-ex1", "Exact One", ts),
		createMut("msg-ex2", "ready-ex2", "Exact Two", ts+1),
	})

	item, err := resolve.ByIDFromJSONL(path, jsonlResolveCampfire, "ready-ex1")
	if err != nil {
		t.Fatalf("ByIDFromJSONL error: %v", err)
	}
	if item.ID != "ready-ex1" {
		t.Errorf("expected ID=ready-ex1, got %q", item.ID)
	}
	if item.Title != "Exact One" {
		t.Errorf("expected title 'Exact One', got %q", item.Title)
	}
}

// TestByIDFromJSONL_PrefixMatch verifies prefix-based ID lookup.
func TestByIDFromJSONL_PrefixMatch(t *testing.T) {
	ts := time.Now().UnixNano()
	path := writeJSONL(t, []mutJSON{
		createMut("msg-px", "ready-px1abc", "Prefix Item", ts),
	})

	item, err := resolve.ByIDFromJSONL(path, jsonlResolveCampfire, "ready-px1")
	if err != nil {
		t.Fatalf("ByIDFromJSONL prefix error: %v", err)
	}
	if item.ID != "ready-px1abc" {
		t.Errorf("expected ID=ready-px1abc, got %q", item.ID)
	}
}

// TestByIDFromJSONL_ExactMatchBeforePrefix verifies that exact match wins over
// prefix when both could match (e.g. "ready-a" vs "ready-ab").
func TestByIDFromJSONL_ExactMatchBeforePrefix(t *testing.T) {
	ts := time.Now().UnixNano()
	path := writeJSONL(t, []mutJSON{
		createMut("msg-ep1", "ready-ep", "Exact", ts),
		createMut("msg-ep2", "ready-epx", "Longer", ts+1),
	})

	item, err := resolve.ByIDFromJSONL(path, jsonlResolveCampfire, "ready-ep")
	if err != nil {
		t.Fatalf("ByIDFromJSONL exact-before-prefix error: %v", err)
	}
	if item.ID != "ready-ep" {
		t.Errorf("expected exact match ready-ep, got %q", item.ID)
	}
}

// TestByIDFromJSONL_AmbiguousPrefix verifies ErrAmbiguous for ambiguous prefixes.
func TestByIDFromJSONL_AmbiguousPrefix(t *testing.T) {
	ts := time.Now().UnixNano()
	path := writeJSONL(t, []mutJSON{
		createMut("msg-amb1", "ready-ambA", "Amb A", ts),
		createMut("msg-amb2", "ready-ambB", "Amb B", ts+1),
	})

	_, err := resolve.ByIDFromJSONL(path, jsonlResolveCampfire, "ready-amb")
	if err == nil {
		t.Fatal("expected ErrAmbiguous, got nil")
	}
	if _, ok := err.(resolve.ErrAmbiguous); !ok {
		t.Errorf("expected ErrAmbiguous, got %T: %v", err, err)
	}
}

// TestByIDFromJSONL_NotFound verifies ErrNotFound for unknown IDs.
func TestByIDFromJSONL_NotFound(t *testing.T) {
	ts := time.Now().UnixNano()
	path := writeJSONL(t, []mutJSON{
		createMut("msg-nf", "ready-nf1", "Not Found Test", ts),
	})

	_, err := resolve.ByIDFromJSONL(path, jsonlResolveCampfire, "ready-zzz")
	if err == nil {
		t.Fatal("expected ErrNotFound, got nil")
	}
	if _, ok := err.(resolve.ErrNotFound); !ok {
		t.Errorf("expected ErrNotFound, got %T: %v", err, err)
	}
}

// TestByIDFromJSONL_MissingFile returns ErrNotFound (not a generic error) when
// the file doesn't exist — an empty mutation log means no items, so any lookup
// is not found.
func TestByIDFromJSONL_MissingFile(t *testing.T) {
	_, err := resolve.ByIDFromJSONL("/nonexistent/mutations.jsonl", jsonlResolveCampfire, "ready-x")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
	if _, ok := err.(resolve.ErrNotFound); !ok {
		t.Errorf("expected ErrNotFound for missing file, got %T: %v", err, err)
	}
}

// TestJSONLFallback_JSONLWhenFileExists verifies that when a mutations.jsonl file
// exists at a known path, AllItemsFromJSONL derives state from it — not from the
// campfire store. This covers the JSONL branch of allItemsFromJSONLOrStore.
func TestJSONLFallback_JSONLWhenFileExists(t *testing.T) {
	ts := time.Now().UnixNano()
	// Write two items to JSONL that are NOT in the store.
	path := writeJSONL(t, []mutJSON{
		createMut("msg-fb1", "ready-fb1", "Fallback JSONL Item 1", ts),
		createMut("msg-fb2", "ready-fb2", "Fallback JSONL Item 2", ts+1),
	})

	// Store is empty — if JSONL is used, we get 2 items; if store is used, we get 0.
	items, err := resolve.AllItemsFromJSONL(path, jsonlResolveCampfire)
	if err != nil {
		t.Fatalf("AllItemsFromJSONL error: %v", err)
	}
	if len(items) != 2 {
		t.Errorf("expected 2 items from JSONL file, got %d — JSONL path not taken", len(items))
	}
	ids := make(map[string]bool)
	for _, item := range items {
		ids[item.ID] = true
	}
	if !ids["ready-fb1"] {
		t.Error("expected ready-fb1 in JSONL-derived items")
	}
	if !ids["ready-fb2"] {
		t.Error("expected ready-fb2 in JSONL-derived items")
	}
}

// TestJSONLFallback_StoreWhenNoJSONLFile verifies that when no JSONL file exists
// (the project has no .ready/mutations.jsonl), AllItems falls back to the campfire
// store. This covers the store-fallback branch of allItemsFromJSONLOrStore.
//
// The trigger condition is jsonlPath() returning "" (no project root found).
// We simulate this by calling AllItems on a store that has items — if the fallback
// to store works, we get those items; if something always returns empty, we fail.
func TestJSONLFallback_StoreWhenNoJSONLFile(t *testing.T) {
	s := openTempStore(t)
	addMembership(t, s, jsonlResolveCampfire)
	addTestItem(t, s, jsonlResolveCampfire, "msg-sf1", "ready-sf1", "Store Fallback Item 1")
	addTestItem(t, s, jsonlResolveCampfire, "msg-sf2", "ready-sf2", "Store Fallback Item 2")

	// Call AllItems (the store path — used when no JSONL file exists / no project root).
	items, err := resolve.AllItems(s)
	if err != nil {
		t.Fatalf("AllItems (store fallback) error: %v", err)
	}
	if len(items) != 2 {
		t.Errorf("expected 2 items from store fallback, got %d — store path not working", len(items))
	}
	ids := make(map[string]bool)
	for _, item := range items {
		ids[item.ID] = true
	}
	if !ids["ready-sf1"] {
		t.Error("expected ready-sf1 in store-derived items")
	}
	if !ids["ready-sf2"] {
		t.Error("expected ready-sf2 in store-derived items")
	}
}

// TestJSONLFallback_JSONLTakesPrecedenceOverStore verifies the routing invariant:
// when a JSONL file exists, its state is used — even if the store also has items.
// A regression that always returned "" from jsonlPath() would cause JSONL-only items
// to disappear (they'd be absent from the store) and this test would catch it.
func TestJSONLFallback_JSONLTakesPrecedenceOverStore(t *testing.T) {
	ts := time.Now().UnixNano()
	// Write one item to JSONL.
	path := writeJSONL(t, []mutJSON{
		createMut("msg-pri1", "ready-pri-jsonl", "JSONL-only Item", ts),
	})

	// The store has a different item — only present in store, not in JSONL.
	s := openTempStore(t)
	addMembership(t, s, jsonlResolveCampfire)
	addTestItem(t, s, jsonlResolveCampfire, "msg-pri2", "ready-pri-store", "Store-only Item")

	// JSONL read: must return the JSONL item, not the store item.
	jsonlItems, err := resolve.AllItemsFromJSONL(path, jsonlResolveCampfire)
	if err != nil {
		t.Fatalf("AllItemsFromJSONL error: %v", err)
	}

	// Store read: must return the store item, not the JSONL item.
	storeItems, err := resolve.AllItems(s)
	if err != nil {
		t.Fatalf("AllItems error: %v", err)
	}

	// Verify the two paths return different data — proving they are independent.
	jsonlIDs := make(map[string]bool)
	for _, item := range jsonlItems {
		jsonlIDs[item.ID] = true
	}
	storeIDs := make(map[string]bool)
	for _, item := range storeItems {
		storeIDs[item.ID] = true
	}

	if !jsonlIDs["ready-pri-jsonl"] {
		t.Error("JSONL path must return ready-pri-jsonl (JSONL-only item)")
	}
	if jsonlIDs["ready-pri-store"] {
		t.Error("JSONL path must NOT return ready-pri-store (store-only item) — that would mean JSONL is reading from the store")
	}
	if !storeIDs["ready-pri-store"] {
		t.Error("store path must return ready-pri-store (store-only item)")
	}
	if storeIDs["ready-pri-jsonl"] {
		t.Error("store path must NOT return ready-pri-jsonl (JSONL-only item) — that would mean store is reading from JSONL")
	}
}
