package main

import (
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	campfirepkg "github.com/campfire-net/campfire/pkg/campfire"
	cfencoding "github.com/campfire-net/campfire/pkg/encoding"
	"github.com/campfire-net/campfire/pkg/identity"
	"github.com/campfire-net/campfire/pkg/message"
	"github.com/campfire-net/campfire/pkg/protocol"
	"github.com/campfire-net/campfire/pkg/store"
	"github.com/campfire-net/campfire/pkg/transport/fs"

	"github.com/campfire-net/ready/pkg/jsonl"
	rdSync "github.com/campfire-net/ready/pkg/sync"
)

// sendToProjectCampfire sends a convention message in two phases:
//  1. Append to local JSONL (always, if project root exists).
//  2. Post to campfire (optional — skipped if no campfire configured,
//     buffered to .ready/pending.jsonl if the send fails).
//
// Returns a synthetic message (with a generated ID) when operating in
// JSONL-only mode (no campfire configured). The campfireID return value is
// empty in that case.
func sendToProjectCampfire(agentID *identity.Identity, s store.Store, payload string, tags []string, antecedents []string) (*message.Message, string, error) {
	campfireID, _, hasCampfire := projectRoot()

	// Phase 1 — build the message object (we always need it for JSONL).
	msg, err := message.NewMessage(agentID.PrivateKey, agentID.PublicKey, []byte(payload), tags, antecedents)
	if err != nil {
		return nil, "", fmt.Errorf("creating message: %w", err)
	}

	// Phase 1 — write to local JSONL.
	if jpPath := jsonlPath(); jpPath != "" {
		w := jsonl.NewWriter(jpPath)
		rec := jsonl.MutationRecord{
			MsgID:       msg.ID,
			CampfireID:  campfireID, // may be empty in JSONL-only mode
			Timestamp:   time.Now().UnixNano(),
			Operation:   extractOperationFromTags(tags),
			Payload:     json.RawMessage(payload),
			Tags:        tags,
			Sender:      agentID.PublicKeyHex(),
			Antecedents: antecedents,
		}
		if err := w.Append(rec); err != nil {
			// JSONL write failed — this is fatal (disk not writable).
			return nil, "", fmt.Errorf("writing to local JSONL: %w", err)
		}
	}

	// Phase 2 — post to campfire (optional).
	if !hasCampfire {
		// No campfire configured — JSONL-only mode, done.
		return msg, "", nil
	}

	client, err := requireClient()
	if err != nil {
		// Client init failed — buffer for later sync and return success.
		fmt.Fprintf(os.Stderr, "warning: campfire client init failed (buffered to pending.jsonl): %v\n", err)
		if bufErr := bufferToPending(msg, campfireID, payload, tags, antecedents); bufErr != nil {
			fmt.Fprintf(os.Stderr, "warning: %v\n", bufErr)
		}
		return msg, campfireID, nil
	}

	// Try to send via client.Send().
	_, sendErr := client.Send(protocol.SendRequest{
		CampfireID:  campfireID,
		Payload:     msg.Payload,
		Tags:        tags,
		Antecedents: antecedents,
	})
	if sendErr != nil {
		// Transport send failed — JSONL write already succeeded, buffer for sync.
		fmt.Fprintf(os.Stderr, "warning: campfire send failed (buffered to pending.jsonl): %v\n", sendErr)
		if bufErr := bufferToPending(msg, campfireID, payload, tags, antecedents); bufErr != nil {
			fmt.Fprintf(os.Stderr, "warning: %v\n", bufErr)
		}
		// Return success — mutation is durable in JSONL.
		return msg, campfireID, nil
	}

	// Campfire send succeeded — update sync cursor and attempt to flush any
	// buffered pending mutations. Both are fire-and-forget: failures are logged
	// as warnings but never returned to the caller.
	if projectDir, ok := readyProjectDir(); ok {
		if markErr := rdSync.MarkSynced(projectDir, msg.ID, time.Now().UnixNano()); markErr != nil {
			fmt.Fprintf(os.Stderr, "warning: could not update sync cursor: %v\n", markErr)
		}
		// Flush pending — attempt to drain .ready/pending.jsonl using the same
		// campfire send path. Build a flusher that re-uses the already-open store.
		m, memberErr := s.GetMembership(campfireID)
		if memberErr == nil && m != nil {
			flushFn := buildFlusher(agentID, s, m, campfireID, projectDir)
			if n, flushErr := rdSync.FlushPending(projectDir, flushFn); flushErr != nil {
				fmt.Fprintf(os.Stderr, "warning: partial pending flush (%d sent): %v\n", n, flushErr)
			}
		}
	}

	return msg, campfireID, nil
}

// buildFlusher returns a Flusher that sends a single pending MutationRecord to campfire.
// It reconstructs the message from the stored record fields and writes it via the
// filesystem transport directly. The original message ID and timestamp from the pending
// record are preserved so that the campfire message ID matches the ID already written
// to mutations.jsonl. This cannot use client.Send() because client.Send() generates a
// new message ID — preserving the original ID is a permanent constraint (D6).
func buildFlusher(agentID *identity.Identity, s store.Store, m *store.Membership, campfireID, projectDir string) rdSync.Flusher {
	return func(rec jsonl.MutationRecord) error {
		// Reconstruct the message preserving the original message ID and timestamp.
		// We build the Message struct directly and re-sign using the same signing
		// logic as message.NewMessage, but with rec.MsgID and rec.Timestamp instead
		// of freshly generated values. This ensures the campfire message ID matches
		// the ID already recorded in mutations.jsonl.
		tags := rec.Tags
		if tags == nil {
			tags = []string{}
		}
		antecedents := rec.Antecedents
		if antecedents == nil {
			antecedents = []string{}
		}
		msg := &message.Message{
			ID:          rec.MsgID,
			Sender:      agentID.PublicKey,
			Payload:     []byte(rec.Payload),
			Tags:        tags,
			Antecedents: antecedents,
			Timestamp:   rec.Timestamp,
			Provenance:  []message.ProvenanceHop{},
		}
		signInput := message.MessageSignInput{
			ID:          msg.ID,
			Payload:     msg.Payload,
			Tags:        msg.Tags,
			Antecedents: msg.Antecedents,
			Timestamp:   msg.Timestamp,
		}
		signBytes, err := cfencoding.Marshal(signInput)
		if err != nil {
			return fmt.Errorf("encoding pending message sign input: %w", err)
		}
		msg.Signature = ed25519.Sign(agentID.PrivateKey, signBytes)
		return sendPrebuiltMessage(agentID, s, m, campfireID, msg)
	}
}

// sendPrebuiltMessage sends a pre-built message (with existing ID/signature) via the
// filesystem transport and stores it locally. Used exclusively by buildFlusher to flush
// pending mutations while preserving their original message IDs.
func sendPrebuiltMessage(agentID *identity.Identity, s store.Store, m *store.Membership, campfireID string, msg *message.Message) error {
	baseDir := fs.DefaultBaseDir()
	if m.TransportDir != "" {
		baseDir = filepath.Dir(m.TransportDir)
	}
	tr := fs.New(baseDir)

	members, err := tr.ListMembers(campfireID)
	if err != nil {
		return fmt.Errorf("listing members: %w", err)
	}
	isMember := false
	for _, mem := range members {
		if fmt.Sprintf("%x", mem.PublicKey) == agentID.PublicKeyHex() {
			isMember = true
			break
		}
	}
	if !isMember {
		return fmt.Errorf("not recognized as a member in the transport directory")
	}

	cfState, err := tr.ReadState(campfireID)
	if err != nil {
		return fmt.Errorf("reading campfire state: %w", err)
	}
	cf := cfState.ToCampfire(members)
	if err := msg.AddHop(
		cfState.PrivateKey, cfState.PublicKey,
		cf.MembershipHash(), len(members),
		cfState.JoinProtocol, cfState.ReceptionRequirements,
		campfirepkg.RoleFull,
	); err != nil {
		return fmt.Errorf("adding provenance hop: %w", err)
	}

	if err := tr.WriteMessage(campfireID, msg); err != nil {
		return fmt.Errorf("writing message: %w", err)
	}

	// Store locally.
	if _, err := s.AddMessage(store.MessageRecordFromMessage(campfireID, msg, store.NowNano())); err != nil {
		// Non-fatal: transport write succeeded.
		fmt.Fprintf(os.Stderr, "warning: failed to cache message locally: %v\n", err)
	}

	return nil
}

// bufferToPending appends a mutation to .ready/pending.jsonl for later sync.
// Returns an error if no project root is found or the write fails.
// Callers log the error as a warning — the primary mutation already succeeded in JSONL.
func bufferToPending(msg *message.Message, campfireID, payload string, tags, antecedents []string) error {
	pp := pendingPath()
	if pp == "" {
		return fmt.Errorf("no project root found — cannot buffer mutation to pending.jsonl")
	}
	w := jsonl.NewWriter(pp)
	rec := jsonl.MutationRecord{
		MsgID:       msg.ID,
		CampfireID:  campfireID,
		Timestamp:   time.Now().UnixNano(),
		Operation:   extractOperationFromTags(tags),
		Payload:     json.RawMessage(payload),
		Tags:        tags,
		Antecedents: antecedents,
	}
	if err := w.Append(rec); err != nil {
		return fmt.Errorf("could not buffer mutation to pending.jsonl: %w", err)
	}
	return nil
}

// extractOperationFromTags returns the first "work:" tag from the tags list.
func extractOperationFromTags(tags []string) string {
	for _, t := range tags {
		if len(t) > 5 && t[:5] == "work:" {
			return t
		}
	}
	return ""
}

// projectRoot walks up from cwd looking for a .campfire/root file.
// Returns (campfireID, projectDir, true) if found.
func projectRoot() (campfireID string, projectDir string, ok bool) {
	dir, err := os.Getwd()
	if err != nil {
		return "", "", false
	}
	for {
		rootFile := filepath.Join(dir, ".campfire", "root")
		data, err := os.ReadFile(rootFile)
		if err == nil {
			id := strings.TrimSpace(string(data))
			if len(id) == 64 {
				return id, dir, true
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", "", false
}

// minInt returns the smaller of two ints.
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
