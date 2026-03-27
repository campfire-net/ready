package storetest_test

import (
	"testing"

	"github.com/campfire-net/ready/pkg/resolve"
	"github.com/campfire-net/ready/pkg/state"
	"github.com/campfire-net/ready/pkg/storetest"
)

// ---------------------------------------------------------------------------
// Create
// ---------------------------------------------------------------------------

// TestStore_Create_DefaultsApplied creates a minimal item and verifies
// that status defaults to inbox and type/priority are stored.
func TestStore_Create_DefaultsApplied(t *testing.T) {
	h := storetest.New(t)
	h.Create("t01", "Task", "p1")

	item := h.MustItem("t01")
	if item.Status != state.StatusInbox {
		t.Errorf("Status: got %q, want %q", item.Status, state.StatusInbox)
	}
	if item.Type != "task" {
		t.Errorf("Type: got %q, want %q", item.Type, "task")
	}
	if item.Priority != "p1" {
		t.Errorf("Priority: got %q, want %q", item.Priority, "p1")
	}
	if item.ETA == "" {
		t.Error("ETA: expected non-empty default ETA for p1")
	}
}

// TestStore_Create_AllFields creates an item with all optional fields
// and verifies that each field round-trips through Derive.
func TestStore_Create_AllFields(t *testing.T) {
	h := storetest.New(t)
	h.Create("t02", "Full item", "p2",
		storetest.WithContext("ctx text"),
		storetest.WithType("decision"),
		storetest.WithLevel("epic"),
		storetest.WithProject("myproject"),
		storetest.WithFor("alice@example.com"),
		storetest.WithBy("bob@example.com"),
		storetest.WithETA("2026-04-01T00:00:00Z"),
		storetest.WithDue("2026-05-01T00:00:00Z"),
		storetest.WithParentID("parent-001"),
	)

	item := h.MustItem("t02")
	if item.Context != "ctx text" {
		t.Errorf("Context: got %q, want %q", item.Context, "ctx text")
	}
	if item.Type != "decision" {
		t.Errorf("Type: got %q, want %q", item.Type, "decision")
	}
	if item.Level != "epic" {
		t.Errorf("Level: got %q, want %q", item.Level, "epic")
	}
	if item.Project != "myproject" {
		t.Errorf("Project: got %q, want %q", item.Project, "myproject")
	}
	if item.For != "alice@example.com" {
		t.Errorf("For: got %q, want %q", item.For, "alice@example.com")
	}
	if item.By != "bob@example.com" {
		t.Errorf("By: got %q, want %q", item.By, "bob@example.com")
	}
	if item.ETA != "2026-04-01T00:00:00Z" {
		t.Errorf("ETA: got %q, want %q", item.ETA, "2026-04-01T00:00:00Z")
	}
	if item.Due != "2026-05-01T00:00:00Z" {
		t.Errorf("Due: got %q, want %q", item.Due, "2026-05-01T00:00:00Z")
	}
	if item.ParentID != "parent-001" {
		t.Errorf("ParentID: got %q, want %q", item.ParentID, "parent-001")
	}
}

// TestStore_Create_ContextDescriptionMirror verifies that item.Context and
// item.Description are both set to the same value when WithContext is used.
func TestStore_Create_ContextDescriptionMirror(t *testing.T) {
	h := storetest.New(t)
	h.Create("t03", "Mirror test", "p1", storetest.WithContext("hello context"))

	item := h.MustItem("t03")
	if item.Context != "hello context" {
		t.Errorf("Context: got %q, want %q", item.Context, "hello context")
	}
	if item.Description != "hello context" {
		t.Errorf("Description: got %q, want %q", item.Description, "hello context")
	}
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

// TestStore_Update_StatusTransition creates an item and updates its status
// to active, verifying the transition is reflected in derived state.
func TestStore_Update_StatusTransition(t *testing.T) {
	h := storetest.New(t)
	msgID := h.Create("t04", "Status item", "p1")

	h.UpdateStatus(msgID, "active")

	item := h.MustItem("t04")
	if item.Status != state.StatusActive {
		t.Errorf("Status: got %q, want %q", item.Status, state.StatusActive)
	}
}

// TestStore_Update_MultipleFields updates two fields in a single work:update
// message and verifies both are applied.
func TestStore_Update_MultipleFields(t *testing.T) {
	h := storetest.New(t)
	msgID := h.Create("t05", "Update fields", "p1")

	h.UpdateFields(msgID, map[string]string{
		"priority": "p0",
		"eta":      "2026-03-26T00:00:00Z",
	})

	item := h.MustItem("t05")
	if item.Priority != "p0" {
		t.Errorf("Priority: got %q, want %q", item.Priority, "p0")
	}
	if item.ETA != "2026-03-26T00:00:00Z" {
		t.Errorf("ETA: got %q, want %q", item.ETA, "2026-03-26T00:00:00Z")
	}
}

// TestStore_Update_ClearSentinel verifies that using "-" as a field value
// clears the field (Due in this case).
func TestStore_Update_ClearSentinel(t *testing.T) {
	h := storetest.New(t)
	msgID := h.Create("t06", "Clear sentinel", "p1", storetest.WithDue("2026-04-01T00:00:00Z"))

	item := h.MustItem("t06")
	if item.Due != "2026-04-01T00:00:00Z" {
		t.Fatalf("pre-condition: Due: got %q, want %q", item.Due, "2026-04-01T00:00:00Z")
	}

	h.UpdateFields(msgID, map[string]string{"due": "-"})

	item = h.MustItem("t06")
	if item.Due != "" {
		t.Errorf("Due after clear: got %q, want empty string", item.Due)
	}
}

// TestStore_Update_WaitingAutoStatus sends a work:status with to=waiting and
// waiting detail fields, then verifies that Derive sets all three waiting fields.
func TestStore_Update_WaitingAutoStatus(t *testing.T) {
	h := storetest.New(t)
	msgID := h.Create("t07", "Waiting item", "p1")

	h.UpdateStatus(msgID, "waiting",
		storetest.WithWaitingOn("vendor response"),
		storetest.WithWaitingType("vendor"),
	)

	item := h.MustItem("t07")
	if item.Status != state.StatusWaiting {
		t.Errorf("Status: got %q, want %q", item.Status, state.StatusWaiting)
	}
	if item.WaitingOn != "vendor response" {
		t.Errorf("WaitingOn: got %q, want %q", item.WaitingOn, "vendor response")
	}
	if item.WaitingType != "vendor" {
		t.Errorf("WaitingType: got %q, want %q", item.WaitingType, "vendor")
	}
	if item.WaitingSince == "" {
		t.Error("WaitingSince: expected non-empty")
	}
}

// ---------------------------------------------------------------------------
// Close
// ---------------------------------------------------------------------------

// TestStore_Close_DoneResolution closes an item with resolution=done and
// verifies status=done and IsTerminal=true.
func TestStore_Close_DoneResolution(t *testing.T) {
	h := storetest.New(t)
	msgID := h.Create("t08", "Done item", "p1")

	h.Close(msgID, "done", "all complete")

	item := h.MustItem("t08")
	if item.Status != state.StatusDone {
		t.Errorf("Status: got %q, want %q", item.Status, state.StatusDone)
	}
	if !state.IsTerminal(item) {
		t.Error("IsTerminal: expected true")
	}
}

// TestStore_Close_CancelledResolution closes an item with resolution=cancelled.
func TestStore_Close_CancelledResolution(t *testing.T) {
	h := storetest.New(t)
	msgID := h.Create("t09", "Cancelled item", "p1")

	h.Close(msgID, "cancelled", "no longer needed")

	item := h.MustItem("t09")
	if item.Status != state.StatusCancelled {
		t.Errorf("Status: got %q, want %q", item.Status, state.StatusCancelled)
	}
	if !state.IsTerminal(item) {
		t.Error("IsTerminal: expected true")
	}
}

// TestStore_Close_FailedResolution closes an item with resolution=failed.
func TestStore_Close_FailedResolution(t *testing.T) {
	h := storetest.New(t)
	msgID := h.Create("t10", "Failed item", "p1")

	h.Close(msgID, "failed", "could not complete")

	item := h.MustItem("t10")
	if item.Status != state.StatusFailed {
		t.Errorf("Status: got %q, want %q", item.Status, state.StatusFailed)
	}
	if !state.IsTerminal(item) {
		t.Error("IsTerminal: expected true")
	}
}

// TestStore_Close_ImplicitUnblock creates two items A and B where B is blocked
// by A. After A is closed, B should no longer be blocked and should appear in Ready.
func TestStore_Close_ImplicitUnblock(t *testing.T) {
	h := storetest.New(t)

	msgA := h.Create("t11a", "Blocker A", "p1")
	msgB := h.Create("t11b", "Blocked B", "p1",
		storetest.WithETA("2000-01-01T00:00:00Z"), // past ETA so it shows in ready
	)

	h.Block("t11a", "t11b", msgA, msgB)

	// Verify B is blocked before closing A.
	b := h.MustItem("t11b")
	if b.Status != state.StatusBlocked {
		t.Fatalf("pre-condition: B should be blocked, got %q", b.Status)
	}

	h.Close(msgA, "done", "blocker complete")

	// After A closes, B should not be blocked.
	b = h.MustItem("t11b")
	if b.Status == state.StatusBlocked {
		t.Errorf("B status: still blocked after A was closed")
	}

	// B should now appear in Ready (status not terminal, not blocked, ETA in past).
	ready := h.Ready()
	found := false
	for _, item := range ready {
		if item.ID == "t11b" {
			found = true
			break
		}
	}
	if !found {
		t.Error("B should appear in Ready after A is closed")
	}
}

// ---------------------------------------------------------------------------
// Update → Close sequence
// ---------------------------------------------------------------------------

// TestStore_UpdateThenClose_AgentWorkflow simulates a full agent workflow:
// create → claim → update status → close, verifying state at each step.
func TestStore_UpdateThenClose_AgentWorkflow(t *testing.T) {
	h := storetest.New(t)
	agent := h.WithSender("agent-pubkey")

	// 1. Create
	msgID := h.Create("t12", "Agent task", "p1")
	item := h.MustItem("t12")
	if item.Status != state.StatusInbox {
		t.Fatalf("after create: Status=%q, want inbox", item.Status)
	}

	// 2. Claim (agent becomes by, status → active)
	agent.Claim(msgID)
	item = h.MustItem("t12")
	if item.Status != state.StatusActive {
		t.Fatalf("after claim: Status=%q, want active", item.Status)
	}
	if item.By != "agent-pubkey" {
		t.Fatalf("after claim: By=%q, want agent-pubkey", item.By)
	}

	// 3. UpdateStatus active (idempotent re-assertion)
	agent.UpdateStatus(msgID, "active", storetest.WithReason("starting work"))
	item = h.MustItem("t12")
	if item.Status != state.StatusActive {
		t.Fatalf("after update-status active: Status=%q, want active", item.Status)
	}

	// 4. Close done
	agent.Close(msgID, "done", "work complete")
	item = h.MustItem("t12")
	if item.Status != state.StatusDone {
		t.Fatalf("after close: Status=%q, want done", item.Status)
	}
	if !state.IsTerminal(item) {
		t.Error("after close: IsTerminal should be true")
	}
}

// ---------------------------------------------------------------------------
// Resolve
// ---------------------------------------------------------------------------

// TestStore_Resolve_ByID creates an item and resolves it by its full ID.
func TestStore_Resolve_ByID(t *testing.T) {
	h := storetest.New(t)
	h.Create("ready-abc123", "Resolve target", "p1")

	item, err := resolve.ByID(h.Store, "ready-abc123")
	if err != nil {
		t.Fatalf("ByID: unexpected error: %v", err)
	}
	if item.ID != "ready-abc123" {
		t.Errorf("ID: got %q, want %q", item.ID, "ready-abc123")
	}
}

// TestStore_Resolve_ByPrefix creates an item and resolves it using a unique prefix.
func TestStore_Resolve_ByPrefix(t *testing.T) {
	h := storetest.New(t)
	h.Create("ready-abc123", "Prefix target", "p1")

	item, err := resolve.ByID(h.Store, "ready-abc")
	if err != nil {
		t.Fatalf("ByID (prefix): unexpected error: %v", err)
	}
	if item.ID != "ready-abc123" {
		t.Errorf("ID: got %q, want %q", item.ID, "ready-abc123")
	}
}

// TestStore_Resolve_AllItems creates three items and verifies AllItemsInCampfire
// returns all three.
func TestStore_Resolve_AllItems(t *testing.T) {
	h := storetest.New(t)
	h.Create("item-r1", "Item one", "p1")
	h.Create("item-r2", "Item two", "p2")
	h.Create("item-r3", "Item three", "p3")

	items, err := resolve.AllItemsInCampfire(h.Store, h.CampfireID)
	if err != nil {
		t.Fatalf("AllItemsInCampfire: unexpected error: %v", err)
	}
	if len(items) != 3 {
		t.Errorf("AllItemsInCampfire: got %d items, want 3", len(items))
	}
	ids := map[string]bool{}
	for _, it := range items {
		ids[it.ID] = true
	}
	for _, want := range []string{"item-r1", "item-r2", "item-r3"} {
		if !ids[want] {
			t.Errorf("AllItemsInCampfire: missing item %q", want)
		}
	}
}
