package e2e_test

import (
	"testing"
)

// TestE2E_Create_ReturnsJSON verifies create --json returns id and title.
func TestE2E_Create_ReturnsJSON(t *testing.T) {
	e := NewEnv(t)
	var item Item
	if err := e.RdJSON(&item, "create",
		"--id", "lc-001",
		"--title", "Create JSON test",
		"--priority", "p1",
		"--type", "task",
		"--for", "test@example.com",
	); err != nil {
		t.Fatalf("create --json: %v", err)
	}
	if item.ID != "lc-001" {
		t.Errorf("id: got %q, want %q", item.ID, "lc-001")
	}
	if item.Title != "Create JSON test" {
		t.Errorf("title: got %q, want %q", item.Title, "Create JSON test")
	}
}

// TestE2E_Create_AppearsInReady verifies a new item appears in rd ready with status=inbox.
func TestE2E_Create_AppearsInReady(t *testing.T) {
	e := NewEnv(t)
	e.RdMustSucceed("create",
		"--id", "lc-002",
		"--title", "Ready test",
		"--priority", "p1",
		"--type", "task",
		"--for", "test@example.com",
	)
	items := e.ReadyItems()
	item, ok := findItem(items, "lc-002")
	if !ok {
		t.Fatalf("item lc-002 not found in ready; got %d items", len(items))
	}
	if item.Status != "inbox" {
		t.Errorf("status: got %q, want %q", item.Status, "inbox")
	}
}

// TestE2E_Create_AppearsInList verifies a new item appears in rd list.
func TestE2E_Create_AppearsInList(t *testing.T) {
	e := NewEnv(t)
	e.RdMustSucceed("create",
		"--id", "lc-003",
		"--title", "List test",
		"--priority", "p2",
		"--type", "task",
		"--for", "test@example.com",
	)
	items := e.ListItems()
	if !containsItem(items, "lc-003") {
		t.Fatalf("item lc-003 not found in list; got %d items", len(items))
	}
}

// TestE2E_Create_ShowByID verifies rd show returns correct fields.
func TestE2E_Create_ShowByID(t *testing.T) {
	e := NewEnv(t)
	e.RdMustSucceed("create",
		"--id", "lc-004",
		"--title", "Show test",
		"--priority", "p2",
		"--type", "decision",
		"--for", "baron@example.com",
		"--context", "some context",
	)
	item := e.ShowItem("lc-004")
	if item.ID != "lc-004" {
		t.Errorf("id: got %q, want %q", item.ID, "lc-004")
	}
	if item.Title != "Show test" {
		t.Errorf("title: got %q", item.Title)
	}
	if item.Type != "decision" {
		t.Errorf("type: got %q, want %q", item.Type, "decision")
	}
	if item.For != "baron@example.com" {
		t.Errorf("for: got %q, want %q", item.For, "baron@example.com")
	}
	if item.Context != "some context" {
		t.Errorf("context: got %q, want %q", item.Context, "some context")
	}
}

// TestE2E_Update_StatusActive verifies rd update --status active transitions status.
func TestE2E_Update_StatusActive(t *testing.T) {
	e := NewEnv(t)
	e.RdMustSucceed("create",
		"--id", "lc-005",
		"--title", "Update status test",
		"--priority", "p1",
		"--type", "task",
		"--for", "test@example.com",
	)
	e.RdMustSucceed("update", "lc-005", "--status", "active")
	item := e.ShowItem("lc-005")
	if item.Status != "active" {
		t.Errorf("status: got %q, want active", item.Status)
	}
}

// TestE2E_Update_Priority verifies rd update --priority changes priority.
func TestE2E_Update_Priority(t *testing.T) {
	e := NewEnv(t)
	e.RdMustSucceed("create",
		"--id", "lc-006",
		"--title", "Update priority test",
		"--priority", "p2",
		"--type", "task",
		"--for", "test@example.com",
	)
	e.RdMustSucceed("update", "lc-006", "--priority", "p0")
	item := e.ShowItem("lc-006")
	if item.Priority != "p0" {
		t.Errorf("priority: got %q, want p0", item.Priority)
	}
}

// TestE2E_Update_Context verifies rd update --context changes context.
func TestE2E_Update_Context(t *testing.T) {
	e := NewEnv(t)
	e.RdMustSucceed("create",
		"--id", "lc-007",
		"--title", "Update context test",
		"--priority", "p2",
		"--type", "task",
		"--for", "test@example.com",
	)
	e.RdMustSucceed("update", "lc-007", "--context", "updated context")
	item := e.ShowItem("lc-007")
	if item.Context != "updated context" {
		t.Errorf("context: got %q, want %q", item.Context, "updated context")
	}
}

// TestE2E_Update_StatusAlias verifies in_progress resolves to active.
func TestE2E_Update_StatusAlias(t *testing.T) {
	e := NewEnv(t)
	e.RdMustSucceed("create",
		"--id", "lc-008",
		"--title", "Status alias test",
		"--priority", "p1",
		"--type", "task",
		"--for", "test@example.com",
	)
	e.RdMustSucceed("update", "lc-008", "--status", "in_progress")
	item := e.ShowItem("lc-008")
	if item.Status != "active" {
		t.Errorf("status: got %q, want active (in_progress alias)", item.Status)
	}
}

// TestE2E_Update_Claim verifies --claim sets status=active and by field.
func TestE2E_Update_Claim(t *testing.T) {
	e := NewEnv(t)
	e.RdMustSucceed("create",
		"--id", "lc-009",
		"--title", "Claim test",
		"--priority", "p1",
		"--type", "task",
		"--for", "test@example.com",
	)
	e.RdMustSucceed("update", "lc-009", "--claim")
	item := e.ShowItem("lc-009")
	if item.Status != "active" {
		t.Errorf("status: got %q, want active", item.Status)
	}
	if item.By == "" {
		t.Error("by field is empty after --claim")
	}
}

// TestE2E_Close_RequiresReason verifies rd close without --reason fails.
func TestE2E_Close_RequiresReason(t *testing.T) {
	e := NewEnv(t)
	e.RdMustSucceed("create",
		"--id", "lc-010",
		"--title", "Close no reason test",
		"--priority", "p2",
		"--type", "task",
		"--for", "test@example.com",
	)
	_, _, code := e.Rd("close", "lc-010")
	if code == 0 {
		t.Fatal("rd close without --reason should fail but exited 0")
	}
}

// TestE2E_Close_Done verifies rd close --reason sets status=done.
func TestE2E_Close_Done(t *testing.T) {
	e := NewEnv(t)
	e.RdMustSucceed("create",
		"--id", "lc-011",
		"--title", "Close done test",
		"--priority", "p2",
		"--type", "task",
		"--for", "test@example.com",
	)
	e.RdMustSucceed("close", "lc-011", "--reason", "done")
	item := e.ShowItem("lc-011")
	if item.Status != "done" {
		t.Errorf("status: got %q, want done", item.Status)
	}
}

// TestE2E_Close_Cancelled verifies --resolution cancelled sets status=cancelled.
func TestE2E_Close_Cancelled(t *testing.T) {
	e := NewEnv(t)
	e.RdMustSucceed("create",
		"--id", "lc-012",
		"--title", "Close cancelled test",
		"--priority", "p2",
		"--type", "task",
		"--for", "test@example.com",
	)
	e.RdMustSucceed("close", "lc-012", "--resolution", "cancelled", "--reason", "nope")
	item := e.ShowItem("lc-012")
	if item.Status != "cancelled" {
		t.Errorf("status: got %q, want cancelled", item.Status)
	}
}

// TestE2E_Close_DisappearsFromReady verifies closed item is gone from rd ready.
func TestE2E_Close_DisappearsFromReady(t *testing.T) {
	e := NewEnv(t)
	e.RdMustSucceed("create",
		"--id", "lc-013",
		"--title", "Disappears from ready test",
		"--priority", "p1",
		"--type", "task",
		"--for", "test@example.com",
	)
	e.RdMustSucceed("close", "lc-013", "--reason", "done")
	items := e.ReadyItems()
	if containsItem(items, "lc-013") {
		t.Fatal("closed item lc-013 still appears in rd ready")
	}
}

// TestE2E_Complete_ClosesAsDone verifies rd complete closes item as done.
func TestE2E_Complete_ClosesAsDone(t *testing.T) {
	e := NewEnv(t)
	e.RdMustSucceed("create",
		"--id", "lc-014",
		"--title", "Complete test",
		"--priority", "p1",
		"--type", "task",
		"--for", "test@example.com",
	)
	e.RdMustSucceed("complete", "lc-014", "--reason", "done", "--branch", "work/test")
	item := e.ShowItem("lc-014")
	if item.Status != "done" {
		t.Errorf("status: got %q, want done", item.Status)
	}
}

// TestE2E_UpdateThenClose_AgentWorkflow exercises the #1 agent workflow pattern.
func TestE2E_UpdateThenClose_AgentWorkflow(t *testing.T) {
	e := NewEnv(t)
	e.RdMustSucceed("create",
		"--id", "lc-015",
		"--title", "Agent workflow test",
		"--priority", "p1",
		"--type", "task",
		"--for", "test@example.com",
	)
	e.RdMustSucceed("update", "lc-015", "--status", "active", "--claim")
	item := e.ShowItem("lc-015")
	if item.Status != "active" {
		t.Fatalf("after claim: status=%q, want active", item.Status)
	}
	e.RdMustSucceed("close", "lc-015", "--reason", "done")
	item = e.ShowItem("lc-015")
	if item.Status != "done" {
		t.Errorf("after close: status=%q, want done", item.Status)
	}
}
