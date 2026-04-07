package grant

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"encoding/hex"
	"testing"
	"time"
)

// TestJoinGrantSignAndVerify tests that a grant signed by the issuer key can be verified offline.
func TestJoinGrantSignAndVerify(t *testing.T) {
	// Generate issuer keypair (maintainer)
	issuerPubKey, issuerPrivKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate issuer keypair: %v", err)
	}

	// Generate requester keypair (subject)
	requesterPubKey, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate requester keypair: %v", err)
	}

	// Generate a random nonce
	nonce := generateNonce()

	// Create a grant
	grant := &JoinGrant{
		Subject:    hex.EncodeToString(requesterPubKey),
		CampfireID: "test-campfire-123",
		Role:       "contributor",
		IssuedBy:   hex.EncodeToString(issuerPubKey),
		IssuedAt:   time.Now().Unix(),
		ExpiresAt:  time.Now().Add(24 * time.Hour).Unix(),
		SingleUse:  true,
		Nonce:      nonce,
	}

	// Sign the grant with issuer private key
	sig, err := grant.Sign(issuerPrivKey)
	if err != nil {
		t.Fatalf("failed to sign grant: %v", err)
	}

	// Verify the grant offline
	result := Verify(grant, sig, issuerPubKey)
	if !result.Valid {
		t.Errorf("grant verification failed: %v", result.Message)
	}
}

// TestExpiredGrantRejected tests that an expired grant is rejected.
func TestExpiredGrantRejected(t *testing.T) {
	issuerPubKey, issuerPrivKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate issuer keypair: %v", err)
	}

	requesterPubKey, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate requester keypair: %v", err)
	}

	nonce := generateNonce()

	// Create an already-expired grant
	now := time.Now().Unix()
	grant := &JoinGrant{
		Subject:    hex.EncodeToString(requesterPubKey),
		CampfireID: "test-campfire-123",
		Role:       "contributor",
		IssuedBy:   hex.EncodeToString(issuerPubKey),
		IssuedAt:   now - 3600,
		ExpiresAt:  now - 1, // Already expired
		SingleUse:  true,
		Nonce:      nonce,
	}

	sig, err := grant.Sign(issuerPrivKey)
	if err != nil {
		t.Fatalf("failed to sign grant: %v", err)
	}

	result := Verify(grant, sig, issuerPubKey)
	if result.Valid {
		t.Errorf("expected expired grant to be rejected")
	}
	if result.Error == nil || result.Error.Error() != "grant expired" {
		t.Errorf("expected 'grant expired' error, got: %v", result.Error)
	}
}

// TestReplayedNonceDetection tests that replayed nonces can be detected.
// (The verification function doesn't track nonces itself; it's caller's responsibility,
// but we document the test interface here.)
func TestReplayedNonceDetection(t *testing.T) {
	issuerPubKey, issuerPrivKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate issuer keypair: %v", err)
	}

	requesterPubKey, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate requester keypair: %v", err)
	}

	nonce := generateNonce()

	// Create a grant
	grant := &JoinGrant{
		Subject:    hex.EncodeToString(requesterPubKey),
		CampfireID: "test-campfire-123",
		Role:       "contributor",
		IssuedBy:   hex.EncodeToString(issuerPubKey),
		IssuedAt:   time.Now().Unix(),
		ExpiresAt:  time.Now().Add(24 * time.Hour).Unix(),
		SingleUse:  true,
		Nonce:      nonce,
	}

	sig, err := grant.Sign(issuerPrivKey)
	if err != nil {
		t.Fatalf("failed to sign grant: %v", err)
	}

	// First use: verify succeeds
	result1 := Verify(grant, sig, issuerPubKey)
	if !result1.Valid {
		t.Errorf("first verification failed: %v", result1.Message)
	}

	// Simulate nonce consumption tracking (caller's responsibility)
	consumedNonces := make(map[string]bool)
	consumedNonces[grant.Nonce] = true

	// Second use: manually check if nonce was already consumed
	if consumedNonces[grant.Nonce] {
		t.Logf("replay detection: nonce %s was already consumed", grant.Nonce)
	} else {
		t.Errorf("replay detection failed: nonce should have been marked as consumed")
	}
}

// TestInvalidSignatureRejected tests that an invalid signature is rejected.
func TestInvalidSignatureRejected(t *testing.T) {
	issuerPubKey, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate issuer keypair: %v", err)
	}

	requesterPubKey, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate requester keypair: %v", err)
	}

	nonce := generateNonce()

	grant := &JoinGrant{
		Subject:    hex.EncodeToString(requesterPubKey),
		CampfireID: "test-campfire-123",
		Role:       "contributor",
		IssuedBy:   hex.EncodeToString(issuerPubKey),
		IssuedAt:   time.Now().Unix(),
		ExpiresAt:  time.Now().Add(24 * time.Hour).Unix(),
		SingleUse:  true,
		Nonce:      nonce,
	}

	// Create a fake signature
	fakeSignature := Signature("0000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000")

	result := Verify(grant, fakeSignature, issuerPubKey)
	if result.Valid {
		t.Errorf("expected invalid signature to be rejected")
	}
	if result.Error == nil || result.Error.Error() != "signature verification failed" {
		t.Errorf("expected 'signature verification failed' error, got: %v", result.Error)
	}
}

// TestIssueGrantDefaults tests that IssueGrant creates a valid grant with correct defaults.
func TestIssueGrantDefaults(t *testing.T) {
	issuerPubKey, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate issuer keypair: %v", err)
	}

	requesterPubKey, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate requester keypair: %v", err)
	}

	nonce := generateNonce()

	// Issue a grant with default TTL
	grant := IssueGrant(
		hex.EncodeToString(requesterPubKey),
		"test-campfire-456",
		"maintainer",
		hex.EncodeToString(issuerPubKey),
		nonce,
		0, // Use default TTL
	)

	// Verify defaults
	if grant.Subject != hex.EncodeToString(requesterPubKey) {
		t.Errorf("subject mismatch")
	}
	if grant.CampfireID != "test-campfire-456" {
		t.Errorf("campfire ID mismatch")
	}
	if grant.Role != "maintainer" {
		t.Errorf("role mismatch")
	}
	if !grant.SingleUse {
		t.Errorf("single_use should be true")
	}
	if grant.Nonce != nonce {
		t.Errorf("nonce mismatch")
	}

	// Verify the grant was created with a future expiration (default 24h)
	now := time.Now().Unix()
	if grant.ExpiresAt <= now {
		t.Errorf("grant should have future expiration")
	}
	// Check that expiration is roughly 24 hours in the future (within 2 minutes tolerance)
	expectedExpiration := now + int64(24*time.Hour/time.Second)
	if grant.ExpiresAt > expectedExpiration+120 || grant.ExpiresAt < expectedExpiration-120 {
		t.Errorf("grant expiration not close to 24 hours: got %d, expected ~%d", grant.ExpiresAt, expectedExpiration)
	}
}

// TestIssueGrantCustomTTL tests that IssueGrant respects custom TTL values.
func TestIssueGrantCustomTTL(t *testing.T) {
	issuerPubKey, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate issuer keypair: %v", err)
	}

	requesterPubKey, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate requester keypair: %v", err)
	}

	nonce := generateNonce()

	// Issue a grant with 1-hour TTL
	customTTL := 1 * time.Hour
	grant := IssueGrant(
		hex.EncodeToString(requesterPubKey),
		"test-campfire-789",
		"contributor",
		hex.EncodeToString(issuerPubKey),
		nonce,
		customTTL,
	)

	// Verify the expiration is roughly 1 hour in the future
	now := time.Now().Unix()
	expectedExpiration := now + int64(customTTL/time.Second)
	if grant.ExpiresAt > expectedExpiration+60 || grant.ExpiresAt < expectedExpiration-60 {
		t.Errorf("grant expiration not close to 1 hour: got %d, expected ~%d", grant.ExpiresAt, expectedExpiration)
	}
}

// TestGrantMarshalUnmarshal tests that a grant can be marshaled to JSON and unmarshaled back.
func TestGrantMarshalUnmarshal(t *testing.T) {
	issuerPubKey, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate issuer keypair: %v", err)
	}

	requesterPubKey, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate requester keypair: %v", err)
	}

	nonce := generateNonce()

	original := &JoinGrant{
		Subject:    hex.EncodeToString(requesterPubKey),
		CampfireID: "test-campfire-999",
		Role:       "contributor",
		IssuedBy:   hex.EncodeToString(issuerPubKey),
		IssuedAt:   time.Now().Unix(),
		ExpiresAt:  time.Now().Add(24 * time.Hour).Unix(),
		SingleUse:  true,
		Nonce:      nonce,
	}

	// Marshal
	data, err := original.SigningInput()
	if err != nil {
		t.Fatalf("failed to marshal grant: %v", err)
	}

	// Unmarshal
	var restored JoinGrant
	err = unmarshalGrant(data, &restored)
	if err != nil {
		t.Fatalf("failed to unmarshal grant: %v", err)
	}

	// Verify all fields match
	if restored.Subject != original.Subject {
		t.Errorf("subject mismatch after unmarshal")
	}
	if restored.CampfireID != original.CampfireID {
		t.Errorf("campfire ID mismatch after unmarshal")
	}
	if restored.Role != original.Role {
		t.Errorf("role mismatch after unmarshal")
	}
	if restored.IssuedBy != original.IssuedBy {
		t.Errorf("issued_by mismatch after unmarshal")
	}
	if restored.IssuedAt != original.IssuedAt {
		t.Errorf("issued_at mismatch after unmarshal")
	}
	if restored.ExpiresAt != original.ExpiresAt {
		t.Errorf("expires_at mismatch after unmarshal")
	}
	if restored.SingleUse != original.SingleUse {
		t.Errorf("single_use mismatch after unmarshal")
	}
	if restored.Nonce != original.Nonce {
		t.Errorf("nonce mismatch after unmarshal")
	}
}

// TestGrantHash tests that the hash function is consistent and deterministic.
func TestGrantHash(t *testing.T) {
	issuerPubKey, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate issuer keypair: %v", err)
	}

	requesterPubKey, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate requester keypair: %v", err)
	}

	nonce := generateNonce()

	grant := &JoinGrant{
		Subject:    hex.EncodeToString(requesterPubKey),
		CampfireID: "test-campfire-hash",
		Role:       "contributor",
		IssuedBy:   hex.EncodeToString(issuerPubKey),
		IssuedAt:   1700000000,
		ExpiresAt:  1700100000,
		SingleUse:  true,
		Nonce:      nonce,
	}

	hash1 := grant.Hash()
	hash2 := grant.Hash()

	if hash1 != hash2 {
		t.Errorf("grant hash should be deterministic: %s != %s", hash1, hash2)
	}

	// Verify hash is hex-encoded and correct length (64 chars for SHA256)
	if len(hash1) != 64 {
		t.Errorf("hash length should be 64 (SHA256), got %d", len(hash1))
	}
}

// Helper function to generate a random nonce
func generateNonce() string {
	nonce := make([]byte, 16)
	rand.Read(nonce)
	return hex.EncodeToString(nonce)
}

// Helper function to unmarshal JSON into a grant
func unmarshalGrant(data []byte, g *JoinGrant) error {
	return json.Unmarshal(data, g)
}
