package main

import (
	"encoding/json"
	"testing"
)

// TestBuildBlockPayload_FieldsAndTags verifies that BuildBlockPayload produces
// the correct JSON fields, exactly one tag (work:block), and two antecedents
// referencing both work:create message IDs per convention §4.6.
func TestBuildBlockPayload_FieldsAndTags(t *testing.T) {
	blockerID := "ready-t01"
	blockedID := "ready-t02"
	blockerMsgID := "msg-create-t01-aaaa-bbbb-cccc-dddddddddddd"
	blockedMsgID := "msg-create-t02-aaaa-bbbb-cccc-dddddddddddd"

	payloadBytes, tags, _, err := BuildBlockPayload(blockerID, blockedID, blockerMsgID, blockedMsgID)
	if err != nil {
		t.Fatalf("BuildBlockPayload returned error: %v", err)
	}

	// Decode and verify payload fields.
	var decoded map[string]interface{}
	if err := json.Unmarshal(payloadBytes, &decoded); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if decoded["blocker_id"] != blockerID {
		t.Errorf("blocker_id=%v, want %q", decoded["blocker_id"], blockerID)
	}
	if decoded["blocked_id"] != blockedID {
		t.Errorf("blocked_id=%v, want %q", decoded["blocked_id"], blockedID)
	}
	if decoded["blocker_msg"] != blockerMsgID {
		t.Errorf("blocker_msg=%v, want %q", decoded["blocker_msg"], blockerMsgID)
	}
	if decoded["blocked_msg"] != blockedMsgID {
		t.Errorf("blocked_msg=%v, want %q", decoded["blocked_msg"], blockedMsgID)
	}

	// Tags: exactly one, must be work:block per convention §4.1.
	if len(tags) != 1 {
		t.Errorf("expected exactly 1 tag, got %d: %v", len(tags), tags)
	}
	if tags[0] != "work:block" {
		t.Errorf("tag[0]=%q, want 'work:block'", tags[0])
	}
}

// TestBuildBlockPayload_Antecedents verifies that both work:create message IDs
// appear in the antecedents for a work:block message per convention §4.6.
// This is the campfire causal ordering invariant: the block message causally
// depends on both item creation messages.
func TestBuildBlockPayload_Antecedents(t *testing.T) {
	blockerMsgID := "msg-create-t01-aaaa-bbbb-cccc-dddddddddddd"
	blockedMsgID := "msg-create-t02-aaaa-bbbb-cccc-dddddddddddd"

	_, _, antecedents, err := BuildBlockPayload("r-t01", "r-t02", blockerMsgID, blockedMsgID)
	if err != nil {
		t.Fatalf("BuildBlockPayload returned error: %v", err)
	}

	// Must have exactly 2 antecedents per §4.6.
	if len(antecedents) != 2 {
		t.Fatalf("expected 2 antecedents, got %d: %v", len(antecedents), antecedents)
	}
	if antecedents[0] != blockerMsgID {
		t.Errorf("antecedents[0]=%q, want blockerMsgID=%q", antecedents[0], blockerMsgID)
	}
	if antecedents[1] != blockedMsgID {
		t.Errorf("antecedents[1]=%q, want blockedMsgID=%q", antecedents[1], blockedMsgID)
	}
}

// TestBuildUnblockPayload_TargetIsBlockMsg verifies that BuildUnblockPayload
// sets target to the work:block message ID (not an item ID) per convention §4.7.
// This is the key invariant: work:unblock references the block message, not the items.
func TestBuildUnblockPayload_TargetIsBlockMsg(t *testing.T) {
	blockMsgID := "msg-block-abc-1234-5678-9abc-def012345678"
	reason := "No longer blocked"

	payloadBytes, tags, antecedents, err := BuildUnblockPayload(blockMsgID, reason)
	if err != nil {
		t.Fatalf("BuildUnblockPayload returned error: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(payloadBytes, &decoded); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	// Target must be the block message ID, not an item ID.
	if decoded["target"] != blockMsgID {
		t.Errorf("target=%v, want blockMsgID=%q", decoded["target"], blockMsgID)
	}
	if decoded["reason"] != reason {
		t.Errorf("reason=%v, want %q", decoded["reason"], reason)
	}

	// Tags: exactly one, must be work:unblock.
	if len(tags) != 1 {
		t.Errorf("expected 1 tag, got %d: %v", len(tags), tags)
	}
	if tags[0] != "work:unblock" {
		t.Errorf("tag[0]=%q, want 'work:unblock'", tags[0])
	}

	// Antecedent is the block message being reversed.
	if len(antecedents) != 1 {
		t.Fatalf("expected 1 antecedent, got %d: %v", len(antecedents), antecedents)
	}
	if antecedents[0] != blockMsgID {
		t.Errorf("antecedent=%q, want blockMsgID=%q", antecedents[0], blockMsgID)
	}
}

// TestBuildUnblockPayload_ReasonOmittedWhenEmpty verifies that reason is omitted
// from the JSON payload when empty per convention §4.7 (omitempty).
func TestBuildUnblockPayload_ReasonOmittedWhenEmpty(t *testing.T) {
	payloadBytes, _, _, err := BuildUnblockPayload("msg-block-xyz", "")
	if err != nil {
		t.Fatalf("BuildUnblockPayload returned error: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(payloadBytes, &decoded); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if _, ok := decoded["reason"]; ok {
		t.Error("reason should be omitted when empty (omitempty), but was present in JSON")
	}
}
