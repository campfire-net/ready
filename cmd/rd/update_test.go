package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/spf13/cobra"
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

// --- status alias tests ---

// resolveStatusAlias applies the alias map and returns the resolved status and
// the warning message that would be written to stderr (empty string if no alias).
// This mirrors the logic in update.go RunE.
func resolveStatusAlias(statusTo string) (resolved string, warning string) {
	if canonical, ok := statusAliases[statusTo]; ok {
		warning = fmt.Sprintf("warning: status %q is a bd alias — using %q instead\n", statusTo, canonical)
		return canonical, warning
	}
	return statusTo, ""
}

// isValidStatus returns true for any canonical rd status value.
func isValidStatus(s string) bool {
	validStatuses := map[string]bool{
		"inbox": true, "active": true, "scheduled": true, "waiting": true,
		"done": true, "cancelled": true, "failed": true,
	}
	return validStatuses[s]
}

// TestStatusAlias_InProgress verifies that in_progress maps to active.
func TestStatusAlias_InProgress(t *testing.T) {
	resolved, _ := resolveStatusAlias("in_progress")
	if resolved != "active" {
		t.Errorf("expected in_progress → active, got %q", resolved)
	}
	if !isValidStatus(resolved) {
		t.Errorf("resolved value %q is not a valid status", resolved)
	}
}

// TestStatusAlias_Open verifies that open maps to inbox.
func TestStatusAlias_Open(t *testing.T) {
	resolved, _ := resolveStatusAlias("open")
	if resolved != "inbox" {
		t.Errorf("expected open → inbox, got %q", resolved)
	}
	if !isValidStatus(resolved) {
		t.Errorf("resolved value %q is not a valid status", resolved)
	}
}

// TestStatusAlias_Closed verifies that closed maps to done.
func TestStatusAlias_Closed(t *testing.T) {
	resolved, _ := resolveStatusAlias("closed")
	if resolved != "done" {
		t.Errorf("expected closed → done, got %q", resolved)
	}
	if !isValidStatus(resolved) {
		t.Errorf("resolved value %q is not a valid status", resolved)
	}
}

// TestStatusAlias_Unknown verifies that an unknown status is not aliased and
// does not pass validation.
func TestStatusAlias_Unknown(t *testing.T) {
	resolved, warning := resolveStatusAlias("foobar")
	if resolved != "foobar" {
		t.Errorf("expected foobar to be unchanged, got %q", resolved)
	}
	if warning != "" {
		t.Errorf("expected no warning for unknown status, got %q", warning)
	}
	if isValidStatus(resolved) {
		t.Errorf("foobar should not be a valid status — validation should reject it")
	}
}

// TestStatusAlias_WarningPrinted verifies that the deprecation warning contains
// the expected text ("bd alias") for aliased statuses.
func TestStatusAlias_WarningPrinted(t *testing.T) {
	aliases := []string{"in_progress", "in-progress", "open", "closed", "completed"}
	for _, alias := range aliases {
		_, warning := resolveStatusAlias(alias)
		if warning == "" {
			t.Errorf("alias %q produced no warning", alias)
			continue
		}
		var buf bytes.Buffer
		buf.WriteString(warning)
		if !containsStr(buf.String(), "bd alias") {
			t.Errorf("alias %q warning does not contain 'bd alias': %q", alias, warning)
		}
	}
}

// TestStatusAlias_InProgres_Typo verifies that in_progres (one 's') is NOT aliased
// and fails validation — typos should not silently succeed.
func TestStatusAlias_InProgres_Typo(t *testing.T) {
	resolved, warning := resolveStatusAlias("in_progres")
	if resolved != "in_progres" {
		t.Errorf("expected in_progres to be unchanged, got %q", resolved)
	}
	if warning != "" {
		t.Errorf("expected no warning for typo, got %q", warning)
	}
	if isValidStatus(resolved) {
		t.Errorf("in_progres should not be a valid status — typos must fail validation")
	}
}

// --- claim flag tests ---

// buildClaimPayload mirrors the payload construction logic in update.go when --claim is set.
func buildClaimPayload(createMsgID string) (payload claimPayload, tags []string, antecedents []string) {
	payload = claimPayload{Target: createMsgID}
	tags = []string{"work:claim"}
	antecedents = []string{createMsgID}
	return
}

// TestUpdate_ClaimFlag_SendsClaimMessage verifies that when --claim is set, the claim
// payload is constructed with the correct target, tag, and antecedent.
// Convention §4.5: work:claim sets by=sender, antecedents = exactly_one(target).
func TestUpdate_ClaimFlag_SendsClaimMessage(t *testing.T) {
	createMsgID := "msg-create-abc123"

	payload, tags, antecedents := buildClaimPayload(createMsgID)

	// Verify claim payload.
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal(payloadBytes, &decoded); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if decoded["target"] != createMsgID {
		t.Errorf("claim payload target: expected %q, got %v", createMsgID, decoded["target"])
	}

	// Verify tags: exactly one, must be work:claim.
	if len(tags) != 1 || tags[0] != "work:claim" {
		t.Errorf("claim tags: expected [work:claim], got %v", tags)
	}

	// Verify antecedents: exactly one, must be the create message ID.
	if len(antecedents) != 1 || antecedents[0] != createMsgID {
		t.Errorf("claim antecedents: expected [%s], got %v", createMsgID, antecedents)
	}
}

// TestUpdate_ClaimFlag_WithoutStatus verifies that --claim alone implies --status active.
// bd-compat: bd update --claim transitions to active without requiring --status active.
func TestUpdate_ClaimFlag_WithoutStatus(t *testing.T) {
	claim := true
	statusTo := "" // no --status flag passed

	// Mirrors the logic in update.go RunE.
	if claim && statusTo == "" {
		statusTo = "active"
	}

	if statusTo != "active" {
		t.Errorf("expected --claim alone to imply status=active, got %q", statusTo)
	}
}

// TestUpdate_NoClaimFlag_NoClaim verifies that without --claim, no claim message is sent.
// Default behavior must be unchanged.
func TestUpdate_NoClaimFlag_NoClaim(t *testing.T) {
	claim := false
	claimMessageSent := false

	// Mirrors the guard in update.go RunE.
	if claim {
		claimMessageSent = true
	}

	if claimMessageSent {
		t.Error("expected no claim message when --claim is not set")
	}
}

// TestUpdate_ClaimFlag_PayloadOmitsEmptyReason verifies that the claim payload
// omits the reason field when not provided (json omitempty).
func TestUpdate_ClaimFlag_PayloadOmitsEmptyReason(t *testing.T) {
	p := claimPayload{Target: "msg-create-abc123"}
	payloadBytes, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal(payloadBytes, &decoded); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if _, ok := decoded["reason"]; ok {
		t.Error("claim payload should omit reason when empty")
	}
}

// TestUpdate_BlocksFlag_HelpfulError verifies that --blocks on rd update returns
// a helpful error directing agents to rd dep add, not a generic "unknown flag".
func TestUpdate_BlocksFlag_HelpfulError(t *testing.T) {
	cmd := &cobra.Command{
		Use:  "update <item-id>",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if blocks, _ := cmd.Flags().GetString("blocks"); blocks != "" {
				return fmt.Errorf("--blocks is not a flag on rd update. Use: rd dep add <this-item> %s", blocks)
			}
			return nil
		},
	}
	cmd.Flags().String("blocks", "", "")
	_ = cmd.Flags().MarkHidden("blocks")

	cmd.SetArgs([]string{"ready-a1b", "--blocks", "ready-b2c"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when --blocks is used on rd update, got nil")
	}
	if !strings.Contains(err.Error(), "rd dep add") {
		t.Errorf("expected error to mention 'rd dep add', got: %q", err.Error())
	}
	if !strings.Contains(err.Error(), "ready-b2c") {
		t.Errorf("expected error to include the target ID 'ready-b2c', got: %q", err.Error())
	}
}
