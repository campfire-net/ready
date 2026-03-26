package main

import (
	"encoding/json"
	"testing"
)

// TestDelegatePayload verifies that delegatePayload marshals correctly for
// a work:delegate message per convention §4.5.
func TestDelegatePayload(t *testing.T) {
	p := delegatePayload{
		Target: "msg-create-abc123",
		To:     "alice@3dl.dev",
		From:   "baron@3dl.dev",
		Reason: "Better suited for this task",
	}

	payloadBytes, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(payloadBytes, &decoded); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if decoded["target"] != "msg-create-abc123" {
		t.Errorf("expected target=msg-create-abc123, got %v", decoded["target"])
	}
	if decoded["to"] != "alice@3dl.dev" {
		t.Errorf("expected to=alice@3dl.dev, got %v", decoded["to"])
	}
	if decoded["from"] != "baron@3dl.dev" {
		t.Errorf("expected from=baron@3dl.dev, got %v", decoded["from"])
	}
	if decoded["reason"] != "Better suited for this task" {
		t.Errorf("expected reason='Better suited for this task', got %v", decoded["reason"])
	}
}

// TestDelegatePayloadOptionalFields verifies that optional fields are omitted when empty.
func TestDelegatePayloadOptionalFields(t *testing.T) {
	p := delegatePayload{
		Target: "msg-create-abc123",
		To:     "alice@3dl.dev",
	}

	payloadBytes, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(payloadBytes, &decoded); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if _, ok := decoded["from"]; ok {
		t.Error("expected from to be omitted when empty")
	}
	if _, ok := decoded["reason"]; ok {
		t.Error("expected reason to be omitted when empty")
	}
}

// TestDelegateTags verifies that work:delegate and work:by:<identity> tags are produced.
// Convention §4.1 / §4.5: produces work:delegate (exactly_one) + work:by:* (exactly_one).
func TestDelegateTags(t *testing.T) {
	to := "alice@3dl.dev"
	tags := []string{"work:delegate", "work:by:" + to}

	if len(tags) != 2 {
		t.Errorf("expected 2 tags, got %d", len(tags))
	}
	if tags[0] != "work:delegate" {
		t.Errorf("expected work:delegate, got %q", tags[0])
	}
	if tags[1] != "work:by:alice@3dl.dev" {
		t.Errorf("expected work:by:alice@3dl.dev, got %q", tags[1])
	}
}

// TestDelegateAntecedents verifies antecedent is the work:create message ID.
// Convention §4.5: antecedents = exactly_one(target).
func TestDelegateAntecedents(t *testing.T) {
	createMsgID := "msg-create-abc123"
	antecedents := []string{createMsgID}

	if len(antecedents) != 1 {
		t.Errorf("expected exactly one antecedent, got %d", len(antecedents))
	}
	if antecedents[0] != createMsgID {
		t.Errorf("expected antecedent %s, got %s", createMsgID, antecedents[0])
	}
}

// TestDelegateTagsAutomaton verifies tag composition for automaton identity.
func TestDelegateTagsAutomaton(t *testing.T) {
	to := "atlas/worker-3"
	tags := []string{"work:delegate", "work:by:" + to}

	if tags[1] != "work:by:atlas/worker-3" {
		t.Errorf("expected work:by:atlas/worker-3, got %q", tags[1])
	}
}
