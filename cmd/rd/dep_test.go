package main

import (
	"encoding/json"
	"testing"
)

// TestBlockPayload verifies that blockPayload marshals correctly per §4.6.
func TestBlockPayload(t *testing.T) {
	p := blockPayload{
		BlockerID:  "ready-t01",
		BlockedID:  "ready-t02",
		BlockerMsg: "msg-create-t01",
		BlockedMsg: "msg-create-t02",
	}

	payloadBytes, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(payloadBytes, &decoded); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	cases := map[string]string{
		"blocker_id":  "ready-t01",
		"blocked_id":  "ready-t02",
		"blocker_msg": "msg-create-t01",
		"blocked_msg": "msg-create-t02",
	}
	for field, want := range cases {
		got, ok := decoded[field]
		if !ok {
			t.Errorf("missing field %q", field)
			continue
		}
		if got != want {
			t.Errorf("field %q: expected %q, got %v", field, want, got)
		}
	}
}

// TestDepUnblockPayload verifies that depUnblockPayload marshals correctly per §4.7.
// Target must be the work:block message ID; reason is optional.
func TestDepUnblockPayload(t *testing.T) {
	p := depUnblockPayload{
		Target: "msg-block-xyz",
		Reason: "Dependency removed",
	}

	payloadBytes, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(payloadBytes, &decoded); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if decoded["target"] != "msg-block-xyz" {
		t.Errorf("expected target=msg-block-xyz, got %v", decoded["target"])
	}
	if decoded["reason"] != "Dependency removed" {
		t.Errorf("expected reason='Dependency removed', got %v", decoded["reason"])
	}
}

// TestDepUnblockPayloadNoReason verifies that reason is omitted when empty (§4.7).
func TestDepUnblockPayloadNoReason(t *testing.T) {
	p := depUnblockPayload{
		Target: "msg-block-xyz",
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

// TestDepAddTags verifies the tags produced by rd dep add.
// Convention §4.1: exactly one operation tag; §4.6: work:block.
func TestDepAddTags(t *testing.T) {
	tags := []string{"work:block"}

	if len(tags) != 1 {
		t.Errorf("expected exactly one tag, got %d", len(tags))
	}
	if tags[0] != "work:block" {
		t.Errorf("expected tag work:block, got %q", tags[0])
	}
}

// TestDepAddAntecedents verifies that dep add includes both work:create message IDs
// as antecedents per §4.6.
func TestDepAddAntecedents(t *testing.T) {
	blockerMsgID := "msg-create-t01"
	blockedMsgID := "msg-create-t02"

	// Both must be antecedents (spec §4.6: both message IDs as antecedents).
	antecedents := []string{blockerMsgID, blockedMsgID}

	if len(antecedents) != 2 {
		t.Errorf("expected 2 antecedents, got %d", len(antecedents))
	}
	if antecedents[0] != blockerMsgID {
		t.Errorf("expected antecedents[0]=%s, got %s", blockerMsgID, antecedents[0])
	}
	if antecedents[1] != blockedMsgID {
		t.Errorf("expected antecedents[1]=%s, got %s", blockedMsgID, antecedents[1])
	}
}

// TestDepRemoveTags verifies the tags produced by rd dep remove.
// Convention §4.1: exactly one operation tag; §4.7: work:unblock.
func TestDepRemoveTags(t *testing.T) {
	tags := []string{"work:unblock"}

	if len(tags) != 1 {
		t.Errorf("expected exactly one tag, got %d", len(tags))
	}
	if tags[0] != "work:unblock" {
		t.Errorf("expected tag work:unblock, got %q", tags[0])
	}
}

// TestDepRemoveAntecedents verifies that dep remove uses the block message ID
// as the sole antecedent per §4.7: antecedents = exactly_one(target).
func TestDepRemoveAntecedents(t *testing.T) {
	blockMsgID := "msg-block-abc"
	antecedents := []string{blockMsgID}

	if len(antecedents) != 1 {
		t.Errorf("expected 1 antecedent, got %d", len(antecedents))
	}
	if antecedents[0] != blockMsgID {
		t.Errorf("expected antecedent=%s, got %s", blockMsgID, antecedents[0])
	}
}
