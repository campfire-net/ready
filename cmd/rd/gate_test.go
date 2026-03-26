package main

import (
	"encoding/json"
	"testing"
)

// TestBuildGatePayload_GateTypeInTag verifies that BuildGatePayload embeds the
// gate type in the tag per convention §4.8: work:gate-type:<type>. This tag is
// how the attention engine and humans identify what kind of escalation is pending.
func TestBuildGatePayload_GateTypeInTag(t *testing.T) {
	gateTypes := []string{"budget", "design", "scope", "review", "human", "stall", "periodic"}

	for _, gt := range gateTypes {
		t.Run("gate_type="+gt, func(t *testing.T) {
			payloadBytes, tags, antecedents, err := BuildGatePayload("msg-create-abc", gt, "test description")
			if err != nil {
				t.Fatalf("BuildGatePayload(%q) returned error: %v", gt, err)
			}

			// Tags: exactly 2 — work:gate + work:gate-type:<type>.
			if len(tags) != 2 {
				t.Errorf("expected 2 tags, got %d: %v", len(tags), tags)
			}
			if tags[0] != "work:gate" {
				t.Errorf("tags[0]=%q, want 'work:gate'", tags[0])
			}
			wantGateTypeTag := "work:gate-type:" + gt
			if tags[1] != wantGateTypeTag {
				t.Errorf("tags[1]=%q, want %q", tags[1], wantGateTypeTag)
			}

			// Payload gate_type field must match.
			var decoded map[string]interface{}
			if err := json.Unmarshal(payloadBytes, &decoded); err != nil {
				t.Fatalf("json.Unmarshal: %v", err)
			}
			if decoded["gate_type"] != gt {
				t.Errorf("gate_type=%v, want %q", decoded["gate_type"], gt)
			}

			// Antecedent is the create message ID.
			if len(antecedents) != 1 || antecedents[0] != "msg-create-abc" {
				t.Errorf("antecedents=%v, want ['msg-create-abc']", antecedents)
			}
		})
	}
}

// TestBuildGatePayload_TargetIsCreateMsg verifies that the gate payload target
// is the work:create message ID per convention §4.8. State derivation uses this
// to find which item is gated.
func TestBuildGatePayload_TargetIsCreateMsg(t *testing.T) {
	createMsgID := "msg-create-xyz-1234-5678-9abc-def012345678"

	payloadBytes, _, _, err := BuildGatePayload(createMsgID, "design", "Confirm approach")
	if err != nil {
		t.Fatalf("BuildGatePayload returned error: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(payloadBytes, &decoded); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if decoded["target"] != createMsgID {
		t.Errorf("target=%v, want %q (must be work:create msg ID, not item ID)", decoded["target"], createMsgID)
	}
}

// TestBuildGatePayload_DescriptionOmittedWhenEmpty verifies that description is
// omitted from the JSON payload when empty (omitempty) per convention §4.8.
func TestBuildGatePayload_DescriptionOmittedWhenEmpty(t *testing.T) {
	payloadBytes, _, _, err := BuildGatePayload("msg-create-abc", "review", "")
	if err != nil {
		t.Fatalf("BuildGatePayload returned error: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(payloadBytes, &decoded); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if _, ok := decoded["description"]; ok {
		t.Error("description should be omitted when empty (omitempty), but was present in JSON")
	}
}

// TestBuildGateResolvePayload_ResolutionInTag verifies that BuildGateResolvePayload
// puts the resolution value into the work:resolution:<resolution> tag per
// convention §4.9. State derivation uses this tag to transition item status.
func TestBuildGateResolvePayload_ResolutionInTag(t *testing.T) {
	cases := []struct {
		resolution  string
		wantTag     string
	}{
		{"approved", "work:resolution:approved"},
		{"rejected", "work:resolution:rejected"},
	}

	for _, tc := range cases {
		t.Run("resolution="+tc.resolution, func(t *testing.T) {
			payloadBytes, tags, antecedents, err := BuildGateResolvePayload("msg-gate-abc", tc.resolution, "test reason")
			if err != nil {
				t.Fatalf("BuildGateResolvePayload(%q) returned error: %v", tc.resolution, err)
			}

			// Tags: exactly 2 — work:gate-resolve + work:resolution:<resolution>.
			if len(tags) != 2 {
				t.Errorf("expected 2 tags, got %d: %v", len(tags), tags)
			}
			if tags[0] != "work:gate-resolve" {
				t.Errorf("tags[0]=%q, want 'work:gate-resolve'", tags[0])
			}
			if tags[1] != tc.wantTag {
				t.Errorf("tags[1]=%q, want %q", tags[1], tc.wantTag)
			}

			// Payload fields.
			var decoded map[string]interface{}
			if err := json.Unmarshal(payloadBytes, &decoded); err != nil {
				t.Fatalf("json.Unmarshal: %v", err)
			}
			if decoded["resolution"] != tc.resolution {
				t.Errorf("resolution=%v, want %q", decoded["resolution"], tc.resolution)
			}
			if decoded["target"] != "msg-gate-abc" {
				t.Errorf("target=%v, want 'msg-gate-abc'", decoded["target"])
			}

			// Antecedent is the gate message being resolved.
			if len(antecedents) != 1 || antecedents[0] != "msg-gate-abc" {
				t.Errorf("antecedents=%v, want ['msg-gate-abc']", antecedents)
			}
		})
	}
}

// TestBuildGateResolvePayload_ReasonOmittedWhenEmpty verifies that reason is
// omitted when empty per convention §4.9 (omitempty).
func TestBuildGateResolvePayload_ReasonOmittedWhenEmpty(t *testing.T) {
	payloadBytes, _, _, err := BuildGateResolvePayload("msg-gate-abc", "approved", "")
	if err != nil {
		t.Fatalf("BuildGateResolvePayload returned error: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(payloadBytes, &decoded); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if _, ok := decoded["reason"]; ok {
		t.Error("reason should be omitted when empty (omitempty), but was present in JSON")
	}
}

// TestValidGateTypes verifies the gate type validation set covers all convention-
// defined types and rejects unknown values.
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
