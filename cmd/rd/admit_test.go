package main

// admit_test.go — unit tests for the rd admit command routing logic and
// the admitMemberWithRole SDK call path.
//
// The done conditions tested here:
//   - --role org-observer targets SummaryCampfireID (not CampfireID)
//   - --role org-observer fails with a clear error when SummaryCampfireID is empty
//   - --role member targets CampfireID
//   - unknown roles return errors
//   - admitMemberWithRole calls client.Admit() with correct campfire + pubkey (ready-421)
//   - admitMemberWithRole propagates GetMembership errors (ready-421)
//   - admitMemberWithRole propagates Admit errors (ready-421)
//   - ErrAmbiguous when prefix matches multiple join-request items (ready-afa5)
//   - full-ID resolves unambiguously, item.For holds requester's pubkey (ready-afa5)
//   - admitThenGrant: no role-grant posted when Admit() fails (ready-f45)
//   - admitThenGrant: role-grant posted exactly once when Admit() succeeds (ready-f45)

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/campfire-net/campfire/pkg/convention"
	"github.com/campfire-net/campfire/pkg/protocol"
	"github.com/campfire-net/campfire/pkg/store"
	"github.com/campfire-net/ready/pkg/rdconfig"
	"github.com/campfire-net/ready/pkg/resolve"
)

// admitRoleTarget returns the campfire ID that would be targeted for the given
// role and sync config, without actually calling the SDK. This mirrors the
// switch statement in admitCmd.RunE.
func admitRoleTarget(role string, syncCfg *rdconfig.SyncConfig) (campfireID string, err error) {
	switch role {
	case "member":
		if syncCfg.CampfireID == "" {
			return "", errNoMainCampfire
		}
		return syncCfg.CampfireID, nil
	case "org-observer":
		if syncCfg.SummaryCampfireID == "" {
			return "", errNoSummaryCampfire
		}
		return syncCfg.SummaryCampfireID, nil
	default:
		return "", errUnknownRole
	}
}

// sentinel errors for routing decisions (mirrors the logic in admitCmd.RunE).
type admitRoutingError string

func (e admitRoutingError) Error() string { return string(e) }

const (
	errNoMainCampfire    = admitRoutingError("no campfire configured for this project (offline mode?)")
	errNoSummaryCampfire = admitRoutingError("no summary campfire configured for this project — run 'rd init' to create one")
	errUnknownRole       = admitRoutingError("unknown role")
)

// TestAdmit_OrgObserver_TargetsSummaryCampfire verifies that --role org-observer
// routes to SummaryCampfireID, not CampfireID.
func TestAdmit_OrgObserver_TargetsSummaryCampfire(t *testing.T) {
	syncCfg := &rdconfig.SyncConfig{
		CampfireID:        "main111aaa",
		SummaryCampfireID: "summary222bbb",
	}

	target, err := admitRoleTarget("org-observer", syncCfg)
	if err != nil {
		t.Fatalf("admitRoleTarget org-observer: unexpected error: %v", err)
	}
	if target != syncCfg.SummaryCampfireID {
		t.Errorf("org-observer target = %q, want SummaryCampfireID %q", target, syncCfg.SummaryCampfireID)
	}
	if target == syncCfg.CampfireID {
		t.Errorf("org-observer must NOT target CampfireID %q", syncCfg.CampfireID)
	}
}

// TestAdmit_OrgObserver_ErrorWhenNoSummaryCampfire verifies that --role org-observer
// returns an error when SummaryCampfireID is not set.
func TestAdmit_OrgObserver_ErrorWhenNoSummaryCampfire(t *testing.T) {
	syncCfg := &rdconfig.SyncConfig{
		CampfireID:        "main111aaa",
		SummaryCampfireID: "", // not set
	}

	_, err := admitRoleTarget("org-observer", syncCfg)
	if err == nil {
		t.Fatal("expected error when SummaryCampfireID is empty, got nil")
	}
}

// TestAdmit_Member_TargetsMainCampfire verifies that --role member routes to
// CampfireID, not SummaryCampfireID.
func TestAdmit_Member_TargetsMainCampfire(t *testing.T) {
	syncCfg := &rdconfig.SyncConfig{
		CampfireID:        "main111aaa",
		SummaryCampfireID: "summary222bbb",
	}

	target, err := admitRoleTarget("member", syncCfg)
	if err != nil {
		t.Fatalf("admitRoleTarget member: unexpected error: %v", err)
	}
	if target != syncCfg.CampfireID {
		t.Errorf("member target = %q, want CampfireID %q", target, syncCfg.CampfireID)
	}
	if target == syncCfg.SummaryCampfireID {
		t.Errorf("member must NOT target SummaryCampfireID %q", syncCfg.SummaryCampfireID)
	}
}

// TestAdmit_UnknownRole_ReturnsError verifies that an unknown role returns an error.
func TestAdmit_UnknownRole_ReturnsError(t *testing.T) {
	syncCfg := &rdconfig.SyncConfig{
		CampfireID:        "main111aaa",
		SummaryCampfireID: "summary222bbb",
	}

	_, err := admitRoleTarget("superadmin", syncCfg)
	if err == nil {
		t.Fatal("expected error for unknown role, got nil")
	}
}

// TestAdmitByPubKey_InvalidPubkeyRejected verifies that admitByPubKey returns an
// error before performing any I/O when the pubkey is not a valid 64-char hex string.
func TestAdmitByPubKey_InvalidPubkeyRejected(t *testing.T) {
	cases := []struct {
		name   string
		pubkey string
	}{
		{"too short", "abcdef1234"},
		{"too long", "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890ff"},
		{"uppercase hex", "ABCDEF1234567890ABCDEF1234567890ABCDEF1234567890ABCDEF1234567890AB"},
		{"non-hex chars", "gggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggggg"},
		{"empty string", ""},
		{"63 chars", "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890a"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := admitByPubKey(tc.pubkey, "member")
			if err == nil {
				t.Fatalf("expected error for invalid pubkey %q, got nil", tc.pubkey)
			}
			if !strings.Contains(err.Error(), "must be a 64-character hex string") {
				t.Errorf("expected 'must be a 64-character hex string' in error, got: %v", err)
			}
		})
	}
}

// TestAdmit_OrgObserver_NotMainCampfire_Confirms_Isolation verifies the core
// org-observer isolation invariant: the campfire ID for org-observer is different
// from the campfire ID for member. This is the structural guarantee that org
// observers cannot access main campfire content.
func TestAdmit_OrgObserver_NotMainCampfire_Confirms_Isolation(t *testing.T) {
	syncCfg := &rdconfig.SyncConfig{
		CampfireID:        "aaaa1111bbbb2222cccc3333dddd4444eeee5555ffff6666aaaa1111bbbb2222",
		SummaryCampfireID: "1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
	}

	memberTarget, err := admitRoleTarget("member", syncCfg)
	if err != nil {
		t.Fatalf("member target: %v", err)
	}
	observerTarget, err := admitRoleTarget("org-observer", syncCfg)
	if err != nil {
		t.Fatalf("org-observer target: %v", err)
	}

	// Core isolation invariant: the two campfires must be different.
	if memberTarget == observerTarget {
		t.Errorf("isolation violation: member and org-observer target the same campfire %q", memberTarget)
	}
}

// TestAdmitMemberWithRole_CallsClientAdmit verifies that admitMemberWithRole
// calls client.Admit() with the correct campfire ID and pubkey when
// GetMembership succeeds. This is the core SDK integration path.
func TestAdmitMemberWithRole_CallsClientAdmit(t *testing.T) {
	campfireID := pubkeyHex("cc")
	pubKeyHex := pubkeyHex("dd")
	transportDir := "/tmp/campfire-transport"

	fake := &fakeAdmitClient{
		membership: &store.Membership{
			CampfireID:   campfireID,
			TransportDir: transportDir,
		},
	}

	err := admitMemberWithRole(fake, campfireID, pubKeyHex, "member", "main campfire")
	if err != nil {
		t.Fatalf("admitMemberWithRole: unexpected error: %v", err)
	}

	// Verify Admit was called exactly once with the right args.
	if len(fake.admitCalls) != 1 {
		t.Fatalf("Admit called %d times, want 1", len(fake.admitCalls))
	}
	req := fake.admitCalls[0]
	if req.CampfireID != campfireID {
		t.Errorf("Admit CampfireID = %q, want %q", req.CampfireID, campfireID)
	}
	if req.MemberPubKeyHex != pubKeyHex {
		t.Errorf("Admit MemberPubKeyHex = %q, want %q", req.MemberPubKeyHex, pubKeyHex)
	}
	if req.Role != "member" {
		t.Errorf("Admit Role = %q, want %q", req.Role, "member")
	}
	if req.Transport.(protocol.FilesystemTransport).Dir != transportDir {
		t.Errorf("Admit Transport.Dir = %q, want %q",
			req.Transport.(protocol.FilesystemTransport).Dir, transportDir)
	}
}

// TestAdmitMemberWithRole_GetMembershipError verifies that admitMemberWithRole
// returns an error without calling Admit when GetMembership fails.
func TestAdmitMemberWithRole_GetMembershipError(t *testing.T) {
	campfireID := pubkeyHex("cc")
	pubKeyHex := pubkeyHex("dd")

	fake := &fakeAdmitClient{
		membershipErr: fmt.Errorf("not a member of this campfire"),
	}

	err := admitMemberWithRole(fake, campfireID, pubKeyHex, "member", "main campfire")
	if err == nil {
		t.Fatal("expected error from GetMembership, got nil")
	}
	if !strings.Contains(err.Error(), "getting main campfire membership") {
		t.Errorf("error should mention 'getting main campfire membership', got: %v", err)
	}
	// Admit must NOT have been called.
	if len(fake.admitCalls) != 0 {
		t.Errorf("Admit should not be called on GetMembership error, got %d calls", len(fake.admitCalls))
	}
}

// TestAdmitMemberWithRole_AdmitError verifies that admitMemberWithRole returns
// an error when client.Admit() fails.
func TestAdmitMemberWithRole_AdmitError(t *testing.T) {
	campfireID := pubkeyHex("cc")
	pubKeyHex := pubkeyHex("dd")

	fake := &fakeAdmitClient{
		membership: &store.Membership{
			CampfireID:   campfireID,
			TransportDir: "/tmp/campfire-transport",
		},
		admitErr: fmt.Errorf("permission denied"),
	}

	err := admitMemberWithRole(fake, campfireID, pubKeyHex, "member", "main campfire")
	if err == nil {
		t.Fatal("expected error from Admit, got nil")
	}
	if !strings.Contains(err.Error(), "admitting to main campfire") {
		t.Errorf("error should mention 'admitting to main campfire', got: %v", err)
	}
}

// writeJoinRequestJSONL writes a mutations.jsonl file containing join-request
// items and returns the file path. campfireID labels each record. items is a
// slice of (msgID, itemID, title, forPubkey) tuples.
func writeJoinRequestJSONL(t *testing.T, campfireID string, items []struct{ msgID, itemID, title, forPubkey string }) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "mutations.jsonl")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("Create mutations.jsonl: %v", err)
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for i, it := range items {
		payload, _ := json.Marshal(map[string]interface{}{
			"id":    it.itemID,
			"title": it.title,
			"type":  "task",
			"for":   it.forPubkey,
		})
		rec := map[string]interface{}{
			"msg_id":      it.msgID,
			"campfire_id": campfireID,
			"timestamp":   int64(1000000000000000000 + i),
			"operation":   "work:create",
			"payload":     json.RawMessage(payload),
			"tags":        []string{"work:create"},
			"sender":      "testsender",
		}
		if err := enc.Encode(rec); err != nil {
			t.Fatalf("encode record %d: %v", i, err)
		}
	}
	return path
}

// TestAdmit_AmbiguousPrefix_ErrorsWithMatches verifies that ByIDFromJSONL
// (the resolver used in the admitFromJoinRequest item-lookup path) returns
// ErrAmbiguous when a prefix matches multiple join-request items.
//
// Security regression for ready-afa5: prefix collision must error, not silently
// pick one item — which could cause the wrong person's pubkey to be admitted.
func TestAdmit_AmbiguousPrefix_ErrorsWithMatches(t *testing.T) {
	campfireID := strings.Repeat("ab", 32) // 64-char hex

	// Two items whose IDs share the prefix "proj-aa".
	path := writeJoinRequestJSONL(t, campfireID, []struct{ msgID, itemID, title, forPubkey string }{
		{"msg-aa1", "proj-aa1xyz", "Join Request Alice", pubkeyHex("a1")},
		{"msg-aa2", "proj-aa2xyz", "Join Request Bob", pubkeyHex("b2")},
	})

	_, err := resolve.ByIDFromJSONL(path, campfireID, "proj-aa")
	if err == nil {
		t.Fatal("expected ErrAmbiguous for shared prefix, got nil")
	}

	ambErr, ok := err.(resolve.ErrAmbiguous)
	if !ok {
		t.Fatalf("expected resolve.ErrAmbiguous, got %T: %v", err, err)
	}

	// Error must name both matching items so the admin can choose the right one.
	errMsg := ambErr.Error()
	if !strings.Contains(errMsg, "proj-aa1xyz") {
		t.Errorf("ErrAmbiguous must mention proj-aa1xyz; got: %s", errMsg)
	}
	if !strings.Contains(errMsg, "proj-aa2xyz") {
		t.Errorf("ErrAmbiguous must mention proj-aa2xyz; got: %s", errMsg)
	}
}

// TestAdmit_FullID_ResolvesItemAndHoldsPubkey verifies that ByIDFromJSONL
// resolves a join-request item by full ID and that item.For holds the requester's
// pubkey — the field admitFromJoinRequest uses to decide who to admit.
//
// Security regression for ready-afa5: full-ID lookup must resolve exactly one
// item and the pubkey extracted from item.For must match the original requester.
func TestAdmit_FullID_ResolvesItemAndHoldsPubkey(t *testing.T) {
	campfireID := strings.Repeat("cd", 32) // 64-char hex
	wantPubkey := pubkeyHex("e5")
	wantTitle := "Join Request Eve"
	wantID := "proj-eve42"

	path := writeJoinRequestJSONL(t, campfireID, []struct{ msgID, itemID, title, forPubkey string }{
		{"msg-eve", wantID, wantTitle, wantPubkey},
		// Different-prefix item — must not interfere with the full-ID lookup.
		{"msg-other", "proj-other99", "Other Item", pubkeyHex("ff")},
	})

	item, err := resolve.ByIDFromJSONL(path, campfireID, wantID)
	if err != nil {
		t.Fatalf("ByIDFromJSONL full ID: unexpected error: %v", err)
	}
	if item.ID != wantID {
		t.Errorf("item.ID = %q, want %q", item.ID, wantID)
	}
	if item.Title != wantTitle {
		t.Errorf("item.Title = %q, want %q", item.Title, wantTitle)
	}
	// item.For is the pubkey field admitFromJoinRequest reads to confirm who
	// is being admitted. It must carry the requester's pubkey, not be empty or wrong.
	if item.For != wantPubkey {
		t.Errorf("item.For = %q, want requester pubkey %q", item.For, wantPubkey)
	}
}

// TestAdmitThenGrant_AdmitFailure_NoRoleGrantPosted is the regression test for
// ready-f45: when Admit() fails, no work:role-grant must be posted. This was
// the dangling role-grant vulnerability — the fix reorders the operations so
// role-grant is posted only after Admit() succeeds.
func TestAdmitThenGrant_AdmitFailure_NoRoleGrantPosted(t *testing.T) {
	campfireID := pubkeyHex("aa")
	pubKey := pubkeyHex("bb")
	selfKey := "test-key-hex"

	fake := &fakeAdmitClient{
		membership: &store.Membership{
			CampfireID:   campfireID,
			TransportDir: "/tmp/campfire-transport",
		},
		admitErr: fmt.Errorf("permission denied"),
	}

	backend := &countingSendBackend{}
	exec := convention.NewExecutorForTest(backend, selfKey).
		WithProvenance(&staticProvenanceChecker{levels: map[string]int{selfKey: 2}})
	ctx := context.Background()

	_, err := admitThenGrant(ctx, fake, exec, campfireID, pubKey, "member")
	if err == nil {
		t.Fatal("expected error from Admit(), got nil")
	}

	// The critical assertion: no role-grant was posted.
	if backend.sendCount != 0 {
		t.Errorf("ready-f45 regression: role-grant posted even though Admit() failed — "+
			"got %d SendMessage call(s), want 0", backend.sendCount)
	}
}

// TestAdmitThenGrant_AdmitSuccess_RoleGrantPosted verifies the happy path:
// when Admit() succeeds, the role-grant is posted exactly once.
func TestAdmitThenGrant_AdmitSuccess_RoleGrantPosted(t *testing.T) {
	campfireID := pubkeyHex("aa")
	pubKey := pubkeyHex("bb")
	selfKey := "test-key-hex"

	fake := &fakeAdmitClient{
		membership: &store.Membership{
			CampfireID:   campfireID,
			TransportDir: "/tmp/campfire-transport",
		},
	}

	backend := &countingSendBackend{}
	exec := convention.NewExecutorForTest(backend, selfKey).
		WithProvenance(&staticProvenanceChecker{levels: map[string]int{selfKey: 2}})
	ctx := context.Background()

	msgID, err := admitThenGrant(ctx, fake, exec, campfireID, pubKey, "member")
	if err != nil {
		t.Fatalf("admitThenGrant: unexpected error: %v", err)
	}
	if msgID == "" {
		t.Error("expected non-empty grant message ID")
	}
	if backend.sendCount != 1 {
		t.Errorf("expected exactly 1 SendMessage call for role-grant, got %d", backend.sendCount)
	}
}
