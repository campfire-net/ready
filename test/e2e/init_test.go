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

	projectDir := t.TempDir()
	stdout, stderr, code := e.RdInDir(projectDir, "init", "--name", "testproj", "--json")
	if code != 0 {
		t.Fatalf("rd init failed (exit %d):\nstderr: %s\nstdout: %s", code, stderr, stdout)
	}

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("JSON parse failed: %v\noutput: %s", err, stdout)
	}

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

// TestE2E_Init_ReportsNoHome verifies init reports when no home campfire exists.
func TestE2E_Init_ReportsNoHome(t *testing.T) {
	e := NewEnv(t)

	// Remove auto-discovered home alias so this env has no home campfire.
	os.Remove(filepath.Join(e.CFHome, "aliases.json"))

	projectDir := t.TempDir()
	stdout, stderr, code := e.RdInDir(projectDir, "init", "--name", "standalone", "--json")
	if code != 0 {
		t.Fatalf("rd init failed (exit %d):\nstderr: %s", code, stderr)
	}

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("JSON parse: %v\noutput: %s", err, stdout)
	}
	if result["has_home"] != false {
		t.Errorf("has_home: got %v, want false (no home alias set in test env)", result["has_home"])
	}
}

// TestE2E_Init_ThenCreateItem verifies the full flow: init → create → show.
func TestE2E_Init_ThenCreateItem(t *testing.T) {
	e := NewEnv(t)

	projectDir := t.TempDir()
	_, stderr, code := e.RdInDir(projectDir, "init", "--name", "testproj")
	if code != 0 {
		t.Fatalf("rd init failed (exit %d): %s", code, stderr)
	}

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

// TestE2E_Register_NoFlags_NoHome verifies rd register with no flags
// succeeds gracefully when no home exists.
func TestE2E_Register_NoFlags_NoHome(t *testing.T) {
	e := NewEnv(t)

	// Remove auto-discovered home alias so this env has no home campfire.
	os.Remove(filepath.Join(e.CFHome, "aliases.json"))

	projectDir := t.TempDir()
	_, stderr, code := e.RdInDir(projectDir, "init", "--name", "standalone")
	if code != 0 {
		t.Fatalf("rd init failed (exit %d): %s", code, stderr)
	}

	// Register with no flags — should succeed with guidance, not error.
	stdout, _, code := e.RdInDir(projectDir, "register", "--json")
	if code != 0 {
		t.Fatalf("rd register (no flags) should succeed, got exit %d", code)
	}

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("JSON parse: %v\noutput: %s", err, stdout)
	}
	if result["registered"] != false {
		t.Errorf("registered: got %v, want false (no home found)", result["registered"])
	}
}

// TestE2E_Register_WithOrg_CreatesHome verifies --org creates home + ready.
func TestE2E_Register_WithOrg_CreatesHome(t *testing.T) {
	e := NewEnv(t)

	projectDir := t.TempDir()
	_, stderr, code := e.RdInDir(projectDir, "init", "--name", "myapp")
	if code != 0 {
		t.Fatalf("rd init failed (exit %d): %s", code, stderr)
	}

	stdout, stderr, code := e.RdInDir(projectDir, "register", "--org", "testorg", "--name", "myapp", "--json")
	if code != 0 {
		t.Fatalf("rd register failed (exit %d):\nstderr: %s\nstdout: %s", code, stderr, stdout)
	}

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("JSON parse: %v\noutput: %s", err, stdout)
	}

	if result["created_home"] != true {
		t.Errorf("created_home: got %v, want true", result["created_home"])
	}
	if result["created_ready"] != true {
		t.Errorf("created_ready: got %v, want true", result["created_ready"])
	}
	if result["registered"] != true {
		t.Errorf("registered: got %v, want true", result["registered"])
	}
	if result["namespace"] != "cf://testorg.ready.myapp" {
		t.Errorf("namespace: got %v, want cf://testorg.ready.myapp", result["namespace"])
	}

	// Verify all three campfire IDs are distinct.
	projectID := result["campfire_id"].(string)
	homeID := result["home_campfire_id"].(string)
	readyID := result["ready_campfire_id"].(string)
	if projectID == homeID || projectID == readyID || homeID == readyID {
		t.Errorf("expected 3 distinct campfire IDs, got project=%s home=%s ready=%s",
			projectID[:12], homeID[:12], readyID[:12])
	}
}

// TestE2E_Register_SecondProject_DiscoverExisting verifies a second project
// discovers the existing home (via config) without --org.
func TestE2E_Register_SecondProject_DiscoverExisting(t *testing.T) {
	e := NewEnv(t)

	// First project: init + register with --org (creates home + ready).
	proj1 := t.TempDir()
	e.RdInDir(proj1, "init", "--name", "proj1")
	stdout1, stderr, code := e.RdInDir(proj1, "register", "--org", "testorg", "--name", "proj1", "--json")
	if code != 0 {
		t.Fatalf("register proj1 failed (exit %d): %s", code, stderr)
	}
	var res1 map[string]interface{}
	json.Unmarshal([]byte(stdout1), &res1)

	// Second project: init + register WITHOUT --org — should discover home.
	proj2 := t.TempDir()
	e.RdInDir(proj2, "init", "--name", "proj2")
	stdout2, stderr, code := e.RdInDir(proj2, "register", "--name", "proj2", "--json")
	if code != 0 {
		t.Fatalf("register proj2 failed (exit %d): %s", code, stderr)
	}
	var res2 map[string]interface{}
	json.Unmarshal([]byte(stdout2), &res2)

	if res2["registered"] != true {
		t.Error("second register should discover home and register")
	}
	if res2["created_home"] != false {
		t.Error("second register should reuse home, not create a new one")
	}
	if res2["created_ready"] != false {
		t.Error("second register should reuse ready namespace")
	}
	if res1["home_campfire_id"] != res2["home_campfire_id"] {
		t.Error("home campfire IDs should match across registrations")
	}
}
