# Work Management Convention

**WG:** (new — proposed WG-4, Work)
**Version:** 0.1
**Status:** Draft
**Date:** 2026-03-25
**Target repo:** campfire/docs/conventions/work-management.md

---

## 1. Problem Statement

Humans and agents need a shared system for tracking work — creating tasks, assigning them, wiring dependencies, and surfacing what needs attention. Today this requires a dedicated storage backend (database, API, sync logic). The campfire protocol already provides append-only messaging, causal threading, named views, and selective compaction — all the primitives needed to track work natively.

This convention defines how work items are represented as campfire messages, how state is derived from the message log, and how attention routing works through named views.

---

## 2. Scope

**In scope:**
- Work item lifecycle as campfire messages
- Tag vocabulary for work operations
- Payload schema for item creation and mutation
- Named view definitions for attention routing
- Delegation model (for/by/delegate)
- Dependency wiring (blocks/blocked_by)
- Gate operations (human escalation)
- Compaction policy
- Conformance requirements

**Not in scope:**
- Orchestration / automaton lifecycle (rudi automaton engine)
- Web rendering (implementation choice)
- Notification delivery (Teams, email — surface implementations)
- Cross-project portfolio queries (higher-level tooling)

---

## 3. Field Classification

| Field | Classification | Rationale |
|-------|---------------|-----------|
| `sender` | verified | Ed25519 public key, must match signature |
| `signature` | verified | Cryptographic proof of authorship |
| `tags` | **TAINTED** | Sender-chosen operation labels |
| `payload` | **TAINTED** | Sender-controlled item data |
| `antecedents` | **TAINTED** | Sender-asserted causal claims |
| `timestamp` | **TAINTED** | Sender's wall clock |

---

## 4. Tag Vocabulary

All work management convention tags use the `work:` prefix.

### 4.1 Operation Tags

| Tag | Semantics | Antecedent required? |
|-----|-----------|---------------------|
| `work:create` | Create a new work item | No |
| `work:claim` | Set `by` field, transition to active | Yes — the `work:create` message |
| `work:status` | Transition status | Yes — the `work:create` message |
| `work:delegate` | Assign or reassign `by` | Yes — the `work:create` message |
| `work:block` | Wire a dependency between items | Yes — both `work:create` messages |
| `work:unblock` | Remove a dependency | Yes — the `work:block` message |
| `work:gate` | Request human escalation | Yes — the `work:create` message |
| `work:gate-resolve` | Human approves/rejects a gate | Yes — the `work:gate` message |
| `work:update` | Modify fields (eta, context, priority, etc.) | Yes — the `work:create` message |
| `work:close` | Done, cancelled, or failed with reason | Yes — the `work:create` message |

### 4.2 Auxiliary Tags

| Tag | Semantics |
|-----|-----------|
| `work:type:<type>` | Item type: task, decision, review, reminder, deadline, prep, message, directive |
| `work:priority:<level>` | Priority: p0, p1, p2, p3 |
| `work:level:<level>` | Hierarchy level: epic, task, subtask |
| `work:project:<name>` | Project namespace |
| `work:for:<identity>` | Who needs the outcome |
| `work:by:<identity>` | Who is doing the work |
| `work:gate-type:<type>` | Gate category: budget, design, scope, review, human, stall, periodic |

### 4.3 Tag Composition Rules

- A message MUST carry exactly one operation tag (§4.1).
- A message MAY carry zero or more auxiliary tags (§4.2).
- `work:create` MUST carry `work:type:*`, `work:for:*`, and `work:priority:*`.
- `work:create` SHOULD carry `work:by:*` if a performer is known. Omission means unassigned.
- `work:delegate` MUST carry `work:by:*` (the new delegate).
- A message MUST NOT carry tags from other convention namespaces simultaneously.

---

## 5. Payload Schema

### 5.1 `work:create` Payload

```json
{
  "id": "ready-a1b",
  "title": "Short human-readable title",
  "context": "Markdown description — enough to act on",
  "eta": "2026-03-25T16:00:00Z",
  "due": "2026-03-26T00:00:00Z"
}
```

Required: `id`, `title`. Optional: `context`, `eta`, `due`.

If `eta` is omitted, it is derived from priority: P0=now, P1=+4h, P2=+24h, P3=+72h.

### 5.2 `work:status` Payload

```json
{
  "item_id": "ready-a1b",
  "from": "inbox",
  "to": "active",
  "reason": "Starting implementation"
}
```

Required: `item_id`, `to`. Optional: `from` (for validation), `reason`.

### 5.3 `work:delegate` Payload

```json
{
  "item_id": "ready-a1b",
  "from": "baron@3dl.dev",
  "to": "implementer@atlas",
  "reason": "Delegating to automaton for implementation"
}
```

Required: `item_id`, `to`. Optional: `from`, `reason`.

**Delegate identity types:**
- Person: email or campfire key (`baron@3dl.dev`)
- Claude Code agent: session identifier (`claude-session-xyz`)
- Open agent: campfire key or registered name (`cf://agents/implementer`)
- Rudi automaton: namespaced identity (`atlas/worker-3`)
- Unassigned: omit `work:by:*` tag or set to empty string

### 5.4 `work:block` Payload

```json
{
  "blocker_id": "ready-c3d",
  "blocked_id": "ready-a1b"
}
```

Both required. The blocker item must be referenced by antecedent.

### 5.5 `work:gate` Payload

```json
{
  "item_id": "ready-a1b",
  "gate_type": "design",
  "description": "Architecture review needed before implementation"
}
```

Required: `item_id`, `gate_type`. Optional: `description`.

### 5.6 `work:gate-resolve` Payload

```json
{
  "item_id": "ready-a1b",
  "resolution": "approved",
  "reason": "Design looks good, proceed"
}
```

Required: `item_id`, `resolution` (approved / rejected). Optional: `reason`.

### 5.7 `work:close` Payload

```json
{
  "item_id": "ready-a1b",
  "resolution": "done",
  "reason": "Shipped and verified"
}
```

Required: `item_id`, `resolution` (done / cancelled / failed). Optional: `reason`.

### 5.8 `work:update` Payload

```json
{
  "item_id": "ready-a1b",
  "fields": {
    "eta": "2026-03-26T12:00:00Z",
    "priority": "p1",
    "context": "Updated description with new requirements"
  }
}
```

Required: `item_id`, `fields` (object with field names → new values).

### 5.9 Waiting Detail

When transitioning to `waiting` status, the `work:status` payload includes:

```json
{
  "item_id": "ready-a1b",
  "to": "waiting",
  "waiting_on": "vendor quote from Raytheon",
  "waiting_type": "vendor"
}
```

Optional: `waiting_on`, `waiting_type` (person / vendor / client / date / event / external / agent).

---

## 6. State Derivation

Current item state is derived by replaying the message log:

1. `work:create` establishes the initial state.
2. Subsequent operations (`work:status`, `work:delegate`, `work:update`, `work:block`, `work:close`) modify derived state.
3. The most recent message for each field wins.
4. `blocked` status is derived: if any open blocker exists (via `work:block` without a corresponding `work:unblock` or blocker `work:close`), the item is blocked regardless of explicit status.

### 6.1 Hierarchy

Items form a strict three-level hierarchy: Epic → Task → Subtask.

Parent-child relationships are established by including `parent_id` in the `work:create` payload. Parents are containers, not gates — parent status is independent of child status.

---

## 7. Named Views (Attention Engine)

These named views SHOULD be registered on any campfire operating the work management convention:

| View name | Predicate | Purpose |
|-----------|-----------|---------|
| `ready` | `status NOT IN (done, cancelled, failed) AND NOT blocked AND (eta < now + 4h OR eta IS NULL)` | What needs attention now |
| `work` | `status = active` | Actively being worked on |
| `pending` | `status IN (waiting, scheduled, blocked)` | Parked items |
| `overdue` | `eta < now AND status NOT IN (done, cancelled, failed)` | Past-due |
| `for:<identity>` | `for = <identity> AND status NOT IN (done, cancelled, failed)` | Everything for a specific person |
| `by:<identity>` | `by = <identity> AND status NOT IN (done, cancelled, failed)` | Everything assigned to a specific performer |
| `delegated:<identity>` | `for = <identity> AND by != <identity> AND by IS NOT NULL AND status NOT IN (done, cancelled, failed)` | Work delegated by a person |
| `gates` | `gate IS NOT NULL AND gate_resolved = false` | Pending human escalations |

Views are parameterized by the querying identity. `rd ready` resolves to the `ready` view filtered by `for:<me> OR by:<me>`.

---

## 8. Delegation Model

Delegation is the core routing mechanism.

### 8.1 Delegation Chain

An item starts with `for: baron` and `by: unassigned`. Baron delegates:

```
work:delegate → by: alice          (Alice is doing it)
work:delegate → by: atlas/worker-3 (automaton is doing it)
work:delegate → by: cf://agents/x  (external agent is doing it)
```

The `for` field does not change on delegation — Baron still needs the outcome. Multiple levels of delegation are supported: Alice can delegate to her own agent. The `for` chain is: Baron → Alice → agent. Each level is a `work:delegate` message with the previous delegate as sender.

### 8.2 Delegation Visibility

The `delegated:<identity>` view shows all items where the identity is the `for` party and someone else is the `by` party. This is Baron's "things I'm waiting on from my delegates" view.

### 8.3 Intelligent Routing

When `by` is unassigned, the item sits in the `for` party's ready queue. The `for` party (or an authorized routing agent) decides who handles it:

- Assign to a specific person or agent
- Assign to a skill-based router (automaton with matching capability)
- Leave unassigned (human will handle it themselves)

Routing agents can watch the `ready` view and auto-assign items based on type, skill tags, priority, and available capacity. This is not part of the convention — it's an implementation choice built on top of the convention's delegation primitives.

---

## 9. Compaction Policy

### 9.1 Never Compact (structural)

- `work:create` — item existence
- `work:close` — final state
- `work:gate` — escalation record
- `work:gate-resolve` — escalation resolution
- `work:block` / `work:unblock` — dependency structure

### 9.2 Compact After Close (operational)

Once an item reaches terminal status (done / cancelled / failed), the following messages for that item MAY be compacted:

- `work:status` — intermediate transitions
- `work:claim` — who worked on it (captured in close reason)
- `work:update` — field changes (final state captured in create + close)
- `work:delegate` — delegation chain (final assignment captured in close)

### 9.3 Compact-to-Archive

Compacted messages MUST be moved to an archive campfire, not deleted. The archive campfire ID is referenced from the work campfire's metadata. Causal chain is preserved — archived messages keep their IDs and antecedent links.

---

## 10. Conformance

### 10.1 MUST

- Operation messages MUST carry exactly one operation tag.
- `work:create` MUST include `work:type:*`, `work:for:*`, `work:priority:*` tags and `id`, `title` in payload.
- `work:close` MUST include `resolution` in payload.
- State-modifying operations MUST include an antecedent to the target item's `work:create` message.
- Compaction MUST follow §9 policy.

### 10.2 SHOULD

- `work:create` SHOULD include `context` sufficient for a cold reader to act on.
- `work:status` SHOULD include `reason`.
- Implementations SHOULD register the named views in §7.

### 10.3 MAY

- Implementations MAY add custom auxiliary tags with a project-specific prefix (e.g., `rudi:skill:<name>`).
- Implementations MAY extend the compaction policy for project-specific operational messages.

---

## 11. Security Considerations

- **Item injection**: A malicious agent could send `work:create` messages to flood the attention queue. Mitigated by campfire membership controls and trust convention verification.
- **Status spoofing**: An agent could send `work:close` for an item it doesn't own. Implementations SHOULD verify that the sender is the `by` party, the `for` party, or an authorized operator.
- **Delegation hijacking**: An agent could `work:delegate` to itself. Implementations SHOULD verify the sender has delegation authority (is the `for` party or a delegate in the chain).
- **ETA manipulation**: Artificially low ETAs could hijack attention priority. Implementations MAY enforce ETA bounds based on priority.
- **Payload injection**: Item context is markdown and TAINTED. Rendering surfaces MUST sanitize.
