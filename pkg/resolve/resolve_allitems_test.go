package resolve_test

import (
	"testing"

	"github.com/campfire-net/ready/pkg/resolve"
)

const (
	campfire1 = "1111111111111111111111111111111111111111111111111111111111111111"
	campfire2 = "2222222222222222222222222222222222222222222222222222222222222222"
)

// TestAllItems_SingleCampfire verifies that AllItems returns all items from a single campfire.
func TestAllItems_SingleCampfire(t *testing.T) {
	s := openTempStore(t)
	addMembership(t, s, campfire1)

	addTestItem(t, s, campfire1, "msg-1", "ready-001", "Item One")
	addTestItem(t, s, campfire1, "msg-2", "ready-002", "Item Two")
	addTestItem(t, s, campfire1, "msg-3", "ready-003", "Item Three")

	items, err := resolve.AllItems(s)
	if err != nil {
		t.Fatalf("AllItems returned error: %v", err)
	}
	if len(items) != 3 {
		t.Errorf("expected 3 items, got %d", len(items))
	}
}

// TestAllItems_MultipleCampfires verifies that AllItems returns items from all campfires.
func TestAllItems_MultipleCampfires(t *testing.T) {
	s := openTempStore(t)
	addMembership(t, s, campfire1)
	addMembership(t, s, campfire2)

	addTestItem(t, s, campfire1, "msg-c1a", "ready-c1a", "Campfire1 Alpha")
	addTestItem(t, s, campfire1, "msg-c1b", "ready-c1b", "Campfire1 Beta")
	addTestItem(t, s, campfire2, "msg-c2a", "ready-c2a", "Campfire2 Alpha")

	items, err := resolve.AllItems(s)
	if err != nil {
		t.Fatalf("AllItems returned error: %v", err)
	}
	if len(items) != 3 {
		t.Errorf("expected 3 items across 2 campfires, got %d", len(items))
	}

	// Verify all IDs are present.
	ids := make(map[string]bool)
	for _, item := range items {
		ids[item.ID] = true
	}
	for _, wantID := range []string{"ready-c1a", "ready-c1b", "ready-c2a"} {
		if !ids[wantID] {
			t.Errorf("expected item %q in results, but it was missing", wantID)
		}
	}
}

// TestAllItems_Dedup verifies that when the same item ID appears in two campfires,
// AllItems returns only one copy (the seen-map dedup).
func TestAllItems_Dedup(t *testing.T) {
	s := openTempStore(t)
	addMembership(t, s, campfire1)
	addMembership(t, s, campfire2)

	// Same item ID in both campfires — only one should be returned.
	addTestItem(t, s, campfire1, "msg-dup-c1", "ready-dup", "Duplicate Item (campfire1)")
	addTestItem(t, s, campfire2, "msg-dup-c2", "ready-dup", "Duplicate Item (campfire2)")

	items, err := resolve.AllItems(s)
	if err != nil {
		t.Fatalf("AllItems returned error: %v", err)
	}
	if len(items) != 1 {
		t.Errorf("expected 1 item after dedup, got %d", len(items))
	}
	if items[0].ID != "ready-dup" {
		t.Errorf("expected ID=ready-dup, got %q", items[0].ID)
	}
}

// TestAllItems_EmptyStore verifies that AllItems returns an empty slice and no error
// when there are no campfire memberships.
func TestAllItems_EmptyStore(t *testing.T) {
	s := openTempStore(t)

	items, err := resolve.AllItems(s)
	if err != nil {
		t.Fatalf("AllItems returned error on empty store: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("expected 0 items on empty store, got %d", len(items))
	}
}

// TestAllItemsInCampfire_FiltersCorrectly verifies that AllItemsInCampfire returns
// only items from the specified campfire, not from other campfires.
func TestAllItemsInCampfire_FiltersCorrectly(t *testing.T) {
	s := openTempStore(t)
	addMembership(t, s, campfire1)
	addMembership(t, s, campfire2)

	addTestItem(t, s, campfire1, "msg-f1a", "ready-f1a", "Campfire1 Item A")
	addTestItem(t, s, campfire1, "msg-f1b", "ready-f1b", "Campfire1 Item B")
	addTestItem(t, s, campfire2, "msg-f2a", "ready-f2a", "Campfire2 Item A")

	items, err := resolve.AllItemsInCampfire(s, campfire1)
	if err != nil {
		t.Fatalf("AllItemsInCampfire returned error: %v", err)
	}
	if len(items) != 2 {
		t.Errorf("expected 2 items from campfire1, got %d", len(items))
	}

	// Verify campfire2 item is not present.
	for _, item := range items {
		if item.ID == "ready-f2a" {
			t.Errorf("AllItemsInCampfire returned campfire2 item %q — should be filtered out", item.ID)
		}
	}

	// Verify both campfire1 items are present.
	ids := make(map[string]bool)
	for _, item := range items {
		ids[item.ID] = true
	}
	for _, wantID := range []string{"ready-f1a", "ready-f1b"} {
		if !ids[wantID] {
			t.Errorf("expected campfire1 item %q in results, but it was missing", wantID)
		}
	}
}

// TestByID_ClosedStore verifies that ByID propagates the error from ListMemberships
// when the store is closed. Same pattern applies to AllItems.
func TestByID_ClosedStore(t *testing.T) {
	s := openTempStore(t)
	// Close the store immediately to force an error on ListMemberships.
	s.Close()

	_, err := resolve.ByID(s, "ready-anything")
	if err == nil {
		t.Fatal("expected error from closed store, got nil")
	}
}
