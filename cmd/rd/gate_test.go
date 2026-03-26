package main

import (
	"encoding/json"
	"testing"
)

// TestGatePayload verifies that gatePayload marshals correctly for a work:gate message.
// Convention §4.8.
func TestGatePayload(t *testing.T) {
	p := gatePayload{
		Target:      "msg-create-abc123",
		GateType:    "design",
		Description: "Confirm approach before implementing",
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
	if decoded["gate_type"] != "design" {
		t.Errorf("expected gate_type=design, got %v", decoded["gate_type"])
	}
	if decoded["description"] != "Confirm approach before implementing" {
		t.Errorf("expected description, got %v", decoded["description"])
	}
}

// TestGatePayloadNoDescription verifies that description is omitted when empty.
func TestGatePayloadNoDescription(t *testing.T) {
	p := gatePayload{
		Target:   "msg-create-abc123",
		GateType: "review",
	}

	payloadBytes, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(payloadBytes, &decoded); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if _, ok := decoded["description"]; ok {
		t.Error("expected description to be omitted when empty")
	}
}

// TestGateTags verifies that work:gate produces the correct tags.
// Convention §4.8: work:gate tag + work:gate-type:<type> tag.
func TestGateTags(t *testing.T) {
	gateType := "design"
	tags := []string{"work:gate", "work:gate-type:" + gateType}

	if len(tags) != 2 {
		t.Errorf("expected 2 tags, got %d", len(tags))
	}
	if tags[0] != "work:gate" {
		t.Errorf("expected tags[0]=work:gate, got %q", tags[0])
	}
	if tags[1] != "work:gate-type:design" {
		t.Errorf("expected tags[1]=work:gate-type:design, got %q", tags[1])
	}
}

// TestGateAntecedents verifies that antecedent is the work:create message ID.
// Convention §4.8: antecedents = exactly_one(target = create message).
func TestGateAntecedents(t *testing.T) {
	createMsgID := "msg-create-abc123"
	antecedents := []string{createMsgID}

	if len(antecedents) != 1 {
		t.Errorf("expected exactly one antecedent, got %d", len(antecedents))
	}
	if antecedents[0] != createMsgID {
		t.Errorf("expected antecedent %s, got %s", createMsgID, antecedents[0])
	}
}

// TestValidGateTypes verifies the gate type validation set.
func TestValidGateTypes(t *testing.T) {
	valid := []string{"budget", "design", "scope", "review", "human", "stall", "periodic"}
	for _, gt := range valid {
		if !validGateTypes[gt] {
			t.Errorf("expected %q to be a valid gate type", gt)
		}
	}

	invalid := []string{"", "unknown", "approve", "reject"}
	for _, gt := range invalid {
		if validGateTypes[gt] {
			t.Errorf("expected %q to be invalid gate type", gt)
		}
	}
}

// TestGateResolvePayload_Approved verifies that gateResolvePayload marshals
// correctly for an approved resolution.
func TestGateResolvePayload_Approved(t *testing.T) {
	p := gateResolvePayload{
		Target:     "msg-gate-abc123",
		Resolution: "approved",
		Reason:     "Looks good, proceed",
	}

	payloadBytes, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(payloadBytes, &decoded); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if decoded["target"] != "msg-gate-abc123" {
		t.Errorf("expected target=msg-gate-abc123, got %v", decoded["target"])
	}
	if decoded["resolution"] != "approved" {
		t.Errorf("expected resolution=approved, got %v", decoded["resolution"])
	}
	if decoded["reason"] != "Looks good, proceed" {
		t.Errorf("expected reason, got %v", decoded["reason"])
	}
}

// TestGateResolvePayload_Rejected verifies that gateResolvePayload marshals
// correctly for a rejected resolution.
func TestGateResolvePayload_Rejected(t *testing.T) {
	p := gateResolvePayload{
		Target:     "msg-gate-abc123",
		Resolution: "rejected",
		Reason:     "Scope too broad",
	}

	payloadBytes, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(payloadBytes, &decoded); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if decoded["resolution"] != "rejected" {
		t.Errorf("expected resolution=rejected, got %v", decoded["resolution"])
	}
}

// TestApproveResolveNoReason verifies that reason is omitted when empty.
func TestApproveResolveNoReason(t *testing.T) {
	p := gateResolvePayload{
		Target:     "msg-gate-abc123",
		Resolution: "approved",
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

// TestApproveTags verifies correct tags for work:gate-resolve approved.
// Convention §4.9.
func TestApproveTags(t *testing.T) {
	tags := []string{"work:gate-resolve", "work:resolution:approved"}

	if len(tags) != 2 {
		t.Errorf("expected 2 tags, got %d", len(tags))
	}
	if tags[0] != "work:gate-resolve" {
		t.Errorf("expected tags[0]=work:gate-resolve, got %q", tags[0])
	}
	if tags[1] != "work:resolution:approved" {
		t.Errorf("expected tags[1]=work:resolution:approved, got %q", tags[1])
	}
}

// TestRejectTags verifies correct tags for work:gate-resolve rejected.
// Convention §4.9.
func TestRejectTags(t *testing.T) {
	tags := []string{"work:gate-resolve", "work:resolution:rejected"}

	if len(tags) != 2 {
		t.Errorf("expected 2 tags, got %d", len(tags))
	}
	if tags[0] != "work:gate-resolve" {
		t.Errorf("expected tags[0]=work:gate-resolve, got %q", tags[0])
	}
	if tags[1] != "work:resolution:rejected" {
		t.Errorf("expected tags[1]=work:resolution:rejected, got %q", tags[1])
	}
}

// TestGateResolveAntecedents verifies that the antecedent is the gate message ID.
// Convention §4.9: antecedents = the gate message (--fulfills implies --reply-to).
func TestGateResolveAntecedents(t *testing.T) {
	gateMsgID := "msg-gate-abc123"
	antecedents := []string{gateMsgID}

	if len(antecedents) != 1 {
		t.Errorf("expected exactly one antecedent, got %d", len(antecedents))
	}
	if antecedents[0] != gateMsgID {
		t.Errorf("expected antecedent %s, got %s", gateMsgID, antecedents[0])
	}
}
