package storetest_test

import (
	"testing"

	"github.com/3dl-dev/ready/pkg/state"
	"github.com/3dl-dev/ready/pkg/storetest"
)

// ---------------------------------------------------------------------------
// Gate lifecycle tests
// ---------------------------------------------------------------------------

// TestStore_Gate_TransitionsToWaiting verifies that sending a work:gate message
// transitions the item to status=waiting with waiting_type="gate".
func TestStore_Gate_TransitionsToWaiting(t *testing.T) {
	h := storetest.New(t)
	msgID := h.Create("gate-01", "Gate item", "p1")

	h.Gate(msgID, "design", "Need approval for API shape")

	item := h.MustItem("gate-01")
	if item.Status != state.StatusWaiting {
		t.Errorf("Status: got %q, want %q", item.Status, state.StatusWaiting)
	}
	if item.WaitingType != "gate" {
		t.Errorf("WaitingType: got %q, want %q", item.WaitingType, "gate")
	}
}

// TestStore_Gate_AppearsInGatesView verifies that a gated item appears in
// ViewItems("gates", "").
func TestStore_Gate_AppearsInGatesView(t *testing.T) {
	h := storetest.New(t)
	msgID := h.Create("gate-02", "Gate view item", "p1")
	h.Gate(msgID, "design", "Needs design approval")

	gateItems := h.ViewItems("gates", "")
	found := false
	for _, item := range gateItems {
		if item.ID == "gate-02" {
			found = true
			break
		}
	}
	if !found {
		t.Error("item should appear in gates view after Gate()")
	}
}

// TestStore_Gate_StoresGateMsgID verifies that item.GateMsgID is set to the
// message ID returned by Gate().
func TestStore_Gate_StoresGateMsgID(t *testing.T) {
	h := storetest.New(t)
	msgID := h.Create("gate-03", "Gate msg ID item", "p1")
	gateMsgID := h.Gate(msgID, "scope", "Scope check needed")

	item := h.MustItem("gate-03")
	if item.GateMsgID == "" {
		t.Fatal("GateMsgID: expected non-empty after Gate()")
	}
	if item.GateMsgID != gateMsgID {
		t.Errorf("GateMsgID: got %q, want %q", item.GateMsgID, gateMsgID)
	}
}

// TestStore_Gate_ApproveTransitionsToActive verifies that GateResolve("approved")
// transitions the item to status=active and clears all waiting fields.
func TestStore_Gate_ApproveTransitionsToActive(t *testing.T) {
	h := storetest.New(t)
	msgID := h.Create("gate-04", "Approve transitions", "p1")
	gateMsgID := h.Gate(msgID, "design", "Need design approval")

	h.GateResolve(gateMsgID, "approved")

	item := h.MustItem("gate-04")
	if item.Status != state.StatusActive {
		t.Errorf("Status: got %q, want %q", item.Status, state.StatusActive)
	}
	if item.WaitingType != "" {
		t.Errorf("WaitingType: got %q, want empty after approval", item.WaitingType)
	}
	if item.GateMsgID != "" {
		t.Errorf("GateMsgID: got %q, want empty after approval", item.GateMsgID)
	}
}

// TestStore_Gate_ApproveRemovesFromGatesView verifies that after approval,
// the item no longer appears in the gates view.
func TestStore_Gate_ApproveRemovesFromGatesView(t *testing.T) {
	h := storetest.New(t)
	msgID := h.Create("gate-05", "Approve removes", "p1")
	gateMsgID := h.Gate(msgID, "budget", "Budget approval needed")

	h.GateResolve(gateMsgID, "approved")

	gateItems := h.ViewItems("gates", "")
	for _, item := range gateItems {
		if item.ID == "gate-05" {
			t.Error("item should not appear in gates view after approval")
		}
	}
}

// TestStore_Gate_ApproveAppearsInWorkView verifies that after approval,
// the item appears in ViewItems("work", "").
func TestStore_Gate_ApproveAppearsInWorkView(t *testing.T) {
	h := storetest.New(t)
	msgID := h.Create("gate-06", "Approve in work view", "p1")
	gateMsgID := h.Gate(msgID, "review", "Review needed")

	h.GateResolve(gateMsgID, "approved")

	workItems := h.ViewItems("work", "")
	found := false
	for _, item := range workItems {
		if item.ID == "gate-06" {
			found = true
			break
		}
	}
	if !found {
		t.Error("item should appear in work view after approval")
	}
}

// TestStore_Gate_RejectKeepsWaiting verifies that GateResolve("rejected")
// leaves the item in status=waiting (unchanged).
func TestStore_Gate_RejectKeepsWaiting(t *testing.T) {
	h := storetest.New(t)
	msgID := h.Create("gate-07", "Reject keeps waiting", "p1")
	gateMsgID := h.Gate(msgID, "human", "Human review required")

	h.GateResolve(gateMsgID, "rejected")

	item := h.MustItem("gate-07")
	if item.Status != state.StatusWaiting {
		t.Errorf("Status: got %q, want %q after rejection", item.Status, state.StatusWaiting)
	}
}

// TestStore_Gate_RejectKeepsInGatesView verifies that after rejection,
// the item still appears in the gates view with GateMsgID preserved.
func TestStore_Gate_RejectKeepsInGatesView(t *testing.T) {
	h := storetest.New(t)
	msgID := h.Create("gate-08", "Reject in gates view", "p1")
	gateMsgID := h.Gate(msgID, "scope", "Scope approval needed")

	h.GateResolve(gateMsgID, "rejected")

	// GateMsgID should still be set after rejection.
	item := h.MustItem("gate-08")
	if item.GateMsgID == "" {
		t.Error("GateMsgID: expected non-empty after rejection")
	}

	// Item should still appear in gates view.
	gateItems := h.ViewItems("gates", "")
	found := false
	for _, gi := range gateItems {
		if gi.ID == "gate-08" {
			found = true
			break
		}
	}
	if !found {
		t.Error("item should still appear in gates view after rejection")
	}
}

// TestStore_Gate_FullWorkflow verifies the complete gate lifecycle:
// create → Claim → Gate("budget","Need approval") → [waiting] →
// GateResolve("approved") → [active] → Close("done","approved and completed").
// Verifies every intermediate state.
func TestStore_Gate_FullWorkflow(t *testing.T) {
	h := storetest.New(t)

	// 1. Create
	msgID := h.Create("gate-09", "Full workflow", "p1")
	item := h.MustItem("gate-09")
	if item.Status != state.StatusInbox {
		t.Fatalf("after create: Status=%q, want inbox", item.Status)
	}

	// 2. Claim → active
	h.Claim(msgID)
	item = h.MustItem("gate-09")
	if item.Status != state.StatusActive {
		t.Fatalf("after claim: Status=%q, want active", item.Status)
	}

	// 3. Gate → waiting
	gateMsgID := h.Gate(msgID, "budget", "Need approval")
	item = h.MustItem("gate-09")
	if item.Status != state.StatusWaiting {
		t.Fatalf("after Gate: Status=%q, want waiting", item.Status)
	}
	if item.WaitingType != "gate" {
		t.Fatalf("after Gate: WaitingType=%q, want gate", item.WaitingType)
	}
	if item.GateMsgID != gateMsgID {
		t.Fatalf("after Gate: GateMsgID=%q, want %q", item.GateMsgID, gateMsgID)
	}
	// Should appear in gates view
	gateItems := h.ViewItems("gates", "")
	gateFound := false
	for _, gi := range gateItems {
		if gi.ID == "gate-09" {
			gateFound = true
			break
		}
	}
	if !gateFound {
		t.Fatal("after Gate: item should appear in gates view")
	}

	// 4. Approve → active
	h.GateResolve(gateMsgID, "approved")
	item = h.MustItem("gate-09")
	if item.Status != state.StatusActive {
		t.Fatalf("after GateResolve(approved): Status=%q, want active", item.Status)
	}
	if item.WaitingType != "" {
		t.Fatalf("after GateResolve(approved): WaitingType=%q, want empty", item.WaitingType)
	}
	if item.GateMsgID != "" {
		t.Fatalf("after GateResolve(approved): GateMsgID=%q, want empty", item.GateMsgID)
	}
	// Should appear in work view
	workItems := h.ViewItems("work", "")
	workFound := false
	for _, wi := range workItems {
		if wi.ID == "gate-09" {
			workFound = true
			break
		}
	}
	if !workFound {
		t.Fatal("after GateResolve(approved): item should appear in work view")
	}
	// Should not appear in gates view
	gateItems = h.ViewItems("gates", "")
	for _, gi := range gateItems {
		if gi.ID == "gate-09" {
			t.Fatal("after GateResolve(approved): item should not appear in gates view")
		}
	}

	// 5. Close → done
	h.Close(msgID, "done", "approved and completed")
	item = h.MustItem("gate-09")
	if item.Status != state.StatusDone {
		t.Fatalf("after Close: Status=%q, want done", item.Status)
	}
	if !state.IsTerminal(item) {
		t.Fatal("after Close: IsTerminal should be true")
	}
}

// TestStore_Gate_SequentialGates verifies that two sequential gates work correctly:
// create → Gate("design") → approve → Gate("budget") → approve → Close("done").
func TestStore_Gate_SequentialGates(t *testing.T) {
	h := storetest.New(t)
	msgID := h.Create("gate-10", "Sequential gates", "p1")

	// First gate: design
	gate1MsgID := h.Gate(msgID, "design", "Design approval needed")
	item := h.MustItem("gate-10")
	if item.Status != state.StatusWaiting {
		t.Fatalf("after first Gate: Status=%q, want waiting", item.Status)
	}
	if item.GateMsgID != gate1MsgID {
		t.Fatalf("after first Gate: GateMsgID=%q, want %q", item.GateMsgID, gate1MsgID)
	}

	// Approve first gate → active
	h.GateResolve(gate1MsgID, "approved")
	item = h.MustItem("gate-10")
	if item.Status != state.StatusActive {
		t.Fatalf("after first approve: Status=%q, want active", item.Status)
	}
	if item.GateMsgID != "" {
		t.Fatalf("after first approve: GateMsgID=%q, want empty", item.GateMsgID)
	}

	// Second gate: budget
	gate2MsgID := h.Gate(msgID, "budget", "Budget approval needed")
	item = h.MustItem("gate-10")
	if item.Status != state.StatusWaiting {
		t.Fatalf("after second Gate: Status=%q, want waiting", item.Status)
	}
	if item.GateMsgID != gate2MsgID {
		t.Fatalf("after second Gate: GateMsgID=%q, want %q", item.GateMsgID, gate2MsgID)
	}
	// Should appear in gates view
	gateItems := h.ViewItems("gates", "")
	gateFound := false
	for _, gi := range gateItems {
		if gi.ID == "gate-10" {
			gateFound = true
			break
		}
	}
	if !gateFound {
		t.Fatal("after second Gate: item should appear in gates view")
	}

	// Approve second gate → active
	h.GateResolve(gate2MsgID, "approved")
	item = h.MustItem("gate-10")
	if item.Status != state.StatusActive {
		t.Fatalf("after second approve: Status=%q, want active", item.Status)
	}
	if item.GateMsgID != "" {
		t.Fatalf("after second approve: GateMsgID=%q, want empty", item.GateMsgID)
	}

	// Close
	h.Close(msgID, "done", "both gates passed")
	item = h.MustItem("gate-10")
	if item.Status != state.StatusDone {
		t.Fatalf("after Close: Status=%q, want done", item.Status)
	}
	if !state.IsTerminal(item) {
		t.Fatal("after Close: IsTerminal should be true")
	}
}
