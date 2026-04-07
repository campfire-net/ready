package main

// cfhome_test.go — tests for CFHome migration (Wave 1).
//
// Tests the detection order for CFHome() in the context of the Wave 1 migration
// from ~/.campfire to ~/.cf for new installs, with support for legacy users.

import (
	"os"
	"path/filepath"
	"testing"
)

// TestCFHome_FlagTakesHighestPriority verifies that the --cf-home flag takes
// precedence over all other sources.
func TestCFHome_FlagTakesHighestPriority(t *testing.T) {
	// Save original state
	oldHome := rdHome
	oldEnv := os.Getenv("CF_HOME")
	defer func() {
		rdHome = oldHome
		if oldEnv != "" {
			os.Setenv("CF_HOME", oldEnv)
		} else {
			os.Unsetenv("CF_HOME")
		}
	}()

	// Set flag and env
	rdHome = "/tmp/flag-cf-home"
	os.Setenv("CF_HOME", "/tmp/env-cf-home")

	result := CFHome()
	if result != "/tmp/flag-cf-home" {
		t.Errorf("expected /tmp/flag-cf-home, got %q", result)
	}
}

// TestCFHome_EnvSecondPriority verifies that CF_HOME env var is used when
// the flag is not set.
func TestCFHome_EnvSecondPriority(t *testing.T) {
	// Save original state
	oldHome := rdHome
	oldEnv := os.Getenv("CF_HOME")
	defer func() {
		rdHome = oldHome
		if oldEnv != "" {
			os.Setenv("CF_HOME", oldEnv)
		} else {
			os.Unsetenv("CF_HOME")
		}
	}()

	// Clear flag, set env
	rdHome = ""
	os.Setenv("CF_HOME", "/tmp/env-cf-home")

	result := CFHome()
	if result != "/tmp/env-cf-home" {
		t.Errorf("expected /tmp/env-cf-home, got %q", result)
	}
}

// TestCFHome_NewInstallPath verifies that ~/.cf is used when it exists and
// no flag/env is set.
func TestCFHome_NewInstallPath(t *testing.T) {
	// Save original state
	oldHome := rdHome
	oldEnv := os.Getenv("CF_HOME")
	defer func() {
		rdHome = oldHome
		if oldEnv != "" {
			os.Setenv("CF_HOME", oldEnv)
		} else {
			os.Unsetenv("CF_HOME")
		}
	}()

	// Create a temporary home directory with only .cf
	tmpHome := t.TempDir()
	newPath := filepath.Join(tmpHome, ".cf")
	if err := os.MkdirAll(newPath, 0755); err != nil {
		t.Fatalf("failed to create .cf directory: %v", err)
	}

	// Save and replace UserHomeDir logic by manipulating the environment
	oldHomeEnv := os.Getenv("HOME")
	defer func() {
		if oldHomeEnv != "" {
			os.Setenv("HOME", oldHomeEnv)
		} else {
			os.Unsetenv("HOME")
		}
	}()
	os.Setenv("HOME", tmpHome)

	rdHome = ""
	os.Unsetenv("CF_HOME")

	result := CFHome()
	expected := filepath.Join(tmpHome, ".cf")
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

// TestCFHome_LegacyUserPath verifies that ~/.campfire is used when it exists
// but ~/.cf does not.
func TestCFHome_LegacyUserPath(t *testing.T) {
	// Save original state
	oldHome := rdHome
	oldEnv := os.Getenv("CF_HOME")
	defer func() {
		rdHome = oldHome
		if oldEnv != "" {
			os.Setenv("CF_HOME", oldEnv)
		} else {
			os.Unsetenv("CF_HOME")
		}
	}()

	// Create a temporary home directory with only .campfire
	tmpHome := t.TempDir()
	legacyPath := filepath.Join(tmpHome, ".campfire")
	if err := os.MkdirAll(legacyPath, 0755); err != nil {
		t.Fatalf("failed to create .campfire directory: %v", err)
	}

	// Save and replace UserHomeDir logic via HOME env
	oldHomeEnv := os.Getenv("HOME")
	defer func() {
		if oldHomeEnv != "" {
			os.Setenv("HOME", oldHomeEnv)
		} else {
			os.Unsetenv("HOME")
		}
	}()
	os.Setenv("HOME", tmpHome)

	rdHome = ""
	os.Unsetenv("CF_HOME")

	result := CFHome()
	expected := filepath.Join(tmpHome, ".campfire")
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

// TestCFHome_NewInstallDefault verifies that ~/.cf is returned when neither
// ~/.cf nor ~/.campfire exists (new install).
func TestCFHome_NewInstallDefault(t *testing.T) {
	// Save original state
	oldHome := rdHome
	oldEnv := os.Getenv("CF_HOME")
	defer func() {
		rdHome = oldHome
		if oldEnv != "" {
			os.Setenv("CF_HOME", oldEnv)
		} else {
			os.Unsetenv("CF_HOME")
		}
	}()

	// Create a temporary home directory with neither .cf nor .campfire
	tmpHome := t.TempDir()

	// Save and replace UserHomeDir logic via HOME env
	oldHomeEnv := os.Getenv("HOME")
	defer func() {
		if oldHomeEnv != "" {
			os.Setenv("HOME", oldHomeEnv)
		} else {
			os.Unsetenv("HOME")
		}
	}()
	os.Setenv("HOME", tmpHome)

	rdHome = ""
	os.Unsetenv("CF_HOME")

	result := CFHome()
	expected := filepath.Join(tmpHome, ".cf")
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

// TestCFHome_NewPathTakesPreferenceOverLegacy verifies that ~/.cf is preferred
// over ~/.campfire when both exist.
func TestCFHome_NewPathTakesPreferenceOverLegacy(t *testing.T) {
	// Save original state
	oldHome := rdHome
	oldEnv := os.Getenv("CF_HOME")
	defer func() {
		rdHome = oldHome
		if oldEnv != "" {
			os.Setenv("CF_HOME", oldEnv)
		} else {
			os.Unsetenv("CF_HOME")
		}
	}()

	// Create a temporary home directory with both .cf and .campfire
	tmpHome := t.TempDir()
	newPath := filepath.Join(tmpHome, ".cf")
	legacyPath := filepath.Join(tmpHome, ".campfire")
	if err := os.MkdirAll(newPath, 0755); err != nil {
		t.Fatalf("failed to create .cf directory: %v", err)
	}
	if err := os.MkdirAll(legacyPath, 0755); err != nil {
		t.Fatalf("failed to create .campfire directory: %v", err)
	}

	// Save and replace UserHomeDir logic via HOME env
	oldHomeEnv := os.Getenv("HOME")
	defer func() {
		if oldHomeEnv != "" {
			os.Setenv("HOME", oldHomeEnv)
		} else {
			os.Unsetenv("HOME")
		}
	}()
	os.Setenv("HOME", tmpHome)

	rdHome = ""
	os.Unsetenv("CF_HOME")

	result := CFHome()
	expected := filepath.Join(tmpHome, ".cf")
	if result != expected {
		t.Errorf("expected %q (new path), got %q", expected, result)
	}
}

// TestCFHome_IdentityPath verifies that IdentityPath() correctly uses the
// resolved CFHome().
func TestCFHome_IdentityPath(t *testing.T) {
	// Save original state
	oldHome := rdHome
	defer func() { rdHome = oldHome }()

	rdHome = "/tmp/test-cf-home"

	result := IdentityPath()
	expected := "/tmp/test-cf-home/identity.json"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}
