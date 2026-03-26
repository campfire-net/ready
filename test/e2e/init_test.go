package e2e_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestE2E_Init_CreatesProjectCampfire verifies rd init creates a campfire,
// writes .campfire/root, and posts convention declarations.
func TestE2E_Init_CreatesProjectCampfire(t *testing.T) {
	e := NewEnv(t)

	// rd init is run in a fresh project directory (no existing .campfire/root).
	// We need a directory WITHOUT .campfire/root — use a new temp dir.
	projectDir := t.TempDir()

	stdout, stderr, code := e.RdInDir(projectDir, "init", "--name", "testproj", "--json")
	if code != 0 {
		t.Fatalf("rd init failed (exit %d):\nstderr: %s\nstdout: %s", code, stderr, stdout)
	}

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("JSON parse failed: %v\noutput: %s", err, stdout)
	}

	// Check output fields.
	if result["campfire_id"] == nil || result["campfire_id"] == "" {
		t.Error("campfire_id should be non-empty")
	}
	if result["name"] != "testproj" {
		t.Errorf("name: got %v, want testproj", result["name"])
	}
	decls, ok := result["declarations"].(float64)
	if !ok || decls < 10 {
		t.Errorf("declarations: got %v, want >= 10", result["declarations"])
	}

	// Verify .campfire/root was written.
	rootFile := filepath.Join(projectDir, ".campfire", "root")
	data, err := os.ReadFile(rootFile)
	if err != nil {
		t.Fatalf("reading .campfire/root: %v", err)
	}
	rootID := strings.TrimSpace(string(data))
	if len(rootID) != 64 {
		t.Errorf(".campfire/root has wrong length %d: %q", len(rootID), rootID)
	}
	if rootID != result["campfire_id"] {
		t.Errorf(".campfire/root (%s) != JSON campfire_id (%s)", rootID, result["campfire_id"])
	}
}

// TestE2E_Init_FailsIfAlreadyInitialized verifies rd init rejects double-init.
func TestE2E_Init_FailsIfAlreadyInitialized(t *testing.T) {
	e := NewEnv(t)

	// The Env already has .campfire/root from NewEnv.
	_, stderr, code := e.Rd("init", "--name", "test")
	if code == 0 {
		t.Fatal("expected rd init to fail when .campfire/root already exists")
	}
	if !strings.Contains(stderr, "already") {
		t.Errorf("expected 'already' in error, got: %q", stderr)
	}
}

// TestE2E_Init_DefaultsNameFromDirectory verifies name is inferred from cwd.
func TestE2E_Init_DefaultsNameFromDirectory(t *testing.T) {
	e := NewEnv(t)

	// Create a named directory.
	projectDir := filepath.Join(t.TempDir(), "my-project")
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatal(err)
	}

	stdout, stderr, code := e.RdInDir(projectDir, "init", "--json")
	if code != 0 {
		t.Fatalf("rd init failed (exit %d):\nstderr: %s", code, stderr)
	}

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("JSON parse: %v\noutput: %s", err, stdout)
	}
	if result["name"] != "my-project" {
		t.Errorf("name: got %v, want my-project", result["name"])
	}
}

// TestE2E_Init_WithOrg verifies that --org records the namespace.
func TestE2E_Init_WithOrg(t *testing.T) {
	e := NewEnv(t)

	projectDir := t.TempDir()
	stdout, stderr, code := e.RdInDir(projectDir, "init", "--name", "platform", "--org", "acme", "--json")
	if code != 0 {
		t.Fatalf("rd init --org failed (exit %d):\nstderr: %s", code, stderr)
	}

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("JSON parse: %v\noutput: %s", err, stdout)
	}
	if result["namespace"] != "cf://acme.ready.platform" {
		t.Errorf("namespace: got %v, want cf://acme.ready.platform", result["namespace"])
	}
}

// TestE2E_Init_ThenCreateItem verifies the full flow: init → create → show.
func TestE2E_Init_ThenCreateItem(t *testing.T) {
	e := NewEnv(t)

	projectDir := t.TempDir()

	// Init the project.
	_, stderr, code := e.RdInDir(projectDir, "init", "--name", "testproj")
	if code != 0 {
		t.Fatalf("rd init failed (exit %d): %s", code, stderr)
	}

	// Create a work item in the new campfire.
	var item Item
	stdout, stderr, code := e.RdInDir(projectDir, "create",
		"--title", "First item",
		"--priority", "p1",
		"--type", "task",
		"--json")
	if code != 0 {
		t.Fatalf("rd create failed (exit %d): %s", code, stderr)
	}
	if err := json.Unmarshal([]byte(stdout), &item); err != nil {
		t.Fatalf("JSON parse: %v", err)
	}
	if item.ID == "" {
		t.Error("created item has empty ID")
	}
}
