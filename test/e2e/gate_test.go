package e2e_test

// gate_test.go — end-to-end tests for the gate/approve/reject escalation cycle.
//
// Done condition (ready-dd3):
//   - Two isolated identities (owner CF_HOME, approver CF_HOME) complete the
//     full gate escalation cycle:
//     (1) owner creates item and calls 'rd gate <id> --gate-type design'
//     (2) 'rd gates' lists the pending gate
//     (3) approver calls 'rd approve <id>' or 'rd reject <id> --reason ...'
//     (4) item status updates correctly after approval/rejection
//   - All four commands exit 0. Exit non-zero for bad inputs.
//
// No mocks. Two real CF_HOME dirs, two real campfire clients, real filesystem transport.
// Both identities share the same project dir (and therefore the same mutations.jsonl).

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestE2E_Gate_ApproveFullCycle verifies the full gate escalation cycle with approval:
//   - Owner creates item, gates it (rd gate <id> --gate-type design)
//   - rd gates lists the pending gate
//   - Approver (second identity, campfire member) calls rd approve <id>
//   - Item transitions to active after approval
func TestE2E_Gate_ApproveFullCycle(t *testing.T) {
	ownerCFHome, approverCFHome, projectDir := gateTwoIdentitySetup(t)

	envFor := func(cfHome string) []string {
		return []string{
			"PATH=" + os.Getenv("PATH"),
			"HOME=" + os.Getenv("HOME"),
			"CF_HOME=" + cfHome,
		}
	}

	rdCmd := func(cfHome string, args ...string) (stdout, stderr string, code int) {
		t.Helper()
		cmd := exec.Command(rdBinary, args...)
		cmd.Dir = projectDir
		cmd.Env = envFor(cfHome)
		var outBuf, errBuf bytes.Buffer
		cmd.Stdout = &outBuf
		cmd.Stderr = &errBuf
		_ = cmd.Run()
		code = 0
		if cmd.ProcessState != nil && !cmd.ProcessState.Success() {
			code = cmd.ProcessState.ExitCode()
		}
		return outBuf.String(), errBuf.String(), code
	}

	// Step 1: Owner creates an item.
	createOut, createStderr, createCode := rdCmd(ownerCFHome, "create",
		"--title", "Gate approve test item",
		"--priority", "p1",
		"--type", "task",
		"--json",
	)
	if createCode != 0 {
		t.Fatalf("rd create (owner) failed (exit %d): %s", createCode, createStderr)
	}

	var item Item
	if err := json.Unmarshal([]byte(createOut), &item); err != nil {
		t.Fatalf("parse create JSON: %v\noutput: %s", err, createOut)
	}
	if item.ID == "" {
		t.Fatal("rd create returned empty ID")
	}

	// Step 2: Owner gates the item.
	gateOut, gateStderr, gateCode := rdCmd(ownerCFHome, "gate", item.ID,
		"--gate-type", "design",
		"--description", "Confirm API shape before implementing",
		"--json",
	)
	if gateCode != 0 {
		t.Fatalf("rd gate (owner) failed (exit %d): %s\nstdout: %s", gateCode, gateStderr, gateOut)
	}

	var gateResult struct {
		ID        string `json:"id"`
		MsgID     string `json:"msg_id"`
		GateType  string `json:"gate_type"`
	}
	if err := json.Unmarshal([]byte(gateOut), &gateResult); err != nil {
		t.Fatalf("parse gate JSON: %v\noutput: %s", err, gateOut)
	}
	if gateResult.ID != item.ID {
		t.Errorf("gate result id=%q, want %q", gateResult.ID, item.ID)
	}
	if gateResult.GateType != "design" {
		t.Errorf("gate_type=%q, want design", gateResult.GateType)
	}
	if gateResult.MsgID == "" {
		t.Error("gate msg_id should be non-empty")
	}

	// Step 3: Verify item is in waiting status with waiting_type=gate.
	showOut, showStderr, showCode := rdCmd(ownerCFHome, "show", item.ID, "--json")
	if showCode != 0 {
		t.Fatalf("rd show after gate (exit %d): %s", showCode, showStderr)
	}
	var gatedItem Item
	if err := json.Unmarshal([]byte(showOut), &gatedItem); err != nil {
		t.Fatalf("parse show JSON after gate: %v\noutput: %s", err, showOut)
	}
	if gatedItem.Status != "waiting" {
		t.Errorf("after gate: status=%q, want waiting", gatedItem.Status)
	}
	if gatedItem.WaitingType != "gate" {
		t.Errorf("after gate: waiting_type=%q, want gate", gatedItem.WaitingType)
	}
	if gatedItem.GateMsgID == "" {
		t.Error("after gate: gate_msg_id should be non-empty")
	}

	// Step 4: rd gates lists the pending gate.
	gatesOut, gatesStderr, gatesCode := rdCmd(ownerCFHome, "gates", "--json")
	if gatesCode != 0 {
		t.Fatalf("rd gates failed (exit %d): %s", gatesCode, gatesStderr)
	}
	var gateItems []Item
	if err := json.Unmarshal([]byte(gatesOut), &gateItems); err != nil {
		t.Fatalf("parse gates JSON: %v\noutput: %s", err, gatesOut)
	}
	gateFound := false
	for _, gi := range gateItems {
		if gi.ID == item.ID {
			gateFound = true
			break
		}
	}
	if !gateFound {
		t.Errorf("item %s should appear in rd gates output after gating", item.ID)
		t.Logf("gates output: %s", gatesOut)
	}

	// Step 5: Approver (second identity) approves the gate.
	approveOut, approveStderr, approveCode := rdCmd(approverCFHome, "approve", item.ID,
		"--reason", "Approved, proceed with design approach",
		"--json",
	)
	if approveCode != 0 {
		t.Fatalf("rd approve (approver) failed (exit %d): %s\nstdout: %s", approveCode, approveStderr, approveOut)
	}

	var approveResult struct {
		ID         string `json:"id"`
		Resolution string `json:"resolution"`
	}
	if err := json.Unmarshal([]byte(approveOut), &approveResult); err != nil {
		t.Fatalf("parse approve JSON: %v\noutput: %s", err, approveOut)
	}
	if approveResult.ID != item.ID {
		t.Errorf("approve result id=%q, want %q", approveResult.ID, item.ID)
	}
	if approveResult.Resolution != "approved" {
		t.Errorf("approve resolution=%q, want approved", approveResult.Resolution)
	}

	// Step 6: Verify item is now active after approval.
	showOut2, showStderr2, showCode2 := rdCmd(ownerCFHome, "show", item.ID, "--json")
	if showCode2 != 0 {
		t.Fatalf("rd show after approve (exit %d): %s", showCode2, showStderr2)
	}
	var approvedItem Item
	if err := json.Unmarshal([]byte(showOut2), &approvedItem); err != nil {
		t.Fatalf("parse show JSON after approve: %v\noutput: %s", err, showOut2)
	}
	if approvedItem.Status != "active" {
		t.Errorf("after approve: status=%q, want active", approvedItem.Status)
	}
	if approvedItem.WaitingType != "" {
		t.Errorf("after approve: waiting_type=%q, want empty", approvedItem.WaitingType)
	}
	if approvedItem.GateMsgID != "" {
		t.Errorf("after approve: gate_msg_id=%q, want empty (gate resolved)", approvedItem.GateMsgID)
	}

	// Step 7: rd gates should no longer list the item.
	gatesOut2, _, gatesCode2 := rdCmd(ownerCFHome, "gates", "--json")
	if gatesCode2 != 0 {
		t.Logf("rd gates after approve: non-zero exit (may be empty list)")
	}
	// Empty array or "no pending gates" — item must not appear.
	if strings.Contains(gatesOut2, item.ID) {
		t.Errorf("item %s should not appear in rd gates after approval", item.ID)
	}
}

// TestE2E_Gate_RejectFullCycle verifies the full gate escalation cycle with rejection:
//   - Owner creates item, gates it (rd gate <id> --gate-type scope)
//   - Approver (second identity, campfire member) calls rd reject <id> --reason ...
//   - Item remains in waiting status after rejection
//   - Gate message ID is still set (gate not cleared on rejection)
func TestE2E_Gate_RejectFullCycle(t *testing.T) {
	ownerCFHome, approverCFHome, projectDir := gateTwoIdentitySetup(t)

	envFor := func(cfHome string) []string {
		return []string{
			"PATH=" + os.Getenv("PATH"),
			"HOME=" + os.Getenv("HOME"),
			"CF_HOME=" + cfHome,
		}
	}

	rdCmd := func(cfHome string, args ...string) (stdout, stderr string, code int) {
		t.Helper()
		cmd := exec.Command(rdBinary, args...)
		cmd.Dir = projectDir
		cmd.Env = envFor(cfHome)
		var outBuf, errBuf bytes.Buffer
		cmd.Stdout = &outBuf
		cmd.Stderr = &errBuf
		_ = cmd.Run()
		code = 0
		if cmd.ProcessState != nil && !cmd.ProcessState.Success() {
			code = cmd.ProcessState.ExitCode()
		}
		return outBuf.String(), errBuf.String(), code
	}

	// Step 1: Owner creates an item.
	createOut, createStderr, createCode := rdCmd(ownerCFHome, "create",
		"--title", "Gate reject test item",
		"--priority", "p2",
		"--type", "task",
		"--json",
	)
	if createCode != 0 {
		t.Fatalf("rd create (owner) failed (exit %d): %s", createCode, createStderr)
	}

	var item Item
	if err := json.Unmarshal([]byte(createOut), &item); err != nil {
		t.Fatalf("parse create JSON: %v\noutput: %s", err, createOut)
	}
	if item.ID == "" {
		t.Fatal("rd create returned empty ID")
	}

	// Step 2: Owner gates the item.
	_, gateStderr, gateCode := rdCmd(ownerCFHome, "gate", item.ID,
		"--gate-type", "scope",
		"--description", "Scope too broad, needs design review",
	)
	if gateCode != 0 {
		t.Fatalf("rd gate (owner) failed (exit %d): %s", gateCode, gateStderr)
	}

	// Step 3: Verify item is in waiting status.
	showOut, showStderr, showCode := rdCmd(ownerCFHome, "show", item.ID, "--json")
	if showCode != 0 {
		t.Fatalf("rd show after gate (exit %d): %s", showCode, showStderr)
	}
	var gatedItem Item
	if err := json.Unmarshal([]byte(showOut), &gatedItem); err != nil {
		t.Fatalf("parse show JSON after gate: %v\noutput: %s", err, showOut)
	}
	if gatedItem.Status != "waiting" {
		t.Errorf("after gate: status=%q, want waiting", gatedItem.Status)
	}
	if gatedItem.WaitingType != "gate" {
		t.Errorf("after gate: waiting_type=%q, want gate", gatedItem.WaitingType)
	}
	if gatedItem.GateMsgID == "" {
		t.Error("after gate: gate_msg_id should be non-empty")
	}

	// Step 4: rd gates lists the pending gate.
	gatesOut, gatesStderr, gatesCode := rdCmd(ownerCFHome, "gates", "--json")
	if gatesCode != 0 {
		t.Fatalf("rd gates failed (exit %d): %s", gatesCode, gatesStderr)
	}
	var gateItems []Item
	if err := json.Unmarshal([]byte(gatesOut), &gateItems); err != nil {
		t.Fatalf("parse gates JSON: %v\noutput: %s", err, gatesOut)
	}
	gateFound := false
	for _, gi := range gateItems {
		if gi.ID == item.ID {
			gateFound = true
			break
		}
	}
	if !gateFound {
		t.Errorf("item %s should appear in rd gates output after gating", item.ID)
		t.Logf("gates output: %s", gatesOut)
	}

	// Step 5: Approver (second identity) rejects the gate.
	rejectOut, rejectStderr, rejectCode := rdCmd(approverCFHome, "reject", item.ID,
		"--reason", "Scope too broad, split into smaller pieces first",
		"--json",
	)
	if rejectCode != 0 {
		t.Fatalf("rd reject (approver) failed (exit %d): %s\nstdout: %s", rejectCode, rejectStderr, rejectOut)
	}

	var rejectResult struct {
		ID         string `json:"id"`
		Resolution string `json:"resolution"`
	}
	if err := json.Unmarshal([]byte(rejectOut), &rejectResult); err != nil {
		t.Fatalf("parse reject JSON: %v\noutput: %s", err, rejectOut)
	}
	if rejectResult.ID != item.ID {
		t.Errorf("reject result id=%q, want %q", rejectResult.ID, item.ID)
	}
	if rejectResult.Resolution != "rejected" {
		t.Errorf("reject resolution=%q, want rejected", rejectResult.Resolution)
	}

	// Step 6: Verify item remains in waiting status after rejection.
	showOut2, showStderr2, showCode2 := rdCmd(ownerCFHome, "show", item.ID, "--json")
	if showCode2 != 0 {
		t.Fatalf("rd show after reject (exit %d): %s", showCode2, showStderr2)
	}
	var rejectedItem Item
	if err := json.Unmarshal([]byte(showOut2), &rejectedItem); err != nil {
		t.Fatalf("parse show JSON after reject: %v\noutput: %s", err, showOut2)
	}
	if rejectedItem.Status != "waiting" {
		t.Errorf("after reject: status=%q, want waiting (rejection does not close the gate)", rejectedItem.Status)
	}
	// GateMsgID should still be set after rejection (gate not cleared).
	if rejectedItem.GateMsgID == "" {
		t.Error("after reject: gate_msg_id should still be set (gate unresolved until approved)")
	}

	// Step 7: rd gates should still list the item (rejection keeps it waiting).
	gatesOut2, gatesStderr2, gatesCode2 := rdCmd(ownerCFHome, "gates", "--json")
	if gatesCode2 != 0 {
		t.Fatalf("rd gates after reject (exit %d): %s", gatesCode2, gatesStderr2)
	}
	var gateItems2 []Item
	if err := json.Unmarshal([]byte(gatesOut2), &gateItems2); err != nil {
		t.Fatalf("parse gates JSON after reject: %v\noutput: %s", err, gatesOut2)
	}
	stillFound := false
	for _, gi := range gateItems2 {
		if gi.ID == item.ID {
			stillFound = true
			break
		}
	}
	if !stillFound {
		t.Errorf("item %s should still appear in rd gates after rejection", item.ID)
	}
}

// TestE2E_Gate_BadInputs verifies that gate/approve/reject fail cleanly on bad inputs:
//   - rd approve on an item with no pending gate → non-zero exit, clear error
//   - rd reject on an item with no pending gate → non-zero exit, clear error
//   - rd gate with no --gate-type → non-zero exit, clear error
//
// Uses NewEnv (campfire already configured via cf create) — no rd init needed.
func TestE2E_Gate_BadInputs(t *testing.T) {
	e := NewEnv(t)

	// Create an item (not gated) — NewEnv already has campfire configured.
	var item Item
	if err := e.RdJSON(&item, "create",
		"--title", "Non-gated item",
		"--priority", "p1",
		"--type", "task",
	); err != nil {
		t.Fatalf("rd create: %v", err)
	}

	// rd approve on non-gated item → error.
	approveStderr := e.RdMustFail("approve", item.ID)
	if !strings.Contains(approveStderr, "no pending gate") {
		t.Errorf("approve non-gated item: error should mention 'no pending gate', got: %q", approveStderr)
	}

	// rd reject on non-gated item → error.
	rejectStderr := e.RdMustFail("reject", item.ID)
	if !strings.Contains(rejectStderr, "no pending gate") {
		t.Errorf("reject non-gated item: error should mention 'no pending gate', got: %q", rejectStderr)
	}

	// rd gate without --gate-type → error.
	gateMissingTypeStderr := e.RdMustFail("gate", item.ID)
	if !strings.Contains(gateMissingTypeStderr, "gate-type") {
		t.Errorf("gate without --gate-type: error should mention 'gate-type', got: %q", gateMissingTypeStderr)
	}
}

// gateTwoIdentitySetup creates two isolated identities (owner and approver) that
// share the same project campfire. Returns:
//   - ownerCFHome: CF_HOME for the campfire owner/creator
//   - approverCFHome: CF_HOME for the approver (second identity, campfire member)
//   - projectDir: shared project directory with .campfire/root and .ready/
//
// Setup sequence:
//  1. cf init for owner identity
//  2. cf init for approver identity
//  3. rd init (owner) — creates campfire and .ready/
//  4. rd admit <approver-pubkey> (owner) — pre-admits the approver
//  5. rd join <campfire-id> (approver) — approver joins the campfire
func gateTwoIdentitySetup(t *testing.T) (ownerCFHome, approverCFHome, projectDir string) {
	t.Helper()

	ownerCFHome = t.TempDir()
	approverCFHome = t.TempDir()
	projectDir = t.TempDir()
	sharedHome := os.Getenv("HOME")

	envFor := func(cfHome string) []string {
		return []string{
			"PATH=" + os.Getenv("PATH"),
			"HOME=" + sharedHome,
			"CF_HOME=" + cfHome,
		}
	}

	runCmdOK := func(name string, env []string, args ...string) string {
		t.Helper()
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%s failed: %v\n%s", name, err, out)
		}
		return string(out)
	}

	rdCmdInDir := func(cfHome string, dir string, args ...string) (stdout, stderr string, code int) {
		t.Helper()
		cmd := exec.Command(rdBinary, args...)
		cmd.Dir = dir
		cmd.Env = envFor(cfHome)
		var outBuf, errBuf bytes.Buffer
		cmd.Stdout = &outBuf
		cmd.Stderr = &errBuf
		_ = cmd.Run()
		code = 0
		if cmd.ProcessState != nil && !cmd.ProcessState.Success() {
			code = cmd.ProcessState.ExitCode()
		}
		return outBuf.String(), errBuf.String(), code
	}

	// 1. cf init for both identities.
	runCmdOK("cf init (owner)", envFor(ownerCFHome), "cf", "init", "--cf-home", ownerCFHome)
	runCmdOK("cf init (approver)", envFor(approverCFHome), "cf", "init", "--cf-home", approverCFHome)

	// 2. rd init (owner) — creates campfire and project structure.
	_, initStderr, initCode := rdCmdInDir(ownerCFHome, projectDir, "init", "--name", "gate-e2e-test", "--confirm")
	if initCode != 0 {
		t.Fatalf("rd init (owner) failed (exit %d): %s", initCode, initStderr)
	}

	// Read campfire ID from .campfire/root.
	campfireIDBytes, err := os.ReadFile(filepath.Join(projectDir, ".campfire", "root"))
	if err != nil {
		t.Fatalf("reading .campfire/root: %v", err)
	}
	campfireID := strings.TrimRight(string(campfireIDBytes), "\r\n")
	if len(campfireID) != 64 {
		t.Fatalf("campfire ID has wrong length %d: %q", len(campfireID), campfireID)
	}

	// 3. Read approver public key.
	approverPubKey := memberPubKeyHex(t, approverCFHome)

	// 4. rd admit <approver-pubkey> (owner) — pre-admits the approver.
	_, admitStderr, admitCode := rdCmdInDir(ownerCFHome, projectDir, "admit", approverPubKey)
	if admitCode != 0 {
		t.Fatalf("rd admit (owner) failed (exit %d): %s", admitCode, admitStderr)
	}

	// 5. rd join <campfire-id> (approver) — approver joins the campfire.
	_, joinStderr, joinCode := rdCmdInDir(approverCFHome, projectDir, "join", campfireID)
	if joinCode != 0 {
		t.Fatalf("rd join (approver) failed (exit %d): %s", joinCode, joinStderr)
	}

	t.Logf("gate setup: owner=%s approver=%s campfire=%s project=%s",
		fmt.Sprintf("...%s", ownerCFHome[len(ownerCFHome)-8:]),
		fmt.Sprintf("...%s", approverCFHome[len(approverCFHome)-8:]),
		campfireID[:8]+"...",
		projectDir,
	)

	return ownerCFHome, approverCFHome, projectDir
}
