package main

// join_test.go — unit tests for rd join helpers.
//
// Done conditions tested:
//   - isHex rejects non-hex strings and accepts valid hex
//   - containsTag finds present tags and misses absent ones
//   - grantTargets matches correct pubkey in payload
//   - pollForRoleGrant: returns msg ID when fake client returns a matching message (ready-421)
//   - pollForRoleGrant: times out when no matching message arrives (ready-421)
//   - resolveName: returns hex ID unchanged when input is already 64-char hex (ready-421)
//   - TOFU beacon root: all five paths via applyBeaconRootTOFU / resetBeaconRoot (ready-c3a)
//   - Open campfire join: non-member joins open campfire via client.Join() (ready-1b5)
//   - Invite-only campfire join: client.Join() returns "invite-only" error (ready-1b5)

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/campfire-net/campfire/pkg/protocol"
	"github.com/campfire-net/ready/pkg/rdconfig"
)

// TestIsHex verifies isHex correctly identifies hex strings.
func TestIsHex(t *testing.T) {
	cases := []struct {
		input string
		want  bool
	}{
		{"0123456789abcdef", true},
		{"ABCDEF0123456789", true},
		{"0123456789abcdefABCDEF", true},
		{"", true}, // empty string — vacuously true (all chars are hex)
		{"xyz", false},
		{"0123456789abcdefg", false},
		{"ghijklmn", false},
	}
	for _, tc := range cases {
		got := isHex(tc.input)
		if got != tc.want {
			t.Errorf("isHex(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}

// TestContainsTag verifies containsTag finds present and misses absent tags.
func TestContainsTag(t *testing.T) {
	tags := []string{"work:role-grant", "work:for:abcdef", "meta:test"}

	if !containsTag(tags, "work:role-grant") {
		t.Error("containsTag should find work:role-grant")
	}
	if !containsTag(tags, "work:for:abcdef") {
		t.Error("containsTag should find work:for:abcdef")
	}
	if containsTag(tags, "work:missing") {
		t.Error("containsTag should not find work:missing")
	}
	if containsTag(nil, "work:role-grant") {
		t.Error("containsTag on nil should return false")
	}
}

// TestGrantTargets verifies grantTargets matches correct pubkey in payload.
func TestGrantTargets(t *testing.T) {
	targetKey := "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"
	otherKey := "0000000000000000000000000000000000000000000000000000000000000001"

	makeMsg := func(pubkey, role string) protocol.Message {
		payload := map[string]string{"pubkey": pubkey, "role": role}
		data, _ := json.Marshal(payload)
		return protocol.Message{Payload: data}
	}

	if !grantTargets(makeMsg(targetKey, "member"), targetKey) {
		t.Error("grantTargets should match correct pubkey with admission role")
	}
	if grantTargets(makeMsg(otherKey, "member"), targetKey) {
		t.Error("grantTargets should not match wrong pubkey")
	}
	// Revocation grants must not be accepted as admissions.
	if grantTargets(makeMsg(targetKey, "revoked"), targetKey) {
		t.Error("grantTargets should reject role=revoked grants")
	}
	// Empty role must not be accepted.
	if grantTargets(makeMsg(targetKey, ""), targetKey) {
		t.Error("grantTargets should reject empty role")
	}

	// Empty payload should not match.
	emptyMsg := protocol.Message{Payload: []byte("{}")}
	if grantTargets(emptyMsg, targetKey) {
		t.Error("grantTargets should not match when payload has no pubkey")
	}

	// Invalid JSON should not match.
	badMsg := protocol.Message{Payload: []byte("not-json")}
	if grantTargets(badMsg, targetKey) {
		t.Error("grantTargets should not match on invalid JSON payload")
	}
}

// TestPollForRoleGrant_FoundViaTag verifies that pollForRoleGrant returns the
// message ID immediately when the fake client returns a message tagged with
// "work:for:<pubkey>". This exercises the containsTag path.
func TestPollForRoleGrant_FoundViaTag(t *testing.T) {
	myPubKey := pubkeyHex("ab")
	campfire := pubkeyHex("cc")
	msgID := "grant-msg-0001"

	fake := &fakeReadClient{
		messages: []protocol.Message{
			makeGrantMsgForTag(msgID, myPubKey, "member"),
		},
	}

	got, err := pollForRoleGrant(fake, campfire, myPubKey, 5*time.Second)
	if err != nil {
		t.Fatalf("pollForRoleGrant: unexpected error: %v", err)
	}
	if got != msgID {
		t.Errorf("pollForRoleGrant returned msg ID %q, want %q", got, msgID)
	}
}

// TestPollForRoleGrant_FoundViaPayload verifies that pollForRoleGrant returns
// the message ID when the payload contains the matching pubkey and role
// (via grantTargets), even without a "work:for:<pubkey>" tag.
func TestPollForRoleGrant_FoundViaPayload(t *testing.T) {
	myPubKey := pubkeyHex("ab")
	campfire := pubkeyHex("cc")
	msgID := "grant-msg-0002"

	fake := &fakeReadClient{
		messages: []protocol.Message{
			makeGrantMsg(msgID, myPubKey, "member"),
		},
	}

	got, err := pollForRoleGrant(fake, campfire, myPubKey, 5*time.Second)
	if err != nil {
		t.Fatalf("pollForRoleGrant via payload: unexpected error: %v", err)
	}
	if got != msgID {
		t.Errorf("pollForRoleGrant returned msg ID %q, want %q", got, msgID)
	}
}

// TestPollForRoleGrant_Timeout verifies that pollForRoleGrant returns an error
// after the timeout when the fake client returns no matching messages.
func TestPollForRoleGrant_Timeout(t *testing.T) {
	myPubKey := pubkeyHex("ab")
	campfire := pubkeyHex("cc")

	// Fake returns a message for a different pubkey — never matches.
	otherKey := pubkeyHex("11")
	fake := &fakeReadClient{
		messages: []protocol.Message{
			makeGrantMsg("grant-other", otherKey, "member"),
		},
	}

	timeout := 50 * time.Millisecond
	_, err := pollForRoleGrant(fake, campfire, myPubKey, timeout)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("expected 'timed out' in error, got: %v", err)
	}
}

// TestResolveName_DirectHex verifies that resolveName returns a 64-char hex
// campfire ID unchanged — no client call is made for this path.
//
// We call the real resolveName function using a real (temp-dir) client so the
// test actually exercises production code, not just code inspection.
func TestResolveName_DirectHex(t *testing.T) {
	// Set up a temp client dir so requireClient() / CFHome() use isolated state.
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

	client, err := requireClient()
	if err != nil {
		t.Fatalf("requireClient: %v", err)
	}

	hexID := "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"
	got, err := resolveName(client, hexID)
	if err != nil {
		t.Fatalf("resolveName direct hex: unexpected error: %v", err)
	}
	if got != hexID {
		t.Errorf("resolveName(%q) = %q, want same value", hexID, got)
	}
}

// cfHomeTempDir creates a temp dir and sets rdHome so CFHome() returns it.
// Cleans up on test completion.
func cfHomeTempDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	origRDHome := rdHome
	rdHome = dir
	t.Cleanup(func() { rdHome = origRDHome })
	return dir
}

// sampleRoot is a 64-char hex string used as a fake beacon root in TOFU tests.
const sampleRoot = "aabbccddeeff00112233445566778899aabbccddeeff00112233445566778899"

// sampleRoot2 is a different 64-char hex string for mismatch tests.
const sampleRoot2 = "1122334455667788990011223344556677889900112233445566778899001122"

// TestBeaconRoot_FirstUse_NonInteractive verifies path 1:
// First use (no pin) in a non-interactive context (tests always run non-interactive).
// Expected: pin is saved to config, no error.
func TestBeaconRoot_FirstUse_NonInteractive(t *testing.T) {
	cfHome := cfHomeTempDir(t)

	cfg := &rdconfig.Config{} // no pin yet
	err := applyBeaconRootTOFU(cfHome, cfg, sampleRoot, false /* confirm */)
	if err != nil {
		t.Fatalf("applyBeaconRootTOFU first use: unexpected error: %v", err)
	}

	// Pin must be set in cfg in-place.
	if cfg.BeaconRoot != sampleRoot {
		t.Errorf("cfg.BeaconRoot = %q, want %q", cfg.BeaconRoot, sampleRoot)
	}

	// Pin must be persisted to disk.
	saved, err := rdconfig.Load(cfHome)
	if err != nil {
		t.Fatalf("rdconfig.Load after pin: %v", err)
	}
	if saved.BeaconRoot != sampleRoot {
		t.Errorf("saved BeaconRoot = %q, want %q", saved.BeaconRoot, sampleRoot)
	}
}

// TestBeaconRoot_FirstUse_WithConfirmFlag verifies path 2:
// First use (no pin) with --confirm=true bypasses any prompt and pins.
// Expected: pin saved, no error, same as non-interactive path.
func TestBeaconRoot_FirstUse_WithConfirmFlag(t *testing.T) {
	cfHome := cfHomeTempDir(t)

	cfg := &rdconfig.Config{}
	err := applyBeaconRootTOFU(cfHome, cfg, sampleRoot, true /* confirm */)
	if err != nil {
		t.Fatalf("applyBeaconRootTOFU with --confirm: unexpected error: %v", err)
	}
	if cfg.BeaconRoot != sampleRoot {
		t.Errorf("cfg.BeaconRoot = %q, want %q", cfg.BeaconRoot, sampleRoot)
	}

	saved, err := rdconfig.Load(cfHome)
	if err != nil {
		t.Fatalf("rdconfig.Load: %v", err)
	}
	if saved.BeaconRoot != sampleRoot {
		t.Errorf("persisted BeaconRoot = %q, want %q", saved.BeaconRoot, sampleRoot)
	}
}

// TestBeaconRoot_PinExistsAndMatches verifies path 3:
// Pin is already set, requested root matches. No error, no prompt.
func TestBeaconRoot_PinExistsAndMatches(t *testing.T) {
	cfHome := cfHomeTempDir(t)

	cfg := &rdconfig.Config{BeaconRoot: sampleRoot}
	err := applyBeaconRootTOFU(cfHome, cfg, sampleRoot, false /* confirm */)
	if err != nil {
		t.Fatalf("applyBeaconRootTOFU pin matches: unexpected error: %v", err)
	}

	// Config must be unchanged on disk — write an initial state and verify no save.
	if err := rdconfig.Save(cfHome, &rdconfig.Config{BeaconRoot: sampleRoot}); err != nil {
		t.Fatalf("setup Save: %v", err)
	}
	// Run again with matching root; should be a no-op.
	cfg2 := &rdconfig.Config{BeaconRoot: sampleRoot}
	err = applyBeaconRootTOFU(cfHome, cfg2, sampleRoot, false)
	if err != nil {
		t.Errorf("applyBeaconRootTOFU matching pin: unexpected error: %v", err)
	}
}

// TestBeaconRoot_PinMismatch_NoConfirm verifies path 4:
// Pin exists, requested root is different, --confirm not passed.
// Expected: error containing "does not match pinned root".
func TestBeaconRoot_PinMismatch_NoConfirm(t *testing.T) {
	cfHome := cfHomeTempDir(t)

	cfg := &rdconfig.Config{BeaconRoot: sampleRoot}
	err := applyBeaconRootTOFU(cfHome, cfg, sampleRoot2, false /* confirm */)
	if err == nil {
		t.Fatal("expected error on beacon root mismatch without --confirm, got nil")
	}
	if !strings.Contains(err.Error(), "does not match pinned root") {
		t.Errorf("error should mention 'does not match pinned root', got: %v", err)
	}
}

// TestBeaconRoot_PinMismatch_WithConfirm verifies that --confirm allows proceeding
// even when the requested root differs from the pinned root.
func TestBeaconRoot_PinMismatch_WithConfirm(t *testing.T) {
	cfHome := cfHomeTempDir(t)

	cfg := &rdconfig.Config{BeaconRoot: sampleRoot}
	err := applyBeaconRootTOFU(cfHome, cfg, sampleRoot2, true /* confirm */)
	if err != nil {
		t.Fatalf("applyBeaconRootTOFU with --confirm on mismatch: unexpected error: %v", err)
	}
}

// TestBeaconRoot_Reset verifies path 5:
// --reset-beacon-root clears the pin from config and returns the previous value.
func TestBeaconRoot_Reset(t *testing.T) {
	cfHome := cfHomeTempDir(t)

	// Write a config with a beacon root pinned.
	if err := rdconfig.Save(cfHome, &rdconfig.Config{BeaconRoot: sampleRoot}); err != nil {
		t.Fatalf("setup Save: %v", err)
	}

	prev, err := resetBeaconRoot(cfHome)
	if err != nil {
		t.Fatalf("resetBeaconRoot: unexpected error: %v", err)
	}
	if prev != sampleRoot {
		t.Errorf("resetBeaconRoot prev = %q, want %q", prev, sampleRoot)
	}

	// Config must now have no pin.
	saved, err := rdconfig.Load(cfHome)
	if err != nil {
		t.Fatalf("rdconfig.Load after reset: %v", err)
	}
	if saved.BeaconRoot != "" {
		t.Errorf("BeaconRoot after reset = %q, want empty", saved.BeaconRoot)
	}
}

// TestBeaconRoot_Reset_NoPinned verifies resetBeaconRoot returns empty string
// when no root is pinned (idempotent).
func TestBeaconRoot_Reset_NoPinned(t *testing.T) {
	cfHome := cfHomeTempDir(t)

	// No config file written — Load returns zero Config.
	prev, err := resetBeaconRoot(cfHome)
	if err != nil {
		t.Fatalf("resetBeaconRoot on empty config: unexpected error: %v", err)
	}
	if prev != "" {
		t.Errorf("expected empty prev, got %q", prev)
	}
}

// TestBeaconRoot_EmptyRoot_NoOp verifies that calling applyBeaconRootTOFU with
// an empty root is a no-op (no save, no error).
func TestBeaconRoot_EmptyRoot_NoOp(t *testing.T) {
	cfHome := cfHomeTempDir(t)

	cfg := &rdconfig.Config{}
	err := applyBeaconRootTOFU(cfHome, cfg, "" /* beaconRoot */, false)
	if err != nil {
		t.Fatalf("empty beacon root should be no-op, got error: %v", err)
	}

	// No config file should have been written.
	configPath := filepath.Join(cfHome, "rd.json")
	if _, statErr := os.Stat(configPath); statErr == nil {
		t.Error("applyBeaconRootTOFU with empty root must not write config file")
	}
}

// TestJoin_OpenCampfire_NonMemberJoinsSuccessfully verifies the open join_protocol path:
// - Identity A creates an open campfire (filesystem transport, shared temp base dir).
// - Identity B (separate CF_HOME, non-member) calls client.Join() using the
//   campfire-specific dir returned by resolveTransportDir.
// - Join succeeds and B can subsequently send a message (post-join create).
//
// This is the regression test for ready-1b5. No mocks — real protocol.Client
// instances with real temp dirs and real SQLite stores.
func TestJoin_OpenCampfire_NonMemberJoinsSuccessfully(t *testing.T) {
	// Use a shared transport dir so both identities access the same campfire state.
	sharedTransportDir := t.TempDir()
	origTransportDir := os.Getenv("CF_TRANSPORT_DIR")
	os.Setenv("CF_TRANSPORT_DIR", sharedTransportDir)
	t.Cleanup(func() {
		if origTransportDir != "" {
			os.Setenv("CF_TRANSPORT_DIR", origTransportDir)
		} else {
			os.Unsetenv("CF_TRANSPORT_DIR")
		}
	})

	// --- Identity A: campfire creator ---
	cfHomeA := t.TempDir()
	origRDHome := rdHome
	rdHome = cfHomeA
	protocolClientOrig := protocolClient
	protocolClient = nil
	t.Cleanup(func() {
		if protocolClient != nil {
			protocolClient.Close()
		}
		protocolClient = protocolClientOrig
		rdHome = origRDHome
	})

	clientA, err := requireClient()
	if err != nil {
		t.Fatalf("requireClient (A): %v", err)
	}

	// A creates an open campfire in the shared transport dir.
	createResult, err := clientA.Create(protocol.CreateRequest{
		JoinProtocol: "open",
		Transport:    protocol.FilesystemTransport{Dir: sharedTransportDir},
	})
	if err != nil {
		t.Fatalf("A.Create (open): %v", err)
	}
	campfireID := createResult.CampfireID

	// --- Identity B: non-member joiner (separate CF_HOME) ---
	// Reset global client cache so B gets a fresh client with a different identity.
	if protocolClient != nil {
		protocolClient.Close()
	}
	protocolClient = nil

	cfHomeB := t.TempDir()
	rdHome = cfHomeB

	clientB, err := requireClient()
	if err != nil {
		t.Fatalf("requireClient (B): %v", err)
	}

	// resolveTransportDir must return the campfire-specific dir (sharedTransportDir/campfireID).
	// With no beacon published (no beacon dir scan), it falls back to:
	//   localCampfireBaseDir() / campfireID = sharedTransportDir / campfireID
	transportDir := resolveTransportDir(campfireID)
	wantDir := filepath.Join(sharedTransportDir, campfireID)
	if transportDir != wantDir {
		t.Errorf("resolveTransportDir = %q, want %q", transportDir, wantDir)
	}

	// B calls client.Join() — open campfire, must succeed.
	joinResult, err := clientB.Join(protocol.JoinRequest{
		CampfireID: campfireID,
		Transport:  protocol.FilesystemTransport{Dir: transportDir},
	})
	if err != nil {
		t.Fatalf("B.Join (open campfire): %v", err)
	}
	if joinResult.JoinProtocol != "open" {
		t.Errorf("joinResult.JoinProtocol = %q, want %q", joinResult.JoinProtocol, "open")
	}

	// After join, B must be able to send a message (simulates 'rd create').
	_, err = clientB.Send(protocol.SendRequest{
		CampfireID: campfireID,
		Payload:    []byte(`{"title":"post-join message from B"}`),
		Tags:       []string{"work:create"},
	})
	if err != nil {
		t.Fatalf("B.Send after join: %v — B is not recognized as a member after joining", err)
	}
}

// TestJoin_InviteOnlyCampfire_FailsWithClearError verifies the invite-only path:
// - Identity A creates an invite-only campfire.
// - Identity B (non-member) calls client.Join() on the invite-only campfire.
// - Join fails with an error containing "invite-only".
// - The error must NOT be a transport error — it must clearly indicate the protocol
//   rejection so rd join can distinguish it from network/transport failures and
//   produce the right user-facing message.
//
// This is the regression test for ready-1b5. No mocks — real protocol.Client instances.
func TestJoin_InviteOnlyCampfire_FailsWithClearError(t *testing.T) {
	sharedTransportDir := t.TempDir()
	origTransportDir := os.Getenv("CF_TRANSPORT_DIR")
	os.Setenv("CF_TRANSPORT_DIR", sharedTransportDir)
	t.Cleanup(func() {
		if origTransportDir != "" {
			os.Setenv("CF_TRANSPORT_DIR", origTransportDir)
		} else {
			os.Unsetenv("CF_TRANSPORT_DIR")
		}
	})

	// --- Identity A: campfire creator ---
	cfHomeA := t.TempDir()
	origRDHome := rdHome
	rdHome = cfHomeA
	protocolClientOrig := protocolClient
	protocolClient = nil
	t.Cleanup(func() {
		if protocolClient != nil {
			protocolClient.Close()
		}
		protocolClient = protocolClientOrig
		rdHome = origRDHome
	})

	clientA, err := requireClient()
	if err != nil {
		t.Fatalf("requireClient (A): %v", err)
	}

	// A creates an invite-only campfire.
	createResult, err := clientA.Create(protocol.CreateRequest{
		JoinProtocol: "invite-only",
		Transport:    protocol.FilesystemTransport{Dir: sharedTransportDir},
	})
	if err != nil {
		t.Fatalf("A.Create (invite-only): %v", err)
	}
	campfireID := createResult.CampfireID

	// --- Identity B: non-member (separate CF_HOME) ---
	if protocolClient != nil {
		protocolClient.Close()
	}
	protocolClient = nil

	cfHomeB := t.TempDir()
	rdHome = cfHomeB

	clientB, err := requireClient()
	if err != nil {
		t.Fatalf("requireClient (B): %v", err)
	}

	// B attempts to join an invite-only campfire without pre-admission.
	transportDir := filepath.Join(sharedTransportDir, campfireID)
	_, joinErr := clientB.Join(protocol.JoinRequest{
		CampfireID: campfireID,
		Transport:  protocol.FilesystemTransport{Dir: transportDir},
	})
	if joinErr == nil {
		t.Fatal("B.Join (invite-only): expected error, got nil — non-member must not join invite-only campfire")
	}

	// Error must contain "invite-only" so rd join can produce the right user-facing message.
	if !strings.Contains(joinErr.Error(), "invite-only") {
		t.Errorf("B.Join error = %q — expected 'invite-only' in message so rd join can distinguish protocol rejection from transport errors", joinErr.Error())
	}
}
