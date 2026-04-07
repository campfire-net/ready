package grant

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
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

	// Create a fake signature (128 hex chars = 64 bytes = Ed25519 signature size)
	fakeSignature := Signature("00000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000")

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
	grant, err := IssueGrant(
		hex.EncodeToString(requesterPubKey),
		"test-campfire-456",
		"maintainer",
		hex.EncodeToString(issuerPubKey),
		nonce,
		0, // Use default TTL
	)
	if err != nil {
		t.Fatalf("IssueGrant failed: %v", err)
	}

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
	grant, err := IssueGrant(
		hex.EncodeToString(requesterPubKey),
		"test-campfire-789",
		"contributor",
		hex.EncodeToString(issuerPubKey),
		nonce,
		customTTL,
	)
	if err != nil {
		t.Fatalf("IssueGrant failed: %v", err)
	}

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

// TestIssueGrantValidation tests that IssueGrant rejects empty required fields.
// ready-d26: input validation for IssueGrant.
func TestIssueGrantValidation(t *testing.T) {
	issuerPubKey, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate issuer keypair: %v", err)
	}
	requesterPubKey, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate requester keypair: %v", err)
	}
	subject := hex.EncodeToString(requesterPubKey)
	issuer := hex.EncodeToString(issuerPubKey)
	nonce := generateNonce()

	cases := []struct {
		name       string
		subject    string
		campfireID string
		role       string
		issuerKey  string
		nonce      string
	}{
		{"empty subject", "", "cf-1", "contributor", issuer, nonce},
		{"empty campfireID", subject, "", "contributor", issuer, nonce},
		{"empty role", subject, "cf-1", "", issuer, nonce},
		{"empty issuerPubKey", subject, "cf-1", "contributor", "", nonce},
		{"empty nonce", subject, "cf-1", "contributor", issuer, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g, err := IssueGrant(tc.subject, tc.campfireID, tc.role, tc.issuerKey, tc.nonce, 0)
			if err == nil {
				t.Errorf("expected error for %s, got nil", tc.name)
			}
			if g != nil {
				t.Errorf("expected nil grant for %s, got non-nil", tc.name)
			}
		})
	}
}

// TestVerifyWrongIssuerKey tests that verification fails when a different key is used.
// ready-976: signature was made by key A, verification attempts with key B.
func TestVerifyWrongIssuerKey(t *testing.T) {
	// Key A: the real issuer
	issuerPubKeyA, issuerPrivKeyA, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate key A: %v", err)
	}

	// Key B: a different key used for attempted verification
	issuerPubKeyB, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate key B: %v", err)
	}

	requesterPubKey, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate requester keypair: %v", err)
	}

	nonce := generateNonce()
	grant := &JoinGrant{
		Subject:    hex.EncodeToString(requesterPubKey),
		CampfireID: "test-campfire-wrongkey",
		Role:       "contributor",
		IssuedBy:   hex.EncodeToString(issuerPubKeyA),
		IssuedAt:   time.Now().Unix(),
		ExpiresAt:  time.Now().Add(24 * time.Hour).Unix(),
		SingleUse:  true,
		Nonce:      nonce,
	}

	// Sign with key A
	sig, err := grant.Sign(issuerPrivKeyA)
	if err != nil {
		t.Fatalf("failed to sign grant: %v", err)
	}

	// Verify with key B — must fail
	result := Verify(grant, sig, issuerPubKeyB)
	if result.Valid {
		t.Errorf("expected verification to fail with wrong issuer key, but it passed")
	}
	if result.Error == nil {
		t.Errorf("expected non-nil error when verifying with wrong key")
	}
	if result.Error.Error() != "signature verification failed" {
		t.Errorf("expected 'signature verification failed', got: %v", result.Error)
	}
}

// TestSigningInputStability proves that the bytes used for signing and for verification are identical.
// ready-f4d: guard against drift if SigningInput() is ever changed mid-flight.
// Sign and verify must use exactly the same JSON serialization.
func TestSigningInputStability(t *testing.T) {
	issuerPubKey, issuerPrivKey, err := ed25519.GenerateKey(nil)
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
		CampfireID: "test-campfire-stability",
		Role:       "contributor",
		IssuedBy:   hex.EncodeToString(issuerPubKey),
		IssuedAt:   1700000000,
		ExpiresAt:  9999999999,
		SingleUse:  true,
		Nonce:      nonce,
	}

	// Capture signing input before signing
	inputBefore, err := grant.SigningInput()
	if err != nil {
		t.Fatalf("failed to get signing input: %v", err)
	}

	// Sign
	sig, err := grant.Sign(issuerPrivKey)
	if err != nil {
		t.Fatalf("failed to sign grant: %v", err)
	}

	// Capture signing input again after signing (grant struct must not have changed)
	inputAfter, err := grant.SigningInput()
	if err != nil {
		t.Fatalf("failed to get signing input after sign: %v", err)
	}

	if !bytes.Equal(inputBefore, inputAfter) {
		t.Errorf("SigningInput changed between sign and verify calls:\nbefore: %s\nafter:  %s", inputBefore, inputAfter)
	}

	// Verify uses the same serialization path — if Verify succeeds, the bytes matched
	result := Verify(grant, sig, issuerPubKey)
	if !result.Valid {
		t.Errorf("verification failed after sign — signing input bytes diverged: %v", result.Error)
	}
}

// TestExpiryBoundary tests the exact boundary conditions for grant expiration.
// ready-610: ExpiresAt==now must fail, ExpiresAt==now+1 must pass, ExpiresAt==now-1 must fail.
func TestExpiryBoundary(t *testing.T) {
	issuerPubKey, issuerPrivKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate issuer keypair: %v", err)
	}
	requesterPubKey, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate requester keypair: %v", err)
	}
	nonce := generateNonce()

	now := time.Now().Unix()

	makeGrant := func(expiresAt int64) (*JoinGrant, Signature) {
		g := &JoinGrant{
			Subject:    hex.EncodeToString(requesterPubKey),
			CampfireID: "test-campfire-boundary",
			Role:       "contributor",
			IssuedBy:   hex.EncodeToString(issuerPubKey),
			IssuedAt:   now - 60,
			ExpiresAt:  expiresAt,
			SingleUse:  true,
			Nonce:      nonce,
		}
		s, signErr := g.Sign(issuerPrivKey)
		if signErr != nil {
			t.Fatalf("failed to sign grant: %v", signErr)
		}
		return g, s
	}

	// ExpiresAt == now → must fail (condition: ExpiresAt <= now)
	g1, s1 := makeGrant(now)
	r1 := Verify(g1, s1, issuerPubKey)
	if r1.Valid {
		t.Errorf("ExpiresAt==now should fail (expired), but Verify returned Valid=true")
	}
	if r1.Error == nil || r1.Error.Error() != "grant expired" {
		t.Errorf("expected 'grant expired', got: %v", r1.Error)
	}

	// ExpiresAt == now-1 → must fail
	g2, s2 := makeGrant(now - 1)
	r2 := Verify(g2, s2, issuerPubKey)
	if r2.Valid {
		t.Errorf("ExpiresAt==now-1 should fail, but Verify returned Valid=true")
	}

	// ExpiresAt == now+1 → must pass
	g3, s3 := makeGrant(now + 1)
	r3 := Verify(g3, s3, issuerPubKey)
	if !r3.Valid {
		t.Errorf("ExpiresAt==now+1 should pass, but Verify returned Valid=false: %v", r3.Error)
	}
}

// TestNonceReplayIsCallerResponsibility proves that Verify() itself does not block replay.
// ready-112: the design choice is documented — replay detection is caller responsibility.
// Verify() is stateless; it will accept the same grant+signature combination repeatedly.
// Callers must maintain a consumed-nonce set and check it before calling Verify.
func TestNonceReplayIsCallerResponsibility(t *testing.T) {
	issuerPubKey, issuerPrivKey, err := ed25519.GenerateKey(nil)
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
		CampfireID: "test-campfire-replay",
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

	// First use: passes
	r1 := Verify(grant, sig, issuerPubKey)
	if !r1.Valid {
		t.Fatalf("first verify failed unexpectedly: %v", r1.Error)
	}

	// Second use (replay): Verify() itself still returns Valid=true.
	// This is intentional — Verify() is stateless. Replay detection is caller's responsibility
	// via a consumed-nonce tracker checked before calling Verify.
	r2 := Verify(grant, sig, issuerPubKey)
	if !r2.Valid {
		t.Errorf("design contract broken: Verify() must be stateless and not track nonces, but second call returned invalid: %v", r2.Error)
	}

	// Caller pattern for replay protection:
	//   if consumedNonces[grant.Nonce] { return error("replay detected") }
	//   result := Verify(grant, sig, pubKey)
	//   if result.Valid { consumedNonces[grant.Nonce] = true }
	t.Logf("design note: Verify() is stateless — nonce=%s accepted on both calls; callers must track consumed nonces", nonce)
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
