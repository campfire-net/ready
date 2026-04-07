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

// TestRequireClient_PicksUpCFHomeEnv verifies that requireClient() resolves
// the config dir from CF_HOME environment variable, not from the rdHome flag.
// This test directly verifies that InitWithConfig(WithConfigDir(CFHome())) +
// CFHome()'s env resolution works without relying on the flag.
//
// The test does NOT set rdHome. It sets CF_HOME to a temp directory, then
// calls requireClient(). This proves WithConfigDir(CFHome()) threads the env-resolved
// path into the protocol client initialization.
func TestRequireClient_PicksUpCFHomeEnv(t *testing.T) {
	dir := t.TempDir()

	// Clear the flag so CFHome() must resolve via env.
	origRDHome := rdHome
	rdHome = ""
	t.Cleanup(func() { rdHome = origRDHome })

	// Set CF_HOME to the temp directory.
	oldCFHome := os.Getenv("CF_HOME")
	os.Setenv("CF_HOME", dir)
	t.Cleanup(func() {
		if oldCFHome != "" {
			os.Setenv("CF_HOME", oldCFHome)
		} else {
			os.Unsetenv("CF_HOME")
		}
	})

	// Reset the client cache.
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

	// Verify the identity and store files were created in the CF_HOME directory.
	idPath := filepath.Join(dir, "identity.json")
	if _, err := os.Stat(idPath); err != nil {
		t.Errorf("identity.json not created at CF_HOME %q: %v", dir, err)
	}

	storePath := filepath.Join(dir, "store.db")
	if _, err := os.Stat(storePath); err != nil {
		t.Errorf("store.db not created at CF_HOME %q: %v", dir, err)
	}

	// Verify the client has a valid public key.
	pubKey := c.PublicKeyHex()
	if pubKey == "" {
		t.Error("PublicKeyHex() returned empty string — identity not loaded")
	}
}

// TestRequireClient_PicksUpConfigDirViaFilesystemDetection verifies that
// requireClient() resolves the config dir via filesystem walk-up (HOME env
// manipulation, no rdHome flag, no CF_HOME env).
//
// Sets HOME to a temp directory with a .cf/ subdirectory present (no identity.json yet),
// clears both rdHome and CF_HOME, then calls requireClient(). This proves CFHome()
// can find .cf/ via walk-up and WithConfigDir(CFHome()) threads it to InitWithConfig().
// The test also implicitly verifies that WithWalkUp() is present in the InitWithConfig()
// call (walk-up behavior is available when initialized).
//
// TODO: WithWalkUp() should be tested more thoroughly in ready-589's e2e test,
// which will have a real campfire hierarchy to verify walk-up against.
func TestRequireClient_PicksUpConfigDirViaFilesystemDetection(t *testing.T) {
	// Create a temporary HOME with .cf/ subdirectory.
	tmpHome := t.TempDir()
	cfDir := filepath.Join(tmpHome, ".cf")
	if err := os.MkdirAll(cfDir, 0755); err != nil {
		t.Fatalf("failed to create .cf directory: %v", err)
	}

	// Clear the flag and env so CFHome() must walk up the filesystem.
	origRDHome := rdHome
	rdHome = ""
	t.Cleanup(func() { rdHome = origRDHome })

	oldCFHome := os.Getenv("CF_HOME")
	os.Unsetenv("CF_HOME")
	t.Cleanup(func() {
		if oldCFHome != "" {
			os.Setenv("CF_HOME", oldCFHome)
		}
	})

	// Replace HOME so os.UserHomeDir() returns our temp directory.
	oldHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpHome)
	t.Cleanup(func() {
		if oldHome != "" {
			os.Setenv("HOME", oldHome)
		} else {
			os.Unsetenv("HOME")
		}
	})

	// Reset the client cache.
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

	// Verify the identity and store files were created in the detected .cf directory.
	idPath := filepath.Join(cfDir, "identity.json")
	if _, err := os.Stat(idPath); err != nil {
		t.Errorf("identity.json not created at detected .cf dir %q: %v", cfDir, err)
	}

	storePath := filepath.Join(cfDir, "store.db")
	if _, err := os.Stat(storePath); err != nil {
		t.Errorf("store.db not created at detected .cf dir %q: %v", cfDir, err)
	}

	// Verify the client has a valid public key.
	pubKey := c.PublicKeyHex()
	if pubKey == "" {
		t.Error("PublicKeyHex() returned empty string — identity not loaded")
	}
}
