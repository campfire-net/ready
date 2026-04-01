package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestRequireClient_SucceedsWithTempConfigDir verifies that requireClient()
// succeeds when pointed at a fresh temp directory: Init generates a new
// Ed25519 identity, opens a SQLite store, and returns a non-nil *Client
// with a non-empty public key.
//
// The test uses rdHome (the --cf-home flag variable) to override CFHome()
// so the real ~/.campfire directory is never touched.
func TestRequireClient_SucceedsWithTempConfigDir(t *testing.T) {
	dir := t.TempDir()

	// Override rdHome so CFHome() returns our temp dir.
	origRDHome := rdHome
	rdHome = dir
	t.Cleanup(func() { rdHome = origRDHome })

	// Reset the package-level cache so prior tests don't bleed in.
	origClient := protocolClient
	protocolClient = nil
	t.Cleanup(func() {
		if protocolClient != nil {
			protocolClient.Close()
		}
		protocolClient = origClient
	})

	c, err := requireClient()
	if err != nil {
		t.Fatalf("requireClient() error: %v", err)
	}
	if c == nil {
		t.Fatal("requireClient() returned nil client")
	}

	pubKey := c.PublicKeyHex()
	if pubKey == "" {
		t.Error("PublicKeyHex() returned empty string — identity not loaded")
	}

	// identity.json must exist in the temp dir.
	idPath := filepath.Join(dir, "identity.json")
	if _, err := os.Stat(idPath); err != nil {
		t.Errorf("identity.json not created at %q: %v", idPath, err)
	}

	// store.db must exist in the temp dir.
	storePath := filepath.Join(dir, "store.db")
	if _, err := os.Stat(storePath); err != nil {
		t.Errorf("store.db not created at %q: %v", storePath, err)
	}
}

// TestRequireClient_CachesClient verifies that requireClient() returns the
// same *Client on repeated calls without reinitializing.
func TestRequireClient_CachesClient(t *testing.T) {
	dir := t.TempDir()

	origRDHome := rdHome
	rdHome = dir
	t.Cleanup(func() { rdHome = origRDHome })

	origClient := protocolClient
	protocolClient = nil
	t.Cleanup(func() {
		if protocolClient != nil {
			protocolClient.Close()
		}
		protocolClient = origClient
	})

	c1, err := requireClient()
	if err != nil {
		t.Fatalf("first requireClient() error: %v", err)
	}

	c2, err := requireClient()
	if err != nil {
		t.Fatalf("second requireClient() error: %v", err)
	}

	// Must be the exact same pointer — no re-initialization.
	if c1 != c2 {
		t.Error("requireClient() returned different *Client on second call — caching broken")
	}
}

// TestRequireClient_IdentityMatchesRequireAgentAndStore verifies that the
// public key returned by the protocol.Client matches the identity loaded by
// the legacy requireAgentAndStore() path, confirming both refer to the same
// keypair.
func TestRequireClient_IdentityMatchesRequireAgentAndStore(t *testing.T) {
	dir := t.TempDir()

	origRDHome := rdHome
	rdHome = dir
	t.Cleanup(func() { rdHome = origRDHome })

	origClient := protocolClient
	protocolClient = nil
	t.Cleanup(func() {
		if protocolClient != nil {
			protocolClient.Close()
		}
		protocolClient = origClient
	})

	// First: initialize via requireClient() so the identity file is created.
	c, err := requireClient()
	if err != nil {
		t.Fatalf("requireClient() error: %v", err)
	}
	clientPubKey := c.PublicKeyHex()

	// Now load the same identity via the legacy path.
	agentID, s, err := requireAgentAndStore()
	if err != nil {
		t.Fatalf("requireAgentAndStore() error: %v", err)
	}
	defer s.Close()

	legacyPubKey := agentID.PublicKeyHex()

	if clientPubKey != legacyPubKey {
		t.Errorf("protocol.Client public key %q != legacy identity public key %q — different keypairs", clientPubKey, legacyPubKey)
	}
}
