// Package grant defines admission grants for joining campfires.
// A JoinGrant is an Ed25519-signed structure that authorizes a requester to join a campfire.
package grant

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// JoinGrant represents a single-use admission grant signed by a maintainer.
// It grants a requester permission to join a campfire with a specific role.
type JoinGrant struct {
	// Subject is the Ed25519 public key of the requester being granted admission.
	Subject string `json:"subject"`
	// CampfireID is the ID of the campfire this grant permits joining.
	CampfireID string `json:"campfire"`
	// Role is the role the requester will receive upon joining (e.g. "contributor", "maintainer").
	Role string `json:"role"`
	// IssuedBy is the Ed25519 public key of the maintainer who issued this grant.
	IssuedBy string `json:"issued_by"`
	// IssuedAt is the Unix timestamp when the grant was issued.
	IssuedAt int64 `json:"issued_at"`
	// ExpiresAt is the Unix timestamp when the grant expires.
	ExpiresAt int64 `json:"expires_at"`
	// SingleUse indicates whether this grant can only be used once (always true for JoinGrants).
	SingleUse bool `json:"single_use"`
	// Nonce is a random hex-encoded value preventing replay attacks.
	Nonce string `json:"nonce"`
}

// Signature is the Ed25519 signature of a JoinGrant (hex-encoded).
type Signature string

// VerifyResult contains the outcome of grant verification.
type VerifyResult struct {
	Valid   bool
	Error   error
	Message string
}

// SigningInput prepares the grant for signing by marshaling to JSON.
// This is the exact data that gets signed with the issuer's private key.
func (g *JoinGrant) SigningInput() ([]byte, error) {
	return json.Marshal(g)
}

// Sign creates an Ed25519 signature over the grant using the provided issuer private key.
// Returns the signature as a hex-encoded string.
func (g *JoinGrant) Sign(issuerPrivKey ed25519.PrivateKey) (Signature, error) {
	if len(issuerPrivKey) != ed25519.PrivateKeySize {
		return "", errors.New("invalid private key length")
	}

	data, err := g.SigningInput()
	if err != nil {
		return "", fmt.Errorf("failed to marshal grant: %w", err)
	}

	sig := ed25519.Sign(issuerPrivKey, data)
	return Signature(hex.EncodeToString(sig)), nil
}

// Verify validates the grant offline using the issuer's public key.
// Returns a VerifyResult with detailed information about the verification outcome.
//
// Verification checks (in order):
// 1. Grant signature against the issuer pubkey
// 2. Expiration: expires_at > now
// 3. SingleUse constraint (nonce not already consumed)
//
// Note: SingleUse nonce validation is caller's responsibility via a consumed-nonce tracker.
// This function only checks that the grant is well-formed and not expired.
func Verify(g *JoinGrant, sig Signature, issuerPubKey ed25519.PublicKey) VerifyResult {
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

	if len(issuerPubKey) != ed25519.PublicKeySize {
		return VerifyResult{
			Valid:   false,
			Error:   errors.New("invalid issuer public key length"),
			Message: fmt.Sprintf("pubkey length %d, expected %d", len(issuerPubKey), ed25519.PublicKeySize),
		}
	}

	data, err := g.SigningInput()
	if err != nil {
		return VerifyResult{
			Valid:   false,
			Error:   fmt.Errorf("failed to marshal grant for verification: %w", err),
			Message: "grant marshal error",
		}
	}

	if !ed25519.Verify(issuerPubKey, data, sigBytes) {
		return VerifyResult{
			Valid:   false,
			Error:   errors.New("signature verification failed"),
			Message: "grant signature does not match issuer public key",
		}
	}

	// Check 2: Validate expiration
	now := time.Now().Unix()
	if g.ExpiresAt <= now {
		return VerifyResult{
			Valid:   false,
			Error:   errors.New("grant expired"),
			Message: fmt.Sprintf("grant expired at %d, current time is %d", g.ExpiresAt, now),
		}
	}

	return VerifyResult{
		Valid:   true,
		Error:   nil,
		Message: "grant valid",
	}
}

// IssueGrant creates a new JoinGrant with the specified subject, campfire, and role.
// The issuer public key and nonce must be provided.
// The grant is valid for the specified TTL duration (default 24 hours).
func IssueGrant(subject, campfireID, role, issuerPubKey, nonce string, ttl time.Duration) *JoinGrant {
	if ttl == 0 {
		ttl = 24 * time.Hour
	}
	now := time.Now()
	return &JoinGrant{
		Subject:    subject,
		CampfireID: campfireID,
		Role:       role,
		IssuedBy:   issuerPubKey,
		IssuedAt:   now.Unix(),
		ExpiresAt:  now.Add(ttl).Unix(),
		SingleUse:  true,
		Nonce:      nonce,
	}
}

// Hash returns a SHA256 hash of the grant for debugging/logging purposes.
func (g *JoinGrant) Hash() string {
	data, _ := g.SigningInput()
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}
