package resolve_test

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/campfire-net/campfire/pkg/store"
	"github.com/third-division/ready/pkg/resolve"
)

const testCampfire = "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"

// openTempStore creates a temporary SQLite store for testing.
func openTempStore(t *testing.T) store.Store {
	t.Helper()
	dir := t.TempDir()
	s, err := store.Open(dir + "/store.db")
	if err != nil {
		t.Fatalf("opening temp store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// addTestItem adds a work:create message for the given item ID to the store.
func addTestItem(t *testing.T, s store.Store, campfireID, msgID, itemID, title string) {
	t.Helper()
	payload, _ := json.Marshal(map[string]interface{}{
		"id":       itemID,
		"title":    title,
		"type":     "task",
		"for":      "baron@3dl.dev",
		"priority": "p1",
	})
	ts := time.Now().UnixNano()
	_, err := s.AddMessage(store.MessageRecord{
		ID:         msgID,
		CampfireID: campfireID,
		Sender:     "testkey",
		Payload:    payload,
		Tags:       []string{"work:create"},
		Timestamp:  ts,
		Signature:  []byte("fakesig-" + msgID), // non-nil: required by NOT NULL constraint
		ReceivedAt: ts,
	})
	if err != nil {
		t.Fatalf("adding message: %v", err)
	}
}

// addMembership registers a campfire membership in the store.
func addMembership(t *testing.T, s store.Store, campfireID string) {
	t.Helper()
	if err := s.AddMembership(store.Membership{
		CampfireID:   campfireID,
		TransportDir: os.TempDir(),
		JoinProtocol: "invite",
		Role:         "full",
		JoinedAt:     time.Now().Unix(),
	}); err != nil {
		// Ignore duplicate errors.
		_ = err
	}
}

// TestByID_ExactMatchBeforePrefix verifies that when itemID="ready-a1b" and
// both "ready-a1b" and "ready-a1bc" exist, ByID returns "ready-a1b" exactly
// rather than returning ErrAmbiguous.
// This is the bug fix: without exact-first logic, HasPrefix matches both IDs,
// causing ErrAmbiguous even though an exact match exists.
func TestByID_ExactMatchBeforePrefix(t *testing.T) {
	s := openTempStore(t)
	addMembership(t, s, testCampfire)

	// Add two items where one ID is a prefix of the other.
	addTestItem(t, s, testCampfire, "msg-a1b", "ready-a1b", "Short ID")
	addTestItem(t, s, testCampfire, "msg-a1bc", "ready-a1bc", "Longer ID")

	// Resolve the exact ID "ready-a1b" — should not be ambiguous.
	item, err := resolve.ByID(s, "ready-a1b")
	if err != nil {
		t.Fatalf("expected exact match for 'ready-a1b', got error: %v", err)
	}
	if item.ID != "ready-a1b" {
		t.Errorf("expected ID=ready-a1b, got %q", item.ID)
	}
	if item.Title != "Short ID" {
		t.Errorf("expected title='Short ID', got %q", item.Title)
	}
}

// TestByID_PrefixMatchWhenNoExact verifies that prefix matching still works
// when there is no exact match — e.g., "ready-a1" matches "ready-a1b" uniquely.
func TestByID_PrefixMatchWhenNoExact(t *testing.T) {
	s := openTempStore(t)
	addMembership(t, s, testCampfire)

	addTestItem(t, s, testCampfire, "msg-a1b2", "ready-a1b2", "Unique item")

	item, err := resolve.ByID(s, "ready-a1b")
	if err != nil {
		t.Fatalf("expected prefix match for 'ready-a1b', got error: %v", err)
	}
	if item.ID != "ready-a1b2" {
		t.Errorf("expected ID=ready-a1b2, got %q", item.ID)
	}
}

// TestByID_AmbiguousPrefix verifies that ErrAmbiguous is returned when a
// prefix matches multiple items but no exact match exists.
func TestByID_AmbiguousPrefix(t *testing.T) {
	s := openTempStore(t)
	addMembership(t, s, testCampfire)

	addTestItem(t, s, testCampfire, "msg-x1", "ready-x1a", "Item A")
	addTestItem(t, s, testCampfire, "msg-x2", "ready-x1b", "Item B")

	_, err := resolve.ByID(s, "ready-x1")
	if err == nil {
		t.Fatal("expected ErrAmbiguous, got nil")
	}
	if _, ok := err.(resolve.ErrAmbiguous); !ok {
		t.Errorf("expected ErrAmbiguous, got %T: %v", err, err)
	}
}

// TestByID_NotFound verifies that ErrNotFound is returned for unknown IDs.
func TestByID_NotFound(t *testing.T) {
	s := openTempStore(t)
	addMembership(t, s, testCampfire)

	_, err := resolve.ByID(s, "ready-zzz")
	if err == nil {
		t.Fatal("expected ErrNotFound, got nil")
	}
	if _, ok := err.(resolve.ErrNotFound); !ok {
		t.Errorf("expected ErrNotFound, got %T: %v", err, err)
	}
}
