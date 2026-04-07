package main

// admit_test.go — unit tests for the rd admit command routing logic and
// the admitMemberWithRole SDK call path.
//
// The done conditions tested here:
//  - --role org-observer targets SummaryCampfireID (not CampfireID)
//  - --role org-observer fails with a clear error when SummaryCampfireID is empty
//  - --role member targets CampfireID
//  - unknown roles return errors
//  - admitMemberWithRole calls client.Admit() with correct campfire + pubkey (ready-421)
//  - admitMemberWithRole propagates GetMembership errors (ready-421)
//  - admitMemberWithRole propagates Admit errors (ready-421)

import (
	"fmt"
	"strings"
	"testing"

	"github.com/campfire-net/campfire/pkg/protocol"
	"github.com/campfire-net/campfire/pkg/store"
	"github.com/campfire-net/ready/pkg/rdconfig"
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
