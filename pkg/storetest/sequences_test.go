package storetest_test

import (
	"testing"

	"github.com/3dl-dev/ready/pkg/state"
	"github.com/3dl-dev/ready/pkg/storetest"
)

// ---------------------------------------------------------------------------
// Show / multi-item read sequences
// ---------------------------------------------------------------------------

// TestStore_Seq_ShowShow_MultiItemRead creates 5 items, derives state, and
// verifies each item is accessible by ID with the correct title and status.
func TestStore_Seq_ShowShow_MultiItemRead(t *testing.T) {
	h := storetest.New(t)

	items := []struct {
		id    string
		title string
	}{
		{"seq-show-1", "Alpha"},
		{"seq-show-2", "Beta"},
		{"seq-show-3", "Gamma"},
		{"seq-show-4", "Delta"},
		{"seq-show-5", "Epsilon"},
	}

	for _, it := range items {
		h.Create(it.id, it.title, "p1")
	}

	derived := h.Derive()
	if len(derived) != 5 {
		t.Errorf("Derive: got %d items, want 5", len(derived))
	}

	for _, it := range items {
		got := h.MustItem(it.id)
		if got.Title != it.title {
			t.Errorf("item %s: Title got %q, want %q", it.id, got.Title, it.title)
		}
		if got.Status != state.StatusInbox {
			t.Errorf("item %s: Status got %q, want inbox", it.id, got.Status)
		}
	}
}

// ---------------------------------------------------------------------------
// Update → Close: work completion
// ---------------------------------------------------------------------------

// TestStore_Seq_UpdateClose_WorkCompletion exercises the standard agent work
// completion path: create → claim → UpdateStatus("active") → Close("done").
// State is verified at each step.
func TestStore_Seq_UpdateClose_WorkCompletion(t *testing.T) {
	h := storetest.New(t)
	agent := h.WithSender("agent-key")

	// Step 1: create.
	createMsgID := h.Create("seq-wc-1", "Work item", "p1")
	item := h.MustItem("seq-wc-1")
	if item.Status != state.StatusInbox {
		t.Fatalf("after create: Status got %q, want inbox", item.Status)
	}

	// Step 2: claim — by set, status → active.
	agent.Claim(createMsgID)
	item = h.MustItem("seq-wc-1")
	if item.Status != state.StatusActive {
		t.Fatalf("after claim: Status got %q, want active", item.Status)
	}
	if item.By != "agent-key" {
		t.Fatalf("after claim: By got %q, want agent-key", item.By)
	}

	// Step 3: UpdateStatus("active") — idempotent.
	agent.UpdateStatus(createMsgID, "active")
	item = h.MustItem("seq-wc-1")
	if item.Status != state.StatusActive {
		t.Fatalf("after UpdateStatus(active): Status got %q, want active", item.Status)
	}

	// Step 4: close done.
	agent.Close(createMsgID, "done", "reason")
	item = h.MustItem("seq-wc-1")
	if item.Status != state.StatusDone {
		t.Fatalf("after close: Status got %q, want done", item.Status)
	}
	if !state.IsTerminal(item) {
		t.Error("after close: IsTerminal expected true")
	}
}

// ---------------------------------------------------------------------------
// Update → Close: waiting interlude
// ---------------------------------------------------------------------------

// TestStore_Seq_UpdateClose_WithWaitingInterlude exercises a workflow where the
// agent pauses on a waiting status then resumes: create → claim →
// UpdateStatus("waiting") → UpdateStatus("active") → Close("done").
// Waiting fields must be set while waiting and cleared when resuming.
func TestStore_Seq_UpdateClose_WithWaitingInterlude(t *testing.T) {
	h := storetest.New(t)
	agent := h.WithSender("agent-key")

	createMsgID := h.Create("seq-wi-1", "Waiting item", "p1")

	agent.Claim(createMsgID)
	item := h.MustItem("seq-wi-1")
	if item.Status != state.StatusActive {
		t.Fatalf("after claim: Status got %q, want active", item.Status)
	}

	// Transition to waiting with detail fields.
	agent.UpdateStatus(createMsgID, "waiting",
		storetest.WithWaitingOn("review"),
		storetest.WithWaitingType("person"),
	)
	item = h.MustItem("seq-wi-1")
	if item.Status != state.StatusWaiting {
		t.Fatalf("after waiting: Status got %q, want waiting", item.Status)
	}
	if item.WaitingOn != "review" {
		t.Errorf("WaitingOn: got %q, want review", item.WaitingOn)
	}
	if item.WaitingType != "person" {
		t.Errorf("WaitingType: got %q, want person", item.WaitingType)
	}
	if item.WaitingSince == "" {
		t.Error("WaitingSince: expected non-empty while waiting")
	}

	// Back to active — waiting fields must be cleared.
	agent.UpdateStatus(createMsgID, "active")
	item = h.MustItem("seq-wi-1")
	if item.Status != state.StatusActive {
		t.Fatalf("after re-active: Status got %q, want active", item.Status)
	}
	if item.WaitingOn != "" {
		t.Errorf("WaitingOn after re-active: got %q, want empty", item.WaitingOn)
	}
	if item.WaitingType != "" {
		t.Errorf("WaitingType after re-active: got %q, want empty", item.WaitingType)
	}
	if item.WaitingSince != "" {
		t.Errorf("WaitingSince after re-active: got %q, want empty", item.WaitingSince)
	}

	// Close done.
	agent.Close(createMsgID, "done", "done")
	item = h.MustItem("seq-wi-1")
	if item.Status != state.StatusDone {
		t.Fatalf("after close: Status got %q, want done", item.Status)
	}
}

// ---------------------------------------------------------------------------
// Update → Update: multi-field batches
// ---------------------------------------------------------------------------

// TestStore_Seq_UpdateUpdate_MultiFieldBatch creates an item, updates priority
// in one work:update, then updates eta in another. Both changes must be
// present without overwriting each other.
func TestStore_Seq_UpdateUpdate_MultiFieldBatch(t *testing.T) {
	h := storetest.New(t)

	createMsgID := h.Create("seq-uu-1", "Multi-field item", "p2")

	// First update: change priority.
	h.UpdateFields(createMsgID, map[string]string{"priority": "p0"})
	item := h.MustItem("seq-uu-1")
	if item.Priority != "p0" {
		t.Fatalf("after first update: Priority got %q, want p0", item.Priority)
	}

	// Second update: set explicit eta.
	const explicitETA = "2026-06-01T00:00:00Z"
	h.UpdateFields(createMsgID, map[string]string{"eta": explicitETA})
	item = h.MustItem("seq-uu-1")
	if item.Priority != "p0" {
		t.Errorf("after second update: Priority got %q, still want p0", item.Priority)
	}
	if item.ETA != explicitETA {
		t.Errorf("after second update: ETA got %q, want %q", item.ETA, explicitETA)
	}
}

// TestStore_Seq_UpdateUpdate_StatusThenFields creates an item, changes its
// status to active, then updates the title. Both the status change and field
// change must be preserved.
func TestStore_Seq_UpdateUpdate_StatusThenFields(t *testing.T) {
	h := storetest.New(t)

	createMsgID := h.Create("seq-uu-2", "Original title", "p1")

	// Status change.
	h.UpdateStatus(createMsgID, "active")
	item := h.MustItem("seq-uu-2")
	if item.Status != state.StatusActive {
		t.Fatalf("after UpdateStatus: Status got %q, want active", item.Status)
	}

	// Field change: update title.
	h.UpdateFields(createMsgID, map[string]string{"title": "New title"})
	item = h.MustItem("seq-uu-2")
	if item.Status != state.StatusActive {
		t.Errorf("after UpdateFields: Status got %q, still want active", item.Status)
	}
	if item.Title != "New title" {
		t.Errorf("after UpdateFields: Title got %q, want \"New title\"", item.Title)
	}
}

// ---------------------------------------------------------------------------
// Close → Close: batch completion
// ---------------------------------------------------------------------------

// TestStore_Seq_CloseClose_BatchCompletion creates 3 items and closes them
// all in sequence. All must end in a terminal state.
func TestStore_Seq_CloseClose_BatchCompletion(t *testing.T) {
	h := storetest.New(t)

	ids := []string{"seq-cc-1", "seq-cc-2", "seq-cc-3"}
	msgs := make([]string, len(ids))
	for i, id := range ids {
		msgs[i] = h.Create(id, "Item "+id, "p1")
	}

	for i, msgID := range msgs {
		h.Close(msgID, "done", "batch complete")
		item := h.MustItem(ids[i])
		if !state.IsTerminal(item) {
			t.Errorf("%s: IsTerminal got false after close", ids[i])
		}
		if item.Status != state.StatusDone {
			t.Errorf("%s: Status got %q, want done", ids[i], item.Status)
		}
	}
}

// TestStore_Seq_CloseClose_UnblocksNext creates a chain A→B→C. Closing A
// makes B ready; closing B makes C ready; closing C leaves all items terminal.
func TestStore_Seq_CloseClose_UnblocksNext(t *testing.T) {
	h := storetest.New(t)

	pastETA := "2000-01-01T00:00:00Z"
	msgA := h.Create("seq-chain-a", "Chain A", "p1", storetest.WithETA(pastETA))
	msgB := h.Create("seq-chain-b", "Chain B", "p1", storetest.WithETA(pastETA))
	msgC := h.Create("seq-chain-c", "Chain C", "p1", storetest.WithETA(pastETA))

	h.Block("seq-chain-a", "seq-chain-b", msgA, msgB)
	h.Block("seq-chain-b", "seq-chain-c", msgB, msgC)

	// Pre-condition: only A is ready.
	readyIDs := func() map[string]bool {
		m := map[string]bool{}
		for _, it := range h.Ready() {
			m[it.ID] = true
		}
		return m
	}
	r := readyIDs()
	if !r["seq-chain-a"] {
		t.Fatalf("initial: seq-chain-a should be ready, got %v", r)
	}
	if r["seq-chain-b"] || r["seq-chain-c"] {
		t.Fatalf("initial: only A should be ready, got %v", r)
	}

	// Close A → B becomes ready.
	h.Close(msgA, "done", "A done")
	r = readyIDs()
	if !r["seq-chain-b"] {
		t.Errorf("after close A: seq-chain-b should be ready, got %v", r)
	}
	if r["seq-chain-c"] {
		t.Errorf("after close A: seq-chain-c should not be ready yet, got %v", r)
	}

	// Close B → C becomes ready.
	h.Close(msgB, "done", "B done")
	r = readyIDs()
	if !r["seq-chain-c"] {
		t.Errorf("after close B: seq-chain-c should be ready, got %v", r)
	}

	// Close C → all terminal.
	h.Close(msgC, "done", "C done")
	for _, id := range []string{"seq-chain-a", "seq-chain-b", "seq-chain-c"} {
		item := h.MustItem(id)
		if !state.IsTerminal(item) {
			t.Errorf("%s: IsTerminal got false after all closed", id)
		}
	}
}

// ---------------------------------------------------------------------------
// Dep → Dep: dependency patterns
// ---------------------------------------------------------------------------

// TestStore_Seq_DepDep_LinearChain creates 4 items and wires them in a linear
// chain: 1→2→3→4. Only item 1 should appear in the ready view.
func TestStore_Seq_DepDep_LinearChain(t *testing.T) {
	h := storetest.New(t)

	pastETA := "2000-01-01T00:00:00Z"
	ids := []string{"seq-lin-1", "seq-lin-2", "seq-lin-3", "seq-lin-4"}
	msgs := make([]string, len(ids))
	for i, id := range ids {
		msgs[i] = h.Create(id, "Linear "+id, "p1", storetest.WithETA(pastETA))
	}

	// Wire: 1→2→3→4.
	for i := 0; i < 3; i++ {
		h.Block(ids[i], ids[i+1], msgs[i], msgs[i+1])
	}

	// Only item 1 should be ready.
	readyIDs := map[string]bool{}
	for _, it := range h.Ready() {
		readyIDs[it.ID] = true
	}

	if !readyIDs["seq-lin-1"] {
		t.Errorf("seq-lin-1 should be ready, got %v", readyIDs)
	}
	for _, id := range ids[1:] {
		if readyIDs[id] {
			t.Errorf("%s should NOT be ready (blocked), got %v", id, readyIDs)
		}
	}

	// Verify items 2, 3, 4 are blocked.
	for _, id := range ids[1:] {
		item := h.MustItem(id)
		if !state.IsBlocked(item) {
			t.Errorf("%s: expected IsBlocked=true", id)
		}
	}
}

// TestStore_Seq_DepDep_StarPattern creates 5 items where A blocks B, C, D, E.
// Only A is initially ready. After closing A, all four dependents become ready.
func TestStore_Seq_DepDep_StarPattern(t *testing.T) {
	h := storetest.New(t)

	pastETA := "2000-01-01T00:00:00Z"
	msgA := h.Create("seq-star-a", "Hub A", "p1", storetest.WithETA(pastETA))
	deps := []string{"seq-star-b", "seq-star-c", "seq-star-d", "seq-star-e"}
	depMsgs := make([]string, len(deps))
	for i, id := range deps {
		depMsgs[i] = h.Create(id, "Dep "+id, "p1", storetest.WithETA(pastETA))
	}

	// A blocks each dep.
	for i, id := range deps {
		h.Block("seq-star-a", id, msgA, depMsgs[i])
	}

	// Pre-condition: only A is ready.
	readyIDs := func() map[string]bool {
		m := map[string]bool{}
		for _, it := range h.Ready() {
			m[it.ID] = true
		}
		return m
	}
	r := readyIDs()
	if !r["seq-star-a"] {
		t.Fatalf("initial: seq-star-a should be ready, got %v", r)
	}
	for _, id := range deps {
		if r[id] {
			t.Errorf("initial: %s should not be ready (blocked by A)", id)
		}
	}

	// Close A → all 4 dependents become ready.
	h.Close(msgA, "done", "hub done")
	r = readyIDs()
	for _, id := range deps {
		if !r[id] {
			t.Errorf("after close A: %s should be ready, got %v", id, r)
		}
	}
}

// ---------------------------------------------------------------------------
// Canonical full agent workflow
// ---------------------------------------------------------------------------

// TestStore_Seq_CanonicalWorkflow exercises the complete agent work lifecycle:
// create(WithFor,p1) → Delegate(to=agent) → agent.Claim →
// UpdateStatus("waiting",WithWaitingOn,WithWaitingType) →
// UpdateStatus("active",WithReason) → UpdateFields(context) →
// Close("done",reason).
// State is verified after every step (7 assertions).
func TestStore_Seq_CanonicalWorkflow(t *testing.T) {
	h := storetest.New(t)
	agent := h.WithSender("agent-key")

	// Step 1: create with for=baron and p1.
	createMsgID := h.Create("seq-canon-1", "Canonical task", "p1",
		storetest.WithFor("baron"),
	)
	item := h.MustItem("seq-canon-1")
	if item.Status != state.StatusInbox {
		t.Fatalf("step1 create: Status got %q, want inbox", item.Status)
	}
	if item.For != "baron" {
		t.Fatalf("step1 create: For got %q, want baron", item.For)
	}

	// Step 2: delegate to agent.
	h.Delegate(createMsgID, "agent-key")
	item = h.MustItem("seq-canon-1")
	// Delegate sets by but does not change status.
	if item.Status != state.StatusInbox {
		t.Fatalf("step2 delegate: Status got %q, want inbox", item.Status)
	}

	// Step 3: agent claims.
	agent.Claim(createMsgID)
	item = h.MustItem("seq-canon-1")
	if item.Status != state.StatusActive {
		t.Fatalf("step3 claim: Status got %q, want active", item.Status)
	}
	if item.By != "agent-key" {
		t.Fatalf("step3 claim: By got %q, want agent-key", item.By)
	}

	// Step 4: agent transitions to waiting (blocked on design review).
	agent.UpdateStatus(createMsgID, "waiting",
		storetest.WithWaitingOn("design"),
		storetest.WithWaitingType("person"),
	)
	item = h.MustItem("seq-canon-1")
	if item.Status != state.StatusWaiting {
		t.Fatalf("step4 waiting: Status got %q, want waiting", item.Status)
	}
	if item.WaitingOn != "design" {
		t.Errorf("step4 waiting: WaitingOn got %q, want design", item.WaitingOn)
	}
	if item.WaitingType != "person" {
		t.Errorf("step4 waiting: WaitingType got %q, want person", item.WaitingType)
	}
	if item.WaitingSince == "" {
		t.Error("step4 waiting: WaitingSince expected non-empty")
	}

	// Step 5: agent resumes — back to active.
	agent.UpdateStatus(createMsgID, "active",
		storetest.WithReason("got answer"),
	)
	item = h.MustItem("seq-canon-1")
	if item.Status != state.StatusActive {
		t.Fatalf("step5 re-active: Status got %q, want active", item.Status)
	}
	// Waiting fields must be cleared.
	if item.WaitingOn != "" || item.WaitingType != "" || item.WaitingSince != "" {
		t.Errorf("step5 re-active: waiting fields not cleared: on=%q type=%q since=%q",
			item.WaitingOn, item.WaitingType, item.WaitingSince)
	}

	// Step 6: agent updates context with progress note.
	agent.UpdateFields(createMsgID, map[string]string{"context": "progress"})
	item = h.MustItem("seq-canon-1")
	if item.Status != state.StatusActive {
		t.Errorf("step6 update fields: Status got %q, still want active", item.Status)
	}
	if item.Context != "progress" {
		t.Errorf("step6 update fields: Context got %q, want progress", item.Context)
	}

	// Step 7: agent closes done with branch reference.
	agent.Close(createMsgID, "done", "Implemented. Branch: work/seq-canon-1")
	item = h.MustItem("seq-canon-1")
	if item.Status != state.StatusDone {
		t.Fatalf("step7 close: Status got %q, want done", item.Status)
	}
	if !state.IsTerminal(item) {
		t.Error("step7 close: IsTerminal expected true")
	}
}
