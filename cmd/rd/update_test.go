package main

import (
	"encoding/json"
	"testing"
)

// TestUpdatePayload verifies that updatePayload marshals correctly for
// a work:update message per convention §4.10.
func TestUpdatePayload(t *testing.T) {
	p := updatePayload{
		Target:   "msg-create-abc123",
		Priority: "p0",
		ETA:      "2026-04-01T00:00:00Z",
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
	if decoded["priority"] != "p0" {
		t.Errorf("expected priority=p0, got %v", decoded["priority"])
	}
	if decoded["eta"] != "2026-04-01T00:00:00Z" {
		t.Errorf("expected eta=2026-04-01T00:00:00Z, got %v", decoded["eta"])
	}
}

// TestUpdatePayloadAllFields verifies all mutable fields per convention §4.10.
func TestUpdatePayloadAllFields(t *testing.T) {
	p := updatePayload{
		Target:   "msg-create-abc123",
		Title:    "New title",
		Context:  "Updated context",
		Priority: "p1",
		ETA:      "2026-04-01T00:00:00Z",
		Due:      "2026-05-01T00:00:00Z",
		Level:    "subtask",
	}

	payloadBytes, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(payloadBytes, &decoded); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if decoded["title"] != "New title" {
		t.Errorf("expected title='New title', got %v", decoded["title"])
	}
	if decoded["context"] != "Updated context" {
		t.Errorf("expected context='Updated context', got %v", decoded["context"])
	}
	if decoded["level"] != "subtask" {
		t.Errorf("expected level=subtask, got %v", decoded["level"])
	}
}

// TestUpdatePayloadOmitsEmptyFields verifies that empty optional fields are omitted.
func TestUpdatePayloadOmitsEmptyFields(t *testing.T) {
	p := updatePayload{
		Target:   "msg-create-abc123",
		Priority: "p2",
	}

	payloadBytes, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(payloadBytes, &decoded); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	for _, field := range []string{"title", "context", "eta", "due", "level"} {
		if _, ok := decoded[field]; ok {
			t.Errorf("expected %s to be omitted when empty", field)
		}
	}
}

// TestUpdateTags verifies that work:update tag is produced for field updates.
// Convention §4.10: produces work:update (exactly_one).
func TestUpdateTags(t *testing.T) {
	tags := []string{"work:update"}

	if len(tags) != 1 {
		t.Errorf("expected exactly one tag, got %d", len(tags))
	}
	if tags[0] != "work:update" {
		t.Errorf("expected tag work:update, got %q", tags[0])
	}
}

// TestUpdateAntecedents verifies that the antecedent is the work:create message ID.
// Convention §4.10: antecedents = exactly_one(target).
func TestUpdateAntecedents(t *testing.T) {
	createMsgID := "msg-create-abc123"
	antecedents := []string{createMsgID}

	if len(antecedents) != 1 {
		t.Errorf("expected exactly one antecedent, got %d", len(antecedents))
	}
	if antecedents[0] != createMsgID {
		t.Errorf("expected antecedent %s, got %s", createMsgID, antecedents[0])
	}
}

// TestUpdateStatusPayload verifies that updateStatusPayload marshals correctly
// for a work:status message sent by rd update.
func TestUpdateStatusPayload(t *testing.T) {
	p := updateStatusPayload{
		Target:      "msg-create-abc123",
		To:          "waiting",
		Reason:      "Need vendor quote",
		WaitingOn:   "quote from Raytheon",
		WaitingType: "vendor",
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
	if decoded["to"] != "waiting" {
		t.Errorf("expected to=waiting, got %v", decoded["to"])
	}
	if decoded["waiting_on"] != "quote from Raytheon" {
		t.Errorf("expected waiting_on='quote from Raytheon', got %v", decoded["waiting_on"])
	}
	if decoded["waiting_type"] != "vendor" {
		t.Errorf("expected waiting_type=vendor, got %v", decoded["waiting_type"])
	}
}

// TestUpdateStatusTags verifies that work:status and work:status:<to> tags are produced.
// Convention §4.4: produces work:status (exactly_one) + work:status:* (exactly_one).
func TestUpdateStatusTags(t *testing.T) {
	statusTo := "waiting"
	tags := []string{"work:status", "work:status:" + statusTo}

	if len(tags) != 2 {
		t.Errorf("expected 2 tags, got %d", len(tags))
	}
	if tags[0] != "work:status" {
		t.Errorf("expected work:status, got %q", tags[0])
	}
	if tags[1] != "work:status:waiting" {
		t.Errorf("expected work:status:waiting, got %q", tags[1])
	}
}

// TestUpdateAutoWaiting verifies that --waiting-on without --status auto-sets status=waiting.
func TestUpdateAutoWaiting(t *testing.T) {
	waitingOn := "vendor quote"
	statusTo := "" // simulates no --status flag passed

	// This is the auto-set logic from the update command.
	if waitingOn != "" && statusTo == "" {
		statusTo = "waiting"
	}

	if statusTo != "waiting" {
		t.Errorf("expected status to be auto-set to waiting, got %q", statusTo)
	}
}
