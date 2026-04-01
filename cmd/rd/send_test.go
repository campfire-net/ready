package main

// send_test.go exercises bufferToPending, client.Send() happy path,
// sendPrebuiltMessage ID-preservation (D6 constraint), buildFlusher signing
// isolation, and bufferToPending Sender field recording.

import (
	"crypto/ed25519"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	campfirepkg "github.com/campfire-net/campfire/pkg/campfire"
	cfencoding "github.com/campfire-net/campfire/pkg/encoding"
	"github.com/campfire-net/campfire/pkg/identity"
	"github.com/campfire-net/campfire/pkg/message"
	"github.com/campfire-net/campfire/pkg/protocol"
	"github.com/campfire-net/campfire/pkg/store"
	"github.com/campfire-net/campfire/pkg/transport/fs"

	"github.com/campfire-net/ready/pkg/jsonl"
)

// TestBufferToPending_NoProjectRoot verifies that bufferToPending returns an
// error when no project root exists (no .ready/ directory in cwd or parents).
// Finding 2: bufferToPending must not silently drop mutations.
func TestBufferToPending_NoProjectRoot(t *testing.T) {
	// Change to a temp dir with no .ready/ and no .campfire/root.
	dir := t.TempDir()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() { os.Chdir(orig) })
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	msg := &message.Message{ID: "test-msg-id-no-project-root"}
	err = bufferToPending(msg, "campfire-test-id", "deadbeef", `{"op":"test"}`, []string{"work:create"}, nil)
	if err == nil {
		t.Fatal("bufferToPending: expected error when no project root, got nil")
	}
}

// TestBufferToPending_WritesRecord verifies that bufferToPending writes a record
// to pending.jsonl when a project root exists.
func TestBufferToPending_WritesRecord(t *testing.T) {
	// Create a minimal project dir with .ready/.
	dir := t.TempDir()
	readyDir := filepath.Join(dir, ".ready")
	if err := os.MkdirAll(readyDir, 0700); err != nil {
		t.Fatalf("mkdir .ready: %v", err)
	}

	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() { os.Chdir(orig) })
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	msg := &message.Message{ID: "test-msg-id-buffer-writes"}
	err = bufferToPending(msg, "campfire-test-id", "deadbeef", `{"op":"test"}`, []string{"work:create"}, nil)
	if err != nil {
		t.Fatalf("bufferToPending: unexpected error: %v", err)
	}

	pendingFile := filepath.Join(readyDir, "pending.jsonl")
	data, err := os.ReadFile(pendingFile)
	if err != nil {
		t.Fatalf("reading pending.jsonl: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("pending.jsonl is empty — expected a buffered record")
	}
}

// TestBufferToPending_RecordsSender verifies that bufferToPending writes the
// senderHex argument to the Sender field of the pending MutationRecord.
// This ensures pending.jsonl is self-describing: any reader can identify who
// authored the mutation without consulting the live identity.
func TestBufferToPending_RecordsSender(t *testing.T) {
	dir := t.TempDir()
	readyDir := filepath.Join(dir, ".ready")
	if err := os.MkdirAll(readyDir, 0755); err != nil {
		t.Fatalf("mkdir .ready: %v", err)
	}

	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() { os.Chdir(orig) })
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	wantSender := "aabbccddeeff0011223344556677889900112233445566778899aabbccddeeff"
	msg := &message.Message{ID: "test-msg-id-sender-recorded"}
	if err := bufferToPending(msg, "campfire-test-id", wantSender, `{"op":"test"}`, []string{"work:create"}, nil); err != nil {
		t.Fatalf("bufferToPending: %v", err)
	}

	pendingFile := filepath.Join(readyDir, "pending.jsonl")
	data, err := os.ReadFile(pendingFile)
	if err != nil {
		t.Fatalf("reading pending.jsonl: %v", err)
	}

	var rec jsonl.MutationRecord
	if err := json.Unmarshal(data, &rec); err != nil {
		t.Fatalf("unmarshal pending record: %v", err)
	}
	if rec.Sender != wantSender {
		t.Errorf("pending record Sender=%q, want %q — Sender field not recorded in pending.jsonl", rec.Sender, wantSender)
	}
}

// newSendTestCampfire sets up a minimal fs-transport campfire environment in
// tmpDir and returns the campfire ID, the identity, the store, and the transport.
// The caller is responsible for closing the store.
func newSendTestCampfire(t *testing.T, tmpDir string) (campfireID string, id *identity.Identity, s store.Store, tr *fs.Transport) {
	t.Helper()

	id, err := identity.Generate()
	if err != nil {
		t.Fatalf("identity.Generate: %v", err)
	}

	// Create campfire (open, no reception requirements, threshold=1).
	cf, err := campfirepkg.New("open", nil, 1)
	if err != nil {
		t.Fatalf("campfire.New: %v", err)
	}
	cf.AddMember(id.PublicKey)
	campfireID = cf.PublicKeyHex()

	// Initialize filesystem transport.
	tr = fs.New(tmpDir)
	if err := tr.Init(cf); err != nil {
		t.Fatalf("tr.Init: %v", err)
	}

	// Write member record for our identity.
	memberRec := campfirepkg.MemberRecord{
		PublicKey: id.PublicKey,
		JoinedAt:  time.Now().UnixNano(),
		Role:      campfirepkg.RoleFull,
	}
	if err := tr.WriteMember(campfireID, memberRec); err != nil {
		t.Fatalf("tr.WriteMember: %v", err)
	}

	// Open store and add membership.
	// TransportDir points to the campfire-specific subdirectory (tmpDir/<campfireID>)
	// so that both client.Send() (ForDir path-rooted) and sendPrebuiltMessage
	// (filepath.Dir + fs.New) resolve the same campfire directory.
	campfireDir := filepath.Join(tmpDir, campfireID)
	dbPath := filepath.Join(tmpDir, "store.db")
	s, err = store.Open(dbPath)
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}

	membership := store.Membership{
		CampfireID:   campfireID,
		TransportDir: campfireDir,
		JoinProtocol: "open",
		Role:         store.PeerRoleCreator,
		JoinedAt:     time.Now().UnixNano(),
		Threshold:    1,
	}
	if err := s.AddMembership(membership); err != nil {
		t.Fatalf("s.AddMembership: %v", err)
	}

	return campfireID, id, s, tr
}

// TestClientSend_HappyPath verifies that client.Send() successfully delivers a
// message to the campfire store. This is the integration test required by the
// item spec (Finding 1): create campfire, send via client.Send(), verify message
// in store.
func TestClientSend_HappyPath(t *testing.T) {
	tmpDir := t.TempDir()
	campfireID, id, s, _ := newSendTestCampfire(t, tmpDir)
	defer s.Close()

	// Create a protocol.Client backed by our real store and identity.
	c := protocol.New(s, id)

	req := protocol.SendRequest{
		CampfireID: campfireID,
		Payload:    []byte(`{"op":"test","title":"HappyPath"}`),
		Tags:       []string{"work:create"},
	}
	msg, err := c.Send(req)
	if err != nil {
		t.Fatalf("client.Send(): %v", err)
	}
	if msg == nil {
		t.Fatal("client.Send() returned nil message")
	}
	if msg.ID == "" {
		t.Fatal("client.Send() returned message with empty ID")
	}

	// Verify the message appears in the store.
	rec, err := s.GetMessage(msg.ID)
	if err != nil {
		t.Fatalf("store.GetMessage(%q): %v", msg.ID, err)
	}
	if rec == nil {
		t.Fatalf("message %q not found in store after client.Send()", msg.ID)
	}
	if rec.CampfireID != campfireID {
		t.Errorf("stored message CampfireID=%q, want %q", rec.CampfireID, campfireID)
	}
}

// TestD6_ExtractOperationFromTags verifies that extractOperationFromTags returns
// the first work: tag, which is the operation name recorded in JSONL and campfire.
// This is the tagging invariant used throughout the D6 constraint path.
func TestD6_ExtractOperationFromTags(t *testing.T) {
	tests := []struct {
		tags []string
		want string
	}{
		{[]string{"work:create"}, "work:create"},
		{[]string{"work:close", "work:resolution:done"}, "work:close"},
		{[]string{"work:status", "work:status:waiting"}, "work:status"},
		{[]string{"work:claim"}, "work:claim"},
		{[]string{}, ""},
		{nil, ""},
		{[]string{"unrelated:tag"}, ""},
		{[]string{"unrelated:tag", "work:update"}, "work:update"},
	}
	for _, tc := range tests {
		got := extractOperationFromTags(tc.tags)
		if got != tc.want {
			t.Errorf("extractOperationFromTags(%v) = %q, want %q", tc.tags, got, tc.want)
		}
	}
}

// TestD6_MessageID_NonEmpty verifies that a message created via message.NewMessage
// always has a non-empty ID. This is a precondition for D6: the ID recorded in
// JSONL must be non-empty so it can be matched against campfire-assigned IDs.
func TestD6_MessageID_NonEmpty(t *testing.T) {
	id, err := identity.Generate()
	if err != nil {
		t.Fatalf("identity.Generate: %v", err)
	}
	msg, err := message.NewMessage(id.PrivateKey, id.PublicKey, []byte(`{"op":"test"}`), []string{"work:create"}, nil)
	if err != nil {
		t.Fatalf("message.NewMessage: %v", err)
	}
	if msg.ID == "" {
		t.Fatal("D6 constraint violated: NewMessage returned message with empty ID")
	}
	if len(msg.ID) < 32 {
		t.Errorf("D6: message ID too short (%d chars), expected at least 32 hex chars: %q", len(msg.ID), msg.ID)
	}
}

// TestD6_TwoDistinctMessages_HaveDifferentIDs verifies that two independently
// created messages always receive different IDs. D6 relies on unique IDs so that
// JSONL records and campfire messages can be matched unambiguously.
func TestD6_TwoDistinctMessages_HaveDifferentIDs(t *testing.T) {
	id, err := identity.Generate()
	if err != nil {
		t.Fatalf("identity.Generate: %v", err)
	}
	msg1, err := message.NewMessage(id.PrivateKey, id.PublicKey, []byte(`{"op":"test1"}`), []string{"work:create"}, nil)
	if err != nil {
		t.Fatalf("NewMessage (1): %v", err)
	}
	msg2, err := message.NewMessage(id.PrivateKey, id.PublicKey, []byte(`{"op":"test2"}`), []string{"work:create"}, nil)
	if err != nil {
		t.Fatalf("NewMessage (2): %v", err)
	}
	if msg1.ID == msg2.ID {
		t.Errorf("D6: two distinct messages share the same ID %q — ID generation is not unique", msg1.ID)
	}
}

// TestSendPrebuiltMessage_PreservesID verifies the D6 constraint: sendPrebuiltMessage
// (the flush path) stores a message whose ID matches the original MutationRecord ID.
// This is Finding 2: the flushed message ID must equal the mutations.jsonl record ID.
func TestSendPrebuiltMessage_PreservesID(t *testing.T) {
	tmpDir := t.TempDir()
	campfireID, id, s, _ := newSendTestCampfire(t, tmpDir)
	defer s.Close()

	// Get the membership record for sendPrebuiltMessage.
	m, err := s.GetMembership(campfireID)
	if err != nil || m == nil {
		t.Fatalf("GetMembership: %v (m=%v)", err, m)
	}

	// Build a message with a known ID — simulating a MutationRecord from mutations.jsonl.
	// We construct it the same way buildFlusher does: directly set fields then sign.
	knownID := "deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef"
	tags := []string{"work:create"}
	antecedents := []string{}
	payload := []byte(`{"op":"flush-test"}`)
	ts := time.Now().UnixNano()

	msg := &message.Message{
		ID:          knownID,
		Sender:      id.PublicKey,
		Payload:     payload,
		Tags:        tags,
		Antecedents: antecedents,
		Timestamp:   ts,
		Provenance:  []message.ProvenanceHop{},
	}
	signInput := message.MessageSignInput{
		ID:          msg.ID,
		Payload:     msg.Payload,
		Tags:        msg.Tags,
		Antecedents: msg.Antecedents,
		Timestamp:   msg.Timestamp,
	}
	signBytes, err := cfencoding.Marshal(signInput)
	if err != nil {
		t.Fatalf("cfencoding.Marshal: %v", err)
	}
	msg.Signature = ed25519.Sign(id.PrivateKey, signBytes)

	// Send via sendPrebuiltMessage — this is the flush path (buildFlusher).
	if err := sendPrebuiltMessage(id, s, m, campfireID, msg); err != nil {
		t.Fatalf("sendPrebuiltMessage: %v", err)
	}

	// Verify the stored message ID matches the original MutationRecord ID (D6 constraint).
	rec, err := s.GetMessage(knownID)
	if err != nil {
		t.Fatalf("store.GetMessage(%q): %v", knownID, err)
	}
	if rec == nil {
		t.Fatalf("flushed message %q not found in store — D6 ID preservation broken", knownID)
	}
	if rec.ID != knownID {
		t.Errorf("stored message ID=%q, want %q — D6 constraint violated: flush changed the message ID", rec.ID, knownID)
	}
	if rec.CampfireID != campfireID {
		t.Errorf("stored message CampfireID=%q, want %q", rec.CampfireID, campfireID)
	}
}

// TestBuildFlusher_SigningIsolation verifies that buildFlusher copies the key material
// at construction time. The flusher signs and sends correctly using the copied keys.
// This test verifies the signing path end-to-end: the flushed message is retrievable
// from the store with the correct sender identity.
func TestBuildFlusher_SigningIsolation(t *testing.T) {
	tmpDir := t.TempDir()
	campfireID, id, s, _ := newSendTestCampfire(t, tmpDir)
	defer s.Close()

	m, err := s.GetMembership(campfireID)
	if err != nil || m == nil {
		t.Fatalf("GetMembership: %v (m=%v)", err, m)
	}

	// Capture the original public key hex.
	origPubHex := id.PublicKeyHex()

	// Build the flusher — at this point the key copies should be made internally.
	flusher := buildFlusher(id, s, m, campfireID, tmpDir)

	// Overwrite the identity keys with a different keypair AFTER buildFlusher returns.
	// If buildFlusher holds a direct reference (no copy), it would sign with these new keys.
	// If it made copies at construction time, it must sign with the original keys.
	newPub, newPriv, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("ed25519.GenerateKey: %v", err)
	}
	copy(id.PrivateKey, newPriv)
	copy(id.PublicKey, newPub)

	// Send a pending record via the flusher.
	knownID := "aabbccddeeff00112233445566778899aabbccddeeff00112233445566778899"
	rec := jsonl.MutationRecord{
		MsgID:       knownID,
		CampfireID:  campfireID,
		Timestamp:   time.Now().UnixNano(),
		Operation:   "work:create",
		Payload:     json.RawMessage(`{"op":"isolation-test"}`),
		Tags:        []string{"work:create"},
		Sender:      origPubHex,
		Antecedents: []string{},
	}
	if err := flusher(rec); err != nil {
		t.Fatalf("flusher: %v", err)
	}

	// The message stored in the campfire must have the correct sender public key.
	// This verifies the flusher uses isolated key copies for signing (not a dangling reference).
	storedMsg, err := s.GetMessage(knownID)
	if err != nil {
		t.Fatalf("store.GetMessage(%q): %v", knownID, err)
	}
	if storedMsg == nil {
		t.Fatalf("message %q not found in store after flush", knownID)
	}
	if storedMsg.Sender != origPubHex {
		t.Errorf("flushed message Sender=%q, want %q — flusher used wrong key", storedMsg.Sender, origPubHex)
	}
}
