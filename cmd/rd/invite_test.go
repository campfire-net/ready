package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"testing"
	"time"
)

func TestDecodeInviteToken_Valid(t *testing.T) {
	// Generate a keypair for the token.
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	campfireID := pubkeyHex("ab")

	payload := invitePayload{
		Version:    1,
		CampfireID: campfireID,
		PrivateKey: hex.EncodeToString(priv.Seed()),
		Role:       "member",
		IssuedAt:   time.Now().Unix(),
		ExpiresAt:  time.Now().Add(2 * time.Hour).Unix(),
		Issuer:     hex.EncodeToString(pub),
	}

	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}

	token := inviteTokenPrefix + base64.RawURLEncoding.EncodeToString(data)

	decoded, err := decodeInviteToken(token)
	if err != nil {
		t.Fatalf("decodeInviteToken returned error: %v", err)
	}

	if decoded.Version != 1 {
		t.Errorf("version = %d, want 1", decoded.Version)
	}
	if decoded.CampfireID != campfireID {
		t.Errorf("campfire_id = %s, want %s", decoded.CampfireID, campfireID)
	}
	if decoded.Role != "member" {
		t.Errorf("role = %s, want member", decoded.Role)
	}
	if decoded.PrivateKey != hex.EncodeToString(priv.Seed()) {
		t.Error("private_key mismatch")
	}
}

func TestDecodeInviteToken_Expired(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	campfireID := pubkeyHex("cd")

	payload := invitePayload{
		Version:    1,
		CampfireID: campfireID,
		PrivateKey: hex.EncodeToString(priv.Seed()),
		Role:       "member",
		IssuedAt:   time.Now().Add(-3 * time.Hour).Unix(),
		ExpiresAt:  time.Now().Add(-1 * time.Hour).Unix(), // already expired
		Issuer:     pubkeyHex("ee"),
	}

	data, _ := json.Marshal(payload)
	token := inviteTokenPrefix + base64.RawURLEncoding.EncodeToString(data)

	_, err := decodeInviteToken(token)
	if err == nil {
		t.Fatal("expected error for expired token, got nil")
	}
	if got := err.Error(); !containsStr(got, "expired") {
		t.Errorf("error = %q, want to contain 'expired'", got)
	}
}

func TestDecodeInviteToken_InvalidVersion(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	campfireID := pubkeyHex("cd")

	payload := invitePayload{
		Version:    99,
		CampfireID: campfireID,
		PrivateKey: hex.EncodeToString(priv.Seed()),
		Role:       "member",
		IssuedAt:   time.Now().Unix(),
		ExpiresAt:  time.Now().Add(1 * time.Hour).Unix(),
		Issuer:     pubkeyHex("ee"),
	}

	data, _ := json.Marshal(payload)
	token := inviteTokenPrefix + base64.RawURLEncoding.EncodeToString(data)

	_, err := decodeInviteToken(token)
	if err == nil {
		t.Fatal("expected error for unsupported version, got nil")
	}
	if got := err.Error(); !containsStr(got, "unsupported token version") {
		t.Errorf("error = %q, want to contain 'unsupported token version'", got)
	}
}

func TestDecodeInviteToken_InvalidCampfireID(t *testing.T) {
	_, priv, _ := ed25519.GenerateKey(rand.Reader)

	payload := invitePayload{
		Version:    1,
		CampfireID: "too-short",
		PrivateKey: hex.EncodeToString(priv.Seed()),
		Role:       "member",
		IssuedAt:   time.Now().Unix(),
		ExpiresAt:  time.Now().Add(1 * time.Hour).Unix(),
		Issuer:     pubkeyHex("ee"),
	}

	data, _ := json.Marshal(payload)
	token := inviteTokenPrefix + base64.RawURLEncoding.EncodeToString(data)

	_, err := decodeInviteToken(token)
	if err == nil {
		t.Fatal("expected error for invalid campfire_id, got nil")
	}
	if got := err.Error(); !containsStr(got, "invalid campfire_id") {
		t.Errorf("error = %q, want to contain 'invalid campfire_id'", got)
	}
}

func TestDecodeInviteToken_InvalidPrivateKey(t *testing.T) {
	campfireID := pubkeyHex("ab")

	payload := invitePayload{
		Version:    1,
		CampfireID: campfireID,
		PrivateKey: "not-a-valid-hex-key",
		Role:       "member",
		IssuedAt:   time.Now().Unix(),
		ExpiresAt:  time.Now().Add(1 * time.Hour).Unix(),
		Issuer:     pubkeyHex("ee"),
	}

	data, _ := json.Marshal(payload)
	token := inviteTokenPrefix + base64.RawURLEncoding.EncodeToString(data)

	_, err := decodeInviteToken(token)
	if err == nil {
		t.Fatal("expected error for invalid private_key, got nil")
	}
	if got := err.Error(); !containsStr(got, "invalid private_key") {
		t.Errorf("error = %q, want to contain 'invalid private_key'", got)
	}
}

func TestDecodeInviteToken_TooShort(t *testing.T) {
	_, err := decodeInviteToken("rdx1_")
	if err == nil {
		t.Fatal("expected error for too-short token, got nil")
	}
}

func TestDecodeInviteToken_BadBase64(t *testing.T) {
	_, err := decodeInviteToken("rdx1_!!!invalid-base64!!!")
	if err == nil {
		t.Fatal("expected error for bad base64, got nil")
	}
}

func TestDecodeInviteToken_BadJSON(t *testing.T) {
	token := inviteTokenPrefix + base64.RawURLEncoding.EncodeToString([]byte("not json"))
	_, err := decodeInviteToken(token)
	if err == nil {
		t.Fatal("expected error for bad JSON, got nil")
	}
}

func TestInviteTokenRoundTrip_KeyReconstruction(t *testing.T) {
	// Verify that a key encoded in the token can be reconstructed correctly.
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	seed := priv.Seed()
	seedHex := hex.EncodeToString(seed)

	// Reconstruct.
	decodedSeed, err := hex.DecodeString(seedHex)
	if err != nil {
		t.Fatal(err)
	}
	reconstructed := ed25519.NewKeyFromSeed(decodedSeed)

	// The reconstructed key should produce identical signatures.
	msg := []byte("test message")
	sig1 := ed25519.Sign(priv, msg)
	sig2 := ed25519.Sign(reconstructed, msg)

	if !ed25519.Verify(priv.Public().(ed25519.PublicKey), msg, sig2) {
		t.Error("reconstructed key signature not verified by original pubkey")
	}
	if !ed25519.Verify(reconstructed.Public().(ed25519.PublicKey), msg, sig1) {
		t.Error("original key signature not verified by reconstructed pubkey")
	}
}

func TestDecodeInviteToken_WrongCampfireIDLength(t *testing.T) {
	// 63 hex chars — one short.
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	payload := invitePayload{
		Version:    1,
		CampfireID: pubkeyHex("ab")[:63],
		PrivateKey: hex.EncodeToString(priv.Seed()),
		Role:       "member",
		IssuedAt:   time.Now().Unix(),
		ExpiresAt:  time.Now().Add(1 * time.Hour).Unix(),
		Issuer:     pubkeyHex("ee"),
	}
	data, _ := json.Marshal(payload)
	token := inviteTokenPrefix + base64.RawURLEncoding.EncodeToString(data)

	_, err := decodeInviteToken(token)
	if err == nil {
		t.Fatal("expected error for 63-char campfire_id")
	}
}

// containsStr is already defined in aliases_test.go — reuse it.
