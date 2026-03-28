package main

// send_test.go exercises bufferToPending and related send.go logic.

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/campfire-net/campfire/pkg/message"
)

// TestBufferToPending_NoProjectRoot verifies that bufferToPending returns an
// error when no project root exists (no .ready/ directory in cwd or parents).
// Finding 2: bufferToPending must not silently drop mutations.
func TestBufferToPending_NoProjectRoot(t *testing.T) {
	// Change to a temp dir with no .ready/ and no .campfire/root.
	dir := t.TempDir()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() { os.Chdir(orig) })
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	msg := &message.Message{ID: "test-msg-id-no-project-root"}
	err = bufferToPending(msg, "campfire-test-id", `{"op":"test"}`, []string{"work:create"}, nil)
	if err == nil {
		t.Fatal("bufferToPending: expected error when no project root, got nil")
	}
}

// TestBufferToPending_WritesRecord verifies that bufferToPending writes a record
// to pending.jsonl when a project root exists.
func TestBufferToPending_WritesRecord(t *testing.T) {
	// Create a minimal project dir with .ready/.
	dir := t.TempDir()
	readyDir := filepath.Join(dir, ".ready")
	if err := os.MkdirAll(readyDir, 0755); err != nil {
		t.Fatalf("mkdir .ready: %v", err)
	}

	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() { os.Chdir(orig) })
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}

	msg := &message.Message{ID: "test-msg-id-buffer-writes"}
	err = bufferToPending(msg, "campfire-test-id", `{"op":"test"}`, []string{"work:create"}, nil)
	if err != nil {
		t.Fatalf("bufferToPending: unexpected error: %v", err)
	}

	pendingFile := filepath.Join(readyDir, "pending.jsonl")
	data, err := os.ReadFile(pendingFile)
	if err != nil {
		t.Fatalf("reading pending.jsonl: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("pending.jsonl is empty — expected a buffered record")
	}
}
