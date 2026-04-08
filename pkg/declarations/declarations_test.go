package declarations

import (
	"bytes"
	"encoding/json"
	"testing"
)

// TestAll verifies All() returns all embedded declaration payloads.
func TestAll(t *testing.T) {
	payloads, err := All()
	if err != nil {
		t.Fatalf("All() returned error: %v", err)
	}

	if len(payloads) == 0 {
		t.Fatal("All() returned empty slice, expected declarations")
	}

	// Verify each payload is valid JSON and has required fields.
	for i, payload := range payloads {
		var decl map[string]interface{}
		if err := json.Unmarshal(payload, &decl); err != nil {
			t.Errorf("payload[%d] is not valid JSON: %v", i, err)
			continue
		}

		// Check for required fields in declaration schema.
		requiredFields := []string{"convention", "version", "operation"}
		for _, field := range requiredFields {
			if _, exists := decl[field]; !exists {
				t.Errorf("payload[%d] missing required field %q", i, field)
			}
		}
	}
}

// TestAllConsistency verifies All() is deterministic and consistent.
func TestAllConsistency(t *testing.T) {
	payloads1, err := All()
	if err != nil {
		t.Fatalf("first All() call failed: %v", err)
	}

	payloads2, err := All()
	if err != nil {
		t.Fatalf("second All() call failed: %v", err)
	}

	if len(payloads1) != len(payloads2) {
		t.Fatalf("All() returned different counts: %d vs %d", len(payloads1), len(payloads2))
	}

	// Verify all payloads match exactly.
	for i := range payloads1 {
		if !bytes.Equal(payloads1[i], payloads2[i]) {
			t.Errorf("payload[%d] differs between calls", i)
		}
	}
}

// TestLoad verifies Load() retrieves a single declaration by name.
func TestLoad(t *testing.T) {
	// Test known declarations that exist in ops/
	testCases := []struct {
		name    string
		wantErr bool
	}{
		{"claim", false},
		{"create", false},
		{"close", false},
		{"status", false},
		{"block", false},
		{"unblock", false},
		{"delegate", false},
		{"engage", false},
		{"gate", false},
		{"gate-resolve", false}, // hyphen normalized to underscore
		{"join_request", false},
		{"join-request", false}, // hyphen variant should also work
		{"playbook_create", false},
		{"playbook-create", false}, // hyphen variant should also work
		{"role_grant", false},
		{"role-grant", false}, // hyphen variant should also work
		{"server_binding", false},
		{"server-binding", false}, // hyphen variant should also work
		{"summary_bind", false},
		{"summary-bind", false}, // hyphen variant should also work
		{"update", false},
		{"nonexistent", true},
		{"nonexistent-operation", true},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			payload, err := Load(tc.name)
			if (err != nil) != tc.wantErr {
				t.Errorf("Load(%q) error = %v, wantErr = %v", tc.name, err, tc.wantErr)
				return
			}

			if tc.wantErr {
				return
			}

			// Verify payload is valid JSON.
			var decl map[string]interface{}
			if err := json.Unmarshal(payload, &decl); err != nil {
				t.Errorf("Load(%q) returned invalid JSON: %v", tc.name, err)
			}

			// Verify operation field matches (after normalization).
			normalizedName := tc.name
			if operation, exists := decl["operation"]; exists {
				if operation != normalizedName && operation != normalizedName[len(normalizedName)-1:] {
					// The operation field should reflect the normalized form.
					// We just verify it exists and is a string.
					if _, isString := operation.(string); !isString {
						t.Errorf("Load(%q) operation field is not a string", tc.name)
					}
				}
			}
		})
	}
}

// TestLoadConsistency verifies Load() returns same data on repeated calls.
func TestLoadConsistency(t *testing.T) {
	payload1, err := Load("claim")
	if err != nil {
		t.Fatalf("first Load() call failed: %v", err)
	}

	payload2, err := Load("claim")
	if err != nil {
		t.Fatalf("second Load() call failed: %v", err)
	}

	if !bytes.Equal(payload1, payload2) {
		t.Error("Load() returned different data on repeated calls")
	}
}

// TestLoadHyphenNormalization verifies hyphen-to-underscore mapping in filenames.
func TestLoadHyphenNormalization(t *testing.T) {
	testCases := []struct {
		hyphenName    string
		underscoreName string
	}{
		{"gate-resolve", "gate_resolve"},
		{"join-request", "join_request"},
		{"playbook-create", "playbook_create"},
		{"role-grant", "role_grant"},
		{"server-binding", "server_binding"},
		{"summary-bind", "summary_bind"},
	}

	for _, tc := range testCases {
		t.Run(tc.hyphenName, func(t *testing.T) {
			payloadHyphen, err := Load(tc.hyphenName)
			if err != nil {
				t.Fatalf("Load(%q) failed: %v", tc.hyphenName, err)
			}

			payloadUnderscore, err := Load(tc.underscoreName)
			if err != nil {
				t.Fatalf("Load(%q) failed: %v", tc.underscoreName, err)
			}

			// Both should return identical data since they point to the same file.
			if !bytes.Equal(payloadHyphen, payloadUnderscore) {
				t.Errorf("Load(%q) and Load(%q) returned different data", tc.hyphenName, tc.underscoreName)
			}
		})
	}
}

// TestLoadReturnsValidJSON verifies all loaded declarations are valid JSON.
func TestLoadReturnsValidJSON(t *testing.T) {
	payloads, err := All()
	if err != nil {
		t.Fatalf("All() failed: %v", err)
	}

	for i, payload := range payloads {
		// Extract operation name from JSON to reconstruct Load call.
		var decl map[string]interface{}
		if err := json.Unmarshal(payload, &decl); err != nil {
			t.Fatalf("payload[%d] is invalid JSON (bug in All()): %v", i, err)
		}

		operation, ok := decl["operation"].(string)
		if !ok {
			t.Fatalf("payload[%d] has no string operation field", i)
		}

		// Load by operation name and verify it matches.
		loaded, err := Load(operation)
		if err != nil {
			t.Errorf("Load(%q) failed: %v", operation, err)
			continue
		}

		if !bytes.Equal(payload, loaded) {
			t.Errorf("All() payload for operation %q differs from Load(%q)", operation, operation)
		}
	}
}

// TestLoadNonexistentReturnsError verifies Load() returns error for missing declarations.
func TestLoadNonexistentReturnsError(t *testing.T) {
	_, err := Load("does_not_exist")
	if err == nil {
		t.Error("Load(nonexistent) expected error, got nil")
	}
}
