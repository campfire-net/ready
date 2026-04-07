package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestFormatCampfireIDForDisplay_DebugFlagShowsHex tests that --debug flag
// causes hex IDs to be displayed unchanged.
func TestFormatCampfireIDForDisplay_DebugFlagShowsHex(t *testing.T) {
	originalDebug := debugOutput
	defer func() { debugOutput = originalDebug }()

	hexID := strings.Repeat("ab", 32) // 64-char hex ID
	debugOutput = true

	result := formatCampfireIDForDisplay(hexID)
	if result != hexID {
		t.Errorf("with --debug, expected hex unchanged, got %q", result)
	}
}

// TestFormatCampfireIDForDisplay_NonHexPassthrough tests that non-64-char
// strings are passed through unchanged.
func TestFormatCampfireIDForDisplay_NonHexPassthrough(t *testing.T) {
	originalDebug := debugOutput
	defer func() { debugOutput = originalDebug }()

	debugOutput = false

	testCases := []string{
		"",           // empty
		"short",      // short string
		"not-64-hex", // not hex length
	}

	for _, tc := range testCases {
		result := formatCampfireIDForDisplay(tc)
		if result != tc {
			t.Errorf("expected non-64-char %q to pass through unchanged, got %q", tc, result)
		}
	}
}

// TestFormatCampfireIDForDisplay_NoConfigFallsback tests that when no
// .ready/config.json exists, the hex ID is returned unchanged.
func TestFormatCampfireIDForDisplay_NoConfigFallsback(t *testing.T) {
	originalDebug := debugOutput
	defer func() { debugOutput = originalDebug }()

	debugOutput = false

	// Create a temporary directory that has no .ready/config.json
	tmpDir, err := os.MkdirTemp("", "test-no-config")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Change to the temp directory
	originalCwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get cwd: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}
	defer os.Chdir(originalCwd)

	hexID := strings.Repeat("cd", 32) // 64-char hex ID
	result := formatCampfireIDForDisplay(hexID)

	if result != hexID {
		t.Errorf("with no config, expected hex unchanged, got %q", result)
	}
}

// TestFormatCampfireIDForDisplay_ResolveProjectName tests that a matching
// hex ID is resolved to the project name from SyncConfig.
func TestFormatCampfireIDForDisplay_ResolveProjectName(t *testing.T) {
	originalDebug := debugOutput
	defer func() { debugOutput = originalDebug }()

	debugOutput = false

	// Create a temporary directory to serve as both CF_HOME and project root.
	tmpDir, err := os.MkdirTemp("", "test-with-config")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	hexID := strings.Repeat("ef", 32) // 64-char hex ID
	const projectName = "myproject"

	// Populate the alias store so naming.AliasStore.Get("myproject") == hexID.
	origCFHome := os.Getenv("CF_HOME")
	os.Setenv("CF_HOME", tmpDir)
	defer func() {
		if origCFHome != "" {
			os.Setenv("CF_HOME", origCFHome)
		} else {
			os.Unsetenv("CF_HOME")
		}
	}()
	origRDHome := rdHome
	rdHome = ""
	defer func() { rdHome = origRDHome }()

	aliasContent := `{"` + projectName + `":"` + hexID + `"}`
	if err := os.WriteFile(filepath.Join(tmpDir, "aliases.json"), []byte(aliasContent), 0600); err != nil {
		t.Fatalf("failed to write aliases.json: %v", err)
	}

	// Create .ready/config.json with project_name pointing to the alias above.
	readyDir := filepath.Join(tmpDir, ".ready")
	if err := os.MkdirAll(readyDir, 0700); err != nil {
		t.Fatalf("failed to create .ready dir: %v", err)
	}
	configContent := `{"project_name": "` + projectName + `"}`
	if err := os.WriteFile(filepath.Join(readyDir, "config.json"), []byte(configContent), 0600); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	// Change to the temp directory so the walk-up finds .ready/config.json.
	originalCwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get cwd: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}
	defer os.Chdir(originalCwd)

	result := formatCampfireIDForDisplay(hexID)

	// Must return the project name, not the raw hex.
	if result == hexID {
		t.Errorf("formatCampfireIDForDisplay returned raw hex %q, want project name %q", result, projectName)
	}
	if result != projectName {
		t.Errorf("formatCampfireIDForDisplay = %q, want %q", result, projectName)
	}
}

// TestShowCommandDisplay_NoHexWithoutDebug verifies that rd show output
// contains the project name — not a raw 64-char hex — when project_name is
// configured and the alias store has a matching entry.
func TestShowCommandDisplay_NoHexWithoutDebug(t *testing.T) {
	origDebug := debugOutput
	defer func() { debugOutput = origDebug }()
	debugOutput = false

	origJSON := jsonOutput
	defer func() { jsonOutput = origJSON }()
	jsonOutput = false

	tmpDir, err := os.MkdirTemp("", "test-show-nohex")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	hexID := strings.Repeat("ab", 32) // 64-char hex campfire ID

	// Point CF_HOME at tmpDir so openStore() and naming.AliasStore both use it.
	origCFHome := os.Getenv("CF_HOME")
	os.Setenv("CF_HOME", tmpDir)
	defer func() {
		if origCFHome != "" {
			os.Setenv("CF_HOME", origCFHome)
		} else {
			os.Unsetenv("CF_HOME")
		}
	}()
	origRDHome := rdHome
	rdHome = ""
	defer func() { rdHome = origRDHome }()

	// Write alias store: "showproject" → hexID.
	aliasContent := `{"showproject":"` + hexID + `"}`
	if err := os.WriteFile(filepath.Join(tmpDir, "aliases.json"), []byte(aliasContent), 0600); err != nil {
		t.Fatalf("WriteFile aliases.json: %v", err)
	}

	// Write .ready/config.json with project_name.
	readyDir := filepath.Join(tmpDir, ".ready")
	if err := os.MkdirAll(readyDir, 0700); err != nil {
		t.Fatalf("MkdirAll .ready: %v", err)
	}
	if err := os.WriteFile(filepath.Join(readyDir, "config.json"), []byte(`{"project_name":"showproject"}`), 0600); err != nil {
		t.Fatalf("WriteFile config.json: %v", err)
	}

	// Write a minimal mutations.jsonl so byIDFromJSONLOrStore uses JSONL (no store needed).
	// campfire_id is the hex we expect to be hidden from output.
	createPayload := `{"id":"ready-show1","title":"Show Test Item","type":"task","for":"agent@test","priority":"p2"}`
	mutation := `{"msg_id":"test-msg-show1","campfire_id":"` + hexID + `","timestamp":1000000000000000000,"operation":"work:create","payload":` + createPayload + `,"tags":["work:create"],"sender":"testsender"}`
	jsonlPath := filepath.Join(readyDir, "mutations.jsonl")
	if err := os.WriteFile(jsonlPath, []byte(mutation+"\n"), 0600); err != nil {
		t.Fatalf("WriteFile mutations.jsonl: %v", err)
	}

	// Chdir to tmpDir so readyProjectDir() walks up and finds .ready/.
	origCwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer os.Chdir(origCwd)

	// Capture os.Stdout by replacing it with a pipe.
	origStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w

	runErr := showCmd.RunE(showCmd, []string{"ready-show1"})

	w.Close()
	os.Stdout = origStdout

	var buf strings.Builder
	outBuf := make([]byte, 4096)
	for {
		n, readErr := r.Read(outBuf)
		if n > 0 {
			buf.Write(outBuf[:n])
		}
		if readErr != nil {
			break
		}
	}
	r.Close()

	if runErr != nil {
		t.Fatalf("showCmd.RunE error: %v", runErr)
	}

	output := buf.String()

	// The Campfire line must show the project name, not the raw hex.
	if !strings.Contains(output, "showproject") {
		t.Errorf("show output does not contain project name %q; output:\n%s", "showproject", output)
	}

	// No 64-char hex token should appear anywhere in the output.
	for _, line := range strings.Split(output, "\n") {
		for _, tok := range strings.Fields(line) {
			if len(tok) == 64 && isValidHex(tok) {
				t.Errorf("show output contains raw 64-char hex %q on line: %q", tok, line)
			}
		}
	}
}

// TestProjectRoot_ProjectNameWinsOverCampfireRoot verifies that when both
// .ready/config.json (ProjectName) and .campfire/root exist with different IDs,
// the ProjectName-resolved ID wins and .campfire/root is rewritten to match.
func TestProjectRoot_ProjectNameWinsOverCampfireRoot(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "test-priority")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Set CF_HOME to tmpDir so AliasStore uses our temp store.
	origCFHome := os.Getenv("CF_HOME")
	os.Setenv("CF_HOME", tmpDir)
	defer func() {
		if origCFHome != "" {
			os.Setenv("CF_HOME", origCFHome)
		} else {
			os.Unsetenv("CF_HOME")
		}
	}()
	origRDHome := rdHome
	rdHome = ""
	defer func() { rdHome = origRDHome }()

	// Populate the alias store: "myproject" → primaryID.
	primaryID := strings.Repeat("aa", 32) // 64-char
	staleID := strings.Repeat("bb", 32)   // different, stale

	aliasFile := filepath.Join(tmpDir, "aliases.json")
	aliasContent := `{"myproject":"` + primaryID + `"}`
	if err := os.WriteFile(aliasFile, []byte(aliasContent), 0600); err != nil {
		t.Fatalf("WriteFile aliases.json: %v", err)
	}

	// Create .ready/config.json with ProjectName=myproject.
	readyDir := filepath.Join(tmpDir, ".ready")
	if err := os.MkdirAll(readyDir, 0700); err != nil {
		t.Fatalf("MkdirAll .ready: %v", err)
	}
	configContent := `{"project_name":"myproject"}`
	if err := os.WriteFile(filepath.Join(readyDir, "config.json"), []byte(configContent), 0600); err != nil {
		t.Fatalf("WriteFile config.json: %v", err)
	}

	// Create .campfire/root with a DIFFERENT (stale) ID.
	campfireDir := filepath.Join(tmpDir, ".campfire")
	if err := os.MkdirAll(campfireDir, 0700); err != nil {
		t.Fatalf("MkdirAll .campfire: %v", err)
	}
	if err := os.WriteFile(filepath.Join(campfireDir, "root"), []byte(staleID), 0600); err != nil {
		t.Fatalf("WriteFile .campfire/root: %v", err)
	}

	// Change to tmpDir so projectRoot() walks up from there.
	origCwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer os.Chdir(origCwd)

	// projectRoot() must return primaryID (ProjectName wins).
	gotID, gotDir, ok := projectRoot()
	if !ok {
		t.Fatal("projectRoot() returned false — expected to find the project")
	}
	if gotID != primaryID {
		t.Errorf("projectRoot() campfireID = %q, want %q (ProjectName must win)", gotID, primaryID)
	}
	if gotDir != tmpDir {
		t.Errorf("projectRoot() dir = %q, want %q", gotDir, tmpDir)
	}

	// .campfire/root must have been rewritten to primaryID.
	rewritten, err := os.ReadFile(filepath.Join(campfireDir, "root"))
	if err != nil {
		t.Fatalf("reading rewritten .campfire/root: %v", err)
	}
	if strings.TrimSpace(string(rewritten)) != primaryID {
		t.Errorf(".campfire/root after rewrite = %q, want %q", strings.TrimSpace(string(rewritten)), primaryID)
	}
}

// TestFormatCampfireIDForDisplay_ResolvesWithRealAliasStore verifies that
// formatCampfireIDForDisplay returns the project name (not hex) when the alias
// store has a matching entry. This is the positive path test missing from the
// original suite.
func TestFormatCampfireIDForDisplay_ResolvesWithRealAliasStore(t *testing.T) {
	originalDebug := debugOutput
	defer func() { debugOutput = originalDebug }()
	debugOutput = false

	tmpDir, err := os.MkdirTemp("", "test-resolve-positive")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	hexID := strings.Repeat("cc", 32) // 64-char

	// Set CF_HOME so AliasStore uses tmpDir.
	origCFHome := os.Getenv("CF_HOME")
	os.Setenv("CF_HOME", tmpDir)
	defer func() {
		if origCFHome != "" {
			os.Setenv("CF_HOME", origCFHome)
		} else {
			os.Unsetenv("CF_HOME")
		}
	}()
	origRDHome := rdHome
	rdHome = ""
	defer func() { rdHome = origRDHome }()

	// Write alias store: "cool.project" → hexID.
	aliasContent := `{"cool.project":"` + hexID + `"}`
	if err := os.WriteFile(filepath.Join(tmpDir, "aliases.json"), []byte(aliasContent), 0600); err != nil {
		t.Fatalf("WriteFile aliases.json: %v", err)
	}

	// Write .ready/config.json with project_name.
	readyDir := filepath.Join(tmpDir, ".ready")
	if err := os.MkdirAll(readyDir, 0700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	configContent := `{"project_name":"cool.project"}`
	if err := os.WriteFile(filepath.Join(readyDir, "config.json"), []byte(configContent), 0600); err != nil {
		t.Fatalf("WriteFile config.json: %v", err)
	}

	origCwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	defer os.Chdir(origCwd)

	result := formatCampfireIDForDisplay(hexID)
	if result == hexID {
		t.Errorf("formatCampfireIDForDisplay returned hex %q, want project name %q", result, "cool.project")
	}
	if result != "cool.project" {
		t.Errorf("formatCampfireIDForDisplay = %q, want %q", result, "cool.project")
	}
}

// Helper to check if a string is valid hex
func isValidHex(s string) bool {
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return len(s) == 64
}
