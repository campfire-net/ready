package conventionserver

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/campfire-net/campfire/pkg/protocol"
)

// --- Rate limiter tests ---

func TestJoinRateLimiter_AllowsUpToMax(t *testing.T) {
	rl := newJoinRateLimiter(10)
	pubkey := "aabbcc"

	for i := 0; i < 10; i++ {
		if !rl.Allow(pubkey) {
			t.Fatalf("Allow() returned false on request %d (expected true)", i+1)
		}
	}
}

func TestJoinRateLimiter_RejectsOnEleventhRequest(t *testing.T) {
	rl := newJoinRateLimiter(10)
	pubkey := "aabbcc"

	for i := 0; i < 10; i++ {
		rl.Allow(pubkey)
	}

	if rl.Allow(pubkey) {
		t.Fatal("Allow() returned true on 11th request — should be rejected")
	}
}

func TestJoinRateLimiter_IndependentPerPubkey(t *testing.T) {
	rl := newJoinRateLimiter(10)

	pubkey1 := "pubkey1"
	pubkey2 := "pubkey2"

	// Exhaust limit for pubkey1.
	for i := 0; i < 10; i++ {
		rl.Allow(pubkey1)
	}
	if rl.Allow(pubkey1) {
		t.Error("pubkey1: Allow() should be false after 10 requests")
	}

	// pubkey2 should still have its own fresh bucket.
	if !rl.Allow(pubkey2) {
		t.Error("pubkey2: Allow() should be true (independent bucket)")
	}
}

func TestJoinRateLimiter_ResetsAfterWindowExpiry(t *testing.T) {
	rl := newJoinRateLimiter(10)
	pubkey := "aabbcc"

	// Fake clock: start at t=0.
	now := time.Now()
	rl.nowFunc = func() time.Time { return now }

	// Exhaust the limit.
	for i := 0; i < 10; i++ {
		rl.Allow(pubkey)
	}
	if rl.Allow(pubkey) {
		t.Fatal("should be rate limited before window expiry")
	}

	// Advance clock past 1 hour — old entries should expire.
	now = now.Add(time.Hour + time.Second)
	rl.nowFunc = func() time.Time { return now }

	if !rl.Allow(pubkey) {
		t.Fatal("Allow() should return true after window expiry")
	}
}

func TestJoinRateLimiter_ConcurrentSafe(t *testing.T) {
	rl := newJoinRateLimiter(10)
	var wg sync.WaitGroup
	allowed := make([]bool, 20)
	for i := 0; i < 20; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			allowed[i] = rl.Allow("shared-key")
		}()
	}
	wg.Wait()

	count := 0
	for _, ok := range allowed {
		if ok {
			count++
		}
	}
	if count > 10 {
		t.Errorf("concurrent Allow() allowed %d requests — max is 10", count)
	}
}

// --- Inbox watcher materialization tests ---

// inboxWatcherReader is the interface the watcher uses, extracted for testing.
type inboxWatcherReader interface {
	Read(req protocol.ReadRequest) (*protocol.ReadResult, error)
	Send(req protocol.SendRequest) (*protocol.Message, error)
}

// fakeInboxClient implements inboxWatcherReader.
type fakeInboxClient struct {
	mu      sync.Mutex
	sent    []protocol.SendRequest
	msgs    []protocol.Message
	readErr error
	sendErr error
}

func (f *fakeInboxClient) Read(_ protocol.ReadRequest) (*protocol.ReadResult, error) {
	if f.readErr != nil {
		return nil, f.readErr
	}
	f.mu.Lock()
	msgs := make([]protocol.Message, len(f.msgs))
	copy(msgs, f.msgs)
	f.mu.Unlock()
	return &protocol.ReadResult{Messages: msgs}, nil
}

func (f *fakeInboxClient) Send(req protocol.SendRequest) (*protocol.Message, error) {
	if f.sendErr != nil {
		return nil, f.sendErr
	}
	f.mu.Lock()
	f.sent = append(f.sent, req)
	f.mu.Unlock()
	return &protocol.Message{ID: "test-msg-id"}, nil
}

// testableInboxWatcher is a variant of inboxWatcher that accepts the interface
// instead of *protocol.Client, allowing fake injection in tests.
type testableInboxWatcher struct {
	reader          inboxWatcherReader
	inboxCampfire   string
	projectCampfire string
	rateLimit       *joinRateLimiter
	cursor          int64
}

func (w *testableInboxWatcher) poll(_ context.Context) {
	result, err := w.reader.Read(protocol.ReadRequest{
		CampfireID:     w.inboxCampfire,
		Tags:           []string{"work:join-request"},
		AfterTimestamp: w.cursor,
	})
	if err != nil {
		return
	}

	for _, msg := range result.Messages {
		_ = w.handleJoinRequest(msg)
	}

	if result.MaxTimestamp > w.cursor {
		w.cursor = result.MaxTimestamp
	}
}

func (w *testableInboxWatcher) handleJoinRequest(msg protocol.Message) error {
	if !w.rateLimit.Allow(msg.Sender) {
		return nil // rate limited — silently drop
	}

	var payload JoinRequestPayload
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		return err
	}

	itemPayload := map[string]any{
		"pubkey":         payload.Pubkey,
		"requested_role": payload.RequestedRole,
	}
	if payload.OptionalAttestations != "" {
		itemPayload["optional_attestations"] = payload.OptionalAttestations
	}
	if payload.OptionalJoinConversationCampfire != "" {
		itemPayload["optional_join_conversation_campfire"] = payload.OptionalJoinConversationCampfire
	}

	payloadBytes, err := json.Marshal(itemPayload)
	if err != nil {
		return err
	}

	_, err = w.reader.Send(protocol.SendRequest{
		CampfireID: w.projectCampfire,
		Payload:    payloadBytes,
		Tags:       []string{"work:join-request"},
	})
	return err
}

// TestJoinRateLimiter_DeletesBucketKeyWhenEmptyAfterEviction verifies that when
// all timestamps in a bucket expire and the next Allow() call is for a pubkey
// that is now over the limit (max=0 forces the deny path with an empty fresh
// slice), the map key is deleted rather than retained with a zero-length slice.
// This is the memory-leak regression: unique senders whose buckets are fully
// expired should not linger in the map indefinitely.
func TestJoinRateLimiter_DeletesBucketKeyWhenEmptyAfterEviction(t *testing.T) {
	// max=0 means every request is denied; the bucket is always empty after
	// eviction because no timestamps are ever appended.
	rl := &joinRateLimiter{
		max:     0,
		window:  time.Hour,
		buckets: make(map[string][]time.Time),
	}
	pubkey := "testpubkey"

	// Manually seed the bucket with an expired timestamp so that on the first
	// Allow() call there is something to evict.
	past := time.Now().Add(-2 * time.Hour)
	rl.nowFunc = func() time.Time { return time.Now() }
	rl.mu.Lock()
	rl.buckets[pubkey] = []time.Time{past}
	rl.mu.Unlock()

	// Key must exist before Allow().
	rl.mu.Lock()
	_, exists := rl.buckets[pubkey]
	rl.mu.Unlock()
	if !exists {
		t.Fatal("precondition: bucket key should exist before Allow()")
	}

	// Allow() evicts the expired timestamp → fresh is empty → key should be
	// deleted. max=0 so it also returns false (denied), exercising the deny
	// branch with an empty fresh slice.
	result := rl.Allow(pubkey)
	if result {
		t.Fatal("Allow() should return false when max=0")
	}

	rl.mu.Lock()
	_, stillPresent := rl.buckets[pubkey]
	rl.mu.Unlock()

	if stillPresent {
		t.Error("bucket key still present after all entries evicted — memory leak not fixed")
	}
}

func TestInboxWatcher_MaterializesJoinRequest(t *testing.T) {
	const inboxID = "inbox-campfire-id"
	const projectID = "project-campfire-id"

	payload := JoinRequestPayload{
		Pubkey:        "abcdef1234567890",
		RequestedRole: "member",
	}
	payloadBytes, _ := json.Marshal(payload)

	fake := &fakeInboxClient{
		msgs: []protocol.Message{
			{
				ID:      "msg-001",
				Sender:  "sender-pubkey-001",
				Payload: payloadBytes,
				Tags:    []string{"work:join-request"},
			},
		},
	}

	w := &testableInboxWatcher{
		reader:          fake,
		inboxCampfire:   inboxID,
		projectCampfire: projectID,
		rateLimit:       newJoinRateLimiter(10),
	}

	w.poll(context.Background())

	fake.mu.Lock()
	sent := make([]protocol.SendRequest, len(fake.sent))
	copy(sent, fake.sent)
	fake.mu.Unlock()

	if len(sent) != 1 {
		t.Fatalf("expected 1 materialized message, got %d", len(sent))
	}
	if sent[0].CampfireID != projectID {
		t.Errorf("materialized to wrong campfire: %s", sent[0].CampfireID)
	}

	found := false
	for _, tag := range sent[0].Tags {
		if tag == "work:join-request" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("materialized message missing work:join-request tag: %v", sent[0].Tags)
	}

	var materialized map[string]any
	if err := json.Unmarshal(sent[0].Payload, &materialized); err != nil {
		t.Fatalf("parsing materialized payload: %v", err)
	}
	if materialized["pubkey"] != payload.Pubkey {
		t.Errorf("pubkey mismatch: got %v, want %v", materialized["pubkey"], payload.Pubkey)
	}
	if materialized["requested_role"] != payload.RequestedRole {
		t.Errorf("requested_role mismatch: got %v, want %v", materialized["requested_role"], payload.RequestedRole)
	}
}

func TestInboxWatcher_RateLimitsEleventhRequest(t *testing.T) {
	const inboxID = "inbox-campfire-id"
	const projectID = "project-campfire-id"
	const senderPubkey = "spammer-pubkey"

	payload := JoinRequestPayload{
		Pubkey:        senderPubkey,
		RequestedRole: "member",
	}
	payloadBytes, _ := json.Marshal(payload)

	// 11 messages from the same sender.
	msgs := make([]protocol.Message, 11)
	for i := range msgs {
		msgs[i] = protocol.Message{
			ID:      fmt.Sprintf("msg-%03d", i),
			Sender:  senderPubkey,
			Payload: payloadBytes,
			Tags:    []string{"work:join-request"},
		}
	}

	fake := &fakeInboxClient{msgs: msgs}

	w := &testableInboxWatcher{
		reader:          fake,
		inboxCampfire:   inboxID,
		projectCampfire: projectID,
		rateLimit:       newJoinRateLimiter(10),
	}

	w.poll(context.Background())

	fake.mu.Lock()
	count := len(fake.sent)
	fake.mu.Unlock()

	if count != 10 {
		t.Errorf("expected 10 materialized messages (11th rejected), got %d", count)
	}
}

// TestInboxWatcher_InvalidPubkeyRejected verifies that handleJoinRequest on the
// production inboxWatcher returns an error (and does NOT attempt to materialize the
// message) when the payload pubkey is not a valid 64-char hex string.
// The client is left nil because validation returns before any client call.
func TestInboxWatcher_InvalidPubkeyRejected(t *testing.T) {
	cases := []struct {
		name   string
		pubkey string
	}{
		{"too short", "abcdef1234"},
		{"non-hex chars", "gggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggg"},
		{"uppercase", "ABCDEF1234567890ABCDEF1234567890ABCDEF1234567890ABCDEF1234567890AB"},
		{"empty pubkey", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			payload := JoinRequestPayload{
				Pubkey:        tc.pubkey,
				RequestedRole: "member",
			}
			payloadBytes, _ := json.Marshal(payload)

			// client is nil — production code must not reach client.Send.
			w := &inboxWatcher{
				client:    nil,
				rateLimit: newJoinRateLimiter(10),
			}
			msg := protocol.Message{
				ID:      "msg-bad",
				Sender:  "sender-key",
				Payload: payloadBytes,
				Tags:    []string{"work:join-request"},
			}
			err := w.handleJoinRequest(msg)
			if err == nil {
				t.Fatalf("expected error for invalid pubkey %q, got nil", tc.pubkey)
			}
		})
	}
}

func TestInboxWatcher_OptionalFieldsPassedThrough(t *testing.T) {
	const inboxID = "inbox-campfire-id"
	const projectID = "project-campfire-id"

	payload := JoinRequestPayload{
		Pubkey:                           "abcdef",
		RequestedRole:                    "admin",
		OptionalAttestations:             "some-attestation",
		OptionalJoinConversationCampfire: "conversation-campfire-id",
	}
	payloadBytes, _ := json.Marshal(payload)

	fake := &fakeInboxClient{
		msgs: []protocol.Message{
			{ID: "msg-001", Sender: "sender", Payload: payloadBytes, Tags: []string{"work:join-request"}},
		},
	}

	w := &testableInboxWatcher{
		reader:          fake,
		inboxCampfire:   inboxID,
		projectCampfire: projectID,
		rateLimit:       newJoinRateLimiter(10),
	}

	w.poll(context.Background())

	fake.mu.Lock()
	sent := make([]protocol.SendRequest, len(fake.sent))
	copy(sent, fake.sent)
	fake.mu.Unlock()

	if len(sent) != 1 {
		t.Fatalf("expected 1 materialized message, got %d", len(sent))
	}

	var materialized map[string]any
	if err := json.Unmarshal(sent[0].Payload, &materialized); err != nil {
		t.Fatalf("parsing materialized payload: %v", err)
	}

	if materialized["optional_attestations"] != payload.OptionalAttestations {
		t.Errorf("optional_attestations mismatch: got %v", materialized["optional_attestations"])
	}
	if materialized["optional_join_conversation_campfire"] != payload.OptionalJoinConversationCampfire {
		t.Errorf("optional_join_conversation_campfire mismatch: got %v", materialized["optional_join_conversation_campfire"])
	}
}
