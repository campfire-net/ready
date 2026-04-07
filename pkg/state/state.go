// Package state derives work item state from a campfire message log.
// The campfire is the backend — state is derived by replaying convention
// messages (work:create, work:status, work:claim, work:close, etc.) in
// timestamp order.
package state

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"time"

	"github.com/campfire-net/campfire/pkg/store"
)

// Status values as defined in the convention spec §4.4.
const (
	StatusInbox     = "inbox"
	StatusActive    = "active"
	StatusScheduled = "scheduled"
	StatusWaiting   = "waiting"
	StatusBlocked   = "blocked"
	StatusDone      = "done"
	StatusCancelled = "cancelled"
	StatusFailed    = "failed"
)

// TerminalStatuses is the set of statuses where an item is no longer active.
var TerminalStatuses = map[string]bool{
	StatusDone:      true,
	StatusCancelled: true,
	StatusFailed:    true,
}

// Item is the derived state of a work item. All fields are derived from
// campfire messages; the campfire message log is the source of truth.
type Item struct {
	// ID is the work item ID (project-prefixed, e.g. "ready-a1b").
	ID string `json:"id"`
	// MsgID is the campfire message ID of the work:create message.
	MsgID string `json:"msg_id"`
	// CampfireID is the campfire this item lives in.
	CampfireID string `json:"campfire_id"`

	Title       string `json:"title"`
	Context     string `json:"context,omitempty"`
	Description string `json:"description,omitempty"` // alias for context, for bd compatibility
	Type        string `json:"type"`
	Level   string `json:"level,omitempty"`
	Project string `json:"project,omitempty"`
	For     string `json:"for"`
	By      string `json:"by,omitempty"`

	Priority string `json:"priority"`
	Status   string `json:"status"`
	ETA      string `json:"eta,omitempty"`
	Due      string `json:"due,omitempty"`

	ParentID  string   `json:"parent_id,omitempty"`
	BlockedBy []string `json:"blocked_by,omitempty"`
	Blocks    []string `json:"blocks,omitempty"`

	// Gate is set when the item requires human escalation. Values: budget, design, scope, review, human, stall.
	Gate string `json:"gate,omitempty"`

	// WaitingOn / WaitingType / WaitingSince are set when status=waiting.
	WaitingOn    string `json:"waiting_on,omitempty"`
	WaitingType  string `json:"waiting_type,omitempty"`
	WaitingSince string `json:"waiting_since,omitempty"`

	// GateMsgID is the campfire message ID of the most recent unfulfilled
	// work:gate message. Cleared when the gate is resolved.
	GateMsgID string `json:"gate_msg_id,omitempty"`

	// CreatedAt is the timestamp of the work:create message (unix nanos).
	CreatedAt int64 `json:"created_at"`
	// UpdatedAt is the timestamp of the most recent state-changing message.
	UpdatedAt int64 `json:"updated_at"`

	// History is the audit trail of status-changing events, in chronological order.
	// Populated from work:create, work:status, work:claim, work:close messages,
	// and from ImportHistory entries embedded in work:update messages.
	History []HistoryEntry `json:"history,omitempty"`
}

// HistoryEntry is a single audit trail entry for a work item.
type HistoryEntry struct {
	// Timestamp is ISO8601 UTC — either the original event time (for imported
	// history) or the campfire message time.
	Timestamp string `json:"timestamp"`
	// FromStatus is the status before this change.
	FromStatus string `json:"from_status"`
	// ToStatus is the status after this change.
	ToStatus string `json:"to_status"`
	// ChangedBy is the actor (email, pubkey hex, or "system").
	ChangedBy string `json:"changed_by"`
	// Note is an optional human-readable description of the change.
	Note string `json:"note,omitempty"`
}

// createPayload mirrors the fields in a work:create message payload.
type createPayload struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Context  string `json:"context"`
	Type     string `json:"type"`
	Level    string `json:"level"`
	Project  string `json:"project"`
	For      string `json:"for"`
	By       string `json:"by"`
	Priority string `json:"priority"`
	ParentID string `json:"parent_id"`
	ETA      string `json:"eta"`
	Due      string `json:"due"`
	Gate     string `json:"gate"`
}

// statusPayload mirrors the fields in a work:status message payload.
type statusPayload struct {
	Target      string `json:"target"`
	To          string `json:"to"`
	Reason      string `json:"reason"`
	WaitingOn   string `json:"waiting_on"`
	WaitingType string `json:"waiting_type"`
}

// claimPayload mirrors the fields in a work:claim message payload.
type claimPayload struct {
	Target string `json:"target"`
	Reason string `json:"reason"`
}

// delegatePayload mirrors the fields in a work:delegate message payload.
type delegatePayload struct {
	Target string `json:"target"`
	To     string `json:"to"`
	From   string `json:"from"`
	Reason string `json:"reason"`
}

// closePayload mirrors the fields in a work:close message payload.
type closePayload struct {
	Target     string `json:"target"`
	Resolution string `json:"resolution"`
	Reason     string `json:"reason"`
}

// updatePayload mirrors the fields in a work:update message payload.
type updatePayload struct {
	Target   string `json:"target"`
	Title    string `json:"title,omitempty"`
	Context  string `json:"context,omitempty"`
	Priority string `json:"priority,omitempty"`
	ETA      string `json:"eta,omitempty"`
	Due      string `json:"due,omitempty"`
	Level    string `json:"level,omitempty"`
	For      string `json:"for,omitempty"`
	By       string `json:"by,omitempty"`
	Gate     string `json:"gate,omitempty"`
	// ImportHistory carries historical audit entries replayed during migration.
	// Each entry is appended to the item's History slice. Original actor and
	// timestamp are preserved in the entry; the campfire message timestamp
	// reflects import time (SendRequest has no Timestamp field in cf 0.14 —
	// see rd item rudi-trl).
	ImportHistory []HistoryEntry `json:"import_history,omitempty"`
}

// blockPayload mirrors the fields in a work:block message payload.
type blockPayload struct {
	BlockerID  string `json:"blocker_id"`
	BlockedID  string `json:"blocked_id"`
	BlockerMsg string `json:"blocker_msg"`
	BlockedMsg string `json:"blocked_msg"`
}

// unblockPayload mirrors the fields in a work:unblock message payload.
type unblockPayload struct {
	Target string `json:"target"`
	Reason string `json:"reason"`
}

// gatePayload mirrors the fields in a work:gate message payload.
type gatePayload struct {
	Target      string `json:"target"`
	GateType    string `json:"gate_type"`
	Description string `json:"description,omitempty"`
}

// gateResolvePayload mirrors the fields in a work:gate-resolve message payload.
type gateResolvePayload struct {
	Target     string `json:"target"`
	Resolution string `json:"resolution"`
	Reason     string `json:"reason,omitempty"`
}

// serverBindingPayload mirrors the fields in a convention:server-binding message payload.
type serverBindingPayload struct {
	Convention string `json:"convention"`
	Operation  string `json:"operation"`
	ServerPubkey string `json:"server_pubkey"`
	ValidFrom  string `json:"valid_from"`
	ValidUntil string `json:"valid_until,omitempty"`
}

// capabilityTokenPayload represents an offline capability token embedded in an operation.
type capabilityTokenPayload struct {
	Subject      string   `json:"subject"`
	Campfire     string   `json:"campfire"`
	Role         string   `json:"role"`
	Operations   []string `json:"operations"`
	IssuedAt     int64    `json:"issued_at"`
	ExpiresAt    int64    `json:"expires_at"`
	BindingMsgID string   `json:"binding_msg_id,omitempty"`
	Signature    string   `json:"signature"`
}

// hasTag reports whether tags contains the given tag.
func hasTag(tags []string, tag string) bool {
	for _, t := range tags {
		if t == tag {
			return true
		}
	}
	return false
}

// etaFromPriority derives the default ETA from priority if none was specified.
// P0=now, P1=+4h, P2=+24h, P3=+72h.
func etaFromPriority(priority string, now time.Time) string {
	switch priority {
	case "p0":
		return now.UTC().Format(time.RFC3339)
	case "p1":
		return now.Add(4 * time.Hour).UTC().Format(time.RFC3339)
	case "p2":
		return now.Add(24 * time.Hour).UTC().Format(time.RFC3339)
	case "p3":
		return now.Add(72 * time.Hour).UTC().Format(time.RFC3339)
	default:
		return now.Add(24 * time.Hour).UTC().Format(time.RFC3339)
	}
}

// resolveItemID finds an item ID by looking up the target in msgIndex,
// falling back to antecedents if target is empty or not found.
func resolveItemID(msgIndex map[string]string, target string, antecedents []string) string {
	if target != "" {
		if id := msgIndex[target]; id != "" {
			return id
		}
	}
	for _, ant := range antecedents {
		if id := msgIndex[ant]; id != "" {
			return id
		}
	}
	return ""
}

// serverBinding represents an active server-binding declaration for a (convention, operation) pair.
type serverBinding struct {
	serverPubkey string
	validFrom    int64 // nanoseconds (unix timestamp converted)
	validUntil   int64 // nanoseconds, 0 means no expiration
	msgID        string
}

// roleInfo represents a member's role state from work:role-grant messages.
type roleInfo struct {
	role      string
	grantedAt int64 // nanoseconds
	expiresAt int64 // nanoseconds, 0 means no expiration
	msgID     string
}

// parseTimestamp converts an RFC3339 or unix timestamp string to int64 nanoseconds.
// Returns 0 if parsing fails.
func parseTimestamp(ts string) int64 {
	// Try RFC3339 first
	if t, err := time.Parse(time.RFC3339, ts); err == nil {
		return t.UnixNano()
	}
	// Try unix seconds
	if unixSec, err := strconv.ParseInt(ts, 10, 64); err == nil {
		return unixSec * 1e9
	}
	return 0
}

// findActiveServerBinding finds the most recent server-binding declaration
// that is valid at the given timestamp for the specified convention and operation.
func findActiveServerBinding(msgs []store.MessageRecord, convention, operation string, atTime int64) *serverBinding {
	var active *serverBinding
	for _, m := range msgs {
		if !hasTag(m.Tags, "convention:server-binding") {
			continue
		}
		var p serverBindingPayload
		if err := json.Unmarshal(m.Payload, &p); err != nil {
			continue
		}
		if p.Convention != convention || p.Operation != operation {
			continue
		}
		validFrom := parseTimestamp(p.ValidFrom)
		if validFrom == 0 || validFrom > atTime {
			// Binding not yet valid at this message's timestamp
			continue
		}
		validUntil := parseTimestamp(p.ValidUntil)
		if validUntil != 0 && validUntil < atTime {
			// Binding has expired
			continue
		}
		// Keep the most recent valid binding
		if active == nil || validFrom > active.validFrom {
			active = &serverBinding{
				serverPubkey: p.ServerPubkey,
				validFrom:    validFrom,
				validUntil:   validUntil,
				msgID:        m.ID,
			}
		}
	}
	return active
}

// findFulfillmentForOperation finds a fulfillment message that matches the given
// target message, sent by the specified sender pubkey.
func findFulfillmentForOperation(msgs []store.MessageRecord, targetMsgID string, senderPubkey string) *store.MessageRecord {
	for i, m := range msgs {
		if hasTag(m.Tags, "fulfills") && m.Sender == senderPubkey {
			// Check if this fulfillment targets the operation
			// Fulfillment messages have the target in their Antecedents
			for _, ant := range m.Antecedents {
				if ant == targetMsgID {
					return &msgs[i]
				}
			}
		}
	}
	return nil
}

// isOperationAuthorized checks if a consequential operation is authorized under
// the server-binding gating rules.
//
// Rules:
// - If no server-binding exists (bypass mode): accept all (Wave 1 behavior)
// - If binding exists and operation has a valid fulfillment from the bound server: accept
// - If binding exists and operation has a valid capability token: accept
// - If binding exists and operation was issued before the binding's valid_from: accept (pre-binding)
// - Otherwise: reject (drop from derived state)
//
// roleMap is reserved for Wave 3+ role-based authorization (currently unused here).
func isOperationAuthorized(msgs []store.MessageRecord, op *store.MessageRecord, convention, operation string, _ map[string]roleInfo) bool {
	// Find the active server-binding at the time of this operation.
	binding := findActiveServerBinding(msgs, convention, operation, op.Timestamp)

	// Bypass mode: if no binding exists, accept all (Wave 1 behavior).
	if binding == nil {
		return true
	}

	// Binding exists. Check for valid fulfillment or capability token.

	// 1. Check for a fulfillment message from the bound server.
	if findFulfillmentForOperation(msgs, op.ID, binding.serverPubkey) != nil {
		return true
	}

	// 2. Check for a valid capability token in the operation payload.
	if hasValidCapabilityToken(op, binding.serverPubkey) {
		return true
	}

	// 3. Check if this operation was issued before the binding became valid (pre-binding items).
	if op.Timestamp < binding.validFrom {
		return true
	}

	// No authorization path succeeded.
	return false
}

// hasValidCapabilityToken checks if an operation message contains a valid
// capability token signed by the specified server pubkey.
//
// For now, this is a stub that returns false. Full implementation requires
// cryptographic signature verification (Ed25519), which is deferred to Wave 2+
// as part of the capability token verification framework.
func hasValidCapabilityToken(op *store.MessageRecord, serverPubkey string) bool {
	// Stub implementation. Full token verification requires:
	// 1. Extract token from operation payload
	// 2. Verify signature against serverPubkey
	// 3. Check expires_at > now
	// 4. Check operation in token's operations list
	// 5. Check subject matches sender
	//
	// For now, always return false to avoid false positives.
	return false
}

// Derive replays all work convention messages from the provided campfire message
// records and returns the derived item states keyed by item ID. Messages must
// be from a single campfire and are processed in timestamp order (the store
// returns them in ascending timestamp order by default).
//
// Two-pass approach:
// Pass 1: Build the role map from work:role-grant messages. Latest grant per pubkey wins.
// Pass 2: Replay operations, with fulfillment gating for consequential ops (close, delegate, gate-resolve).
func Derive(campfireID string, msgs []store.MessageRecord) map[string]*Item {
	// Pass 1: Build role map from work:role-grant messages.
	// Map[pubkey] -> roleInfo
	roleMap := make(map[string]roleInfo)
	for _, m := range msgs {
		if !hasTag(m.Tags, "work:role-grant") {
			continue
		}
		var p struct {
			Pubkey    string `json:"pubkey"`
			Role      string `json:"role"`
			GrantedAt int64  `json:"granted_at,omitempty"`
			ExpiresAt int64  `json:"expires_at,omitempty"`
		}
		if err := json.Unmarshal(m.Payload, &p); err != nil {
			continue
		}
		if p.Pubkey == "" {
			continue
		}
		grantedAt := m.Timestamp
		if p.GrantedAt != 0 {
			grantedAt = p.GrantedAt
		}
		// Only update role map if this grant is more recent than what's already there.
		existing, ok := roleMap[p.Pubkey]
		if !ok || grantedAt >= existing.grantedAt {
			roleMap[p.Pubkey] = roleInfo{
				role:      p.Role,
				grantedAt: grantedAt,
				expiresAt: p.ExpiresAt,
				msgID:     m.ID,
			}
		}
	}

	// Pass 2: Replay operations with fulfillment gating.
	// msgIndex: campfire message ID → item ID (for resolving target references)
	msgIndex := make(map[string]string)
	items := make(map[string]*Item)

	// blockEdges tracks blocker→blocked relationships.
	// When a blocker item closes, its entries are removed.
	type blockEdge struct {
		blockerID  string
		blockedID  string
		blockMsgID string // campfire message ID of the work:block message
	}
	var blockEdges []blockEdge
	// blockMsgIndex maps the work:block message ID to the edge's blockerID+blockedID
	// for removal by work:unblock.
	blockMsgIndex := make(map[string]struct {
		blockerID string
		blockedID string
	})

	// gateMsgIndex maps a gate message ID → item ID, used by work:gate-resolve.
	gateMsgIndex := make(map[string]string)

	for _, m := range msgs {
		switch {
		case hasTag(m.Tags, "work:create"):
			var p createPayload
			if err := json.Unmarshal(m.Payload, &p); err != nil {
				continue
			}
			if p.ID == "" {
				continue
			}
			now := time.Unix(0, m.Timestamp)
			eta := p.ETA
			if eta == "" {
				eta = etaFromPriority(p.Priority, now)
			}
			item := &Item{
				ID:          p.ID,
				MsgID:       m.ID,
				CampfireID:  campfireID,
				Title:       p.Title,
				Context:     p.Context,
				Description: p.Context, // alias for bd compatibility
				Type:        p.Type,
				Level:      p.Level,
				Project:    p.Project,
				For:        p.For,
				By:         p.By,
				Priority:   p.Priority,
				Status:     StatusInbox,
				ETA:        eta,
				Due:        p.Due,
				ParentID:   p.ParentID,
				Gate:       p.Gate,
				CreatedAt:  m.Timestamp,
				UpdatedAt:  m.Timestamp,
			}
			item.History = append(item.History, HistoryEntry{
				Timestamp:  time.Unix(0, m.Timestamp).UTC().Format(time.RFC3339),
				FromStatus: StatusInbox,
				ToStatus:   StatusInbox,
				ChangedBy:  m.Sender,
				Note:       "created",
			})
			items[p.ID] = item
			msgIndex[m.ID] = p.ID

		case hasTag(m.Tags, "work:status"):
			var p statusPayload
			if err := json.Unmarshal(m.Payload, &p); err != nil {
				continue
			}
			itemID := resolveItemID(msgIndex, p.Target, m.Antecedents)
			item, ok := items[itemID]
			if !ok {
				continue
			}
			prevStatus := item.Status
			item.Status = p.To
			item.UpdatedAt = m.Timestamp
			item.History = append(item.History, HistoryEntry{
				Timestamp:  time.Unix(0, m.Timestamp).UTC().Format(time.RFC3339),
				FromStatus: prevStatus,
				ToStatus:   p.To,
				ChangedBy:  m.Sender,
				Note:       p.Reason,
			})
			if p.To == StatusWaiting {
				item.WaitingOn = p.WaitingOn
				item.WaitingType = p.WaitingType
				item.WaitingSince = time.Unix(0, m.Timestamp).UTC().Format(time.RFC3339)
			} else {
				item.WaitingOn = ""
				item.WaitingType = ""
				item.WaitingSince = ""
			}

		case hasTag(m.Tags, "work:claim"):
			var p claimPayload
			if err := json.Unmarshal(m.Payload, &p); err != nil {
				continue
			}
			itemID := resolveItemID(msgIndex, p.Target, m.Antecedents)
			item, ok := items[itemID]
			if !ok {
				continue
			}
			// Claim sets by=sender and transitions to active.
			prevStatus := item.Status
			item.By = m.Sender
			item.Status = StatusActive
			item.UpdatedAt = m.Timestamp
			item.History = append(item.History, HistoryEntry{
				Timestamp:  time.Unix(0, m.Timestamp).UTC().Format(time.RFC3339),
				FromStatus: prevStatus,
				ToStatus:   StatusActive,
				ChangedBy:  m.Sender,
			})

		case hasTag(m.Tags, "work:delegate"):
			// Check fulfillment gating for consequential operation.
			if !isOperationAuthorized(msgs, m, "work", "delegate", roleMap) {
				continue
			}

			var p delegatePayload
			if err := json.Unmarshal(m.Payload, &p); err != nil {
				continue
			}
			itemID := resolveItemID(msgIndex, p.Target, m.Antecedents)
			item, ok := items[itemID]
			if !ok {
				continue
			}
			item.By = p.To
			item.UpdatedAt = m.Timestamp

		case hasTag(m.Tags, "work:close"):
			// Check fulfillment gating for consequential operation.
			if !isOperationAuthorized(msgs, m, "work", "close", roleMap) {
				continue
			}

			var p closePayload
			if err := json.Unmarshal(m.Payload, &p); err != nil {
				continue
			}
			itemID := resolveItemID(msgIndex, p.Target, m.Antecedents)
			item, ok := items[itemID]
			if !ok {
				continue
			}
			// Resolution maps to terminal status.
			prevStatus := item.Status
			switch p.Resolution {
			case "done":
				item.Status = StatusDone
			case "cancelled":
				item.Status = StatusCancelled
			case "failed":
				item.Status = StatusFailed
			default:
				item.Status = StatusDone
			}
			item.UpdatedAt = m.Timestamp
			item.History = append(item.History, HistoryEntry{
				Timestamp:  time.Unix(0, m.Timestamp).UTC().Format(time.RFC3339),
				FromStatus: prevStatus,
				ToStatus:   item.Status,
				ChangedBy:  m.Sender,
				Note:       p.Reason,
			})
			// Implicit unblock: remove all block edges where this item is the blocker.
			// Also clean up blockMsgIndex so stale entries don't linger.
			var newEdges []blockEdge
			for _, edge := range blockEdges {
				if edge.blockerID != item.ID {
					newEdges = append(newEdges, edge)
				} else {
					delete(blockMsgIndex, edge.blockMsgID)
				}
			}
			blockEdges = newEdges

		case hasTag(m.Tags, "work:update"):
			var p updatePayload
			if err := json.Unmarshal(m.Payload, &p); err != nil {
				continue
			}
			itemID := resolveItemID(msgIndex, p.Target, m.Antecedents)
			item, ok := items[itemID]
			if !ok {
				continue
			}
			// Apply field updates. The sentinel "-" clears a field.
			if p.Title != "" {
				item.Title = clearOrSet(p.Title)
			}
			if p.Context != "" {
				item.Context = clearOrSet(p.Context)
				item.Description = item.Context // keep alias in sync
			}
			if p.Priority != "" {
				item.Priority = clearOrSet(p.Priority)
			}
			if p.ETA != "" {
				item.ETA = clearOrSet(p.ETA)
			}
			if p.Due != "" {
				item.Due = clearOrSet(p.Due)
			}
			if p.Level != "" {
				item.Level = clearOrSet(p.Level)
			}
			if p.For != "" {
				item.For = clearOrSet(p.For)
			}
			if p.By != "" {
				item.By = clearOrSet(p.By)
			}
			if p.Gate != "" {
				item.Gate = clearOrSet(p.Gate)
			}
			// ImportHistory: append original history entries from migration replay.
			if len(p.ImportHistory) > 0 {
				item.History = append(item.History, p.ImportHistory...)
			}
			item.UpdatedAt = m.Timestamp

		case hasTag(m.Tags, "work:block"):
			var p blockPayload
			if err := json.Unmarshal(m.Payload, &p); err != nil {
				continue
			}
			if p.BlockerID == "" || p.BlockedID == "" {
				continue
			}
			blockEdges = append(blockEdges, blockEdge{
				blockerID:  p.BlockerID,
				blockedID:  p.BlockedID,
				blockMsgID: m.ID,
			})
			blockMsgIndex[m.ID] = struct {
				blockerID string
				blockedID string
			}{p.BlockerID, p.BlockedID}

		case hasTag(m.Tags, "work:unblock"):
			var p unblockPayload
			if err := json.Unmarshal(m.Payload, &p); err != nil {
				continue
			}
			targetMsg := p.Target
			if targetMsg == "" {
				for _, ant := range m.Antecedents {
					targetMsg = ant
					break
				}
			}
			if edge, ok := blockMsgIndex[targetMsg]; ok {
				var newEdges []blockEdge
				for _, e := range blockEdges {
					if e.blockerID != edge.blockerID || e.blockedID != edge.blockedID {
						newEdges = append(newEdges, e)
					}
				}
				blockEdges = newEdges
				delete(blockMsgIndex, targetMsg)
			}

		case hasTag(m.Tags, "work:gate"):
			// work:gate implicitly transitions item to waiting with waiting_type=gate.
			// The gate message is always sent as --future in a full implementation.
			// TODO: when campfire transport supports --future, this should be sent
			// with --future and resolved via cf await. For now, we send normally.
			var p gatePayload
			if err := json.Unmarshal(m.Payload, &p); err != nil {
				continue
			}
			itemID := resolveItemID(msgIndex, p.Target, m.Antecedents)
			item, ok := items[itemID]
			if !ok {
				continue
			}
			item.Status = StatusWaiting
			item.WaitingType = "gate"
			item.WaitingOn = p.Description
			item.WaitingSince = time.Unix(0, m.Timestamp).UTC().Format(time.RFC3339)
			item.GateMsgID = m.ID
			item.UpdatedAt = m.Timestamp
			// Register gate message ID → item ID for gate-resolve lookup.
			gateMsgIndex[m.ID] = itemID

		case hasTag(m.Tags, "work:gate-resolve"):
			// Check fulfillment gating for consequential operation.
			if !isOperationAuthorized(msgs, m, "work", "gate-resolve", roleMap) {
				continue
			}

			// work:gate-resolve fulfills the gate future. Target is the work:gate message.
			// approved → transition to active; rejected → remain waiting.
			var p gateResolvePayload
			if err := json.Unmarshal(m.Payload, &p); err != nil {
				continue
			}
			// Resolve via target (gate msg ID) or antecedents.
			gateMsgID := p.Target
			if gateMsgID == "" {
				for _, ant := range m.Antecedents {
					if _, ok := gateMsgIndex[ant]; ok {
						gateMsgID = ant
						break
					}
				}
			}
			itemID := gateMsgIndex[gateMsgID]
			if itemID == "" {
				continue
			}
			item, ok := items[itemID]
			if !ok {
				continue
			}
			if p.Resolution == "approved" {
				item.Status = StatusActive
				item.WaitingOn = ""
				item.WaitingType = ""
				item.WaitingSince = ""
				item.GateMsgID = ""
			}
			// rejected: item remains waiting; gate stays open.
			// The by party should revise approach and re-gate or resume.
			item.UpdatedAt = m.Timestamp
		}
	}

	// Apply derived block status. An item is blocked if at least one of its
	// blocker items is non-terminal. Only apply to non-terminal items.
	for _, edge := range blockEdges {
		blocker, blockerOK := items[edge.blockerID]
		blocked, blockedOK := items[edge.blockedID]
		if !blockerOK || !blockedOK {
			continue
		}
		if TerminalStatuses[blocked.Status] {
			continue
		}
		if !TerminalStatuses[blocker.Status] {
			blocked.Status = StatusBlocked
		}
		// Wire up the relationships on the items.
		blocked.BlockedBy = appendUnique(blocked.BlockedBy, edge.blockerID)
		blocker.Blocks = appendUnique(blocker.Blocks, edge.blockedID)
	}

	return items
}

// DeriveFromStore loads all messages from the given campfire and derives item state.
func DeriveFromStore(s store.Store, campfireID string) (map[string]*Item, error) {
	msgs, err := s.ListMessages(campfireID, 0, store.MessageFilter{})
	if err != nil {
		return nil, err
	}
	return Derive(campfireID, msgs), nil
}

// DeriveFromJSONL reads all MutationRecords from the given JSONL file path,
// converts them to store.MessageRecord, and derives item state by replaying
// the mutation log. The campfireID is inferred from the first record's
// CampfireID field; callers may pass an empty string to use that default,
// or pass an explicit value to override (useful in tests).
//
// Returns an empty map (not an error) when the file does not exist —
// a missing mutations.jsonl is valid for a freshly initialised project.
func DeriveFromJSONL(path string) (map[string]*Item, error) {
	return DeriveFromJSONLWithCampfire(path, "")
}

// DeriveFromJSONLWithCampfire is like DeriveFromJSONL but accepts an explicit
// campfireID override. When campfireID is empty, it is inferred from the first
// record's CampfireID field (falling back to an empty string).
func DeriveFromJSONLWithCampfire(path, campfireID string) (map[string]*Item, error) {
	records, err := readMutations(path)
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return make(map[string]*Item), nil
	}
	// Infer campfireID from first record when not provided.
	if campfireID == "" {
		campfireID = records[0].CampfireID
	}
	// Convert MutationRecords → store.MessageRecord.
	msgs := make([]store.MessageRecord, len(records))
	for i, r := range records {
		msgs[i] = store.MessageRecord{
			ID:          r.MsgID,
			CampfireID:  r.CampfireID,
			Timestamp:   r.Timestamp,
			Payload:     []byte(r.Payload),
			Tags:        r.Tags,
			Sender:      r.Sender,
			Antecedents: r.Antecedents,
			ReceivedAt:  r.Timestamp,
		}
	}
	return Derive(campfireID, msgs), nil
}

// mutationRecord is a minimal local type that mirrors jsonl.MutationRecord.
// We define it here to avoid an import cycle (jsonl imports nothing from state,
// but state must not import jsonl either). The JSON field names match exactly.
type mutationRecord struct {
	MsgID       string          `json:"msg_id"`
	CampfireID  string          `json:"campfire_id"`
	Timestamp   int64           `json:"timestamp"`
	Operation   string          `json:"operation"`
	Payload     json.RawMessage `json:"payload"`
	Tags        []string        `json:"tags"`
	Sender      string          `json:"sender"`
	Antecedents []string        `json:"antecedents,omitempty"`
}

// readMutations opens the JSONL file at path and returns all valid records
// sorted by Timestamp ascending. Returns nil, nil when the file does not exist.
func readMutations(path string) ([]mutationRecord, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("state: open %s: %w", path, err)
	}
	defer f.Close()

	var records []mutationRecord
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 10*1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var r mutationRecord
		if err := json.Unmarshal(line, &r); err != nil {
			continue // skip malformed lines — resilience
		}
		records = append(records, r)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("state: read %s: %w", path, err)
	}
	// Sort by timestamp ascending (matches store behaviour).
	sort.Slice(records, func(i, j int) bool {
		return records[i].Timestamp < records[j].Timestamp
	})
	return records, nil
}

// ClearSentinel is the value that clears a field in work:update.
const ClearSentinel = "-"

// clearOrSet returns "" if val is the clear sentinel, otherwise returns val.
func clearOrSet(val string) string {
	if val == ClearSentinel {
		return ""
	}
	return val
}

// appendUnique appends val to slice only if not already present.
func appendUnique(slice []string, val string) []string {
	for _, v := range slice {
		if v == val {
			return slice
		}
	}
	return append(slice, val)
}

// IsBlocked reports whether item is currently blocked.
func IsBlocked(item *Item) bool {
	return item.Status == StatusBlocked
}

// IsTerminal reports whether item is in a terminal state.
func IsTerminal(item *Item) bool {
	return TerminalStatuses[item.Status]
}
