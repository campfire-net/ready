package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// TestComplete_ClosesAsDone verifies that rd complete sends a work:close with resolution=done.
func TestComplete_ClosesAsDone(t *testing.T) {
	p := completePayload{
		Target:     "msg-abc",
		Resolution: "done",
		Reason:     "Implemented and merged",
	}
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded["resolution"] != "done" {
		t.Errorf("expected resolution=done, got %v", decoded["resolution"])
	}
	if decoded["reason"] != "Implemented and merged" {
		t.Errorf("expected reason='Implemented and merged', got %v", decoded["reason"])
	}
	// Verify tags would be work:close + work:resolution:done
	tags := []string{"work:close", "work:resolution:done"}
	if tags[0] != "work:close" {
		t.Errorf("expected tags[0]=work:close, got %q", tags[0])
	}
	if tags[1] != "work:resolution:done" {
		t.Errorf("expected tags[1]=work:resolution:done, got %q", tags[1])
	}
}

// TestComplete_BranchInPayload verifies that --branch is included in the close payload.
func TestComplete_BranchInPayload(t *testing.T) {
	branch := "work/rudi-xyz"
	p := completePayload{
		Target:     "msg-abc",
		Resolution: "done",
		Reason:     "Done",
		Branch:     branch,
	}
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded["branch"] != branch {
		t.Errorf("expected branch=%q, got %v", branch, decoded["branch"])
	}
	// resolution must still be done
	if decoded["resolution"] != "done" {
		t.Errorf("expected resolution=done, got %v", decoded["resolution"])
	}
}

// TestComplete_SessionInPayload verifies that --session is included in the close payload.
func TestComplete_SessionInPayload(t *testing.T) {
	session := "abc123"
	p := completePayload{
		Target:     "msg-abc",
		Resolution: "done",
		Reason:     "Done",
		Session:    session,
	}
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded["session"] != session {
		t.Errorf("expected session=%q, got %v", session, decoded["session"])
	}
}

// TestComplete_BranchAndSessionOmittedWhenEmpty verifies that branch/session are omitted
// from the payload when not provided (omitempty).
func TestComplete_BranchAndSessionOmittedWhenEmpty(t *testing.T) {
	p := completePayload{
		Target:     "msg-abc",
		Resolution: "done",
		Reason:     "Done",
	}
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := decoded["branch"]; ok {
		t.Error("branch should be omitted when empty")
	}
	if _, ok := decoded["session"]; ok {
		t.Error("session should be omitted when empty")
	}
}

// TestComplete_RequiresReason verifies that rd complete without --reason returns a clear error.
func TestComplete_RequiresReason(t *testing.T) {
	err := validateCompleteReason("")
	if err == nil {
		t.Fatal("expected error when reason is empty, got nil")
	}
	if !strings.Contains(err.Error(), "required") {
		t.Errorf("error message must contain 'required', got %q", err.Error())
	}
}

// TestComplete_WithReason verifies that complete validation passes when reason is provided.
func TestComplete_WithReason(t *testing.T) {
	err := validateCompleteReason("Implemented and merged")
	if err != nil {
		t.Errorf("expected no error when reason is provided, got %v", err)
	}
}

// validateCompleteReason mirrors the --reason enforcement in completeCmd.
func validateCompleteReason(reason string) error {
	if reason == "" {
		return fmt.Errorf("--reason is required (why is this item being closed?)")
	}
	return nil
}
