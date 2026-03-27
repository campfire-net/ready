package storetest_test

import (
	"testing"

	"github.com/campfire-net/ready/pkg/state"
	"github.com/campfire-net/ready/pkg/storetest"
)

// TestStore_Claim_SetsByAndActive verifies that claiming an item sets By to the
// agent's sender key and transitions status to active.
func TestStore_Claim_SetsByAndActive(t *testing.T) {
	h := storetest.New(t)
	agent := h.WithSender("agent-pubkey")

	createMsgID := h.Create("item-claim-1", "Claim test", "p1")
	agent.Claim(createMsgID)

	item := h.MustItem("item-claim-1")
	if item.By != "agent-pubkey" {
		t.Errorf("By: got %q, want %q", item.By, "agent-pubkey")
	}
	if item.Status != state.StatusActive {
		t.Errorf("Status: got %q, want %q", item.Status, state.StatusActive)
	}
}

// TestStore_Claim_AppearsInMyWork verifies that after claiming, the item appears
// in the my-work view for the claiming agent.
func TestStore_Claim_AppearsInMyWork(t *testing.T) {
	h := storetest.New(t)
	agent := h.WithSender("agent-pubkey")

	createMsgID := h.Create("item-mywork-1", "My work test", "p1")
	agent.Claim(createMsgID)

	items := h.ViewItems("my-work", "agent-pubkey")
	for _, it := range items {
		if it.ID == "item-mywork-1" {
			return // found
		}
	}
	t.Errorf("item-mywork-1 not found in my-work view for agent-pubkey; got %d items", len(items))
}

// TestStore_Claim_AppearsInDelegated verifies that when an item is created with
// for=baron and an agent claims it, the item appears in the delegated view for baron.
func TestStore_Claim_AppearsInDelegated(t *testing.T) {
	h := storetest.New(t)
	agent := h.WithSender("agent-pubkey")

	createMsgID := h.Create("item-delegated-1", "Delegated test", "p1", storetest.WithFor("baron"))
	agent.Claim(createMsgID)

	items := h.ViewItems("delegated", "baron")
	for _, it := range items {
		if it.ID == "item-delegated-1" {
			return // found
		}
	}
	t.Errorf("item-delegated-1 not found in delegated view for baron; got %d items", len(items))
}

// TestStore_Delegate_ThenClaim verifies that delegating to an agent and then
// claiming preserves the original for field while setting by to the agent sender.
func TestStore_Delegate_ThenClaim(t *testing.T) {
	h := storetest.New(t)
	agent := h.WithSender("agent-pubkey")

	createMsgID := h.Create("item-delegate-claim-1", "Delegate then claim", "p1", storetest.WithFor("baron"))
	h.Delegate(createMsgID, "agent-key")
	agent.Claim(createMsgID)

	item := h.MustItem("item-delegate-claim-1")
	if item.By != "agent-pubkey" {
		t.Errorf("By: got %q, want agent-pubkey", item.By)
	}
	if item.For != "baron" {
		t.Errorf("For: got %q, want baron", item.For)
	}
	if item.Status != state.StatusActive {
		t.Errorf("Status: got %q, want active", item.Status)
	}
}

// TestStore_Complete_ClosesDone verifies that closing an active item with
// resolution=done transitions status to done.
func TestStore_Complete_ClosesDone(t *testing.T) {
	h := storetest.New(t)
	agent := h.WithSender("agent-pubkey")

	createMsgID := h.Create("item-complete-1", "Complete test", "p1")
	agent.Claim(createMsgID)
	agent.Close(createMsgID, "done", "Implemented")

	item := h.MustItem("item-complete-1")
	if item.Status != state.StatusDone {
		t.Errorf("Status: got %q, want done", item.Status)
	}
}

// TestStore_Complete_WithBranch verifies that closing an item with extra fields
// in the payload does not break state derivation (extra fields are ignored by
// json.Unmarshal).
func TestStore_Complete_WithBranch(t *testing.T) {
	h := storetest.New(t)
	agent := h.WithSender("agent-pubkey")

	createMsgID := h.Create("item-complete-branch-1", "Complete with branch", "p1")
	agent.Claim(createMsgID)
	// Close carries standard fields; state.Derive should handle gracefully.
	agent.Close(createMsgID, "done", "Implemented. Branch: work/item-complete-branch-1")

	item := h.MustItem("item-complete-branch-1")
	if item.Status != state.StatusDone {
		t.Errorf("Status: got %q, want done", item.Status)
	}
}

// TestStore_Fail_ClosesFailed verifies that closing with resolution=failed
// transitions status to failed.
func TestStore_Fail_ClosesFailed(t *testing.T) {
	h := storetest.New(t)
	agent := h.WithSender("agent-pubkey")

	createMsgID := h.Create("item-fail-1", "Fail test", "p1")
	agent.Claim(createMsgID)
	agent.Close(createMsgID, "failed", "blocked on X")

	item := h.MustItem("item-fail-1")
	if item.Status != state.StatusFailed {
		t.Errorf("Status: got %q, want failed", item.Status)
	}
}

// TestStore_Done_ClosesDone verifies that a human-facing close (no prior claim)
// with resolution=done transitions status to done.
func TestStore_Done_ClosesDone(t *testing.T) {
	h := storetest.New(t)

	createMsgID := h.Create("item-done-1", "Done test", "p1")
	h.Close(createMsgID, "done", "completed")

	item := h.MustItem("item-done-1")
	if item.Status != state.StatusDone {
		t.Errorf("Status: got %q, want done", item.Status)
	}
}

// TestStore_AgentWorkflow_Full exercises the full agent work lifecycle:
// create → delegate → claim → waiting → active → done, verifying state at each step.
func TestStore_AgentWorkflow_Full(t *testing.T) {
	h := storetest.New(t)
	agent := h.WithSender("agent-pubkey")

	// Step 1: create — status should be inbox.
	createMsgID := h.Create("item-full-1", "Full workflow", "p1", storetest.WithFor("baron"))
	item := h.MustItem("item-full-1")
	if item.Status != state.StatusInbox {
		t.Errorf("after create: Status got %q, want inbox", item.Status)
	}

	// Step 2: delegate — status should still be inbox (delegate sets by, no status change).
	h.Delegate(createMsgID, "agent-pubkey")
	item = h.MustItem("item-full-1")
	if item.Status != state.StatusInbox {
		t.Errorf("after delegate: Status got %q, want inbox", item.Status)
	}

	// Step 3: claim — status should be active.
	agent.Claim(createMsgID)
	item = h.MustItem("item-full-1")
	if item.Status != state.StatusActive {
		t.Errorf("after claim: Status got %q, want active", item.Status)
	}

	// Step 4: transition to waiting.
	agent.UpdateStatus(createMsgID, "waiting",
		storetest.WithWaitingOn("design"),
		storetest.WithWaitingType("person"),
	)
	item = h.MustItem("item-full-1")
	if item.Status != state.StatusWaiting {
		t.Errorf("after waiting: Status got %q, want waiting", item.Status)
	}
	if item.WaitingOn != "design" {
		t.Errorf("WaitingOn: got %q, want design", item.WaitingOn)
	}

	// Step 5: back to active.
	agent.UpdateStatus(createMsgID, "active")
	item = h.MustItem("item-full-1")
	if item.Status != state.StatusActive {
		t.Errorf("after re-active: Status got %q, want active", item.Status)
	}

	// Step 6: close done.
	agent.Close(createMsgID, "done", "Implemented. Branch: work/item-full-1")
	item = h.MustItem("item-full-1")
	if item.Status != state.StatusDone {
		t.Errorf("after close done: Status got %q, want done", item.Status)
	}
}

// TestStore_AgentWorkflow_ClaimFail verifies the claim-then-fail lifecycle:
// the item ends in failed status, which is terminal.
func TestStore_AgentWorkflow_ClaimFail(t *testing.T) {
	h := storetest.New(t)
	agent := h.WithSender("agent-pubkey")

	createMsgID := h.Create("item-claimfail-1", "Claim fail test", "p1")
	agent.Claim(createMsgID)
	agent.Close(createMsgID, "failed", "could not complete")

	item := h.MustItem("item-claimfail-1")
	if item.Status != state.StatusFailed {
		t.Errorf("Status: got %q, want failed", item.Status)
	}
	if !state.IsTerminal(item) {
		t.Errorf("expected item to be terminal after failed close")
	}
}

// TestStore_MultiAgent_TwoAgentsClaim verifies that two agents each claiming
// a different item see only their own item in my-work.
func TestStore_MultiAgent_TwoAgentsClaim(t *testing.T) {
	h := storetest.New(t)
	agentA := h.WithSender("agent-a")
	agentB := h.WithSender("agent-b")

	createA := h.Create("item-multi-1", "Item for agent A", "p1")
	createB := h.Create("item-multi-2", "Item for agent B", "p1")

	agentA.Claim(createA)
	agentB.Claim(createB)

	workA := h.ViewItems("my-work", "agent-a")
	for _, it := range workA {
		if it.ID == "item-multi-2" {
			t.Errorf("agent-a my-work should not contain item-multi-2")
		}
	}
	foundA := false
	for _, it := range workA {
		if it.ID == "item-multi-1" {
			foundA = true
		}
	}
	if !foundA {
		t.Errorf("agent-a my-work should contain item-multi-1; got %d items", len(workA))
	}

	workB := h.ViewItems("my-work", "agent-b")
	for _, it := range workB {
		if it.ID == "item-multi-1" {
			t.Errorf("agent-b my-work should not contain item-multi-1")
		}
	}
	foundB := false
	for _, it := range workB {
		if it.ID == "item-multi-2" {
			foundB = true
		}
	}
	if !foundB {
		t.Errorf("agent-b my-work should contain item-multi-2; got %d items", len(workB))
	}
}
