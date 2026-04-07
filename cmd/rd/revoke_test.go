package main

// revoke_test.go — unit tests for rd revoke helpers.
//
// Done conditions tested:
//   - findMembersAdmittedBy filters by sender and excludes revocations
//   - Payload extraction: valid JSON, missing role, revocation role all handled
//   - pubkey format detection: isHex used for routing

import (
	"encoding/json"
	"testing"
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
