package main

import (
	"testing"

	"github.com/campfire-net/ready/pkg/state"
)

func makeListItem(id, status string) *state.Item {
	return &state.Item{
		ID:     id,
		Title:  "Test " + id,
		Status: status,
	}
}

// TestList_MultipleStatus_ORSemantics verifies that providing multiple --status
// flags returns items matching ANY of the given statuses (OR semantics).
// This is the core behavior of rudi-19a: rd list --status inbox --status active
// must return both inbox and active items.
func TestList_MultipleStatus_ORSemantics(t *testing.T) {
	items := []*state.Item{
		makeListItem("t1", state.StatusInbox),
		makeListItem("t2", state.StatusActive),
		makeListItem("t3", state.StatusWaiting),
		makeListItem("t4", state.StatusDone),
	}

	result := applyListFilters(items, []string{state.StatusInbox, state.StatusActive}, "", "", "", "", "", false)

	if len(result) != 2 {
		t.Errorf("expected 2 items (inbox + active), got %d", len(result))
	}
	ids := map[string]bool{}
	for _, item := range result {
		ids[item.ID] = true
	}
	if !ids["t1"] {
		t.Errorf("expected t1 (inbox) in result, missing")
	}
	if !ids["t2"] {
		t.Errorf("expected t2 (active) in result, missing")
	}
	if ids["t3"] {
		t.Errorf("t3 (waiting) must not appear when not in status filter")
	}
	if ids["t4"] {
		t.Errorf("t4 (done) must not appear when not in status filter")
	}
}

// TestList_SingleStatus verifies backward compatibility: a single --status flag
// still filters to exactly that status (no regression from StringVar → StringArrayVar).
func TestList_SingleStatus(t *testing.T) {
	items := []*state.Item{
		makeListItem("t1", state.StatusInbox),
		makeListItem("t2", state.StatusActive),
		makeListItem("t3", state.StatusDone),
	}

	result := applyListFilters(items, []string{state.StatusActive}, "", "", "", "", "", false)

	if len(result) != 1 {
		t.Errorf("expected 1 item (active), got %d", len(result))
	}
	if result[0].ID != "t2" {
		t.Errorf("expected t2 (active), got %s", result[0].ID)
	}
}

// TestList_NoStatus_DefaultExcludesTerminal verifies that when no --status is
// provided, terminal items (done, cancelled, failed) are excluded by default.
// This is the existing behavior that must be preserved unchanged.
func TestList_NoStatus_DefaultExcludesTerminal(t *testing.T) {
	items := []*state.Item{
		makeListItem("t1", state.StatusInbox),
		makeListItem("t2", state.StatusActive),
		makeListItem("t3", state.StatusDone),
		makeListItem("t4", state.StatusCancelled),
		makeListItem("t5", state.StatusFailed),
		makeListItem("t6", state.StatusWaiting),
	}

	result := applyListFilters(items, nil, "", "", "", "", "", false)

	// Terminal statuses must be excluded.
	for _, item := range result {
		if state.IsTerminal(item) {
			t.Errorf("terminal item %s (status=%s) must not appear in default list", item.ID, item.Status)
		}
	}
	// Non-terminal items must be included.
	if len(result) != 3 {
		t.Errorf("expected 3 non-terminal items (inbox+active+waiting), got %d", len(result))
	}
}

// TestList_NoStatus_AllFlagIncludesTerminal verifies that --all overrides the
// default terminal exclusion when no --status is given.
func TestList_NoStatus_AllFlagIncludesTerminal(t *testing.T) {
	items := []*state.Item{
		makeListItem("t1", state.StatusActive),
		makeListItem("t2", state.StatusDone),
		makeListItem("t3", state.StatusCancelled),
	}

	result := applyListFilters(items, nil, "", "", "", "", "", true)

	if len(result) != 3 {
		t.Errorf("expected 3 items with --all, got %d", len(result))
	}
}

// TestList_StatusAlias_InProgress verifies that items with status=active match
// the alias filter "in_progress" (bd-compat alias).
func TestList_StatusAlias_InProgress(t *testing.T) {
	items := []*state.Item{
		makeListItem("t1", state.StatusActive),
		makeListItem("t2", state.StatusInbox),
		makeListItem("t3", state.StatusDone),
	}

	result := applyListFilters(items, []string{"in_progress"}, "", "", "", "", "", false)

	if len(result) != 1 {
		t.Errorf("expected 1 item (active via in_progress alias), got %d", len(result))
	}
	if len(result) > 0 && result[0].ID != "t1" {
		t.Errorf("expected t1 (active), got %s", result[0].ID)
	}
}

// TestList_StatusAlias_Closed verifies that items with status=done match
// the alias filter "closed" (bd-compat alias).
func TestList_StatusAlias_Closed(t *testing.T) {
	items := []*state.Item{
		makeListItem("t1", state.StatusActive),
		makeListItem("t2", state.StatusDone),
		makeListItem("t3", state.StatusCancelled),
	}

	result := applyListFilters(items, []string{"closed"}, "", "", "", "", "", false)

	if len(result) != 1 {
		t.Errorf("expected 1 item (done via closed alias), got %d", len(result))
	}
	if len(result) > 0 && result[0].ID != "t2" {
		t.Errorf("expected t2 (done), got %s", result[0].ID)
	}
}

// TestList_StatusAlias_MixedCanonicalAndAlias verifies that a filter containing
// both an alias ("in_progress") and a canonical status ("inbox") matches items
// of both statuses.
func TestList_StatusAlias_MixedCanonicalAndAlias(t *testing.T) {
	items := []*state.Item{
		makeListItem("t1", state.StatusActive),
		makeListItem("t2", state.StatusInbox),
		makeListItem("t3", state.StatusWaiting),
		makeListItem("t4", state.StatusDone),
	}

	result := applyListFilters(items, []string{"in_progress", state.StatusInbox}, "", "", "", "", "", false)

	if len(result) != 2 {
		t.Errorf("expected 2 items (active via in_progress + inbox), got %d", len(result))
	}
	ids := map[string]bool{}
	for _, item := range result {
		ids[item.ID] = true
	}
	if !ids["t1"] {
		t.Errorf("expected t1 (active via in_progress alias)")
	}
	if !ids["t2"] {
		t.Errorf("expected t2 (inbox canonical)")
	}
	if ids["t3"] {
		t.Errorf("t3 (waiting) must not appear")
	}
}

// TestList_StatusAlias_Unknown verifies that an unknown/unrecognised filter value
// matches no items (not an alias, not a canonical status).
func TestList_StatusAlias_Unknown(t *testing.T) {
	items := []*state.Item{
		makeListItem("t1", state.StatusActive),
		makeListItem("t2", state.StatusInbox),
	}

	result := applyListFilters(items, []string{"foobar"}, "", "", "", "", "", false)

	if len(result) != 0 {
		t.Errorf("expected 0 items for unknown filter, got %d", len(result))
	}
}

// TestList_MultipleStatus_IncludesTerminalExplicitly verifies that providing a
// terminal status explicitly in --status returns those items (no default exclusion
// when statuses are explicitly specified).
func TestList_MultipleStatus_IncludesTerminalExplicitly(t *testing.T) {
	items := []*state.Item{
		makeListItem("t1", state.StatusInbox),
		makeListItem("t2", state.StatusDone),
		makeListItem("t3", state.StatusCancelled),
	}

	// Explicitly asking for done and cancelled must return them.
	result := applyListFilters(items, []string{state.StatusDone, state.StatusCancelled}, "", "", "", "", "", false)

	if len(result) != 2 {
		t.Errorf("expected 2 items (done+cancelled explicitly requested), got %d", len(result))
	}
	ids := map[string]bool{}
	for _, item := range result {
		ids[item.ID] = true
	}
	if !ids["t2"] {
		t.Errorf("expected t2 (done) in result")
	}
	if !ids["t3"] {
		t.Errorf("expected t3 (cancelled) in result")
	}
}
