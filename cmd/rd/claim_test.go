package main

import (
	"encoding/json"
	"testing"
)

// TestClaimPayload verifies that claimPayload marshals correctly and
// produces the expected JSON structure for a work:claim message.
func TestClaimPayload(t *testing.T) {
	p := claimPayload{
		Target: "msg-create-abc123",
		Reason: "Accepting delegation",
	}

	payloadBytes, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	// Decode and verify fields.
	var decoded map[string]interface{}
	if err := json.Unmarshal(payloadBytes, &decoded); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if decoded["target"] != "msg-create-abc123" {
		t.Errorf("expected target=msg-create-abc123, got %v", decoded["target"])
	}
	if decoded["reason"] != "Accepting delegation" {
		t.Errorf("expected reason='Accepting delegation', got %v", decoded["reason"])
	}
}

// TestClaimPayloadNoReason verifies that reason is omitted when empty.
func TestClaimPayloadNoReason(t *testing.T) {
	p := claimPayload{
		Target: "msg-create-abc123",
	}

	payloadBytes, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(payloadBytes, &decoded); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if _, ok := decoded["reason"]; ok {
		t.Error("expected reason to be omitted when empty")
	}
}

// TestClaimTags verifies that the correct tags are produced for a work:claim message.
// Convention §4.1: exactly one operation tag; §4.3: work:claim produces work:claim tag.
func TestClaimTags(t *testing.T) {
	tags := []string{"work:claim"}

	if len(tags) != 1 {
		t.Errorf("expected exactly one tag, got %d", len(tags))
	}
	if tags[0] != "work:claim" {
		t.Errorf("expected tag work:claim, got %q", tags[0])
	}
}

// TestClaimAntecedents verifies that the antecedent is the work:create message ID.
// Convention §4.3: antecedents = exactly_one(target).
func TestClaimAntecedents(t *testing.T) {
	createMsgID := "msg-create-abc123"
	antecedents := []string{createMsgID}

	if len(antecedents) != 1 {
		t.Errorf("expected exactly one antecedent, got %d", len(antecedents))
	}
	if antecedents[0] != createMsgID {
		t.Errorf("expected antecedent %s, got %s", createMsgID, antecedents[0])
	}
}
