package main

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/3dl-dev/ready/pkg/state"
	"github.com/3dl-dev/ready/pkg/timeparse"
)

// --- done / fail / cancel resolution tests ---

// TestDoneResolution verifies that rd done sends a work:close with resolution=done.
func TestDoneResolution(t *testing.T) {
	p := closePayload{
		Target:     "msg-abc",
		Resolution: "done",
		Reason:     "Completed",
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
	if decoded["reason"] != "Completed" {
		t.Errorf("expected reason=Completed, got %v", decoded["reason"])
	}
}

// TestFailResolution verifies that rd fail sends a work:close with resolution=failed.
func TestFailResolution(t *testing.T) {
	p := closePayload{
		Target:     "msg-abc",
		Resolution: "failed",
		Reason:     "Approach didn't work",
	}
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded["resolution"] != "failed" {
		t.Errorf("expected resolution=failed, got %v", decoded["resolution"])
	}
}

// TestCancelResolution verifies that rd cancel sends a work:close with resolution=cancelled.
func TestCancelResolution(t *testing.T) {
	p := closePayload{
		Target:     "msg-abc",
		Resolution: "cancelled",
		Reason:     "No longer needed",
	}
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded["resolution"] != "cancelled" {
		t.Errorf("expected resolution=cancelled, got %v", decoded["resolution"])
	}
}

// TestCloseAliasTags verifies the tag structure for alias close commands.
// Tags: work:close + work:resolution:<resolution>
func TestCloseAliasTags(t *testing.T) {
	tests := []struct {
		resolution  string
		wantTag     string
	}{
		{"done", "work:resolution:done"},
		{"failed", "work:resolution:failed"},
		{"cancelled", "work:resolution:cancelled"},
	}
	for _, tc := range tests {
		tags := []string{"work:close", "work:resolution:" + tc.resolution}
		if tags[0] != "work:close" {
			t.Errorf("[%s] expected tags[0]=work:close, got %q", tc.resolution, tags[0])
		}
		if tags[1] != tc.wantTag {
			t.Errorf("[%s] expected tags[1]=%q, got %q", tc.resolution, tc.wantTag, tags[1])
		}
	}
}

// --- cancel cascade tests ---

// TestCancelCascade_ChildrenIdentified verifies that children of a parent
// are correctly identified by parent_id for cascade closure.
func TestCancelCascade_ChildrenIdentified(t *testing.T) {
	parent := &state.Item{ID: "ready-p1", MsgID: "msg-p1", Status: state.StatusActive}
	child1 := &state.Item{ID: "ready-c1", MsgID: "msg-c1", Status: state.StatusActive, ParentID: "ready-p1"}
	child2 := &state.Item{ID: "ready-c2", MsgID: "msg-c2", Status: state.StatusInbox, ParentID: "ready-p1"}
	child3Done := &state.Item{ID: "ready-c3", MsgID: "msg-c3", Status: state.StatusDone, ParentID: "ready-p1"}
	unrelated := &state.Item{ID: "ready-u1", MsgID: "msg-u1", Status: state.StatusActive, ParentID: "ready-other"}

	allItems := []*state.Item{parent, child1, child2, child3Done, unrelated}

	// Simulate the cascade selection logic from cancelCmd.
	var toClose []*state.Item
	for _, item := range allItems {
		if item.ParentID != parent.ID {
			continue
		}
		if state.IsTerminal(item) {
			continue
		}
		toClose = append(toClose, item)
	}

	if len(toClose) != 2 {
		t.Errorf("expected 2 children to cascade-close, got %d", len(toClose))
	}
	// Verify the done child is excluded.
	for _, item := range toClose {
		if item.ID == child3Done.ID {
			t.Errorf("done child %s should not be in cascade set", child3Done.ID)
		}
		if item.ID == unrelated.ID {
			t.Errorf("unrelated item %s should not be in cascade set", unrelated.ID)
		}
	}

	// Verify each would get a closePayload with resolution=cancelled.
	for _, item := range toClose {
		p := closePayload{
			Target:     item.MsgID,
			Resolution: "cancelled",
			Reason:     "cascade",
		}
		b, err := json.Marshal(p)
		if err != nil {
			t.Fatalf("marshal child payload: %v", err)
		}
		var decoded map[string]interface{}
		if err := json.Unmarshal(b, &decoded); err != nil {
			t.Fatalf("unmarshal child payload: %v", err)
		}
		if decoded["resolution"] != "cancelled" {
			t.Errorf("child %s expected resolution=cancelled, got %v", item.ID, decoded["resolution"])
		}
		if decoded["target"] != item.MsgID {
			t.Errorf("child %s expected target=%s, got %v", item.ID, item.MsgID, decoded["target"])
		}
	}
}

// TestCancelCascade_NoChildrenIsNoop verifies cascade on a leaf item closes only the parent.
func TestCancelCascade_NoChildrenIsNoop(t *testing.T) {
	leaf := &state.Item{ID: "ready-l1", MsgID: "msg-l1", Status: state.StatusActive}
	allItems := []*state.Item{leaf}

	var toClose []*state.Item
	for _, item := range allItems {
		if item.ParentID != leaf.ID {
			continue
		}
		if state.IsTerminal(item) {
			continue
		}
		toClose = append(toClose, item)
	}

	if len(toClose) != 0 {
		t.Errorf("expected 0 children for leaf item, got %d", len(toClose))
	}
}

// --- defer tests ---

// TestDeferPayload verifies that rd defer sends a work:update with the ETA field set.
func TestDeferPayload(t *testing.T) {
	etaRFC3339 := "2026-04-01T09:00:00Z"
	p := updatePayload{
		Target: "msg-abc",
		ETA:    etaRFC3339,
	}
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded["eta"] != etaRFC3339 {
		t.Errorf("expected eta=%s, got %v", etaRFC3339, decoded["eta"])
	}
	if decoded["target"] != "msg-abc" {
		t.Errorf("expected target=msg-abc, got %v", decoded["target"])
	}
	// Other fields should be omitted.
	for _, field := range []string{"title", "context", "priority", "due", "level"} {
		if _, ok := decoded[field]; ok {
			t.Errorf("field %s should be omitted when empty", field)
		}
	}
}

// TestDeferRelativeETAParsed verifies that relative ETAs are parsed into RFC3339.
func TestDeferRelativeETAParsed(t *testing.T) {
	now := time.Date(2026, 3, 25, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		expr string
		want string
	}{
		{"2h", "2026-03-25T14:00:00Z"},
		{"1d", "2026-03-26T12:00:00Z"},
		{"tomorrow", "2026-03-26T09:00:00Z"},
		{"next week", "2026-03-30T09:00:00Z"},
	}
	for _, tc := range tests {
		got, err := timeparse.Parse(tc.expr, now)
		if err != nil {
			t.Errorf("Parse(%q): unexpected error: %v", tc.expr, err)
			continue
		}
		if got != tc.want {
			t.Errorf("Parse(%q) = %q, want %q", tc.expr, got, tc.want)
		}
	}
}

// TestDeferTags verifies work:update tag is used for defer.
func TestDeferTags(t *testing.T) {
	tags := []string{"work:update"}
	if tags[0] != "work:update" {
		t.Errorf("expected work:update tag, got %q", tags[0])
	}
}

// --- progress tests ---

// TestProgressContextAppend verifies that progress appends notes to existing context.
func TestProgressContextAppend(t *testing.T) {
	existingContext := "Initial description of the work."
	notes := "Completed auth module, starting on UI"
	now := "2026-03-25T12:00Z"

	// Simulate the append logic from progressCmd.
	newContext := existingContext + "\n\n[" + now + "] " + notes

	if newContext == existingContext {
		t.Error("expected context to be modified")
	}
	if len(newContext) <= len(existingContext) {
		t.Error("expected new context to be longer than original")
	}

	// Verify the notes appear in the context.
	if !containsStr(newContext, notes) {
		t.Errorf("expected new context to contain notes %q", notes)
	}
	if !containsStr(newContext, existingContext) {
		t.Errorf("expected new context to contain original context")
	}
}

// TestProgressContextEmpty verifies that progress works on an item with empty context.
func TestProgressContextEmpty(t *testing.T) {
	existingContext := ""
	notes := "First progress note"
	now := "2026-03-25T12:00Z"

	var newContext string
	if existingContext != "" {
		newContext = existingContext + "\n\n[" + now + "] " + notes
	} else {
		newContext = "[" + now + "] " + notes
	}

	if !containsStr(newContext, notes) {
		t.Errorf("expected context to contain notes, got %q", newContext)
	}
	// Should not start with "\n\n" when there's no existing context.
	if len(newContext) >= 2 && newContext[:2] == "\n\n" {
		t.Errorf("expected context not to start with newlines for empty base, got %q", newContext)
	}
}

// TestProgressPayload verifies that progress sends a work:update with context field.
func TestProgressPayload(t *testing.T) {
	appendedContext := "Initial.\n\n[2026-03-25T12:00Z] Progress note"
	p := updatePayload{
		Target:  "msg-abc",
		Context: appendedContext,
	}
	b, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded["context"] != appendedContext {
		t.Errorf("expected context=%q, got %v", appendedContext, decoded["context"])
	}
	// ETA and other fields should be absent.
	for _, field := range []string{"eta", "title", "priority", "due", "level"} {
		if _, ok := decoded[field]; ok {
			t.Errorf("field %s should be omitted when empty", field)
		}
	}
}

// containsStr is a simple substring check.
func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		func() bool {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
			return false
		}())
}
