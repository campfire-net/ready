package playbook_test

import (
	"encoding/json"
	"regexp"
	"testing"

	"github.com/third-division/ready/pkg/playbook"
)

var itemIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{2,63}$`)

// sampleItemsJSON is a valid template items JSON for testing.
var sampleItemsJSON = []byte(`[
  {
    "title": "Step 1: {{project}} setup",
    "type": "task",
    "level": "task",
    "priority": "p1",
    "context": "Set up the {{project}} scaffolding",
    "deps": []
  },
  {
    "title": "Step 2: {{project}} implementation",
    "type": "task",
    "level": "task",
    "priority": "p1",
    "context": "Implement the core feature for {{project}}",
    "deps": [0]
  }
]`)

func TestParse_Valid(t *testing.T) {
	tmpl, err := playbook.Parse("test-playbook", "Test Playbook", "A test playbook", sampleItemsJSON)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if tmpl.ID != "test-playbook" {
		t.Errorf("expected ID test-playbook, got %q", tmpl.ID)
	}
	if tmpl.Title != "Test Playbook" {
		t.Errorf("expected title 'Test Playbook', got %q", tmpl.Title)
	}
	if len(tmpl.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(tmpl.Items))
	}
	if tmpl.Items[0].Title != "Step 1: {{project}} setup" {
		t.Errorf("unexpected title: %q", tmpl.Items[0].Title)
	}
	if len(tmpl.Items[1].Deps) != 1 || tmpl.Items[1].Deps[0] != 0 {
		t.Errorf("unexpected deps on item[1]: %v", tmpl.Items[1].Deps)
	}
}

func TestParse_InvalidID(t *testing.T) {
	_, err := playbook.Parse("BAD_ID", "Title", "", sampleItemsJSON)
	if err == nil {
		t.Fatal("expected error for invalid ID")
	}
}

func TestParse_EmptyTitle(t *testing.T) {
	_, err := playbook.Parse("valid-id", "", "", sampleItemsJSON)
	if err == nil {
		t.Fatal("expected error for empty title")
	}
}

func TestParse_EmptyItems(t *testing.T) {
	_, err := playbook.Parse("valid-id", "Title", "", []byte(`[]`))
	if err == nil {
		t.Fatal("expected error for empty items")
	}
}

func TestParse_InvalidJSON(t *testing.T) {
	_, err := playbook.Parse("valid-id", "Title", "", []byte(`not json`))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestParse_InvalidType(t *testing.T) {
	bad := []byte(`[{"title":"T","type":"badtype","priority":"p1","deps":[]}]`)
	_, err := playbook.Parse("valid-id", "Title", "", bad)
	if err == nil {
		t.Fatal("expected error for invalid type")
	}
}

func TestParse_InvalidPriority(t *testing.T) {
	bad := []byte(`[{"title":"T","type":"task","priority":"high","deps":[]}]`)
	_, err := playbook.Parse("valid-id", "Title", "", bad)
	if err == nil {
		t.Fatal("expected error for invalid priority")
	}
}

func TestParse_OutOfRangeDep(t *testing.T) {
	bad := []byte(`[{"title":"T","type":"task","priority":"p1","deps":[5]}]`)
	_, err := playbook.Parse("valid-id", "Title", "", bad)
	if err == nil {
		t.Fatal("expected error for out-of-range dep index")
	}
}

func TestParse_SelfDep(t *testing.T) {
	bad := []byte(`[{"title":"T","type":"task","priority":"p1","deps":[0]}]`)
	_, err := playbook.Parse("valid-id", "Title", "", bad)
	if err == nil {
		t.Fatal("expected error for self-dependency")
	}
}

func TestValidate_CircularDep(t *testing.T) {
	// A→B→A cycle: item 0 deps on 1, item 1 deps on 0.
	circular := []byte(`[
		{"title":"A","type":"task","priority":"p1","deps":[1]},
		{"title":"B","type":"task","priority":"p1","deps":[0]}
	]`)
	_, err := playbook.Parse("circ-pb", "Circular", "", circular)
	if err == nil {
		t.Fatal("expected error for circular dependency")
	}
}

func TestValidate_ThreeNodeCycle(t *testing.T) {
	// A→B→C→A
	circular := []byte(`[
		{"title":"A","type":"task","priority":"p1","deps":[2]},
		{"title":"B","type":"task","priority":"p1","deps":[0]},
		{"title":"C","type":"task","priority":"p1","deps":[1]}
	]`)
	_, err := playbook.Parse("circ3-pb", "Circular3", "", circular)
	if err == nil {
		t.Fatal("expected error for 3-node circular dependency")
	}
}

func TestParseFull(t *testing.T) {
	full := map[string]interface{}{
		"id":    "full-pb",
		"title": "Full Playbook",
		"items": []map[string]interface{}{
			{"title": "Item 1", "type": "task", "priority": "p2", "deps": []int{}},
		},
	}
	data, _ := json.Marshal(full)
	tmpl, err := playbook.ParseFull(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tmpl.ID != "full-pb" {
		t.Errorf("expected ID full-pb, got %q", tmpl.ID)
	}
}

func TestExpand_VariableSubstitution(t *testing.T) {
	tmpl, err := playbook.Parse("sre-pb", "SRE Playbook", "", sampleItemsJSON)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	vars := map[string]string{"project": "myapp"}
	items, err := playbook.Expand(tmpl, "myapp", vars)
	if err != nil {
		t.Fatalf("expand failed: %v", err)
	}

	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if items[0].Title != "Step 1: myapp setup" {
		t.Errorf("unexpected title after substitution: %q", items[0].Title)
	}
	if items[0].Context != "Set up the myapp scaffolding" {
		t.Errorf("unexpected context after substitution: %q", items[0].Context)
	}
	if items[1].Title != "Step 2: myapp implementation" {
		t.Errorf("unexpected title after substitution: %q", items[1].Title)
	}
}

func TestExpand_IDGeneration(t *testing.T) {
	tmpl, err := playbook.Parse("id-pb", "ID Test", "", sampleItemsJSON)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	items, err := playbook.Expand(tmpl, "myproject", nil)
	if err != nil {
		t.Fatalf("expand failed: %v", err)
	}

	seen := map[string]bool{}
	for _, item := range items {
		if !itemIDPattern.MatchString(item.ID) {
			t.Errorf("item ID %q does not match required pattern", item.ID)
		}
		if seen[item.ID] {
			t.Errorf("duplicate item ID %q", item.ID)
		}
		seen[item.ID] = true
	}
}

func TestExpand_DepWiring(t *testing.T) {
	tmpl, err := playbook.Parse("dep-pb", "Dep Test", "", sampleItemsJSON)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	items, err := playbook.Expand(tmpl, "proj", nil)
	if err != nil {
		t.Fatalf("expand failed: %v", err)
	}

	// item[0] has no deps
	if len(items[0].Deps) != 0 {
		t.Errorf("item[0] expected no deps, got %v", items[0].Deps)
	}
	// item[1] should depend on item[0]'s ID
	if len(items[1].Deps) != 1 {
		t.Fatalf("item[1] expected 1 dep, got %d", len(items[1].Deps))
	}
	if items[1].Deps[0] != items[0].ID {
		t.Errorf("item[1] dep ID %q != item[0] ID %q", items[1].Deps[0], items[0].ID)
	}
}

func TestExpand_ItemCount(t *testing.T) {
	// 4 items, chain A→B→C→D
	chain := []byte(`[
		{"title":"A","type":"task","priority":"p1","deps":[]},
		{"title":"B","type":"task","priority":"p1","deps":[0]},
		{"title":"C","type":"task","priority":"p1","deps":[1]},
		{"title":"D","type":"task","priority":"p1","deps":[2]}
	]`)
	tmpl, err := playbook.Parse("chain-pb", "Chain", "", chain)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	items, err := playbook.Expand(tmpl, "chain", nil)
	if err != nil {
		t.Fatalf("expand failed: %v", err)
	}
	if len(items) != 4 {
		t.Errorf("expected 4 items, got %d", len(items))
	}
	// Verify chain
	if items[3].Deps[0] != items[2].ID {
		t.Errorf("chain dep mismatch: D.dep=%q, C.id=%q", items[3].Deps[0], items[2].ID)
	}
}

func TestExpand_UnknownVariableLeftAsIs(t *testing.T) {
	tmpl, err := playbook.Parse("var-pb", "Var Test", "", sampleItemsJSON)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	// No variables provided — {{project}} stays as-is.
	items, err := playbook.Expand(tmpl, "proj", nil)
	if err != nil {
		t.Fatalf("expand failed: %v", err)
	}
	if items[0].Title != "Step 1: {{project}} setup" {
		t.Errorf("expected unresolved placeholder, got %q", items[0].Title)
	}
}

func TestExpand_NoProject(t *testing.T) {
	tmpl, err := playbook.Parse("np-pb", "No Project", "", sampleItemsJSON)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	_, err = playbook.Expand(tmpl, "", nil)
	if err == nil {
		t.Fatal("expected error for empty project")
	}
}

func TestItemsJSON(t *testing.T) {
	tmpl, err := playbook.Parse("ij-pb", "Items JSON", "", sampleItemsJSON)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	data, err := tmpl.ItemsJSON()
	if err != nil {
		t.Fatalf("ItemsJSON failed: %v", err)
	}
	// Re-parse to verify round-trip.
	var items []playbook.TemplateItem
	if err := json.Unmarshal(data, &items); err != nil {
		t.Fatalf("re-parse failed: %v", err)
	}
	if len(items) != len(tmpl.Items) {
		t.Errorf("round-trip item count mismatch: %d != %d", len(items), len(tmpl.Items))
	}
}
