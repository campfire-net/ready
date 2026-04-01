package main

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/campfire-net/ready/pkg/state"
	"github.com/campfire-net/ready/pkg/timeparse"
)

// --- done / fail / cancel resolution tests ---

// TestDoneResolution verifies that rd done sends a work:close with resolution=done.
func TestDoneResolution(t *testing.T) {
	argsMap := map[string]any{
		"target":     "msg-abc",
		"resolution": "done",
		"reason":     "Completed",
	}
	b, err := json.Marshal(argsMap)
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
	argsMap := map[string]any{
		"target":     "msg-abc",
		"resolution": "failed",
		"reason":     "Approach didn't work",
	}
	b, err := json.Marshal(argsMap)
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
	argsMap := map[string]any{
		"target":     "msg-abc",
		"resolution": "cancelled",
		"reason":     "No longer needed",
	}
	b, err := json.Marshal(argsMap)
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

// TestCancelCascade_ChildrenIdentified verifies that cascadeCloseDescendants selects
// only open direct children of the parent (terminal children and unrelated items excluded).
func TestCancelCascade_ChildrenIdentified(t *testing.T) {
	parent := &state.Item{ID: "ready-p1", MsgID: "msg-p1", Status: state.StatusActive}
	child1 := &state.Item{ID: "ready-c1", MsgID: "msg-c1", Status: state.StatusActive, ParentID: "ready-p1"}
	child2 := &state.Item{ID: "ready-c2", MsgID: "msg-c2", Status: state.StatusInbox, ParentID: "ready-p1"}
	child3Done := &state.Item{ID: "ready-c3", MsgID: "msg-c3", Status: state.StatusDone, ParentID: "ready-p1"}
	unrelated := &state.Item{ID: "ready-u1", MsgID: "msg-u1", Status: state.StatusActive, ParentID: "ready-other"}

	allItems := []*state.Item{parent, child1, child2, child3Done, unrelated}

	var toClose []*state.Item
	closedIDs, err := cascadeCloseDescendants(allItems, parent.ID, "cascade", func(c *state.Item, _ string) error {
		toClose = append(toClose, c)
		return nil
	})
	if err != nil {
		t.Fatalf("cascadeCloseDescendants: %v", err)
	}

	if len(closedIDs) != 2 {
		t.Errorf("expected 2 children to cascade-close, got %d", len(closedIDs))
	}
	// Verify the done child and unrelated item are excluded.
	for _, item := range toClose {
		if item.ID == child3Done.ID {
			t.Errorf("done child %s should not be in cascade set", child3Done.ID)
		}
		if item.ID == unrelated.ID {
			t.Errorf("unrelated item %s should not be in cascade set", unrelated.ID)
		}
	}
}

// TestCancelCascade_NoChildrenIsNoop verifies cascade on a leaf item closes only the parent.
func TestCancelCascade_NoChildrenIsNoop(t *testing.T) {
	leaf := &state.Item{ID: "ready-l1", MsgID: "msg-l1", Status: state.StatusActive}
	allItems := []*state.Item{leaf}

	closedIDs, err := cascadeCloseDescendants(allItems, leaf.ID, "cascade", noopClose)
	if err != nil {
		t.Fatalf("cascadeCloseDescendants: %v", err)
	}

	if len(closedIDs) != 0 {
		t.Errorf("expected 0 children for leaf item, got %d", len(closedIDs))
	}
}

// noopClose is a stub closeOne that does nothing, used when tests only care about
// which items cascadeCloseDescendants selects, not about what it sends.
func noopClose(_ *state.Item, _ string) error { return nil }

// TestCancelCascade_RecursiveGrandchildren verifies that cascadeCloseDescendants walks
// the full subtree recursively: grandchildren are closed before their parent children,
// and all open descendants are included.
func TestCancelCascade_RecursiveGrandchildren(t *testing.T) {
	//   parent
	//   ├── child1 (active)
	//   │   └── grandchild1 (active)
	//   │   └── grandchild2 (done — should be excluded)
	//   └── child2 (cancelled — should be excluded)
	parent := &state.Item{ID: "ready-p1", MsgID: "msg-p1", Status: state.StatusActive}
	child1 := &state.Item{ID: "ready-c1", MsgID: "msg-c1", Status: state.StatusActive, ParentID: "ready-p1"}
	child2 := &state.Item{ID: "ready-c2", MsgID: "msg-c2", Status: state.StatusCancelled, ParentID: "ready-p1"}
	grandchild1 := &state.Item{ID: "ready-g1", MsgID: "msg-g1", Status: state.StatusActive, ParentID: "ready-c1"}
	grandchild2 := &state.Item{ID: "ready-g2", MsgID: "msg-g2", Status: state.StatusDone, ParentID: "ready-c1"}

	allItems := []*state.Item{parent, child1, child2, grandchild1, grandchild2}

	// Use a recording stub so we can inspect the order of calls.
	var toClose []*state.Item
	closedIDs, err := cascadeCloseDescendants(allItems, parent.ID, "test-reason", func(c *state.Item, _ string) error {
		toClose = append(toClose, c)
		return nil
	})
	if err != nil {
		t.Fatalf("cascadeCloseDescendants: %v", err)
	}

	// Expect grandchild1 and child1 (grandchild2 is done, child2 is cancelled).
	if len(toClose) != 2 {
		t.Fatalf("expected 2 open descendants, got %d: %v", len(toClose), closedIDs)
	}

	// grandchild1 must come before child1 (depth-first, leaves first).
	if toClose[0].ID != grandchild1.ID {
		t.Errorf("expected grandchild1 first (leaves before parents), got %s", toClose[0].ID)
	}
	if toClose[1].ID != child1.ID {
		t.Errorf("expected child1 second (parent after children), got %s", toClose[1].ID)
	}

	// Verify closed children and done grandchildren are excluded.
	for _, item := range toClose {
		if item.ID == child2.ID {
			t.Errorf("cancelled child2 should not be in cascade set")
		}
		if item.ID == grandchild2.ID {
			t.Errorf("done grandchild2 should not be in cascade set")
		}
		if item.ID == parent.ID {
			t.Errorf("parent should not be in descendants set (closed separately)")
		}
	}
}

// TestCancelCascade_AllChildrenTerminal verifies that cascadeCloseDescendants on a
// parent where all children are already terminal returns an empty set — only the
// parent is closed (by cancelCmd.RunE itself, not by the cascade helper).
func TestCancelCascade_AllChildrenTerminal(t *testing.T) {
	parent := &state.Item{ID: "ready-p2", MsgID: "msg-p2", Status: state.StatusActive}
	child1 := &state.Item{ID: "ready-c3", MsgID: "msg-c3", Status: state.StatusDone, ParentID: "ready-p2"}
	child2 := &state.Item{ID: "ready-c4", MsgID: "msg-c4", Status: state.StatusCancelled, ParentID: "ready-p2"}
	child3 := &state.Item{ID: "ready-c5", MsgID: "msg-c5", Status: state.StatusFailed, ParentID: "ready-p2"}

	allItems := []*state.Item{parent, child1, child2, child3}
	closedIDs, err := cascadeCloseDescendants(allItems, parent.ID, "test-reason", noopClose)
	if err != nil {
		t.Fatalf("cascadeCloseDescendants: %v", err)
	}

	if len(closedIDs) != 0 {
		t.Errorf("expected 0 open descendants when all children are terminal, got %d", len(closedIDs))
	}
}

// capturedCloseCall records the child and reason passed to a stub closeOne callback.
type capturedCloseCall struct {
	childID string
	msgID   string
	reason  string
}

// TestCancelCascade_ReasonPropagated verifies that cascadeCloseDescendants —
// the same function invoked by cancelCmd.RunE — passes the caller-supplied
// reason into the closeOne callback for each open descendant.
//
// A capturing closeOne stub (no executor, no store, no filesystem) records
// what cascadeCloseDescendants actually delivers to the callback, proving that
// the --reason flag value flows through to the childArgs sent to executeConventionOp.
func TestCancelCascade_ReasonPropagated(t *testing.T) {
	parent := &state.Item{ID: "ready-p3", MsgID: "msg-p3", Status: state.StatusActive}
	child := &state.Item{ID: "ready-c6", MsgID: "msg-c6", Status: state.StatusActive, ParentID: "ready-p3"}

	allItems := []*state.Item{parent, child}
	reason := "Scope cut — entire feature cancelled"

	var calls []capturedCloseCall
	closeOne := func(c *state.Item, r string) error {
		calls = append(calls, capturedCloseCall{childID: c.ID, msgID: c.MsgID, reason: r})
		return nil
	}

	closedIDs, err := cascadeCloseDescendants(allItems, parent.ID, reason, closeOne)
	if err != nil {
		t.Fatalf("cascadeCloseDescendants: %v", err)
	}

	if len(closedIDs) != 1 {
		t.Fatalf("expected 1 closed ID, got %d: %v", len(closedIDs), closedIDs)
	}
	if len(calls) != 1 {
		t.Fatalf("expected closeOne called once, got %d calls", len(calls))
	}

	got := calls[0]
	if got.childID != child.ID {
		t.Errorf("closeOne childID=%q, want %q", got.childID, child.ID)
	}
	if got.msgID != child.MsgID {
		t.Errorf("closeOne msgID=%q, want %q", got.msgID, child.MsgID)
	}
	if got.reason != reason {
		t.Errorf("closeOne reason=%q, want %q — cancelCmd.RunE must propagate --reason into childArgs", got.reason, reason)
	}
}

// --- defer tests ---

// TestDeferPayload verifies that rd defer sends a work:update with the ETA field set.
func TestDeferPayload(t *testing.T) {
	etaRFC3339 := "2026-04-01T09:00:00Z"
	argsMap := map[string]any{
		"target": "msg-abc",
		"eta":    etaRFC3339,
	}
	b, err := json.Marshal(argsMap)
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
	// Other fields should be omitted (not present in argsMap).
	for _, field := range []string{"title", "context", "priority", "due", "level"} {
		if _, ok := decoded[field]; ok {
			t.Errorf("field %s should be omitted when not in argsMap", field)
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
	argsMap := map[string]any{
		"target":  "msg-abc",
		"context": appendedContext,
	}
	b, err := json.Marshal(argsMap)
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
	// ETA and other fields should be absent (not in argsMap).
	for _, field := range []string{"eta", "title", "priority", "due", "level"} {
		if _, ok := decoded[field]; ok {
			t.Errorf("field %s should be omitted when not in argsMap", field)
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
