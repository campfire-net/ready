package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	campfirepkg "github.com/campfire-net/campfire/pkg/campfire"
	"github.com/campfire-net/campfire/pkg/identity"
	"github.com/campfire-net/campfire/pkg/message"
	"github.com/campfire-net/campfire/pkg/store"
	"github.com/campfire-net/campfire/pkg/transport"
	"github.com/campfire-net/campfire/pkg/transport/fs"
)

// sendToProjectCampfire sends a message to the project campfire (resolved via
// .campfire/root walk-up). Returns the sent message and the campfire ID.
func sendToProjectCampfire(agentID *identity.Identity, s store.Store, payload string, tags []string, antecedents []string) (*message.Message, string, error) {
	campfireID, _, ok := projectRoot()
	if !ok {
		return nil, "", fmt.Errorf("no .campfire/root found — run 'cf swarm start' or 'cf create' first")
	}
	m, err := s.GetMembership(campfireID)
	if err != nil {
		return nil, "", fmt.Errorf("querying membership: %w", err)
	}
	if m == nil {
		return nil, "", fmt.Errorf("not a member of campfire %s", campfireID[:minInt(12, len(campfireID))])
	}
	msg, err := sendViaMembership(agentID, s, m, campfireID, payload, tags, antecedents)
	if err != nil {
		return nil, "", err
	}
	return msg, campfireID, nil
}

// sendViaMembership routes a message through the transport indicated by the membership record.
func sendViaMembership(agentID *identity.Identity, s store.Store, m *store.Membership, campfireID, payload string, tags, antecedents []string) (*message.Message, error) {
	switch transport.ResolveType(*m) {
	case transport.TypePeerHTTP:
		return nil, fmt.Errorf("p2p-http transport not supported by rd (use cf send)")
	case transport.TypeGitHub:
		return nil, fmt.Errorf("github transport not supported by rd (use cf send)")
	default:
		return sendFilesystem(agentID, s, m, campfireID, payload, tags, antecedents)
	}
}

// sendFilesystem sends a message via filesystem transport and stores it locally.
func sendFilesystem(agentID *identity.Identity, s store.Store, m *store.Membership, campfireID, payload string, tags, antecedents []string) (*message.Message, error) {
	baseDir := fs.DefaultBaseDir()
	if m.TransportDir != "" {
		baseDir = filepath.Dir(m.TransportDir)
	}
	tr := fs.New(baseDir)

	members, err := tr.ListMembers(campfireID)
	if err != nil {
		return nil, fmt.Errorf("listing members: %w", err)
	}
	isMember := false
	for _, mem := range members {
		if fmt.Sprintf("%x", mem.PublicKey) == agentID.PublicKeyHex() {
			isMember = true
			break
		}
	}
	if !isMember {
		return nil, fmt.Errorf("not recognized as a member in the transport directory")
	}

	msg, err := message.NewMessage(agentID.PrivateKey, agentID.PublicKey, []byte(payload), tags, antecedents)
	if err != nil {
		return nil, fmt.Errorf("creating message: %w", err)
	}

	cfState, err := tr.ReadState(campfireID)
	if err != nil {
		return nil, fmt.Errorf("reading campfire state: %w", err)
	}
	cf := cfState.ToCampfire(members)
	if err := msg.AddHop(
		cfState.PrivateKey, cfState.PublicKey,
		cf.MembershipHash(), len(members),
		cfState.JoinProtocol, cfState.ReceptionRequirements,
		campfirepkg.RoleFull,
	); err != nil {
		return nil, fmt.Errorf("adding provenance hop: %w", err)
	}

	if err := tr.WriteMessage(campfireID, msg); err != nil {
		return nil, fmt.Errorf("writing message: %w", err)
	}

	// Store locally.
	if _, err := s.AddMessage(store.MessageRecordFromMessage(campfireID, msg, store.NowNano())); err != nil {
		// Non-fatal: transport write succeeded.
		fmt.Fprintf(os.Stderr, "warning: failed to cache message locally: %v\n", err)
	}

	return msg, nil
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
