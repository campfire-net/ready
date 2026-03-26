package storetest_test

import (
	"testing"

	"github.com/3dl-dev/ready/pkg/storetest"
)

// TestHarness_CreateAndDerive creates an item, derives state, and verifies
// the item exists with the correct fields.
func TestHarness_CreateAndDerive(t *testing.T) {
	h := storetest.New(t)

	msgID := h.Create("ready-abc",
		"Test item",
		"p1",
		storetest.WithType("task"),
		storetest.WithContext("Some context"),
		storetest.WithProject("myproject"),
		storetest.WithFor("alice@example.com"),
	)
	if msgID == "" {
		t.Fatal("expected non-empty message ID from Create")
	}

	item := h.MustItem("ready-abc")

	if item.Title != "Test item" {
		t.Errorf("Title: got %q, want %q", item.Title, "Test item")
	}
	if item.Priority != "p1" {
		t.Errorf("Priority: got %q, want %q", item.Priority, "p1")
	}
	if item.Type != "task" {
		t.Errorf("Type: got %q, want %q", item.Type, "task")
	}
	if item.Context != "Some context" {
		t.Errorf("Context: got %q, want %q", item.Context, "Some context")
	}
	if item.Project != "myproject" {
		t.Errorf("Project: got %q, want %q", item.Project, "myproject")
	}
	if item.For != "alice@example.com" {
		t.Errorf("For: got %q, want %q", item.For, "alice@example.com")
	}
	if item.Status != "inbox" {
		t.Errorf("Status: got %q, want %q", item.Status, "inbox")
	}
	if item.MsgID != msgID {
		t.Errorf("MsgID: got %q, want %q", item.MsgID, msgID)
	}
}

// TestHarness_AutoIncrementTimestamp verifies that two consecutive Create calls
// produce items with strictly increasing CreatedAt timestamps in derived state.
func TestHarness_AutoIncrementTimestamp(t *testing.T) {
	h := storetest.New(t)

	h.Create("ready-t1", "First item", "p2")
	h.Create("ready-t2", "Second item", "p2")

	first := h.MustItem("ready-t1")
	second := h.MustItem("ready-t2")

	if first.CreatedAt >= second.CreatedAt {
		t.Errorf("expected first.CreatedAt (%d) < second.CreatedAt (%d)",
			first.CreatedAt, second.CreatedAt)
	}
}

// TestHarness_WithSender creates an item with the default sender, then claims it
// via a different sender, and verifies the By field reflects the claiming sender.
func TestHarness_WithSender(t *testing.T) {
	h := storetest.New(t)

	createMsgID := h.Create("ready-ws1", "Multi-agent item", "p1")

	// Claim with a different sender.
	agent := h.WithSender("agent@example.com")
	agent.Claim(createMsgID)

	item := h.MustItem("ready-ws1")

	if item.By != "agent@example.com" {
		t.Errorf("By: got %q, want %q", item.By, "agent@example.com")
	}
	if item.Status != "active" {
		t.Errorf("Status after claim: got %q, want %q", item.Status, "active")
	}
}
