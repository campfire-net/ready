package main

import (
	"encoding/json"
	"testing"

	"github.com/3dl-dev/ready/pkg/state"
)

// TestBuildCreatePayload_RequiredFields verifies that BuildCreatePayload produces
// the correct JSON for the required fields (id, title, type, for, priority).
func TestBuildCreatePayload_RequiredFields(t *testing.T) {
	payloadBytes, tags, err := BuildCreatePayload(
		"ready-test-001", "My Task", "", "task", "", "", "baron@3dl.dev", "", "p2", "", "", "",
	)
	if err != nil {
		t.Fatalf("BuildCreatePayload returned error: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(payloadBytes, &decoded); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	if decoded["id"] != "ready-test-001" {
		t.Errorf("id=%v, want 'ready-test-001'", decoded["id"])
	}
	if decoded["title"] != "My Task" {
		t.Errorf("title=%v, want 'My Task'", decoded["title"])
	}
	if decoded["type"] != "task" {
		t.Errorf("type=%v, want 'task'", decoded["type"])
	}
	if decoded["for"] != "baron@3dl.dev" {
		t.Errorf("for=%v, want 'baron@3dl.dev'", decoded["for"])
	}
	if decoded["priority"] != "p2" {
		t.Errorf("priority=%v, want 'p2'", decoded["priority"])
	}

	// Tags: work:create, work:type:task, work:for:baron@3dl.dev, work:priority:p2
	wantMinTags := []string{"work:create", "work:type:task", "work:for:baron@3dl.dev", "work:priority:p2"}
	tagSet := make(map[string]bool)
	for _, tag := range tags {
		tagSet[tag] = true
	}
	for _, wt := range wantMinTags {
		if !tagSet[wt] {
			t.Errorf("missing required tag %q in %v", wt, tags)
		}
	}
}

// TestBuildCreatePayload_OptionalTagsWithLevel verifies that work:level:<level>
// tag is added when level is provided, and absent when not.
func TestBuildCreatePayload_OptionalTagsWithLevel(t *testing.T) {
	// With level.
	_, tags, err := BuildCreatePayload("r-001", "T", "", "task", "epic", "", "baron", "", "p1", "", "", "")
	if err != nil {
		t.Fatalf("BuildCreatePayload (with level) error: %v", err)
	}
	hasLevel := false
	for _, tag := range tags {
		if tag == "work:level:epic" {
			hasLevel = true
		}
	}
	if !hasLevel {
		t.Errorf("expected 'work:level:epic' tag when level=epic, got %v", tags)
	}

	// Without level.
	_, tagsNoLevel, err := BuildCreatePayload("r-001", "T", "", "task", "", "", "baron", "", "p1", "", "", "")
	if err != nil {
		t.Fatalf("BuildCreatePayload (no level) error: %v", err)
	}
	for _, tag := range tagsNoLevel {
		if tag == "work:level:" {
			t.Errorf("spurious 'work:level:' tag when level is empty, got %v", tagsNoLevel)
		}
	}
}

// TestBuildCreatePayload_OptionalTagsWithBy verifies that work:by:<by> tag is
// added when by is provided, and absent when not. The by tag is how agents
// discover work assigned to them.
func TestBuildCreatePayload_OptionalTagsWithBy(t *testing.T) {
	// With by.
	_, tags, err := BuildCreatePayload("r-001", "T", "", "task", "", "", "baron", "atlas/worker", "p1", "", "", "")
	if err != nil {
		t.Fatalf("BuildCreatePayload (with by) error: %v", err)
	}
	hasBy := false
	for _, tag := range tags {
		if tag == "work:by:atlas/worker" {
			hasBy = true
		}
	}
	if !hasBy {
		t.Errorf("expected 'work:by:atlas/worker' tag when by=atlas/worker, got %v", tags)
	}

	// Without by (default: unassigned).
	_, tagsNoBy, err := BuildCreatePayload("r-001", "T", "", "task", "", "", "baron", "", "p1", "", "", "")
	if err != nil {
		t.Fatalf("BuildCreatePayload (no by) error: %v", err)
	}
	for _, tag := range tagsNoBy {
		if tag == "work:by:" {
			t.Errorf("spurious 'work:by:' tag when by is empty, got %v", tagsNoBy)
		}
	}
}

// TestBuildCreatePayload_OptionalTagsWithProject verifies that work:project:<project>
// tag is added when project is provided.
func TestBuildCreatePayload_OptionalTagsWithProject(t *testing.T) {
	_, tags, err := BuildCreatePayload("r-001", "T", "", "task", "", "ready", "baron", "", "p1", "", "", "")
	if err != nil {
		t.Fatalf("BuildCreatePayload error: %v", err)
	}
	hasProject := false
	for _, tag := range tags {
		if tag == "work:project:ready" {
			hasProject = true
		}
	}
	if !hasProject {
		t.Errorf("expected 'work:project:ready' tag when project=ready, got %v", tags)
	}
}

// TestBuildCreatePayload_OptionalFieldsOmittedFromJSON verifies that optional
// JSON fields (context, level, project, by, parent_id, eta, due) are omitted
// when empty, keeping the payload lean.
func TestBuildCreatePayload_OptionalFieldsOmittedFromJSON(t *testing.T) {
	payloadBytes, _, err := BuildCreatePayload("r-001", "Task", "", "task", "", "", "baron", "", "p3", "", "", "")
	if err != nil {
		t.Fatalf("BuildCreatePayload error: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(payloadBytes, &decoded); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	omitWhenEmpty := []string{"context", "level", "project", "by", "parent_id", "eta", "due"}
	for _, field := range omitWhenEmpty {
		if _, ok := decoded[field]; ok {
			t.Errorf("field %q should be omitted when empty (omitempty), but was present", field)
		}
	}
}

// TestBuildCreatePayload_DefaultTypeTask verifies that the type "task" is a
// sensible default. Agents omit --type in ~99% of cases (session log insight).
// This test does not set a default (validation is caller's responsibility), but
// verifies the payload correctly carries the type when "task" is passed.
func TestBuildCreatePayload_DefaultTypeTask(t *testing.T) {
	payloadBytes, tags, err := BuildCreatePayload("r-001", "My item", "", "task", "", "", "baron", "", "p2", "", "", "")
	if err != nil {
		t.Fatalf("BuildCreatePayload error: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(payloadBytes, &decoded); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if decoded["type"] != "task" {
		t.Errorf("type=%v, want 'task'", decoded["type"])
	}

	// work:type:task tag must be present.
	hasTypeTag := false
	for _, tag := range tags {
		if tag == "work:type:task" {
			hasTypeTag = true
		}
	}
	if !hasTypeTag {
		t.Errorf("expected 'work:type:task' tag, got %v", tags)
	}
}

// TestBuildCreatePayload_DefaultPriorityP2 verifies that priority p2 produces
// sensible output. Agents rarely use --priority (session log: <1% of creates).
// Verifying p2 is a valid default that produces the correct priority tag.
func TestBuildCreatePayload_DefaultPriorityP2(t *testing.T) {
	_, tags, err := BuildCreatePayload("r-001", "My item", "", "task", "", "", "baron", "", "p2", "", "", "")
	if err != nil {
		t.Fatalf("BuildCreatePayload error: %v", err)
	}

	hasPriorityTag := false
	for _, tag := range tags {
		if tag == "work:priority:p2" {
			hasPriorityTag = true
		}
	}
	if !hasPriorityTag {
		t.Errorf("expected 'work:priority:p2' tag for default p2 priority, got %v", tags)
	}
}

// TestBuildCreatePayload_FlagNameContext verifies that the context field in the
// payload matches the --context flag name (not --description). Agents historically
// tried --description and --context; only --context exists.
func TestBuildCreatePayload_FlagNameContext(t *testing.T) {
	contextValue := "This is the context for the item"
	payloadBytes, _, err := BuildCreatePayload("r-001", "T", contextValue, "task", "", "", "baron", "", "p2", "", "", "")
	if err != nil {
		t.Fatalf("BuildCreatePayload error: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(payloadBytes, &decoded); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	// The JSON field is "context" (not "description").
	if decoded["context"] != contextValue {
		t.Errorf("JSON 'context'=%v, want %q (flag is --context, not --description)", decoded["context"], contextValue)
	}
	if _, ok := decoded["description"]; ok {
		t.Error("JSON should have 'context' not 'description' — agents must use --context flag")
	}
}

// TestCreateCloseSequence_CloseTargetsCreateMsg verifies the create→close sequence:
// the close payload target must be the same message ID referenced in the create,
// not the item's short ID. This is the #1 workflow (2,936 pairs in session logs).
func TestCreateCloseSequence_CloseTargetsCreateMsg(t *testing.T) {
	// Step 1: BuildCreatePayload for a new item.
	createPayloadBytes, _, err := BuildCreatePayload(
		"ready-seq-001", "Sequence task", "do the thing", "task", "", "", "baron", "", "p2", "", "", "",
	)
	if err != nil {
		t.Fatalf("BuildCreatePayload error: %v", err)
	}

	var createDecoded map[string]interface{}
	if err := json.Unmarshal(createPayloadBytes, &createDecoded); err != nil {
		t.Fatalf("json.Unmarshal(create): %v", err)
	}

	// The campfire assigns a message ID when the message is sent. Simulate that.
	simulatedMsgID := "msg-cafebabe-seq-0000-0000-000000000001"

	// Step 2: BuildCloseMessage (from logic_test.go's buildCloseMessage) targeting
	// the create message ID. Using the Item that would be derived from the create.
	item := &state.Item{
		ID:    createDecoded["id"].(string),
		MsgID: simulatedMsgID, // This is the campfire message ID from the create.
	}

	closePayload, closeTags, closeAntecedents := buildCloseMessage(item, "done", "Completed sequence task")

	// The close payload target must be the create message ID, not the item ID.
	if closePayload.Target != simulatedMsgID {
		t.Errorf("close target=%q, want simulatedMsgID=%q (must reference the campfire message, not item ID)", closePayload.Target, simulatedMsgID)
	}
	if closePayload.Target == item.ID {
		t.Errorf("close target must NOT be item.ID=%q — it must be the campfire message ID", item.ID)
	}

	// Close tags must include work:close and work:resolution:done.
	hasCloseTag := false
	hasDoneTag := false
	for _, tag := range closeTags {
		if tag == "work:close" {
			hasCloseTag = true
		}
		if tag == "work:resolution:done" {
			hasDoneTag = true
		}
	}
	if !hasCloseTag {
		t.Errorf("close tags missing 'work:close', got %v", closeTags)
	}
	if !hasDoneTag {
		t.Errorf("close tags missing 'work:resolution:done', got %v", closeTags)
	}

	// Close antecedent must be the create message ID.
	if len(closeAntecedents) != 1 || closeAntecedents[0] != simulatedMsgID {
		t.Errorf("close antecedents=%v, want [%q]", closeAntecedents, simulatedMsgID)
	}
}
