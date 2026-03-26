package e2e_test

import (
	"testing"
)

// TestE2E_Ready_ShowsInboxItems verifies a new P1 item appears in rd ready.
func TestE2E_Ready_ShowsInboxItems(t *testing.T) {
	e := NewEnv(t)
	e.RdMustSucceed("create",
		"--id", "at-001",
		"--title", "Ready shows inbox",
		"--priority", "p1",
		"--type", "task",
		"--for", "test@example.com",
	)
	items := e.ReadyItems()
	if !containsItem(items, "at-001") {
		t.Fatalf("item at-001 not found in ready; got %d items", len(items))
	}
}

// TestE2E_Ready_ExcludesTerminal verifies closed items don't appear in rd ready.
func TestE2E_Ready_ExcludesTerminal(t *testing.T) {
	e := NewEnv(t)
	e.RdMustSucceed("create",
		"--id", "at-002",
		"--title", "Terminal exclusion test",
		"--priority", "p1",
		"--type", "task",
		"--for", "test@example.com",
	)
	e.RdMustSucceed("close", "at-002", "--reason", "done")
	items := e.ReadyItems()
	if containsItem(items, "at-002") {
		t.Fatal("closed item at-002 should not appear in rd ready")
	}
}

// TestE2E_Ready_ExcludesBlocked verifies blocked items don't appear in rd ready.
func TestE2E_Ready_ExcludesBlocked(t *testing.T) {
	e := NewEnv(t)
	e.RdMustSucceed("create",
		"--id", "at-003a",
		"--title", "Blocker",
		"--priority", "p1",
		"--type", "task",
		"--for", "test@example.com",
	)
	e.RdMustSucceed("create",
		"--id", "at-003b",
		"--title", "Blocked item",
		"--priority", "p1",
		"--type", "task",
		"--for", "test@example.com",
	)
	// at-003b is blocked by at-003a
	e.RdMustSucceed("dep", "add", "at-003b", "at-003a")

	items := e.ReadyItems()
	if !containsItem(items, "at-003a") {
		t.Error("blocker at-003a should be in ready")
	}
	if containsItem(items, "at-003b") {
		t.Error("blocked item at-003b should not be in ready")
	}
}

// TestE2E_List_DefaultExcludesTerminal verifies rd list excludes done/cancelled by default.
func TestE2E_List_DefaultExcludesTerminal(t *testing.T) {
	e := NewEnv(t)
	e.RdMustSucceed("create",
		"--id", "at-004a",
		"--title", "Open item",
		"--priority", "p2",
		"--type", "task",
		"--for", "test@example.com",
	)
	e.RdMustSucceed("create",
		"--id", "at-004b",
		"--title", "Closed item",
		"--priority", "p2",
		"--type", "task",
		"--for", "test@example.com",
	)
	e.RdMustSucceed("close", "at-004b", "--reason", "done")

	var items []Item
	if err := e.RdJSON(&items, "list"); err != nil {
		t.Fatalf("list: %v", err)
	}
	if !containsItem(items, "at-004a") {
		t.Error("open item at-004a should be in list")
	}
	if containsItem(items, "at-004b") {
		t.Error("closed item at-004b should not be in default list")
	}
}

// TestE2E_List_AllIncludesTerminal verifies rd list --all includes terminal items.
func TestE2E_List_AllIncludesTerminal(t *testing.T) {
	e := NewEnv(t)
	e.RdMustSucceed("create",
		"--id", "at-005a",
		"--title", "Open item",
		"--priority", "p2",
		"--type", "task",
		"--for", "test@example.com",
	)
	e.RdMustSucceed("create",
		"--id", "at-005b",
		"--title", "Closed item",
		"--priority", "p2",
		"--type", "task",
		"--for", "test@example.com",
	)
	e.RdMustSucceed("close", "at-005b", "--reason", "done")

	items := e.ListItems() // uses --all
	if !containsItem(items, "at-005a") {
		t.Error("open item at-005a should be in --all list")
	}
	if !containsItem(items, "at-005b") {
		t.Error("closed item at-005b should be in --all list")
	}
}

// TestE2E_List_MultiStatusOR verifies --status is repeatable with OR semantics.
func TestE2E_List_MultiStatusOR(t *testing.T) {
	e := NewEnv(t)
	e.RdMustSucceed("create",
		"--id", "at-006a",
		"--title", "Inbox item",
		"--priority", "p2",
		"--type", "task",
		"--for", "test@example.com",
	)
	e.RdMustSucceed("create",
		"--id", "at-006b",
		"--title", "Active item",
		"--priority", "p2",
		"--type", "task",
		"--for", "test@example.com",
	)
	e.RdMustSucceed("create",
		"--id", "at-006c",
		"--title", "Done item",
		"--priority", "p2",
		"--type", "task",
		"--for", "test@example.com",
	)
	e.RdMustSucceed("update", "at-006b", "--status", "active")
	e.RdMustSucceed("close", "at-006c", "--reason", "done")

	var items []Item
	if err := e.RdJSON(&items, "list", "--status", "inbox", "--status", "active"); err != nil {
		t.Fatalf("list --status inbox --status active: %v", err)
	}
	if !containsItem(items, "at-006a") {
		t.Error("inbox item at-006a should match --status inbox")
	}
	if !containsItem(items, "at-006b") {
		t.Error("active item at-006b should match --status active")
	}
	if containsItem(items, "at-006c") {
		t.Error("done item at-006c should not match --status inbox --status active")
	}
}

// TestE2E_List_StatusAlias verifies in_progress alias resolves in list filter.
func TestE2E_List_StatusAlias(t *testing.T) {
	e := NewEnv(t)
	e.RdMustSucceed("create",
		"--id", "at-007",
		"--title", "Status alias list test",
		"--priority", "p1",
		"--type", "task",
		"--for", "test@example.com",
	)
	e.RdMustSucceed("update", "at-007", "--status", "active")

	var items []Item
	if err := e.RdJSON(&items, "list", "--status", "in_progress"); err != nil {
		t.Fatalf("list --status in_progress: %v", err)
	}
	if !containsItem(items, "at-007") {
		t.Error("active item at-007 should match --status in_progress alias")
	}
}

// TestE2E_List_ByPriority verifies --priority filter works.
func TestE2E_List_ByPriority(t *testing.T) {
	e := NewEnv(t)
	e.RdMustSucceed("create",
		"--id", "at-008a",
		"--title", "P0 item",
		"--priority", "p0",
		"--type", "task",
		"--for", "test@example.com",
	)
	e.RdMustSucceed("create",
		"--id", "at-008b",
		"--title", "P2 item",
		"--priority", "p2",
		"--type", "task",
		"--for", "test@example.com",
	)

	var items []Item
	if err := e.RdJSON(&items, "list", "--priority", "p0"); err != nil {
		t.Fatalf("list --priority p0: %v", err)
	}
	if !containsItem(items, "at-008a") {
		t.Error("p0 item at-008a should match --priority p0")
	}
	if containsItem(items, "at-008b") {
		t.Error("p2 item at-008b should not match --priority p0")
	}
}

// TestE2E_List_JSONFieldContract verifies key JSON field types and aliases.
func TestE2E_List_JSONFieldContract(t *testing.T) {
	e := NewEnv(t)
	e.RdMustSucceed("create",
		"--id", "at-009",
		"--title", "Field contract test",
		"--priority", "p1",
		"--type", "task",
		"--for", "test@example.com",
		"--context", "some context text",
	)
	items := e.ListItems()
	item, ok := findItem(items, "at-009")
	if !ok {
		t.Fatal("item at-009 not found in list")
	}
	if item.ID == "" {
		t.Error("id should be non-empty string")
	}
	if item.CreatedAt == 0 {
		t.Error("created_at should be non-zero int64")
	}
	if item.Context != "some context text" {
		t.Errorf("context: got %q, want %q", item.Context, "some context text")
	}
	// description is an alias for context
	if item.Description != "some context text" {
		t.Errorf("description (alias for context): got %q, want %q", item.Description, "some context text")
	}
}
