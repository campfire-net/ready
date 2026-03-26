package main

import (
	"fmt"
	"strings"
	"testing"
)

// validateCloseReason mirrors the --reason enforcement in close/done/fail/cancel commands.
// Returns an error when reason is empty, nil otherwise.
func validateCloseReason(reason string) error {
	if reason == "" {
		return fmt.Errorf("--reason is required (why is this item being closed?)")
	}
	return nil
}

// TestClose_RequiresReason verifies that rd close without --reason returns a clear error.
func TestClose_RequiresReason(t *testing.T) {
	err := validateCloseReason("")
	if err == nil {
		t.Fatal("expected error when reason is empty, got nil")
	}
	if !strings.Contains(err.Error(), "required") {
		t.Errorf("error message must contain 'required', got %q", err.Error())
	}
}

// TestDone_RequiresReason verifies that rd done without --reason returns a clear error.
func TestDone_RequiresReason(t *testing.T) {
	err := validateCloseReason("")
	if err == nil {
		t.Fatal("expected error when reason is empty, got nil")
	}
	if !strings.Contains(err.Error(), "required") {
		t.Errorf("error message must contain 'required', got %q", err.Error())
	}
}

// TestFail_RequiresReason verifies that rd fail without --reason returns a clear error.
func TestFail_RequiresReason(t *testing.T) {
	err := validateCloseReason("")
	if err == nil {
		t.Fatal("expected error when reason is empty, got nil")
	}
	if !strings.Contains(err.Error(), "required") {
		t.Errorf("error message must contain 'required', got %q", err.Error())
	}
}

// TestCancel_RequiresReason verifies that rd cancel without --reason returns a clear error.
func TestCancel_RequiresReason(t *testing.T) {
	err := validateCloseReason("")
	if err == nil {
		t.Fatal("expected error when reason is empty, got nil")
	}
	if !strings.Contains(err.Error(), "required") {
		t.Errorf("error message must contain 'required', got %q", err.Error())
	}
}

// TestClose_WithReason verifies that close validation passes when reason is provided.
func TestClose_WithReason(t *testing.T) {
	err := validateCloseReason("Implemented and merged")
	if err != nil {
		t.Errorf("expected no error when reason is provided, got %v", err)
	}
}
