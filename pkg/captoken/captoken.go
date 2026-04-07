// Package captoken defines short-TTL Ed25519 capability tokens for offline work operations.
// Tokens are issued by the convention server and verified by clients offline without
// a server round-trip.
package captoken

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// Token represents a short-TTL capability token signed by the convention server.
// Clients use tokens to perform offline operations without a server round-trip.
type Token struct {
	// Subject is the Ed25519 public key of the operation's sender.
	Subject string `json:"subject"`
	// CampfireID is the ID of the campfire this token grants access to.
	CampfireID string `json:"campfire"`
	// Role is the role of the token holder (e.g. "contributor", "maintainer").
	Role string `json:"role"`
	// Operations is the list of allowed operations (e.g. "work:create", "work:close").
	Operations []string `json:"operations"`
	// IssuedAt is the Unix timestamp when the token was issued.
	IssuedAt int64 `json:"issued_at"`
	// ExpiresAt is the Unix timestamp when the token expires.
	ExpiresAt int64 `json:"expires_at"`
	// BindingMsgID is the campfire message ID of the server-binding declaration.
	BindingMsgID string `json:"binding_msg_id"`
}

// Signature is the Ed25519 signature of a Token (hex-encoded).
type Signature string

// VerifyResult contains the outcome of token verification.
type VerifyResult struct {
	Valid   bool
	Error   error
	Message string
}

// SigningInput prepares the token for signing by marshaling to JSON.
// This is the exact data that gets signed with the server's private key.
func (t *Token) SigningInput() ([]byte, error) {
	return json.Marshal(t)
}

// Sign creates an Ed25519 signature over the token using the provided private key.
// Returns the signature as a hex-encoded string.
func (t *Token) Sign(privKey ed25519.PrivateKey) (Signature, error) {
	if len(privKey) != ed25519.PrivateKeySize {
		return "", errors.New("invalid private key length")
	}

	data, err := t.SigningInput()
	if err != nil {
		return "", fmt.Errorf("failed to marshal token: %w", err)
	}

	sig := ed25519.Sign(privKey, data)
	return Signature(hex.EncodeToString(sig)), nil
}

// Verify validates the token offline using the bound server's public key.
// Returns a VerifyResult with detailed information about the verification outcome.
//
// Verification checks (in order):
// 1. Token signature against the bound server pubkey
// 2. Expiration: expires_at > now
// 3. Operation is in token.operations
// 4. Subject matches the operation's sender
func Verify(t *Token, sig Signature, serverPubKey ed25519.PublicKey, operation string, operationSender string) VerifyResult {
	// Check 1: Validate signature
	sigBytes, err := hex.DecodeString(string(sig))
	if err != nil {
		return VerifyResult{
			Valid:   false,
			Error:   errors.New("invalid signature encoding"),
			Message: "signature is not valid hex",
		}
	}

	if len(sigBytes) != ed25519.SignatureSize {
		return VerifyResult{
			Valid:   false,
			Error:   errors.New("invalid signature length"),
			Message: fmt.Sprintf("signature length %d, expected %d", len(sigBytes), ed25519.SignatureSize),
		}
	}

	if len(serverPubKey) != ed25519.PublicKeySize {
		return VerifyResult{
			Valid:   false,
			Error:   errors.New("invalid server public key length"),
			Message: fmt.Sprintf("pubkey length %d, expected %d", len(serverPubKey), ed25519.PublicKeySize),
		}
	}

	data, err := t.SigningInput()
	if err != nil {
		return VerifyResult{
			Valid:   false,
			Error:   fmt.Errorf("failed to marshal token for verification: %w", err),
			Message: "token marshal error",
		}
	}

	if !ed25519.Verify(serverPubKey, data, sigBytes) {
		return VerifyResult{
			Valid:   false,
			Error:   errors.New("signature verification failed"),
			Message: "token signature does not match server public key",
		}
	}

	// Check 2: Validate expiration
	now := time.Now().Unix()
	if t.ExpiresAt <= now {
		return VerifyResult{
			Valid:   false,
			Error:   errors.New("token expired"),
			Message: fmt.Sprintf("token expired at %d, current time is %d", t.ExpiresAt, now),
		}
	}

	// Check 3: Operation in token.operations
	if !isOperationAllowed(t.Operations, operation) {
		return VerifyResult{
			Valid:   false,
			Error:   errors.New("operation not allowed by token"),
			Message: fmt.Sprintf("operation %q not in token operations: %v", operation, t.Operations),
		}
	}

	// Check 4: Subject matches sender
	if t.Subject != operationSender {
		return VerifyResult{
			Valid:   false,
			Error:   errors.New("subject mismatch"),
			Message: fmt.Sprintf("token subject %q does not match sender %q", t.Subject, operationSender),
		}
	}

	return VerifyResult{
		Valid:   true,
		Error:   nil,
		Message: "token valid",
	}
}

// isOperationAllowed checks if an operation is in the allowed list.
func isOperationAllowed(operations []string, target string) bool {
	for _, op := range operations {
		if op == target {
			return true
		}
	}
	return false
}

// DefaultTTLFor returns the recommended TTL (in seconds) for a given operation.
// TTLs by operation severity:
// - Bulk safe ops (work:create, work:claim, work:update): 48h
// - Consequential ops (work:close, work:delegate): 24h
// - High-stakes (work:gate-resolve): 4h
func DefaultTTLFor(operation string) time.Duration {
	switch operation {
	case "work:create", "work:claim", "work:update":
		return 48 * time.Hour
	case "work:close", "work:delegate":
		return 24 * time.Hour
	case "work:gate-resolve":
		return 4 * time.Hour
	default:
		return 24 * time.Hour // conservative default
	}
}

// IssueToken creates a new token with the specified operations, subject, and campfire.
// issuedAt and expiresAt are set based on current time and DefaultTTLFor the first operation.
func IssueToken(subject, campfireID, role, bindingMsgID string, operations []string) *Token {
	now := time.Now()
	issuedAt := now.Unix()
	// Use the TTL of the first operation, or 24h as a safe default.
	ttl := DefaultTTLFor(operations[0])
	if len(operations) == 0 {
		ttl = 24 * time.Hour
	}
	expiresAt := now.Add(ttl).Unix()

	return &Token{
		Subject:      subject,
		CampfireID:   campfireID,
		Role:         role,
		Operations:   operations,
		IssuedAt:     issuedAt,
		ExpiresAt:    expiresAt,
		BindingMsgID: bindingMsgID,
	}
}

// Hash returns a SHA256 hash of the token for debugging/logging purposes.
func (t *Token) Hash() string {
	data, _ := t.SigningInput()
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}
