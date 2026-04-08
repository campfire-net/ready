package main

import (
	"encoding/json"
	"testing"

	"github.com/campfire-net/ready/pkg/state"
)

// buildBlockArgsMap constructs the argsMap for a work:block operation,
// mirroring the logic in depAddCmd.RunE.
func buildBlockArgsMap(blockerID, blockedID, blockerMsgID, blockedMsgID string) (map[string]any, []string, []string) {
	argsMap := map[string]any{
		"blocker_id":  blockerID,
		"blocked_id":  blockedID,
		"blocker_msg": blockerMsgID,
		"blocked_msg": blockedMsgID,
	}
	tags := []string{"work:block"}
	antecedents := []string{blockerMsgID, blockedMsgID}
	return argsMap, tags, antecedents
}

// buildUnblockArgsMap constructs the argsMap for a work:unblock operation,
// mirroring the logic in depRemoveCmd.RunE.
func buildUnblockArgsMap(blockMsgID, reason string) (map[string]any, []string, []string) {
	argsMap := map[string]any{
		"target": blockMsgID,
	}
	if reason != "" {
		argsMap["reason"] = reason
	}
	tags := []string{"work:unblock"}
	antecedents := []string{blockMsgID}
	return argsMap, tags, antecedents
}

// TestBuildBlockArgsMap_FieldsAndTags verifies that buildBlockArgsMap produces
// the correct JSON fields, exactly one tag (work:block), and two antecedents
// referencing both work:create message IDs per convention §4.6.
func TestBuildBlockArgsMap_FieldsAndTags(t *testing.T) {
	blockerID := "ready-t01"
	blockedID := "ready-t02"
	blockerMsgID := "msg-create-t01-aaaa-bbbb-cccc-dddddddddddd"
	blockedMsgID := "msg-create-t02-aaaa-bbbb-cccc-dddddddddddd"

	argsMap, tags, _ := buildBlockArgsMap(blockerID, blockedID, blockerMsgID, blockedMsgID)

	payloadBytes, err := json.Marshal(argsMap)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
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

// TestBuildBlockArgsMap_Antecedents verifies that both work:create message IDs
// appear in the antecedents for a work:block message per convention §4.6.
// This is the campfire causal ordering invariant: the block message causally
// depends on both item creation messages.
func TestBuildBlockArgsMap_Antecedents(t *testing.T) {
	blockerMsgID := "msg-create-t01-aaaa-bbbb-cccc-dddddddddddd"
	blockedMsgID := "msg-create-t02-aaaa-bbbb-cccc-dddddddddddd"

	_, _, antecedents := buildBlockArgsMap("r-t01", "r-t02", blockerMsgID, blockedMsgID)

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

// TestBuildUnblockArgsMap_TargetIsBlockMsg verifies that buildUnblockArgsMap
// sets target to the work:block message ID (not an item ID) per convention §4.7.
// This is the key invariant: work:unblock references the block message, not the items.
func TestBuildUnblockArgsMap_TargetIsBlockMsg(t *testing.T) {
	blockMsgID := "msg-block-abc-1234-5678-9abc-def012345678"
	reason := "No longer blocked"

	argsMap, tags, antecedents := buildUnblockArgsMap(blockMsgID, reason)

	payloadBytes, err := json.Marshal(argsMap)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
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

// TestBuildUnblockArgsMap_ReasonOmittedWhenEmpty verifies that reason is omitted
// from the JSON payload when empty per convention §4.7 (omit when not in argsMap).
func TestBuildUnblockArgsMap_ReasonOmittedWhenEmpty(t *testing.T) {
	argsMap, _, _ := buildUnblockArgsMap("msg-block-xyz", "")

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

// TestBuildDepTree_CyclicDependencies verifies that buildDepTree handles cyclic
// dependencies correctly without infinite recursion. The bug occurred when:
// 1. A blocks B, B blocks A (simple cycle)
// 2. A third item or reference path leads to a node in the cycle
// 3. After processing a child and deleting from visited, the same node could be
//    revisited on another path, causing infinite recursion.
//
// Fix: Keep cycle detection state persistent across all branches, not just the
// current recursion path. Use a separate inPath map for the current recursion path.
func TestBuildDepTree_CyclicDependencies(t *testing.T) {
	// Create items with a cycle: A blocks B, B blocks A
	itemA := &state.Item{
		ID:     "ready-a",
		Title:  "Item A",
		Status: "ready",
		Blocks: []string{"ready-b"},
	}
	itemB := &state.Item{
		ID:     "ready-b",
		Title:  "Item B",
		Status: "ready",
		Blocks: []string{"ready-a"},
	}

	items := map[string]*state.Item{
		"ready-a": itemA,
		"ready-b": itemB,
	}

	// buildDepTree should not panic or stack overflow
	visited := make(map[string]bool)
	tree := buildDepTree("ready-a", items, visited)

	// Verify the root was processed
	if tree == nil {
		t.Fatal("buildDepTree returned nil")
	}
	if tree.ID != "ready-a" {
		t.Errorf("root ID=%q, want 'ready-a'", tree.ID)
	}

	// Verify we have children (B)
	if len(tree.Children) != 1 {
		t.Fatalf("expected 1 child, got %d", len(tree.Children))
	}
	child := tree.Children[0]
	if child.ID != "ready-b" {
		t.Errorf("child ID=%q, want 'ready-b'", child.ID)
	}

	// B should have A as a child, and it should be marked as cycle
	if len(child.Children) != 1 {
		t.Fatalf("expected B to have 1 child (A), got %d", len(child.Children))
	}
	cycleChild := child.Children[0]
	if cycleChild.ID != "ready-a" {
		t.Errorf("B's child ID=%q, want 'ready-a'", cycleChild.ID)
	}
	// The second reference to A should be marked as "(cycle)"
	if !containsString(cycleChild.Status, "(cycle)") {
		t.Errorf("B's child A status=%q, should contain '(cycle)'", cycleChild.Status)
	}
}

// TestBuildDepTree_CyclicDependencies_WithThirdPath verifies that buildDepTree
// handles the specific case where a cyclic node is reachable via a third path.
// This tests the original bug: A blocks B, B blocks A, and a third reference to A
// or B. After deleting visited[B] after processing B's children, if A appears
// again (directly or through a third reference), visited[A] should not be deleted
// until the entire traversal completes.
func TestBuildDepTree_CyclicDependencies_WithThirdPath(t *testing.T) {
	// A blocks B
	// B blocks A (creating cycle)
	// C blocks A (third reference path to node in cycle)
	itemA := &state.Item{
		ID:     "ready-a",
		Title:  "Item A",
		Status: "ready",
		Blocks: []string{"ready-b"},
	}
	itemB := &state.Item{
		ID:     "ready-b",
		Title:  "Item B",
		Status: "ready",
		Blocks: []string{"ready-a"},
	}
	itemC := &state.Item{
		ID:     "ready-c",
		Title:  "Item C",
		Status: "ready",
		Blocks: []string{"ready-a"},
	}

	items := map[string]*state.Item{
		"ready-a": itemA,
		"ready-b": itemB,
		"ready-c": itemC,
	}

	// Start from C, which references A, which references B, which references A again
	visited := make(map[string]bool)
	tree := buildDepTree("ready-c", items, visited)

	if tree == nil {
		t.Fatal("buildDepTree returned nil")
	}
	if tree.ID != "ready-c" {
		t.Errorf("root ID=%q, want 'ready-c'", tree.ID)
	}

	// C should have A as a child
	if len(tree.Children) != 1 {
		t.Fatalf("expected 1 child for C, got %d", len(tree.Children))
	}
	childA := tree.Children[0]
	if childA.ID != "ready-a" {
		t.Errorf("child ID=%q, want 'ready-a'", childA.ID)
	}

	// A should have B as a child
	if len(childA.Children) != 1 {
		t.Fatalf("expected 1 child for A, got %d", len(childA.Children))
	}
	childB := childA.Children[0]
	if childB.ID != "ready-b" {
		t.Errorf("child B ID=%q, want 'ready-b'", childB.ID)
	}

	// B's child should be A marked as cycle
	if len(childB.Children) != 1 {
		t.Fatalf("expected 1 child for B, got %d", len(childB.Children))
	}
	childA2 := childB.Children[0]
	if childA2.ID != "ready-a" {
		t.Errorf("B's child ID=%q, want 'ready-a'", childA2.ID)
	}
	if !containsString(childA2.Status, "(cycle)") {
		t.Errorf("B's child A status=%q, should contain '(cycle)'", childA2.Status)
	}
}

// containsString is a helper to check if a string contains a substring.
func containsString(s, substring string) bool {
	for i := 0; i <= len(s)-len(substring); i++ {
		if s[i:i+len(substring)] == substring {
			return true
		}
	}
	return false
}
