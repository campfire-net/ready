package storetest_test

import (
	"testing"

	"github.com/3dl-dev/ready/pkg/state"
	"github.com/3dl-dev/ready/pkg/storetest"
)

// TestStore_Dep_AddBlocksItem creates items A and B, blocks B on A, and
// verifies that IsBlocked(B) is true.
func TestStore_Dep_AddBlocksItem(t *testing.T) {
	h := storetest.New(t)
	msgA := h.Create("dep-a1", "Blocker A", "p1")
	msgB := h.Create("dep-b1", "Blocked B", "p1")

	h.Block("dep-a1", "dep-b1", msgA, msgB)

	b := h.MustItem("dep-b1")
	if !state.IsBlocked(b) {
		t.Errorf("IsBlocked(B): got false, want true")
	}
}

// TestStore_Dep_RemoveUnblocks blocks B on A then unblocks it and verifies
// that IsBlocked(B) is false.
func TestStore_Dep_RemoveUnblocks(t *testing.T) {
	h := storetest.New(t)
	msgA := h.Create("dep-a2", "Blocker A", "p1")
	msgB := h.Create("dep-b2", "Blocked B", "p1")

	blockMsgID := h.Block("dep-a2", "dep-b2", msgA, msgB)

	b := h.MustItem("dep-b2")
	if !state.IsBlocked(b) {
		t.Fatalf("pre-condition: B should be blocked, got IsBlocked=false")
	}

	h.Unblock(blockMsgID)

	b = h.MustItem("dep-b2")
	if state.IsBlocked(b) {
		t.Errorf("IsBlocked(B) after unblock: got true, want false")
	}
}

// TestStore_Dep_MultipleBlockers creates B blocked by both A and C.
// Closing A leaves B still blocked. Closing C unblocks B.
func TestStore_Dep_MultipleBlockers(t *testing.T) {
	h := storetest.New(t)
	msgA := h.Create("dep-a3", "Blocker A", "p1")
	msgC := h.Create("dep-c3", "Blocker C", "p1")
	msgB := h.Create("dep-b3", "Blocked B", "p1")

	h.Block("dep-a3", "dep-b3", msgA, msgB)
	h.Block("dep-c3", "dep-b3", msgC, msgB)

	b := h.MustItem("dep-b3")
	if !state.IsBlocked(b) {
		t.Fatalf("pre-condition: B should be blocked by A and C")
	}

	// Close A — B is still blocked by C.
	h.Close(msgA, "done", "A complete")
	b = h.MustItem("dep-b3")
	if !state.IsBlocked(b) {
		t.Errorf("after closing A: B should still be blocked by C")
	}

	// Close C — B is now unblocked.
	h.Close(msgC, "done", "C complete")
	b = h.MustItem("dep-b3")
	if state.IsBlocked(b) {
		t.Errorf("after closing A and C: B should be unblocked")
	}
}

// TestStore_Dep_CascadeUnblock creates a chain A→B→C. Closing A unblocks B;
// closing B unblocks C.
func TestStore_Dep_CascadeUnblock(t *testing.T) {
	h := storetest.New(t)
	msgA := h.Create("dep-a4", "Chain A", "p1")
	msgB := h.Create("dep-b4", "Chain B", "p1")
	msgC := h.Create("dep-c4", "Chain C", "p1")

	h.Block("dep-a4", "dep-b4", msgA, msgB)
	h.Block("dep-b4", "dep-c4", msgB, msgC)

	// Both B and C should be blocked initially.
	if !state.IsBlocked(h.MustItem("dep-b4")) {
		t.Fatal("pre-condition: B should be blocked by A")
	}
	if !state.IsBlocked(h.MustItem("dep-c4")) {
		t.Fatal("pre-condition: C should be blocked by B")
	}

	// Close A → B is unblocked, C remains blocked by B.
	h.Close(msgA, "done", "A complete")
	if state.IsBlocked(h.MustItem("dep-b4")) {
		t.Errorf("after closing A: B should be unblocked")
	}
	if !state.IsBlocked(h.MustItem("dep-c4")) {
		t.Errorf("after closing A: C should still be blocked by B")
	}

	// Close B → C is unblocked.
	h.Close(msgB, "done", "B complete")
	if state.IsBlocked(h.MustItem("dep-c4")) {
		t.Errorf("after closing B: C should be unblocked")
	}
}

// TestStore_Dep_DiamondDep creates a diamond: D blocked by B and C, both blocked
// by A. After closing A, B and C unblock; D remains blocked until both B and C
// are closed.
func TestStore_Dep_DiamondDep(t *testing.T) {
	h := storetest.New(t)
	msgA := h.Create("dep-a5", "Root A", "p1")
	msgB := h.Create("dep-b5", "Branch B", "p1")
	msgC := h.Create("dep-c5", "Branch C", "p1")
	msgD := h.Create("dep-d5", "Diamond D", "p1")

	h.Block("dep-a5", "dep-b5", msgA, msgB)
	h.Block("dep-a5", "dep-c5", msgA, msgC)
	h.Block("dep-b5", "dep-d5", msgB, msgD)
	h.Block("dep-c5", "dep-d5", msgC, msgD)

	// Pre-condition: B, C, D all blocked.
	if !state.IsBlocked(h.MustItem("dep-b5")) {
		t.Fatal("pre-condition: B should be blocked by A")
	}
	if !state.IsBlocked(h.MustItem("dep-c5")) {
		t.Fatal("pre-condition: C should be blocked by A")
	}
	if !state.IsBlocked(h.MustItem("dep-d5")) {
		t.Fatal("pre-condition: D should be blocked by B and C")
	}

	// Close A → B and C unblocked; D still blocked by B and C.
	h.Close(msgA, "done", "A complete")
	if state.IsBlocked(h.MustItem("dep-b5")) {
		t.Errorf("after closing A: B should be unblocked")
	}
	if state.IsBlocked(h.MustItem("dep-c5")) {
		t.Errorf("after closing A: C should be unblocked")
	}
	if !state.IsBlocked(h.MustItem("dep-d5")) {
		t.Errorf("after closing A: D should still be blocked (B and C not closed)")
	}

	// Close B → D still blocked by C.
	h.Close(msgB, "done", "B complete")
	if !state.IsBlocked(h.MustItem("dep-d5")) {
		t.Errorf("after closing B: D should still be blocked by C")
	}

	// Close C → D unblocked.
	h.Close(msgC, "done", "C complete")
	if state.IsBlocked(h.MustItem("dep-d5")) {
		t.Errorf("after closing C: D should be unblocked")
	}
}

// TestStore_Dep_BlockedByField verifies that after blocking B on A, B.BlockedBy
// contains A's item ID.
func TestStore_Dep_BlockedByField(t *testing.T) {
	h := storetest.New(t)
	msgA := h.Create("dep-a6", "Blocker A", "p1")
	msgB := h.Create("dep-b6", "Blocked B", "p1")

	h.Block("dep-a6", "dep-b6", msgA, msgB)

	b := h.MustItem("dep-b6")
	found := false
	for _, id := range b.BlockedBy {
		if id == "dep-a6" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("B.BlockedBy: want \"dep-a6\" in %v", b.BlockedBy)
	}
}

// TestStore_Dep_BlocksField verifies that after blocking B on A, A.Blocks
// contains B's item ID.
func TestStore_Dep_BlocksField(t *testing.T) {
	h := storetest.New(t)
	msgA := h.Create("dep-a7", "Blocker A", "p1")
	msgB := h.Create("dep-b7", "Blocked B", "p1")

	h.Block("dep-a7", "dep-b7", msgA, msgB)

	a := h.MustItem("dep-a7")
	found := false
	for _, id := range a.Blocks {
		if id == "dep-b7" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("A.Blocks: want \"dep-b7\" in %v", a.Blocks)
	}
}

// TestStore_Dep_BatchWiring creates a linear chain of 5 items (1→2→3→4→5).
// Only item 1 is initially ready. Closing each item in sequence makes the next
// ready.
func TestStore_Dep_BatchWiring(t *testing.T) {
	h := storetest.New(t)

	// Create all items with a past ETA so they qualify for the ready view.
	pastETA := "2000-01-01T00:00:00Z"
	msg := make([]string, 5)
	for i := 0; i < 5; i++ {
		id := "dep-chain" + string(rune('1'+i))
		msg[i] = h.Create(id, "Chain item "+string(rune('1'+i)), "p1",
			storetest.WithETA(pastETA),
		)
	}

	// Wire: 1→2→3→4→5 (each blocks the next).
	for i := 0; i < 4; i++ {
		blockerID := "dep-chain" + string(rune('1'+i))
		blockedID := "dep-chain" + string(rune('2'+i))
		h.Block(blockerID, blockedID, msg[i], msg[i+1])
	}

	// Only item 1 should be ready.
	readyIDs := func() map[string]bool {
		m := map[string]bool{}
		for _, it := range h.Ready() {
			m[it.ID] = true
		}
		return m
	}

	r := readyIDs()
	if !r["dep-chain1"] {
		t.Fatalf("initial: want dep-chain1 in ready, got %v", r)
	}
	for _, id := range []string{"dep-chain2", "dep-chain3", "dep-chain4", "dep-chain5"} {
		if r[id] {
			t.Errorf("initial: %q should not be in ready", id)
		}
	}

	// Close each item and verify the next one becomes ready.
	sequence := []string{"dep-chain1", "dep-chain2", "dep-chain3", "dep-chain4"}
	for i, closeID := range sequence {
		h.Close(msg[i], "done", closeID+" complete")
		r = readyIDs()
		nextID := "dep-chain" + string(rune('2'+i))
		if !r[nextID] {
			t.Errorf("after closing %s: want %s in ready, got %v", closeID, nextID, r)
		}
	}
}

// TestStore_Dep_RemoveRestoresReady blocks B on A, then unblocks it, and
// verifies that B appears in the ready view even though A is still open.
func TestStore_Dep_RemoveRestoresReady(t *testing.T) {
	h := storetest.New(t)
	msgA := h.Create("dep-a9", "Blocker A", "p1")
	msgB := h.Create("dep-b9", "Blocked B", "p1",
		storetest.WithETA("2000-01-01T00:00:00Z"), // past ETA so it qualifies for ready
	)

	blockMsgID := h.Block("dep-a9", "dep-b9", msgA, msgB)

	b := h.MustItem("dep-b9")
	if !state.IsBlocked(b) {
		t.Fatalf("pre-condition: B should be blocked")
	}

	h.Unblock(blockMsgID)

	b = h.MustItem("dep-b9")
	if state.IsBlocked(b) {
		t.Errorf("after unblock: B should not be blocked")
	}

	// B should appear in ready (A is still open, but B is no longer blocked).
	ready := h.Ready()
	found := false
	for _, item := range ready {
		if item.ID == "dep-b9" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("B should appear in Ready after unblock (A still open)")
	}
}
