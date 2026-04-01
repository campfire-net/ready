package main

import (
	"encoding/json"
	"testing"
)

// buildGateArgsMap constructs the argsMap for a work:gate operation,
// mirroring the logic in gateCmd.RunE.
func buildGateArgsMap(createMsgID, gateType, description string) (map[string]any, []string, []string) {
	argsMap := map[string]any{
		"target":    createMsgID,
		"gate_type": gateType,
	}
	if description != "" {
		argsMap["description"] = description
	}
	tags := []string{"work:gate", "work:gate-type:" + gateType}
	antecedents := []string{createMsgID}
	return argsMap, tags, antecedents
}

// buildGateResolveArgsMap constructs the argsMap for a work:gate-resolve operation,
// mirroring the logic in approveCmd and rejectCmd.
func buildGateResolveArgsMap(gateMsgID, resolution, reason string) (map[string]any, []string, []string) {
	argsMap := map[string]any{
		"target":     gateMsgID,
		"resolution": resolution,
	}
	if reason != "" {
		argsMap["reason"] = reason
	}
	tags := []string{"work:gate-resolve", "work:resolution:" + resolution}
	antecedents := []string{gateMsgID}
	return argsMap, tags, antecedents
}

// TestBuildGateArgsMap_GateTypeInTag verifies that buildGateArgsMap embeds the
// gate type in the tag per convention §4.8: work:gate-type:<type>. This tag is
// how the attention engine and humans identify what kind of escalation is pending.
func TestBuildGateArgsMap_GateTypeInTag(t *testing.T) {
	gateTypes := []string{"budget", "design", "scope", "review", "human", "stall", "periodic"}

	for _, gt := range gateTypes {
		t.Run("gate_type="+gt, func(t *testing.T) {
			argsMap, tags, antecedents := buildGateArgsMap("msg-create-abc", gt, "test description")

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
			payloadBytes, err := json.Marshal(argsMap)
			if err != nil {
				t.Fatalf("json.Marshal: %v", err)
			}
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

// TestBuildGateArgsMap_TargetIsCreateMsg verifies that the gate argsMap target
// is the work:create message ID per convention §4.8. State derivation uses this
// to find which item is gated.
func TestBuildGateArgsMap_TargetIsCreateMsg(t *testing.T) {
	createMsgID := "msg-create-xyz-1234-5678-9abc-def012345678"

	argsMap, _, _ := buildGateArgsMap(createMsgID, "design", "Confirm approach")

	payloadBytes, err := json.Marshal(argsMap)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(payloadBytes, &decoded); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if decoded["target"] != createMsgID {
		t.Errorf("target=%v, want %q (must be work:create msg ID, not item ID)", decoded["target"], createMsgID)
	}
}

// TestBuildGateArgsMap_DescriptionOmittedWhenEmpty verifies that description is
// omitted from the JSON payload when empty (not in argsMap) per convention §4.8.
func TestBuildGateArgsMap_DescriptionOmittedWhenEmpty(t *testing.T) {
	argsMap, _, _ := buildGateArgsMap("msg-create-abc", "review", "")

	payloadBytes, err := json.Marshal(argsMap)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(payloadBytes, &decoded); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if _, ok := decoded["description"]; ok {
		t.Error("description should be omitted when empty, but was present in JSON")
	}
}

// TestBuildGateResolveArgsMap_ResolutionInTag verifies that buildGateResolveArgsMap
// puts the resolution value into the work:resolution:<resolution> tag per
// convention §4.9. State derivation uses this tag to transition item status.
func TestBuildGateResolveArgsMap_ResolutionInTag(t *testing.T) {
	cases := []struct {
		resolution string
		wantTag    string
	}{
		{"approved", "work:resolution:approved"},
		{"rejected", "work:resolution:rejected"},
	}

	for _, tc := range cases {
		t.Run("resolution="+tc.resolution, func(t *testing.T) {
			argsMap, tags, antecedents := buildGateResolveArgsMap("msg-gate-abc", tc.resolution, "test reason")

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
			payloadBytes, err := json.Marshal(argsMap)
			if err != nil {
				t.Fatalf("json.Marshal: %v", err)
			}
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

// TestBuildGateResolveArgsMap_ReasonOmittedWhenEmpty verifies that reason is
// omitted when not in argsMap per convention §4.9.
func TestBuildGateResolveArgsMap_ReasonOmittedWhenEmpty(t *testing.T) {
	argsMap, _, _ := buildGateResolveArgsMap("msg-gate-abc", "approved", "")

	payloadBytes, err := json.Marshal(argsMap)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(payloadBytes, &decoded); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if _, ok := decoded["reason"]; ok {
		t.Error("reason should be omitted when empty, but was present in JSON")
	}
}

// TestGateTypes_ConventionDefined verifies the gate type values defined by the convention.
// The executor validates gate_type against the enum in gate.json.
// Convention §4.8: valid types are budget, design, scope, review, human, stall, periodic.
func TestGateTypes_ConventionDefined(t *testing.T) {
	// These are the values the executor will accept from gate.json's enum.
	validTypes := []string{"budget", "design", "scope", "review", "human", "stall", "periodic"}
	for _, gt := range validTypes {
		argsMap, _, _ := buildGateArgsMap("msg-create-abc", gt, "")
		if argsMap["gate_type"] != gt {
			t.Errorf("expected gate_type=%q in argsMap, got %v", gt, argsMap["gate_type"])
		}
	}
}
