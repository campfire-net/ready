package captoken

import (
	"crypto/ed25519"
	"testing"
	"time"
)

// TestTokenSignAndVerify tests that a token signed by the server key can be verified offline.
func TestTokenSignAndVerify(t *testing.T) {
	// Generate server keypair
	serverPubKey, serverPrivKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate server keypair: %v", err)
	}

	// Generate client keypair for the subject
	clientPubKey, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate client keypair: %v", err)
	}

	// Create a token
	token := &Token{
		Subject:      clientPubKey.String(),
		CampfireID:   "test-campfire-123",
		Role:         "contributor",
		Operations:   []string{"work:create", "work:claim", "work:update"},
		IssuedAt:     time.Now().Unix(),
		ExpiresAt:    time.Now().Add(48 * time.Hour).Unix(),
		BindingMsgID: "binding-msg-456",
	}

	// Sign the token with server private key
	sig, err := token.Sign(serverPrivKey)
	if err != nil {
		t.Fatalf("failed to sign token: %v", err)
	}

	// Verify the token offline
	result := Verify(token, sig, serverPubKey, "work:create", clientPubKey.String())
	if !result.Valid {
		t.Errorf("token verification failed: %v", result.Message)
	}
}

// TestExpiredTokenRejected tests that an expired token is rejected.
func TestExpiredTokenRejected(t *testing.T) {
	serverPubKey, serverPrivKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate server keypair: %v", err)
	}

	clientPubKey, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate client keypair: %v", err)
	}

	// Create an already-expired token
	now := time.Now().Unix()
	token := &Token{
		Subject:      clientPubKey.String(),
		CampfireID:   "test-campfire-123",
		Role:         "contributor",
		Operations:   []string{"work:create"},
		IssuedAt:     now - 3600,
		ExpiresAt:    now - 1, // Already expired
		BindingMsgID: "binding-msg-456",
	}

	sig, err := token.Sign(serverPrivKey)
	if err != nil {
		t.Fatalf("failed to sign token: %v", err)
	}

	result := Verify(token, sig, serverPubKey, "work:create", clientPubKey.String())
	if result.Valid {
		t.Errorf("expected expired token to be rejected")
	}
	if result.Error == nil || result.Error.Error() != "token expired" {
		t.Errorf("expected 'token expired' error, got: %v", result.Error)
	}
}

// TestWrongOperationRejected tests that an operation not in the token is rejected.
func TestWrongOperationRejected(t *testing.T) {
	serverPubKey, serverPrivKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate server keypair: %v", err)
	}

	clientPubKey, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate client keypair: %v", err)
	}

	// Create a token that only allows work:create
	token := &Token{
		Subject:      clientPubKey.String(),
		CampfireID:   "test-campfire-123",
		Role:         "contributor",
		Operations:   []string{"work:create"}, // Only work:create
		IssuedAt:     time.Now().Unix(),
		ExpiresAt:    time.Now().Add(24 * time.Hour).Unix(),
		BindingMsgID: "binding-msg-456",
	}

	sig, err := token.Sign(serverPrivKey)
	if err != nil {
		t.Fatalf("failed to sign token: %v", err)
	}

	// Try to use work:close, which is not allowed
	result := Verify(token, sig, serverPubKey, "work:close", clientPubKey.String())
	if result.Valid {
		t.Errorf("expected operation not in token to be rejected")
	}
	if result.Error == nil || result.Error.Error() != "operation not allowed by token" {
		t.Errorf("expected 'operation not allowed by token' error, got: %v", result.Error)
	}
}

// TestMismatchedSubjectRejected tests that a token used by the wrong subject is rejected.
func TestMismatchedSubjectRejected(t *testing.T) {
	serverPubKey, serverPrivKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate server keypair: %v", err)
	}

	clientPubKey, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate client keypair: %v", err)
	}

	otherPubKey, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate other keypair: %v", err)
	}

	// Create a token for clientPubKey
	token := &Token{
		Subject:      clientPubKey.String(),
		CampfireID:   "test-campfire-123",
		Role:         "contributor",
		Operations:   []string{"work:create"},
		IssuedAt:     time.Now().Unix(),
		ExpiresAt:    time.Now().Add(24 * time.Hour).Unix(),
		BindingMsgID: "binding-msg-456",
	}

	sig, err := token.Sign(serverPrivKey)
	if err != nil {
		t.Fatalf("failed to sign token: %v", err)
	}

	// Try to verify with a different sender
	result := Verify(token, sig, serverPubKey, "work:create", otherPubKey.String())
	if result.Valid {
		t.Errorf("expected mismatched subject to be rejected")
	}
	if result.Error == nil || result.Error.Error() != "subject mismatch" {
		t.Errorf("expected 'subject mismatch' error, got: %v", result.Error)
	}
}

// TestWrongServerKeyRejected tests that a token with the wrong server key fails verification.
func TestWrongServerKeyRejected(t *testing.T) {
	serverPubKey, serverPrivKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate server keypair: %v", err)
	}

	wrongPubKey, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate wrong keypair: %v", err)
	}

	clientPubKey, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("failed to generate client keypair: %v", err)
	}

	// Create and sign token with correct key
	token := &Token{
		Subject:      clientPubKey.String(),
		CampfireID:   "test-campfire-123",
		Role:         "contributor",
		Operations:   []string{"work:create"},
		IssuedAt:     time.Now().Unix(),
		ExpiresAt:    time.Now().Add(24 * time.Hour).Unix(),
		BindingMsgID: "binding-msg-456",
	}

	sig, err := token.Sign(serverPrivKey)
	if err != nil {
		t.Fatalf("failed to sign token: %v", err)
	}

	// Try to verify with wrong server key
	result := Verify(token, sig, wrongPubKey, "work:create", clientPubKey.String())
	if result.Valid {
		t.Errorf("expected wrong server key to fail verification")
	}
	if result.Error == nil || result.Error.Error() != "signature verification failed" {
		t.Errorf("expected 'signature verification failed' error, got: %v", result.Error)
	}
}

// TestDefaultTTLFor tests that the correct TTL is returned for each operation type.
func TestDefaultTTLFor(t *testing.T) {
	tests := []struct {
		operation string
		expected  time.Duration
	}{
		{"work:create", 48 * time.Hour},
		{"work:claim", 48 * time.Hour},
		{"work:update", 48 * time.Hour},
		{"work:close", 24 * time.Hour},
		{"work:delegate", 24 * time.Hour},
		{"work:gate-resolve", 4 * time.Hour},
		{"unknown:operation", 24 * time.Hour}, // conservative default
	}

	for _, tt := range tests {
		t.Run(tt.operation, func(t *testing.T) {
			ttl := DefaultTTLFor(tt.operation)
			if ttl != tt.expected {
				t.Errorf("operation %q: expected TTL %v, got %v", tt.operation, tt.expected, ttl)
			}
		})
	}
}

// TestIssueToken tests that IssueToken creates a token with correct TTL and timestamps.
func TestIssueToken(t *testing.T) {
	clientPubKey, _, _ := ed25519.GenerateKey(nil)
	operations := []string{"work:create", "work:claim"}

	token := IssueToken(clientPubKey.String(), "test-cf", "contributor", "binding-123", operations)

	if token.Subject != clientPubKey.String() {
		t.Errorf("subject mismatch: expected %q, got %q", clientPubKey.String(), token.Subject)
	}
	if token.CampfireID != "test-cf" {
		t.Errorf("campfire ID mismatch: expected %q, got %q", "test-cf", token.CampfireID)
	}
	if token.Role != "contributor" {
		t.Errorf("role mismatch: expected %q, got %q", "contributor", token.Role)
	}

	// Check that TTL is approximately 48h (for work:create)
	expectedExpiry := time.Now().Add(48 * time.Hour)
	actualExpiry := time.Unix(token.ExpiresAt, 0)
	diff := expectedExpiry.Sub(actualExpiry).Abs()
	if diff > 5*time.Second { // Allow 5 seconds of variance
		t.Errorf("expiry time off by %v, expected ~%v, got %v", diff, expectedExpiry, actualExpiry)
	}

	// Check that issuedAt is approximately now
	expectedIssued := time.Now()
	actualIssued := time.Unix(token.IssuedAt, 0)
	diff = expectedIssued.Sub(actualIssued).Abs()
	if diff > 5*time.Second {
		t.Errorf("issued time off by %v, expected ~%v, got %v", diff, expectedIssued, actualIssued)
	}
}

// TestTokenHash tests that Hash() returns a consistent hex string.
func TestTokenHash(t *testing.T) {
	token := &Token{
		Subject:      "test-subject-key",
		CampfireID:   "test-campfire-123",
		Role:         "contributor",
		Operations:   []string{"work:create"},
		IssuedAt:     1000,
		ExpiresAt:    2000,
		BindingMsgID: "binding-456",
	}

	hash1 := token.Hash()
	hash2 := token.Hash()

	if hash1 != hash2 {
		t.Errorf("hash should be deterministic: %q != %q", hash1, hash2)
	}

	// Hash should be hex-encoded
	if len(hash1) != 64 { // SHA256 hex = 64 chars
		t.Errorf("expected 64-char hex hash, got %d chars: %q", len(hash1), hash1)
	}
}

// TestMultipleOperations tests that a token can verify multiple operations.
func TestMultipleOperations(t *testing.T) {
	serverPubKey, serverPrivKey, _ := ed25519.GenerateKey(nil)
	clientPubKey, _, _ := ed25519.GenerateKey(nil)

	token := &Token{
		Subject:      clientPubKey.String(),
		CampfireID:   "test-campfire-123",
		Role:         "contributor",
		Operations:   []string{"work:create", "work:claim", "work:update"},
		IssuedAt:     time.Now().Unix(),
		ExpiresAt:    time.Now().Add(48 * time.Hour).Unix(),
		BindingMsgID: "binding-msg-456",
	}

	sig, _ := token.Sign(serverPrivKey)

	// Test each operation
	for _, op := range []string{"work:create", "work:claim", "work:update"} {
		result := Verify(token, sig, serverPubKey, op, clientPubKey.String())
		if !result.Valid {
			t.Errorf("operation %q should be valid: %v", op, result.Message)
		}
	}

	// Test an operation not in the token
	result := Verify(token, sig, serverPubKey, "work:close", clientPubKey.String())
	if result.Valid {
		t.Errorf("operation work:close should be rejected")
	}
}
