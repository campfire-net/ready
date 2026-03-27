package storetest_test

import (
	"encoding/json"
	"testing"

	"github.com/campfire-net/ready/pkg/state"
	"github.com/campfire-net/ready/pkg/storetest"
)

// TestStore_JSON_ListFieldPresence creates an item with ALL fields populated,
// derives state, marshals to JSON, and verifies all always-present keys exist.
func TestStore_JSON_ListFieldPresence(t *testing.T) {
	h := storetest.New(t)
	h.Create("item-full", "Full Title", "p1",
		storetest.WithContext("some context"),
		storetest.WithFor("baron"),
		storetest.WithBy("agent"),
		storetest.WithProject("ready"),
		storetest.WithLevel("task"),
		storetest.WithType("task"),
		storetest.WithETA("2026-04-01T00:00:00Z"),
		storetest.WithDue("2026-05-01T00:00:00Z"),
	)

	item := h.MustItem("item-full")
	data, err := json.Marshal(item)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	// Always-present fields (not omitempty, or always set by Derive)
	required := []string{
		"id", "msg_id", "campfire_id",
		"title", "type", "for",
		"priority", "status",
		"created_at", "updated_at",
	}
	for _, key := range required {
		if _, ok := m[key]; !ok {
			t.Errorf("missing required key %q in JSON output", key)
		}
	}

	// These fields were set explicitly and must also be present
	populated := []string{"context", "description", "level", "project", "by", "eta", "due"}
	for _, key := range populated {
		if _, ok := m[key]; !ok {
			t.Errorf("missing populated key %q in JSON output", key)
		}
	}
}

// TestStore_JSON_DescriptionEqualsContext verifies that context and description
// are both present and equal (description is an alias for context).
func TestStore_JSON_DescriptionEqualsContext(t *testing.T) {
	h := storetest.New(t)
	h.Create("item-ctx", "Title", "p2", storetest.WithContext("my context value"))

	item := h.MustItem("item-ctx")
	data, err := json.Marshal(item)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	ctx, ok1 := m["context"]
	desc, ok2 := m["description"]
	if !ok1 {
		t.Fatal("missing key 'context'")
	}
	if !ok2 {
		t.Fatal("missing key 'description'")
	}
	if ctx != desc {
		t.Errorf("context %q != description %q", ctx, desc)
	}
}

// TestStore_JSON_EmptyFieldsOmitted verifies that optional empty fields
// are not present in the JSON output (omitempty behavior).
func TestStore_JSON_EmptyFieldsOmitted(t *testing.T) {
	h := storetest.New(t)
	// Minimal creation — no context, no by, no level, no project, no parent, no due
	h.Create("item-min", "Minimal", "p3")

	item := h.MustItem("item-min")
	data, err := json.Marshal(item)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	// These omitempty fields should be absent when not set
	omitWhenEmpty := []string{
		"context", "description", "level", "project", "by",
		"due", "parent_id",
		"blocked_by", "blocks",
		"gate", "waiting_on", "waiting_type", "waiting_since", "gate_msg_id",
	}
	for _, key := range omitWhenEmpty {
		if val, ok := m[key]; ok {
			// Slices marshal as null or [] — check for non-nil non-empty
			// json.Unmarshal into map[string]any gives nil for JSON null
			if val != nil {
				t.Errorf("expected key %q to be absent or null, got %v", key, val)
			}
		}
	}
}

// TestStore_JSON_IDIsString verifies that the id field is a JSON string.
func TestStore_JSON_IDIsString(t *testing.T) {
	h := storetest.New(t)
	h.Create("item-str", "Title", "p1")

	item := h.MustItem("item-str")
	data, err := json.Marshal(item)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	idVal, ok := m["id"]
	if !ok {
		t.Fatal("missing key 'id'")
	}
	if _, isStr := idVal.(string); !isStr {
		t.Errorf("id is not a string, got %T: %v", idVal, idVal)
	}
}

// TestStore_JSON_BlocksIsArray verifies that the "blocks" field on item A
// is a JSON array after item B is blocked on A.
func TestStore_JSON_BlocksIsArray(t *testing.T) {
	h := storetest.New(t)
	msgA := h.Create("item-blocker", "Blocker", "p1")
	msgB := h.Create("item-blocked", "Blocked", "p2")
	h.Block("item-blocker", "item-blocked", msgA, msgB)

	item := h.MustItem("item-blocker")
	data, err := json.Marshal(item)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	val, ok := m["blocks"]
	if !ok {
		t.Fatal("missing key 'blocks'")
	}
	if _, isSlice := val.([]any); !isSlice {
		t.Errorf("blocks is not a JSON array, got %T: %v", val, val)
	}
}

// TestStore_JSON_BlockedByIsArray verifies that the "blocked_by" field on item B
// is a JSON array after item B is blocked on A.
func TestStore_JSON_BlockedByIsArray(t *testing.T) {
	h := storetest.New(t)
	msgA := h.Create("ba-blocker", "Blocker", "p1")
	msgB := h.Create("ba-blocked", "Blocked", "p2")
	h.Block("ba-blocker", "ba-blocked", msgA, msgB)

	item := h.MustItem("ba-blocked")
	data, err := json.Marshal(item)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	val, ok := m["blocked_by"]
	if !ok {
		t.Fatal("missing key 'blocked_by'")
	}
	if _, isSlice := val.([]any); !isSlice {
		t.Errorf("blocked_by is not a JSON array, got %T: %v", val, val)
	}
}

// TestStore_JSON_CreatedAtIsNumber verifies that created_at is a JSON number.
func TestStore_JSON_CreatedAtIsNumber(t *testing.T) {
	h := storetest.New(t)
	h.Create("item-ts", "Title", "p1")

	item := h.MustItem("item-ts")
	data, err := json.Marshal(item)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	val, ok := m["created_at"]
	if !ok {
		t.Fatal("missing key 'created_at'")
	}
	// json.Unmarshal into map[string]any represents JSON numbers as float64
	if _, isNum := val.(float64); !isNum {
		t.Errorf("created_at is not a number (float64), got %T: %v", val, val)
	}
}

// TestStore_JSON_AgentPythonPattern creates 3 items, derives state, marshals as
// a JSON array, and verifies the pattern used by agent/Python consumers:
// accessing core fields on each element as non-nil values.
func TestStore_JSON_AgentPythonPattern(t *testing.T) {
	h := storetest.New(t)
	h.Create("ap-1", "Task One", "p1", storetest.WithContext("context one"))
	h.Create("ap-2", "Task Two", "p2", storetest.WithContext("context two"))
	h.Create("ap-3", "Task Three", "p3", storetest.WithContext("context three"))

	derived := h.Derive()
	items := make([]*state.Item, 0, len(derived))
	for _, item := range derived {
		items = append(items, item)
	}

	data, err := json.Marshal(items)
	if err != nil {
		t.Fatalf("json.Marshal slice: %v", err)
	}

	var parsed []map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("json.Unmarshal slice: %v", err)
	}

	if len(parsed) < 3 {
		t.Fatalf("expected at least 3 items in array, got %d", len(parsed))
	}

	for i, elem := range parsed {
		for _, field := range []string{"id", "title", "status", "priority", "description"} {
			val, ok := elem[field]
			if !ok {
				t.Errorf("item[%d]: missing key %q", i, field)
				continue
			}
			if val == nil {
				t.Errorf("item[%d]: key %q is nil", i, field)
			}
		}
	}
}

// TestStore_JSON_ShowSingleItem creates 1 item, derives, marshals a single Item
// (not array), and verifies all core fields are accessible.
func TestStore_JSON_ShowSingleItem(t *testing.T) {
	h := storetest.New(t)
	h.Create("item-single", "Single Item", "p0",
		storetest.WithContext("single context"),
		storetest.WithFor("baron"),
		storetest.WithBy("agent"),
	)

	item := h.MustItem("item-single")
	data, err := json.Marshal(item)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	coreFields := []string{"id", "msg_id", "campfire_id", "title", "status", "priority", "for", "by", "type", "context", "description"}
	for _, field := range coreFields {
		val, ok := m[field]
		if !ok {
			t.Errorf("missing core field %q", field)
			continue
		}
		if val == nil {
			t.Errorf("core field %q is nil", field)
		}
	}
}

// TestStore_JSON_GoldenRoundTrip defines a golden JSON string with all fields,
// unmarshals to state.Item, re-marshals, and verifies field names are unchanged.
// This is the regression gate for the JSON contract.
func TestStore_JSON_GoldenRoundTrip(t *testing.T) {
	golden := `{
		"id": "ready-abc",
		"msg_id": "msg-001",
		"campfire_id": "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890",
		"title": "Golden item",
		"context": "golden context",
		"description": "golden context",
		"type": "task",
		"level": "task",
		"project": "ready",
		"for": "baron",
		"by": "agent",
		"priority": "p1",
		"status": "inbox",
		"eta": "2026-04-01T00:00:00Z",
		"due": "2026-05-01T00:00:00Z",
		"parent_id": "ready-parent",
		"blocked_by": ["ready-x"],
		"blocks": ["ready-y"],
		"gate": "review",
		"waiting_on": "waiting on someone",
		"waiting_type": "person",
		"waiting_since": "2026-03-01T00:00:00Z",
		"gate_msg_id": "msg-gate-001",
		"created_at": 1743033600000000000,
		"updated_at": 1743033700000000000
	}`

	var item state.Item
	if err := json.Unmarshal([]byte(golden), &item); err != nil {
		t.Fatalf("unmarshal golden: %v", err)
	}

	data, err := json.Marshal(&item)
	if err != nil {
		t.Fatalf("re-marshal: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal re-marshaled: %v", err)
	}

	// All golden fields must survive the round trip
	goldenFields := []string{
		"id", "msg_id", "campfire_id",
		"title", "context", "description",
		"type", "level", "project",
		"for", "by",
		"priority", "status",
		"eta", "due",
		"parent_id", "blocked_by", "blocks",
		"gate", "waiting_on", "waiting_type", "waiting_since", "gate_msg_id",
		"created_at", "updated_at",
	}
	for _, field := range goldenFields {
		if _, ok := m[field]; !ok {
			t.Errorf("field %q lost in round trip", field)
		}
	}
}
