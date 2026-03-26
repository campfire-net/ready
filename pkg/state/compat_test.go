package state

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestItem_JSONHasDescriptionField verifies that marshaling an Item to JSON
// produces a "description" key and that it matches the "context" value.
func TestItem_JSONHasDescriptionField(t *testing.T) {
	item := &Item{
		ID:          "ready-001",
		Title:       "Test item",
		Context:     "Some context text",
		Description: "Some context text",
		Type:        "task",
		For:         "baron",
		Priority:    "p2",
		Status:      StatusInbox,
	}

	data, err := json.Marshal(item)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	desc, ok := m["description"]
	if !ok {
		t.Fatal("expected 'description' key in JSON output, not found")
	}
	ctx, ok := m["context"]
	if !ok {
		t.Fatal("expected 'context' key in JSON output, not found")
	}
	if desc != ctx {
		t.Errorf("description=%q != context=%q", desc, ctx)
	}
}

// TestItem_JSONHasBlocksField verifies the "blocks" field is present in JSON
// when set (part of the stable field contract).
func TestItem_JSONHasBlocksField(t *testing.T) {
	item := &Item{
		ID:       "ready-002",
		Title:    "Blocking item",
		Type:     "task",
		For:      "baron",
		Priority: "p1",
		Status:   StatusActive,
		Blocks:   []string{"ready-003", "ready-004"},
	}

	data, err := json.Marshal(item)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	raw, ok := m["blocks"]
	if !ok {
		t.Fatal("expected 'blocks' key in JSON output, not found")
	}
	arr, ok := raw.([]any)
	if !ok {
		t.Fatalf("expected 'blocks' to be array, got %T", raw)
	}
	if len(arr) != 2 {
		t.Errorf("expected 2 blocks, got %d", len(arr))
	}
}

// TestItem_JSONFieldStability is the contract test. It unmarshals a golden
// JSON string and verifies round-trip fidelity. If a field is renamed, this
// test will fail — that's the point.
func TestItem_JSONFieldStability(t *testing.T) {
	golden := `{
		"id": "ready-abc",
		"msg_id": "msg-001",
		"campfire_id": "cf-001",
		"title": "Stable field test",
		"context": "ctx text",
		"description": "ctx text",
		"type": "task",
		"level": "task",
		"project": "ready",
		"for": "baron",
		"by": "claude",
		"priority": "p1",
		"status": "active",
		"eta": "2026-03-25T12:00:00Z",
		"due": "2026-03-26T12:00:00Z",
		"parent_id": "ready-parent",
		"blocked_by": ["ready-x"],
		"blocks": ["ready-y"],
		"gate": "review",
		"waiting_on": "someone",
		"waiting_type": "person",
		"waiting_since": "2026-03-24T00:00:00Z",
		"gate_msg_id": "msg-gate",
		"created_at": 1000000000,
		"updated_at": 2000000000
	}`

	var item Item
	if err := json.Unmarshal([]byte(golden), &item); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	// Verify key fields parsed correctly.
	checks := []struct {
		name string
		got  string
		want string
	}{
		{"id", item.ID, "ready-abc"},
		{"msg_id", item.MsgID, "msg-001"},
		{"campfire_id", item.CampfireID, "cf-001"},
		{"title", item.Title, "Stable field test"},
		{"context", item.Context, "ctx text"},
		{"description", item.Description, "ctx text"},
		{"type", item.Type, "task"},
		{"for", item.For, "baron"},
		{"by", item.By, "claude"},
		{"priority", item.Priority, "p1"},
		{"status", item.Status, "active"},
		{"gate", item.Gate, "review"},
		{"waiting_on", item.WaitingOn, "someone"},
		{"waiting_type", item.WaitingType, "person"},
		{"gate_msg_id", item.GateMsgID, "msg-gate"},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("field %s: got %q, want %q", c.name, c.got, c.want)
		}
	}

	// Re-marshal and check output contains both context and description.
	data, err := json.Marshal(item)
	if err != nil {
		t.Fatalf("re-marshal failed: %v", err)
	}
	out := string(data)
	for _, field := range []string{`"context"`, `"description"`, `"blocks"`, `"blocked_by"`} {
		if !strings.Contains(out, field) {
			t.Errorf("re-marshaled JSON missing field %s", field)
		}
	}
}
