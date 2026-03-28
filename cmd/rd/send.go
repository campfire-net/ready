package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	campfirepkg "github.com/campfire-net/campfire/pkg/campfire"
	"github.com/campfire-net/campfire/pkg/identity"
	"github.com/campfire-net/campfire/pkg/message"
	"github.com/campfire-net/campfire/pkg/store"
	"github.com/campfire-net/campfire/pkg/transport"
	"github.com/campfire-net/campfire/pkg/transport/fs"

	"github.com/campfire-net/ready/pkg/jsonl"
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

	m, err := s.GetMembership(campfireID)
	if err != nil || m == nil {
		// Campfire is configured but we're not a member yet, or store lookup
		// failed. Buffer for later sync and return success.
		fmt.Fprintf(os.Stderr, "warning: not a member of campfire %s — mutation buffered to pending.jsonl\n",
			campfireID[:minInt(12, len(campfireID))])
		if bufErr := bufferToPending(msg, campfireID, payload, tags, antecedents); bufErr != nil {
			fmt.Fprintf(os.Stderr, "warning: %v\n", bufErr)
		}
		return msg, campfireID, nil
	}

	// Try to add a provenance hop and send via transport.
	if sendErr := addHopAndSend(agentID, s, m, campfireID, msg); sendErr != nil {
		// Transport send failed — JSONL write already succeeded, buffer for sync.
		fmt.Fprintf(os.Stderr, "warning: campfire send failed (buffered to pending.jsonl): %v\n", sendErr)
		if bufErr := bufferToPending(msg, campfireID, payload, tags, antecedents); bufErr != nil {
			fmt.Fprintf(os.Stderr, "warning: %v\n", bufErr)
		}
		// Return success — mutation is durable in JSONL.
		return msg, campfireID, nil
	}

	return msg, campfireID, nil
}

// addHopAndSend adds a provenance hop to msg and writes it via the filesystem transport.
func addHopAndSend(agentID *identity.Identity, s store.Store, m *store.Membership, campfireID string, msg *message.Message) error {
	switch transport.ResolveType(*m) {
	case transport.TypePeerHTTP:
		return fmt.Errorf("p2p-http transport not supported by rd (use cf send)")
	case transport.TypeGitHub:
		return fmt.Errorf("github transport not supported by rd (use cf send)")
	default:
		return sendFilesystem(agentID, s, m, campfireID, msg)
	}
}

// sendViaMembership routes a message through the transport indicated by the membership record.
// Kept for callers in init.go that need to send declarations (no JSONL buffering needed).
func sendViaMembership(agentID *identity.Identity, s store.Store, m *store.Membership, campfireID, payload string, tags, antecedents []string) (*message.Message, error) {
	msg, err := message.NewMessage(agentID.PrivateKey, agentID.PublicKey, []byte(payload), tags, antecedents)
	if err != nil {
		return nil, fmt.Errorf("creating message: %w", err)
	}
	switch transport.ResolveType(*m) {
	case transport.TypePeerHTTP:
		return nil, fmt.Errorf("p2p-http transport not supported by rd (use cf send)")
	case transport.TypeGitHub:
		return nil, fmt.Errorf("github transport not supported by rd (use cf send)")
	default:
		if err := sendFilesystem(agentID, s, m, campfireID, msg); err != nil {
			return nil, err
		}
		return msg, nil
	}
}

// sendFilesystem sends a pre-built message via filesystem transport and stores it locally.
func sendFilesystem(agentID *identity.Identity, s store.Store, m *store.Membership, campfireID string, msg *message.Message) error {
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
