package main

// convention_server_test.go — regression tests for requireConventionServer().
//
// Regression test for ready-3ce: Non-owner joiners must not see the
// "could not start in-process convention server" warning when their client
// is not a member of the inbox campfire configured in the project's sync config.

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/campfire-net/ready/pkg/rdconfig"
)

// TestRequireConventionServer_NonMemberInboxSuppressesWarning is the regression
// test for ready-3ce. When a non-owner identity runs any rd command against a
// project that has an InboxCampfireID configured, and the current identity is
// NOT a member of that inbox campfire, requireConventionServer() must NOT print
// the "could not start in-process convention server" warning to stderr.
//
// Scenario:
//   - Owner creates project with inbox campfire ID in sync config.
//   - Member (non-owner) runs "rd ready" — their client has no membership for
//     the inbox campfire.
//   - Expected: no warning on stderr about inbox campfire membership.
func TestRequireConventionServer_NonMemberInboxSuppressesWarning(t *testing.T) {
	// Set up temp cf home for the non-owner client.
	cfHome := t.TempDir()

	origRDHome := rdHome
	rdHome = cfHome
	t.Cleanup(func() { rdHome = origRDHome })

	origClient := protocolClient
	protocolClient = nil
	t.Cleanup(func() {
		if protocolClient != nil {
			protocolClient.Close()
		}
		protocolClient = origClient
	})

	// Initialize the non-owner client (creates identity + store in cfHome).
	client, err := requireClient()
	if err != nil {
		t.Fatalf("requireClient(): %v", err)
	}

	// Create a project directory with .campfire/root and .ready/config.json.
	projectDir := t.TempDir()
	campfireRootDir := filepath.Join(projectDir, ".campfire")
	if err := os.MkdirAll(campfireRootDir, 0755); err != nil {
		t.Fatalf("creating .campfire dir: %v", err)
	}

	// Fake project campfire ID (64 hex chars).
	fakeProjectCampfireID := strings.Repeat("ab", 32)
	if err := os.WriteFile(filepath.Join(campfireRootDir, "root"), []byte(fakeProjectCampfireID), 0600); err != nil {
		t.Fatalf("writing .campfire/root: %v", err)
	}

	// Fake inbox campfire ID — the non-owner client is NOT a member of this.
	fakeInboxID := strings.Repeat("cd", 32)

	// Write sync config with inbox campfire ID set.
	syncCfg := rdconfig.SyncConfig{
		CampfireID:      fakeProjectCampfireID,
		InboxCampfireID: fakeInboxID,
	}
	syncData, err := json.MarshalIndent(syncCfg, "", "  ")
	if err != nil {
		t.Fatalf("marshalling sync config: %v", err)
	}
	readyDir := filepath.Join(projectDir, ".ready")
	if err := os.MkdirAll(readyDir, 0700); err != nil {
		t.Fatalf("creating .ready dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(readyDir, "config.json"), syncData, 0600); err != nil {
		t.Fatalf("writing sync config: %v", err)
	}

	// Change working directory to the project dir so projectRoot() finds it.
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(projectDir); err != nil {
		t.Fatalf("Chdir to project dir: %v", err)
	}
	t.Cleanup(func() { os.Chdir(origWd) })

	// Redirect stderr to a buffer so we can inspect it.
	origStderr := os.Stderr
	r, w, pipeErr := os.Pipe()
	if pipeErr != nil {
		t.Fatalf("os.Pipe(): %v", pipeErr)
	}
	os.Stderr = w
	t.Cleanup(func() { os.Stderr = origStderr })

	// Call requireConventionServer. The non-owner client has no membership for
	// fakeInboxID, so GetMembership returns nil — the inbox option must be
	// silently skipped, producing no warning.
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately so the server doesn't actually start polling.
	requireConventionServer(ctx, client)

	// Flush and read stderr.
	w.Close()
	var buf bytes.Buffer
	io.Copy(&buf, r)
	os.Stderr = origStderr

	stderrOutput := buf.String()
	if strings.Contains(stderrOutput, "could not start in-process convention server") {
		t.Errorf("requireConventionServer printed warning for non-member inbox campfire:\n%s", stderrOutput)
	}
}
