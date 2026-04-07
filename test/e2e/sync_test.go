package e2e_test

import (
	"encoding/json"
	"os"
	"testing"
)

// TestE2E_SyncStatus_OfflineMode verifies that rd sync status works in offline mode
// and reports 0 pending with no campfire configured.
func TestE2E_SyncStatus_OfflineMode(t *testing.T) {
	e := NewEnvOffline(t)

	_, _, code := e.Rd("init", "--offline", "--name", "sync-status-test")
	if code != 0 {
		t.Fatalf("rd init --offline failed")
	}

	// rd sync status should succeed and report offline mode.
	stdout, stderr, code := e.Rd("sync", "status")
	if code != 0 {
		t.Fatalf("rd sync status failed (exit %d):\nstderr: %s\nstdout: %s", code, stderr, stdout)
	}
	if !contains(stdout, "offline") {
		t.Errorf("sync status offline mode output should mention offline, got: %q", stdout)
	}
}

// TestE2E_SyncStatus_JSON_OfflineMode verifies rd sync status --json output in offline mode.
func TestE2E_SyncStatus_JSON_OfflineMode(t *testing.T) {
	e := NewEnvOffline(t)

	_, _, code := e.Rd("init", "--offline", "--name", "sync-status-json-test")
	if code != 0 {
		t.Fatalf("rd init --offline failed")
	}

	stdout, stderr, code := e.Rd("sync", "status", "--json")
	if code != 0 {
		t.Fatalf("rd sync status --json failed (exit %d):\nstderr: %s\nstdout: %s", code, stderr, stdout)
	}

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("parse sync status JSON: %v\noutput: %s", err, stdout)
	}

	// pending_count should be 0.
	if v, ok := result["pending_count"]; !ok {
		t.Error("sync status JSON missing pending_count")
	} else if v.(float64) != 0 {
		t.Errorf("pending_count: got %v, want 0", v)
	}

	// has_synced should be false.
	if v, ok := result["has_synced"]; !ok {
		t.Error("sync status JSON missing has_synced")
	} else if v.(bool) {
		t.Error("has_synced should be false in a fresh offline project")
	}

	// campfire_configured should be false.
	if v, ok := result["campfire_configured"]; !ok {
		t.Error("sync status JSON missing campfire_configured")
	} else if v.(bool) {
		t.Error("campfire_configured should be false in offline mode")
	}
}

// TestE2E_SyncStatus_PendingCountAfterBuffering verifies that rd sync status
// reports the correct pending count when pending.jsonl contains buffered records.
//
// We create a campfire-backed project, sabotage the transport so campfire sends
// fail, then create two items — both buffer to pending.jsonl via the real
// bufferToPending path. This tests the actual write path, not a hand-crafted fixture.
func TestE2E_SyncStatus_PendingCountAfterBuffering(t *testing.T) {
	e := NewEnv(t) // campfire-backed — we need a real campfire for sends to fail

	// Create one item successfully to establish a baseline.
	var firstItem Item
	if err := e.RdJSON(&firstItem, "create",
		"--title", "item before transport failure",
		"--priority", "p1",
		"--type", "task",
	); err != nil {
		t.Fatalf("create first item: %v", err)
	}

	// Sabotage the campfire transport by removing the transport directory.
	// e.TransportDir is the authoritative path returned by cf create --json.
	campfireTransportDir := e.TransportDir
	if _, err := os.Stat(campfireTransportDir); err != nil {
		t.Fatalf("cannot locate campfire transport dir at %s: %v", campfireTransportDir, err)
	}
	if err := os.RemoveAll(campfireTransportDir); err != nil {
		t.Fatalf("removing transport dir: %v", err)
	}

	// Create two items — campfire sends fail, so both buffer to pending.jsonl
	// via the real bufferToPending call in sendToProjectCampfire.
	for i, title := range []string{"buffered item one", "buffered item two"} {
		stdout, stderr, code := e.Rd("create",
			"--title", title,
			"--priority", "p2",
			"--type", "task",
			"--json",
		)
		// Command succeeds (JSONL write is phase 1 — campfire failure is non-fatal).
		if code != 0 {
			t.Fatalf("create buffered item %d failed (exit %d):\nstderr: %s\nstdout: %s", i+1, code, stderr, stdout)
		}
	}

	// rd sync status --json should report pending_count = 2.
	stdout, stderr, code := e.Rd("sync", "status", "--json")
	if code != 0 {
		t.Fatalf("rd sync status failed (exit %d):\nstderr: %s\nstdout: %s", code, stderr, stdout)
	}

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("parse sync status JSON: %v\noutput: %s", err, stdout)
	}

	if v, ok := result["pending_count"]; !ok {
		t.Error("sync status JSON missing pending_count")
	} else if v.(float64) != 2 {
		t.Errorf("pending_count: got %v, want 2", v)
	}
}
