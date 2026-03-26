package e2e_test

import (
	"strings"
	"testing"
)

// TestE2E_Claim_SetsActive verifies rd claim sets status=active and by field.
func TestE2E_Claim_SetsActive(t *testing.T) {
	e := NewEnv(t)
	item := createItem(e, t, "Claim test", "p1", "task")
	e.RdMustSucceed("claim", item.ID)
	got := e.ShowItem(item.ID)
	if got.Status != "active" {
		t.Errorf("status: got %q, want active", got.Status)
	}
	if got.By == "" {
		t.Error("by field is empty after claim")
	}
}

// TestE2E_Claim_UpdateWithClaim verifies rd update --claim sets status=active.
func TestE2E_Claim_UpdateWithClaim(t *testing.T) {
	e := NewEnv(t)
	item := createItem(e, t, "Update claim test", "p1", "task")
	e.RdMustSucceed("update", item.ID, "--claim")
	if got := e.ShowItem(item.ID); got.Status != "active" {
		t.Errorf("status: got %q, want active", got.Status)
	}
}

// TestE2E_Complete_WithBranch verifies rd complete --branch closes as done.
func TestE2E_Complete_WithBranch(t *testing.T) {
	e := NewEnv(t)
	item := createItem(e, t, "Complete with branch test", "p1", "task")
	e.RdMustSucceed("complete", item.ID, "--reason", "done", "--branch", "work/test")
	if got := e.ShowItem(item.ID); got.Status != "done" {
		t.Errorf("status: got %q, want done", got.Status)
	}
}

// TestE2E_Done_ClosesItem verifies rd done closes item as done.
func TestE2E_Done_ClosesItem(t *testing.T) {
	e := NewEnv(t)
	item := createItem(e, t, "Done test", "p1", "task")
	e.RdMustSucceed("done", item.ID, "--reason", "done")
	if got := e.ShowItem(item.ID); got.Status != "done" {
		t.Errorf("status: got %q, want done", got.Status)
	}
}

// TestE2E_Fail_ClosesItem verifies rd fail closes item as failed.
func TestE2E_Fail_ClosesItem(t *testing.T) {
	e := NewEnv(t)
	item := createItem(e, t, "Fail test", "p1", "task")
	e.RdMustSucceed("fail", item.ID, "--reason", "blocked externally")
	if got := e.ShowItem(item.ID); got.Status != "failed" {
		t.Errorf("status: got %q, want failed", got.Status)
	}
}

// TestE2E_Cancel_ClosesItem verifies rd cancel closes item as cancelled.
func TestE2E_Cancel_ClosesItem(t *testing.T) {
	e := NewEnv(t)
	item := createItem(e, t, "Cancel test", "p1", "task")
	e.RdMustSucceed("cancel", item.ID, "--reason", "descoped")
	if got := e.ShowItem(item.ID); got.Status != "cancelled" {
		t.Errorf("status: got %q, want cancelled", got.Status)
	}
}

// TestE2E_Dep_AddAndUnblock verifies dep add blocks/unblocks via rd close.
func TestE2E_Dep_AddAndUnblock(t *testing.T) {
	e := NewEnv(t)
	blocker := createItem(e, t, "Blocker", "p1", "task")
	blocked := createItem(e, t, "Blocked", "p1", "task")
	e.RdMustSucceed("dep", "add", blocked.ID, blocker.ID)

	ready := e.ReadyItems()
	if !containsItem(ready, blocker.ID) {
		t.Error("blocker should be in ready")
	}
	if containsItem(ready, blocked.ID) {
		t.Error("blocked item should not be in ready")
	}

	e.RdMustSucceed("close", blocker.ID, "--reason", "done")
	if !containsItem(e.ReadyItems(), blocked.ID) {
		t.Error("blocked item should be in ready after blocker closed")
	}
}

// TestE2E_Seq_UpdateThenClose exercises the update→close sequence.
func TestE2E_Seq_UpdateThenClose(t *testing.T) {
	e := NewEnv(t)
	item := createItem(e, t, "Update then close", "p1", "task")
	e.RdMustSucceed("update", item.ID, "--status", "active")
	e.RdMustSucceed("close", item.ID, "--reason", "done")
	if got := e.ShowItem(item.ID); got.Status != "done" {
		t.Errorf("status: got %q, want done", got.Status)
	}
}

// TestE2E_Seq_CloseCloseUnblocks verifies closing a chain unblocks sequentially.
func TestE2E_Seq_CloseCloseUnblocks(t *testing.T) {
	e := NewEnv(t)
	a := createItem(e, t, "A", "p1", "task")
	b := createItem(e, t, "B", "p1", "task")
	c := createItem(e, t, "C", "p1", "task")
	e.RdMustSucceed("dep", "add", b.ID, a.ID)
	e.RdMustSucceed("dep", "add", c.ID, b.ID)

	e.RdMustSucceed("close", a.ID, "--reason", "done")
	e.RdMustSucceed("close", b.ID, "--reason", "done")

	if !containsItem(e.ReadyItems(), c.ID) {
		t.Error("C should be in ready after chain closed")
	}
}

// TestE2E_Seq_CanonicalWorkflow exercises the full agent workflow: create→claim→update→complete.
func TestE2E_Seq_CanonicalWorkflow(t *testing.T) {
	e := NewEnv(t)
	item := createItem(e, t, "Canonical workflow", "p1", "task")
	e.RdMustSucceed("claim", item.ID)
	e.RdMustSucceed("update", item.ID, "--context", "in progress")
	e.RdMustSucceed("complete", item.ID, "--reason", "implemented and merged", "--branch", "work/test")

	got, ok := findItem(e.ListItems(), item.ID)
	if !ok {
		t.Fatal("item not found in --all list")
	}
	if got.Status != "done" {
		t.Errorf("status: got %q, want done", got.Status)
	}
}

// TestE2E_Error_CloseNoReason verifies rd close without --reason exits non-zero.
func TestE2E_Error_CloseNoReason(t *testing.T) {
	e := NewEnv(t)
	item := createItem(e, t, "Error test", "p2", "task")
	_, _, code := e.Rd("close", item.ID)
	if code == 0 {
		t.Fatal("rd close without --reason should exit non-zero")
	}
}

// TestE2E_Error_UnknownFlag_Blocks verifies --blocks produces a helpful error.
func TestE2E_Error_UnknownFlag_Blocks(t *testing.T) {
	e := NewEnv(t)
	a := createItem(e, t, "A", "p1", "task")
	b := createItem(e, t, "B", "p1", "task")
	stderr := e.RdMustFail("update", a.ID, "--blocks", b.ID)
	if !strings.Contains(stderr, "dep") && !strings.Contains(stderr, "unknown flag") {
		t.Errorf("expected error mentioning dep or unknown flag, got: %q", stderr)
	}
}

// TestE2E_Error_UnknownFlag_Description verifies --description produces a helpful error.
func TestE2E_Error_UnknownFlag_Description(t *testing.T) {
	e := NewEnv(t)
	stderr := e.RdMustFail("create",
		"--title", "Test",
		"--priority", "p1",
		"--type", "task",
		"--for", "test@example.com",
		"--description", "text",
	)
	if !strings.Contains(stderr, "context") && !strings.Contains(stderr, "unknown flag") {
		t.Errorf("expected error mentioning context or unknown flag, got: %q", stderr)
	}
}
