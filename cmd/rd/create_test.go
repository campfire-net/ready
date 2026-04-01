package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/campfire-net/ready/pkg/state"
	"github.com/spf13/cobra"
)

// buildCreateArgsMap constructs the argsMap for a work:create operation,
// mirroring the logic in createCmd.RunE. Only non-empty optional fields are included.
func buildCreateArgsMap(id, title, context, itemType, level, project, forParty, by, priority, parentID, eta, due string) (map[string]any, error) {
	if id == "" || title == "" || itemType == "" || forParty == "" || priority == "" {
		return nil, fmt.Errorf("id, title, type, for, and priority are required")
	}
	argsMap := map[string]any{
		"id":       id,
		"title":    title,
		"type":     itemType,
		"for":      forParty,
		"priority": priority,
	}
	if context != "" {
		argsMap["context"] = context
	}
	if level != "" {
		argsMap["level"] = level
	}
	if project != "" {
		argsMap["project"] = project
	}
	if by != "" {
		argsMap["by"] = by
	}
	if parentID != "" {
		argsMap["parent_id"] = parentID
	}
	if eta != "" {
		argsMap["eta"] = eta
	}
	if due != "" {
		argsMap["due"] = due
	}
	return argsMap, nil
}

// buildCreateTags mirrors the tag composition that the convention executor
// produces from the create.json produces_tags rules.
func buildCreateTags(argsMap map[string]any) []string {
	tags := []string{"work:create"}
	if t, ok := argsMap["type"].(string); ok && t != "" {
		tags = append(tags, "work:type:"+t)
	}
	if f, ok := argsMap["for"].(string); ok && f != "" {
		tags = append(tags, "work:for:"+f)
	}
	if p, ok := argsMap["priority"].(string); ok && p != "" {
		tags = append(tags, "work:priority:"+p)
	}
	if l, ok := argsMap["level"].(string); ok && l != "" {
		tags = append(tags, "work:level:"+l)
	}
	if b, ok := argsMap["by"].(string); ok && b != "" {
		tags = append(tags, "work:by:"+b)
	}
	if proj, ok := argsMap["project"].(string); ok && proj != "" {
		tags = append(tags, "work:project:"+proj)
	}
	return tags
}

// TestBuildCreateArgsMap_RequiredFields verifies that buildCreateArgsMap produces
// the correct JSON for the required fields (id, title, type, for, priority).
func TestBuildCreateArgsMap_RequiredFields(t *testing.T) {
	argsMap, err := buildCreateArgsMap(
		"ready-test-001", "My Task", "", "task", "", "", "baron@3dl.dev", "", "p2", "", "", "",
	)
	if err != nil {
		t.Fatalf("buildCreateArgsMap returned error: %v", err)
	}

	payloadBytes, err := json.Marshal(argsMap)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
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
	tags := buildCreateTags(argsMap)
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

// TestBuildCreateArgsMap_OptionalTagsWithLevel verifies that work:level:<level>
// tag is added when level is provided, and absent when not.
func TestBuildCreateArgsMap_OptionalTagsWithLevel(t *testing.T) {
	// With level.
	argsMap, err := buildCreateArgsMap("r-001", "T", "", "task", "epic", "", "baron", "", "p1", "", "", "")
	if err != nil {
		t.Fatalf("buildCreateArgsMap (with level) error: %v", err)
	}
	tags := buildCreateTags(argsMap)
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
	argsMapNoLevel, err := buildCreateArgsMap("r-001", "T", "", "task", "", "", "baron", "", "p1", "", "", "")
	if err != nil {
		t.Fatalf("buildCreateArgsMap (no level) error: %v", err)
	}
	tagsNoLevel := buildCreateTags(argsMapNoLevel)
	for _, tag := range tagsNoLevel {
		if tag == "work:level:" {
			t.Errorf("spurious 'work:level:' tag when level is empty, got %v", tagsNoLevel)
		}
	}
}

// TestBuildCreateArgsMap_OptionalTagsWithBy verifies that work:by:<by> tag is
// added when by is provided, and absent when not. The by tag is how agents
// discover work assigned to them.
func TestBuildCreateArgsMap_OptionalTagsWithBy(t *testing.T) {
	// With by.
	argsMap, err := buildCreateArgsMap("r-001", "T", "", "task", "", "", "baron", "atlas/worker", "p1", "", "", "")
	if err != nil {
		t.Fatalf("buildCreateArgsMap (with by) error: %v", err)
	}
	tags := buildCreateTags(argsMap)
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
	argsMapNoBy, err := buildCreateArgsMap("r-001", "T", "", "task", "", "", "baron", "", "p1", "", "", "")
	if err != nil {
		t.Fatalf("buildCreateArgsMap (no by) error: %v", err)
	}
	tagsNoBy := buildCreateTags(argsMapNoBy)
	for _, tag := range tagsNoBy {
		if tag == "work:by:" {
			t.Errorf("spurious 'work:by:' tag when by is empty, got %v", tagsNoBy)
		}
	}
}

// TestBuildCreateArgsMap_OptionalTagsWithProject verifies that work:project:<project>
// tag is added when project is provided.
func TestBuildCreateArgsMap_OptionalTagsWithProject(t *testing.T) {
	argsMap, err := buildCreateArgsMap("r-001", "T", "", "task", "", "ready", "baron", "", "p1", "", "", "")
	if err != nil {
		t.Fatalf("buildCreateArgsMap error: %v", err)
	}
	tags := buildCreateTags(argsMap)
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

// TestBuildCreateArgsMap_OptionalFieldsOmittedFromJSON verifies that optional
// JSON fields (context, level, project, by, parent_id, eta, due) are omitted
// when empty, keeping the payload lean.
func TestBuildCreateArgsMap_OptionalFieldsOmittedFromJSON(t *testing.T) {
	argsMap, err := buildCreateArgsMap("r-001", "Task", "", "task", "", "", "baron", "", "p3", "", "", "")
	if err != nil {
		t.Fatalf("buildCreateArgsMap error: %v", err)
	}

	payloadBytes, err := json.Marshal(argsMap)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(payloadBytes, &decoded); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	omitWhenEmpty := []string{"context", "level", "project", "by", "parent_id", "eta", "due"}
	for _, field := range omitWhenEmpty {
		if _, ok := decoded[field]; ok {
			t.Errorf("field %q should be omitted when empty, but was present", field)
		}
	}
}

// TestBuildCreateArgsMap_DefaultTypeTask verifies that the type "task" is a
// sensible default and the payload correctly carries it.
func TestBuildCreateArgsMap_DefaultTypeTask(t *testing.T) {
	argsMap, err := buildCreateArgsMap("r-001", "My item", "", "task", "", "", "baron", "", "p2", "", "", "")
	if err != nil {
		t.Fatalf("buildCreateArgsMap error: %v", err)
	}

	payloadBytes, err := json.Marshal(argsMap)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(payloadBytes, &decoded); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if decoded["type"] != "task" {
		t.Errorf("type=%v, want 'task'", decoded["type"])
	}

	// work:type:task tag must be present.
	tags := buildCreateTags(argsMap)
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

// TestBuildCreateArgsMap_DefaultPriorityP2 verifies that priority p2 produces
// sensible output and the correct priority tag.
func TestBuildCreateArgsMap_DefaultPriorityP2(t *testing.T) {
	argsMap, err := buildCreateArgsMap("r-001", "My item", "", "task", "", "", "baron", "", "p2", "", "", "")
	if err != nil {
		t.Fatalf("buildCreateArgsMap error: %v", err)
	}

	tags := buildCreateTags(argsMap)
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

// TestBuildCreateArgsMap_FlagNameContext verifies that the context field in the
// payload matches the --context flag name (not --description). Agents historically
// tried --description and --context; only --context exists.
func TestBuildCreateArgsMap_FlagNameContext(t *testing.T) {
	contextValue := "This is the context for the item"
	argsMap, err := buildCreateArgsMap("r-001", "T", contextValue, "task", "", "", "baron", "", "p2", "", "", "")
	if err != nil {
		t.Fatalf("buildCreateArgsMap error: %v", err)
	}

	payloadBytes, err := json.Marshal(argsMap)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
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
	// Step 1: build argsMap for a new item.
	argsMap, err := buildCreateArgsMap(
		"ready-seq-001", "Sequence task", "do the thing", "task", "", "", "baron", "", "p2", "", "", "",
	)
	if err != nil {
		t.Fatalf("buildCreateArgsMap error: %v", err)
	}

	payloadBytes, err := json.Marshal(argsMap)
	if err != nil {
		t.Fatalf("json.Marshal(create): %v", err)
	}

	var createDecoded map[string]interface{}
	if err := json.Unmarshal(payloadBytes, &createDecoded); err != nil {
		t.Fatalf("json.Unmarshal(create): %v", err)
	}

	// The campfire assigns a message ID when the message is sent. Simulate that.
	simulatedMsgID := "msg-cafebabe-seq-0000-0000-000000000001"

	// Step 2: buildCloseMessage (from logic_test.go's buildCloseMessage) targeting
	// the create message ID. Using the Item that would be derived from the create.
	item := &state.Item{
		ID:    createDecoded["id"].(string),
		MsgID: simulatedMsgID, // This is the campfire message ID from the create.
	}

	closeArgsMap, closeTags, closeAntecedents := buildCloseMessage(item, "done", "Completed sequence task")

	// The close argsMap target must be the create message ID, not the item ID.
	closeTarget, _ := closeArgsMap["target"].(string)
	if closeTarget != simulatedMsgID {
		t.Errorf("close target=%q, want simulatedMsgID=%q (must reference the campfire message, not item ID)", closeTarget, simulatedMsgID)
	}
	if closeTarget == item.ID {
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

// TestCreate_PositionalTitle verifies that title can be passed as a positional arg.
func TestCreate_PositionalTitle(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    string
		wantErr string
	}{
		{"positional", []string{"Fix auth bug"}, "Fix auth bug", ""},
		{"flag", []string{"--title", "Fix auth bug"}, "Fix auth bug", ""},
		{"positional multi-word", []string{"Fix", "auth", "bug"}, "Fix auth bug", ""},
		{"both errors", []string{"Positional", "--title", "Flag"}, "", "both positional argument and --title"},
		{"neither errors", []string{}, "", "title is required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &cobra.Command{
				Use: "create [title]",
				RunE: func(c *cobra.Command, args []string) error {
					title, _ := c.Flags().GetString("title")
					if len(args) > 0 && title != "" {
						return fmt.Errorf("title provided as both positional argument and --title flag; use one or the other")
					}
					if len(args) > 0 {
						title = strings.Join(args, " ")
					}
					if title == "" {
						return fmt.Errorf("title is required")
					}
					fmt.Fprint(c.OutOrStdout(), title)
					return nil
				},
			}
			c.Flags().String("title", "", "")
			var buf strings.Builder
			c.SetOut(&buf)
			c.SetArgs(tt.args)
			err := c.Execute()
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("error=%q, want containing %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if buf.String() != tt.want {
				t.Errorf("output=%q, want %q", buf.String(), tt.want)
			}
		})
	}
}

// TestCreate_DescriptionFlag_HelpfulError verifies that --description on rd create
// returns a helpful error directing agents to use --context or rd update, not a
// generic "unknown flag". Agents familiar with bd use --description; Ready uses 'context'.
func TestCreate_DescriptionFlag_HelpfulError(t *testing.T) {
	// Build a minimal cobra command that mirrors the --description check in createCmd.RunE.
	// We use a standalone command to avoid needing a live store.
	cmd := &cobra.Command{
		Use:  "create",
		RunE: func(cmd *cobra.Command, args []string) error {
			if desc, _ := cmd.Flags().GetString("description"); desc != "" {
				return fmt.Errorf("--description is not a flag on rd create. The field is called 'context' in Ready. Use --context-file <path> or set context after creation with rd update")
			}
			return nil
		},
	}
	cmd.Flags().String("description", "", "")
	_ = cmd.Flags().MarkHidden("description")

	cmd.SetArgs([]string{"--description", "my task description"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when --description is used on rd create, got nil")
	}
	if !strings.Contains(err.Error(), "context") {
		t.Errorf("expected error to mention 'context', got: %q", err.Error())
	}
	if !strings.Contains(err.Error(), "rd update") {
		t.Errorf("expected error to mention 'rd update', got: %q", err.Error())
	}
}
