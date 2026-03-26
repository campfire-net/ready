package main

import (
	"testing"
	"time"

	"github.com/3dl-dev/ready/pkg/state"
	"github.com/3dl-dev/ready/pkg/views"
)

func makeTestItem(id, status, gate string) *state.Item {
	return &state.Item{
		ID:       id,
		Title:    "Test " + id,
		Status:   status,
		Priority: "p1",
		Type:     "task",
		Gate:     gate,
	}
}

// TestPendingViewFilter verifies that the pending command uses PendingFilter correctly.
func TestPendingViewFilter(t *testing.T) {
	items := []*state.Item{
		makeTestItem("t1", state.StatusWaiting, ""),
		makeTestItem("t2", state.StatusScheduled, ""),
		makeTestItem("t3", state.StatusBlocked, ""),
		makeTestItem("t4", state.StatusActive, ""),
		makeTestItem("t5", state.StatusInbox, ""),
		makeTestItem("t6", state.StatusDone, ""),
	}

	filter := views.PendingFilter()
	result := views.Apply(items, filter)

	if len(result) != 3 {
		t.Errorf("expected 3 pending items (waiting+scheduled+blocked), got %d", len(result))
	}
	for _, item := range result {
		switch item.Status {
		case state.StatusWaiting, state.StatusScheduled, state.StatusBlocked:
			// expected
		default:
			t.Errorf("unexpected status %q in pending view", item.Status)
		}
	}
}

// TestWorkViewFilter verifies that the work command uses WorkFilter correctly.
func TestWorkViewFilter(t *testing.T) {
	items := []*state.Item{
		makeTestItem("t1", state.StatusActive, ""),
		makeTestItem("t2", state.StatusActive, ""),
		makeTestItem("t3", state.StatusInbox, ""),
		makeTestItem("t4", state.StatusDone, ""),
	}

	filter := views.WorkFilter()
	result := views.Apply(items, filter)

	if len(result) != 2 {
		t.Errorf("expected 2 active items, got %d", len(result))
	}
	for _, item := range result {
		if item.Status != state.StatusActive {
			t.Errorf("expected all work items to be active, got %q", item.Status)
		}
	}
}

// TestWorkViewWithForFilter verifies that --for uses MyWorkFilter.
func TestWorkViewWithForFilter(t *testing.T) {
	items := []*state.Item{
		{ID: "t1", Status: state.StatusActive, By: "me@test.com", Title: "mine"},
		{ID: "t2", Status: state.StatusActive, By: "other@test.com", Title: "theirs"},
		{ID: "t3", Status: state.StatusDone, By: "me@test.com", Title: "done mine"},
	}

	filter := views.MyWorkFilter("me@test.com")
	result := views.Apply(items, filter)

	if len(result) != 1 {
		t.Errorf("expected 1 item for me@test.com (not done), got %d", len(result))
	}
	if result[0].ID != "t1" {
		t.Errorf("expected t1, got %s", result[0].ID)
	}
}

// TestFocusViewNoGate verifies FocusFilter with no gate returns ready items.
func TestFocusViewNoGate(t *testing.T) {
	now := time.Now()
	nearETA := now.Add(1 * time.Hour).UTC().Format(time.RFC3339)
	farETA := now.Add(10 * time.Hour).UTC().Format(time.RFC3339)

	items := []*state.Item{
		{ID: "t1", Status: state.StatusActive, Priority: "p1", ETA: nearETA, Title: "near", Gate: ""},
		{ID: "t2", Status: state.StatusActive, Priority: "p1", ETA: farETA, Title: "far", Gate: "design"},
		{ID: "t3", Status: state.StatusDone, Priority: "p1", ETA: nearETA, Title: "done", Gate: ""},
	}

	filter := views.FocusFilter("")
	result := views.Apply(items, filter)

	// Only t1 should match: active, near ETA, not terminal.
	if len(result) != 1 {
		t.Errorf("expected 1 item (near ETA, not terminal), got %d", len(result))
	}
	if result[0].ID != "t1" {
		t.Errorf("expected t1, got %s", result[0].ID)
	}
}

// TestFocusViewWithGateFilter verifies FocusFilter with gateType narrows to gate items.
func TestFocusViewWithGateFilter(t *testing.T) {
	now := time.Now()
	nearETA := now.Add(1 * time.Hour).UTC().Format(time.RFC3339)

	items := []*state.Item{
		{ID: "t1", Status: state.StatusActive, Priority: "p1", ETA: nearETA, Title: "design gate", Gate: "design"},
		{ID: "t2", Status: state.StatusActive, Priority: "p1", ETA: nearETA, Title: "no gate", Gate: ""},
		{ID: "t3", Status: state.StatusActive, Priority: "p1", ETA: nearETA, Title: "budget gate", Gate: "budget"},
	}

	filter := views.FocusFilter("design")
	result := views.Apply(items, filter)

	if len(result) != 1 {
		t.Errorf("expected 1 item with gate=design, got %d", len(result))
	}
	if result[0].ID != "t1" {
		t.Errorf("expected t1 (gate=design), got %s", result[0].ID)
	}
}

// TestFocusViewGateNotReady verifies that gate items with far ETAs don't appear.
func TestFocusViewGateNotReady(t *testing.T) {
	now := time.Now()
	farETA := now.Add(10 * time.Hour).UTC().Format(time.RFC3339)

	items := []*state.Item{
		{ID: "t1", Status: state.StatusActive, Priority: "p1", ETA: farETA, Title: "far design gate", Gate: "design"},
	}

	filter := views.FocusFilter("design")
	result := views.Apply(items, filter)

	if len(result) != 0 {
		t.Errorf("expected 0 items (ETA too far), got %d", len(result))
	}
}

// TestFocusViewReturnsAllGateTypes verifies FocusFilter with different gate types.
func TestFocusViewAllGateTypes(t *testing.T) {
	now := time.Now()
	nearETA := now.Add(30 * time.Minute).UTC().Format(time.RFC3339)
	gateTypes := []string{"budget", "design", "scope", "review", "human", "stall"}

	for _, gt := range gateTypes {
		item := &state.Item{
			ID:     "t-" + gt,
			Status: state.StatusActive,
			ETA:    nearETA,
			Title:  gt + " gate",
			Gate:   gt,
		}
		filter := views.FocusFilter(gt)
		result := views.Apply([]*state.Item{item}, filter)
		if len(result) != 1 {
			t.Errorf("gate type %q: expected 1 result, got %d", gt, len(result))
		}
	}
}
