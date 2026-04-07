package main

// revoke_test.go — unit tests for rd revoke helpers.
//
// Done conditions tested:
//   - findMembersAdmittedBy filters by sender and excludes revocations (payload parsing)
//   - findMembersAdmittedBy calls real function against fake client (ready-421)
//   - pubkey format detection: isHex used for routing

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/campfire-net/campfire/pkg/protocol"
)

// TestFindMembersAdmittedBy_PayloadParsing verifies that the JSON payload
// extraction used in findMembersAdmittedBy correctly identifies non-revocation
// grants vs revocations.
//
// We test the payload parsing logic directly rather than calling
// findMembersAdmittedBy (which requires a live campfire client).
func TestFindMembersAdmittedBy_PayloadParsing(t *testing.T) {
	type grantPayload struct {
		Pubkey string `json:"pubkey"`
		Role   string `json:"role"`
	}

	cases := []struct {
		name       string
		payload    string
		wantPubkey string
		wantRole   string
		isGrant    bool // true = should be included in admitted list
	}{
		{
			name:       "member grant",
			payload:    `{"pubkey":"aabbcc1122334455aabbcc1122334455aabbcc1122334455aabbcc1122334455","role":"member"}`,
			wantPubkey: "aabbcc1122334455aabbcc1122334455aabbcc1122334455aabbcc1122334455",
			wantRole:   "member",
			isGrant:    true,
		},
		{
			name:       "agent grant",
			payload:    `{"pubkey":"1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef","role":"agent"}`,
			wantPubkey: "1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef",
			wantRole:   "agent",
			isGrant:    true,
		},
		{
			name:       "revocation — excluded",
			payload:    `{"pubkey":"aabbcc1122334455aabbcc1122334455aabbcc1122334455aabbcc1122334455","role":"revoked"}`,
			wantPubkey: "aabbcc1122334455aabbcc1122334455aabbcc1122334455aabbcc1122334455",
			wantRole:   "revoked",
			isGrant:    false, // revocations are not grants
		},
		{
			name:       "missing pubkey — excluded",
			payload:    `{"role":"member"}`,
			wantPubkey: "",
			wantRole:   "member",
			isGrant:    false,
		},
		{
			name:    "invalid JSON — excluded",
			payload: `not-json`,
			isGrant: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var payload grantPayload
			err := json.Unmarshal([]byte(tc.payload), &payload)

			if tc.name == "invalid JSON — excluded" {
				if err == nil {
					t.Error("expected JSON parse error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected parse error: %v", err)
			}

			if payload.Pubkey != tc.wantPubkey {
				t.Errorf("pubkey = %q, want %q", payload.Pubkey, tc.wantPubkey)
			}
			if payload.Role != tc.wantRole {
				t.Errorf("role = %q, want %q", payload.Role, tc.wantRole)
			}

			// Apply the same filter as findMembersAdmittedBy.
			isGrant := payload.Role != "revoked" && payload.Pubkey != ""
			if isGrant != tc.isGrant {
				t.Errorf("isGrant = %v, want %v", isGrant, tc.isGrant)
			}
		})
	}
}

// TestRevoke_PubkeyDetection verifies the routing logic that determines
// whether to use the arg as a pubkey directly or to resolve it via naming.
func TestRevoke_PubkeyDetection(t *testing.T) {
	validPubkey := "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890"
	shortHex := "abcdef1234"
	name := "myorg.ready/myproject"

	cases := []struct {
		input       string
		isPubkeyArg bool // true = arg should be treated as raw pubkey
	}{
		{validPubkey, true},
		{shortHex, false},  // too short — not a 64-char pubkey
		{name, false},      // contains non-hex chars — resolve via naming
	}

	for _, tc := range cases {
		isPubkey := len(tc.input) == 64 && isHex(tc.input)
		if isPubkey != tc.isPubkeyArg {
			t.Errorf("input %q: isPubkey = %v, want %v", tc.input, isPubkey, tc.isPubkeyArg)
		}
	}
}

// TestFindMembersAdmittedBy_RealFunctionCallsClient verifies that
// findMembersAdmittedBy calls the real production function with the fake
// client and returns the set of admitted pubkeys (excluding revocations and
// missing pubkeys).
//
// This replaces the earlier no-op test that only checked JSON parsing.
func TestFindMembersAdmittedBy_RealFunctionCallsClient(t *testing.T) {
	memberKey := pubkeyHex("aa")
	agentKey := pubkeyHex("bb")
	revokedKey := pubkeyHex("cc")
	campfire := pubkeyHex("dd")
	senderKey := pubkeyHex("ee")

	fake := &fakeReadClient{
		messages: []protocol.Message{
			makeGrantMsg("msg-member", memberKey, "member"),
			makeGrantMsg("msg-agent", agentKey, "agent"),
			makeGrantMsg("msg-revoked", revokedKey, "revoked"), // should be excluded
			{ID: "msg-nopubkey", Payload: []byte(`{"role":"member"}`), Tags: []string{"work:role-grant"}}, // excluded
		},
	}

	admitted, err := findMembersAdmittedBy(fake, campfire, senderKey)
	if err != nil {
		t.Fatalf("findMembersAdmittedBy: unexpected error: %v", err)
	}

	// Build a set for easy lookup.
	admittedSet := map[string]bool{}
	for _, k := range admitted {
		admittedSet[k] = true
	}

	if !admittedSet[memberKey] {
		t.Errorf("member key %q...: should be in admitted list", memberKey[:12])
	}
	if !admittedSet[agentKey] {
		t.Errorf("agent key %q...: should be in admitted list", agentKey[:12])
	}
	if admittedSet[revokedKey] {
		t.Errorf("revoked key %q...: must NOT be in admitted list", revokedKey[:12])
	}
	if len(admitted) != 2 {
		t.Errorf("admitted count = %d, want 2", len(admitted))
	}
}

// TestFindMembersAdmittedBy_ClientError verifies that findMembersAdmittedBy
// propagates errors from the campfire client.
func TestFindMembersAdmittedBy_ClientError(t *testing.T) {
	campfire := pubkeyHex("dd")
	senderKey := pubkeyHex("ee")

	fake := &fakeReadClient{err: fmt.Errorf("network error")}

	_, err := findMembersAdmittedBy(fake, campfire, senderKey)
	if err == nil {
		t.Fatal("expected error from client, got nil")
	}
}

// TestFindMembersAdmittedBy_Deduplication verifies that when the same pubkey
// appears in multiple grants, it is returned only once.
func TestFindMembersAdmittedBy_Deduplication(t *testing.T) {
	memberKey := pubkeyHex("aa")
	campfire := pubkeyHex("dd")
	senderKey := pubkeyHex("ee")

	fake := &fakeReadClient{
		messages: []protocol.Message{
			makeGrantMsg("msg-1", memberKey, "member"),
			makeGrantMsg("msg-2", memberKey, "agent"), // same key, different role
		},
	}

	admitted, err := findMembersAdmittedBy(fake, campfire, senderKey)
	if err != nil {
		t.Fatalf("findMembersAdmittedBy dedup: unexpected error: %v", err)
	}
	if len(admitted) != 1 {
		t.Errorf("admitted count = %d, want 1 (deduplication)", len(admitted))
	}
}
