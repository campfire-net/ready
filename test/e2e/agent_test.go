package e2e_test

import (
	"strings"
	"testing"
)

// TestE2E_Claim_SetsActive verifies rd claim sets status=active and by field.
func TestE2E_Claim_SetsActive(t *testing.T) {
	e := NewEnv(t)
	e.RdMustSucceed("create",
		"--id", "ag-001",
		"--title", "Claim test",
		"--priority", "p1",
		"--type", "task",
		"--for", "test@example.com",
	)
	e.RdMustSucceed("claim", "ag-001")
	item := e.ShowItem("ag-001")
	if item.Status != "active" {
		t.Errorf("status: got %q, want active", item.Status)
	}
	if item.By == "" {
		t.Error("by field is empty after claim")
	}
}

// TestE2E_Claim_UpdateWithClaim verifies rd update --claim sets status=active.
func TestE2E_Claim_UpdateWithClaim(t *testing.T) {
	e := NewEnv(t)
	e.RdMustSucceed("create",
		"--id", "ag-002",
		"--title", "Update claim test",
		"--priority", "p1",
		"--type", "task",
		"--for", "test@example.com",
	)
	e.RdMustSucceed("update", "ag-002", "--claim")
	item := e.ShowItem("ag-002")
	if item.Status != "active" {
		t.Errorf("status: got %q, want active", item.Status)
	}
}

// TestE2E_Complete_WithBranch verifies rd complete --branch closes as done.
func TestE2E_Complete_WithBranch(t *testing.T) {
	e := NewEnv(t)
	e.RdMustSucceed("create",
		"--id", "ag-003",
		"--title", "Complete with branch test",
		"--priority", "p1",
		"--type", "task",
		"--for", "test@example.com",
	)
	e.RdMustSucceed("complete", "ag-003", "--reason", "done", "--branch", "work/test")
	item := e.ShowItem("ag-003")
	if item.Status != "done" {
		t.Errorf("status: got %q, want done", item.Status)
	}
}

// TestE2E_Done_ClosesItem verifies rd done closes item as done.
func TestE2E_Done_ClosesItem(t *testing.T) {
	e := NewEnv(t)
	e.RdMustSucceed("create",
		"--id", "ag-004",
		"--title", "Done test",
		"--priority", "p1",
		"--type", "task",
		"--for", "test@example.com",
	)
	e.RdMustSucceed("done", "ag-004", "--reason", "done")
	item := e.ShowItem("ag-004")
	if item.Status != "done" {
		t.Errorf("status: got %q, want done", item.Status)
	}
}

// TestE2E_Fail_ClosesItem verifies rd fail closes item as failed.
func TestE2E_Fail_ClosesItem(t *testing.T) {
	e := NewEnv(t)
	e.RdMustSucceed("create",
		"--id", "ag-005",
		"--title", "Fail test",
		"--priority", "p1",
		"--type", "task",
		"--for", "test@example.com",
	)
	e.RdMustSucceed("fail", "ag-005", "--reason", "blocked externally")
	item := e.ShowItem("ag-005")
	if item.Status != "failed" {
		t.Errorf("status: got %q, want failed", item.Status)
	}
}

// TestE2E_Cancel_ClosesItem verifies rd cancel closes item as cancelled.
func TestE2E_Cancel_ClosesItem(t *testing.T) {
	e := NewEnv(t)
	e.RdMustSucceed("create",
		"--id", "ag-006",
		"--title", "Cancel test",
		"--priority", "p1",
		"--type", "task",
		"--for", "test@example.com",
	)
	e.RdMustSucceed("cancel", "ag-006", "--reason", "descoped")
	item := e.ShowItem("ag-006")
	if item.Status != "cancelled" {
		t.Errorf("status: got %q, want cancelled", item.Status)
	}
}

// TestE2E_Dep_AddAndUnblock verifies dep add blocks/unblocks via rd close.
func TestE2E_Dep_AddAndUnblock(t *testing.T) {
	e := NewEnv(t)
	e.RdMustSucceed("create",
		"--id", "ag-007a",
		"--title", "Blocker",
		"--priority", "p1",
		"--type", "task",
		"--for", "test@example.com",
	)
	e.RdMustSucceed("create",
		"--id", "ag-007b",
		"--title", "Blocked",
		"--priority", "p1",
		"--type", "task",
		"--for", "test@example.com",
	)
	e.RdMustSucceed("dep", "add", "ag-007b", "ag-007a")

	// Only blocker should be in ready
	ready := e.ReadyItems()
	if !containsItem(ready, "ag-007a") {
		t.Error("blocker ag-007a should be in ready")
	}
	if containsItem(ready, "ag-007b") {
		t.Error("blocked ag-007b should not be in ready")
	}

	// Close the blocker — blocked item should unblock
	e.RdMustSucceed("close", "ag-007a", "--reason", "done")
	ready = e.ReadyItems()
	if !containsItem(ready, "ag-007b") {
		t.Error("ag-007b should be in ready after blocker closed")
	}
}

// TestE2E_Seq_UpdateThenClose exercises the update→close sequence.
func TestE2E_Seq_UpdateThenClose(t *testing.T) {
	e := NewEnv(t)
	e.RdMustSucceed("create",
		"--id", "ag-008",
		"--title", "Update then close",
		"--priority", "p1",
		"--type", "task",
		"--for", "test@example.com",
	)
	e.RdMustSucceed("update", "ag-008", "--status", "active")
	e.RdMustSucceed("close", "ag-008", "--reason", "done")
	item := e.ShowItem("ag-008")
	if item.Status != "done" {
		t.Errorf("status: got %q, want done", item.Status)
	}
}

// TestE2E_Seq_CloseCloseUnblocks verifies closing a chain unblocks sequentially.
func TestE2E_Seq_CloseCloseUnblocks(t *testing.T) {
	e := NewEnv(t)
	for _, id := range []string{"ag-009a", "ag-009b", "ag-009c"} {
		e.RdMustSucceed("create",
			"--id", id,
			"--title", id,
			"--priority", "p1",
			"--type", "task",
			"--for", "test@example.com",
		)
	}
	// C blocked by B, B blocked by A
	e.RdMustSucceed("dep", "add", "ag-009b", "ag-009a")
	e.RdMustSucceed("dep", "add", "ag-009c", "ag-009b")

	e.RdMustSucceed("close", "ag-009a", "--reason", "done")
	e.RdMustSucceed("close", "ag-009b", "--reason", "done")

	ready := e.ReadyItems()
	if !containsItem(ready, "ag-009c") {
		t.Error("ag-009c should be in ready after chain closed")
	}
}

// TestE2E_Seq_CanonicalWorkflow exercises the full agent workflow: create→claim→update→complete.
func TestE2E_Seq_CanonicalWorkflow(t *testing.T) {
	e := NewEnv(t)
	e.RdMustSucceed("create",
		"--id", "ag-010",
		"--title", "Canonical workflow",
		"--priority", "p1",
		"--type", "task",
		"--for", "test@example.com",
	)
	e.RdMustSucceed("claim", "ag-010")
	e.RdMustSucceed("update", "ag-010", "--context", "in progress")
	e.RdMustSucceed("complete", "ag-010", "--reason", "implemented and merged", "--branch", "work/test")

	items := e.ListItems()
	item, ok := findItem(items, "ag-010")
	if !ok {
		t.Fatal("ag-010 not found in --all list")
	}
	if item.Status != "done" {
		t.Errorf("status: got %q, want done", item.Status)
	}
}

// TestE2E_Error_CloseNoReason verifies rd close without --reason exits non-zero.
func TestE2E_Error_CloseNoReason(t *testing.T) {
	e := NewEnv(t)
	e.RdMustSucceed("create",
		"--id", "ag-011",
		"--title", "Error test",
		"--priority", "p2",
		"--type", "task",
		"--for", "test@example.com",
	)
	_, _, code := e.Rd("close", "ag-011")
	if code == 0 {
		t.Fatal("rd close without --reason should exit non-zero")
	}
}

// TestE2E_Error_UnknownFlag_Blocks verifies --blocks flag produces a helpful error.
func TestE2E_Error_UnknownFlag_Blocks(t *testing.T) {
	e := NewEnv(t)
	e.RdMustSucceed("create",
		"--id", "ag-012a",
		"--title", "A",
		"--priority", "p1",
		"--type", "task",
		"--for", "test@example.com",
	)
	e.RdMustSucceed("create",
		"--id", "ag-012b",
		"--title", "B",
		"--priority", "p1",
		"--type", "task",
		"--for", "test@example.com",
	)
	stderr := e.RdMustFail("update", "ag-012a", "--blocks", "ag-012b")
	// Should mention the correct command
	if !strings.Contains(stderr, "dep") && !strings.Contains(stderr, "unknown flag") {
		t.Errorf("expected error mentioning dep or unknown flag, got: %q", stderr)
	}
}

// TestE2E_Error_UnknownFlag_Description verifies --description produces a helpful error.
func TestE2E_Error_UnknownFlag_Description(t *testing.T) {
	e := NewEnv(t)
	stderr := e.RdMustFail("create",
		"--id", "ag-013",
		"--title", "Test",
		"--priority", "p1",
		"--type", "task",
		"--for", "test@example.com",
		"--description", "text",
	)
	// Should mention context or unknown flag
	if !strings.Contains(stderr, "context") && !strings.Contains(stderr, "unknown flag") {
		t.Errorf("expected error mentioning context or unknown flag, got: %q", stderr)
	}
}
