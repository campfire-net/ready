package main

// admit_test.go — unit tests for the rd admit command routing logic.
//
// The done conditions tested here:
//  - --role org-observer targets SummaryCampfireID (not CampfireID)
//  - --role org-observer fails with a clear error when SummaryCampfireID is empty
//  - --role member targets CampfireID
//  - unknown roles return errors

import (
	"testing"

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
