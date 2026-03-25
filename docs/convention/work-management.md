# Work Management Convention

**WG:** (new — proposed WG-4, Work)
**Version:** 0.3
**Status:** Draft
**Date:** 2026-03-25
**Target repo:** campfire/docs/conventions/work-management.md

---

## 1. Problem Statement

Humans and agents need a shared system for tracking work — creating tasks, assigning them, wiring dependencies, and surfacing what needs attention. Today this requires a dedicated storage backend (database, API, sync logic). The campfire protocol already provides append-only messaging, causal threading, named views, selective compaction, and executable conventions — all the primitives needed to track work natively.

This convention defines work operations as `convention:operation` declarations. The campfire runtime parses them into CLI subcommands and MCP tools automatically — no wrapper CLI needed for basic operations.

---

## 2. Scope

**In scope:**
- Work item lifecycle as convention operations
- Executable `convention:operation` declarations for each operation
- Named view definitions as S-expression predicates
- Futures integration for gate operations and delegation
- Delegation model (for/by/delegate)
- Dependency wiring (blocks/blocked_by)
- Gate operations (human escalation via futures)
- Compaction policy
- Conformance checker reference
- Rate limiting

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

## 4. Convention Declarations

Each operation is a `convention:operation` message posted to the work campfire. The campfire runtime parses these into callable tools (MCP for agents, CLI subcommands for humans). Tags, payloads, antecedents, and validation are all derived from the declaration — callers provide only the args.

### 4.1 Tag Vocabulary

**Operation tags** (exactly one per message, produced automatically by declarations):

`work:create`, `work:claim`, `work:status`, `work:delegate`, `work:block`, `work:unblock`, `work:gate`, `work:gate-resolve`, `work:update`, `work:close`

**Auxiliary tags** (zero or more, composed from args via glob rules):

| Tag pattern | Composed from arg | Cardinality |
|-------------|-------------------|-------------|
| `work:type:<type>` | `type` | exactly_one |
| `work:priority:<level>` | `priority` | exactly_one |
| `work:level:<level>` | `level` | at_most_one |
| `work:project:<name>` | `project` | at_most_one |
| `work:for:<identity>` | `for` | exactly_one |
| `work:by:<identity>` | `by` | at_most_one |
| `work:gate-type:<type>` | `gate_type` | exactly_one |
| `work:status:<status>` | `to` | exactly_one |
| `work:resolution:<res>` | `resolution` | exactly_one |

Tag composition is automatic. The caller passes `--type task --for baron@3dl.dev --priority p1` and the executor produces `work:type:task`, `work:for:baron@3dl.dev`, `work:priority:p1` tags. Tags are indexes — the payload is authoritative if they disagree.

**Tag composition rules:**
- A message MUST carry exactly one operation tag.
- A message MAY carry zero or more auxiliary tags.
- A message MUST NOT carry tags from other convention namespaces simultaneously.

### 4.2 `work:create`

Create a new work item.

```json
{
  "convention": "work",
  "version": "0.3",
  "operation": "create",
  "description": "Create a new work item",
  "args": [
    {"name": "id", "type": "string", "required": true, "max_length": 64, "pattern": "^[a-z0-9][a-z0-9-]{2,63}$"},
    {"name": "title", "type": "string", "required": true, "max_length": 256},
    {"name": "context", "type": "string", "max_length": 65536},
    {"name": "type", "type": "enum", "required": true, "values": ["task", "decision", "review", "reminder", "deadline", "prep", "message", "directive"]},
    {"name": "for", "type": "string", "required": true, "max_length": 256},
    {"name": "priority", "type": "enum", "required": true, "values": ["p0", "p1", "p2", "p3"]},
    {"name": "level", "type": "enum", "values": ["epic", "task", "subtask"]},
    {"name": "by", "type": "string", "max_length": 256},
    {"name": "project", "type": "string", "max_length": 64},
    {"name": "parent_id", "type": "string", "max_length": 64},
    {"name": "eta", "type": "string", "max_length": 32},
    {"name": "due", "type": "string", "max_length": 32}
  ],
  "produces_tags": [
    {"tag": "work:create", "cardinality": "exactly_one"},
    {"tag": "work:type:*", "cardinality": "exactly_one"},
    {"tag": "work:for:*", "cardinality": "exactly_one"},
    {"tag": "work:priority:*", "cardinality": "exactly_one"},
    {"tag": "work:level:*", "cardinality": "at_most_one"},
    {"tag": "work:by:*", "cardinality": "at_most_one"},
    {"tag": "work:project:*", "cardinality": "at_most_one"}
  ],
  "antecedents": "none",
  "signing": "member_key",
  "rate_limit": {"max": 50, "per": "sender", "window": "1h"}
}
```

`parent_id` establishes hierarchy (see §6.1). Dependencies between items are wired separately via `work:block`.

If `eta` is omitted, it is derived from priority: P0=now, P1=+4h, P2=+24h, P3=+72h.

**CLI invocation:**
```bash
cf work create \
  --id ready-a1b \
  --title "Implement widget parser" \
  --context "Parse incoming widget payloads per spec v2" \
  --type task \
  --for baron@3dl.dev \
  --priority p1 \
  --level task
```

### 4.3 `work:claim`

Accept delegation and transition to active. If the sender is not already the `by` party, claim also sets `by` to the sender.

```json
{
  "convention": "work",
  "version": "0.3",
  "operation": "claim",
  "description": "Accept work and transition to active",
  "args": [
    {"name": "target", "type": "message_id", "required": true},
    {"name": "reason", "type": "string", "max_length": 1024}
  ],
  "produces_tags": [
    {"tag": "work:claim", "cardinality": "exactly_one"}
  ],
  "antecedents": "exactly_one(target)",
  "signing": "member_key"
}
```

**Claim vs. delegate**: `work:delegate` is the `for` party saying "you do this." `work:claim` is the `by` party saying "I accept and am starting." An item can be delegated without being claimed (assigned but not yet started), or claimed without a prior delegate (self-assignment).

When a prior `work:delegate` was sent with `--future`, the claim message SHOULD use `--fulfills` to complete the delegation future (see §5).

**CLI invocation:**
```bash
cf work claim --target <create-msg-id> --reason "Accepting delegation"

# If fulfilling a delegation future:
cf work claim --target <create-msg-id> --fulfills <delegate-msg-id>
```

### 4.4 `work:status`

Transition an item's status.

```json
{
  "convention": "work",
  "version": "0.3",
  "operation": "status",
  "description": "Transition item status",
  "args": [
    {"name": "target", "type": "message_id", "required": true},
    {"name": "to", "type": "enum", "required": true, "values": ["inbox", "active", "scheduled", "waiting", "done", "cancelled", "failed"]},
    {"name": "from", "type": "enum", "values": ["inbox", "active", "scheduled", "waiting", "blocked"]},
    {"name": "reason", "type": "string", "max_length": 1024},
    {"name": "waiting_on", "type": "string", "max_length": 256},
    {"name": "waiting_type", "type": "enum", "values": ["person", "vendor", "client", "date", "event", "external", "agent", "gate"]}
  ],
  "produces_tags": [
    {"tag": "work:status", "cardinality": "exactly_one"},
    {"tag": "work:status:*", "cardinality": "exactly_one"}
  ],
  "antecedents": "exactly_one(target)",
  "signing": "member_key"
}
```

`waiting_since` is derived from the message timestamp when `to=waiting`.

**CLI invocation:**
```bash
cf work status --target <create-msg-id> --to waiting \
  --reason "Need vendor quote" \
  --waiting-on "vendor quote from Raytheon" \
  --waiting-type vendor
```

### 4.5 `work:delegate`

Assign or reassign the performer. Delegation is optionally a future — the delegator can `cf await` the claim.

```json
{
  "convention": "work",
  "version": "0.3",
  "operation": "delegate",
  "description": "Assign or reassign performer",
  "args": [
    {"name": "target", "type": "message_id", "required": true},
    {"name": "to", "type": "string", "required": true, "max_length": 256},
    {"name": "from", "type": "string", "max_length": 256},
    {"name": "reason", "type": "string", "max_length": 1024}
  ],
  "produces_tags": [
    {"tag": "work:delegate", "cardinality": "exactly_one"},
    {"tag": "work:by:*", "cardinality": "exactly_one"}
  ],
  "antecedents": "exactly_one(target)",
  "signing": "member_key"
}
```

**Delegate identity types:**
- Person: email or campfire key (`baron@3dl.dev`)
- Claude Code agent: session identifier (`claude-session-xyz`)
- Open agent: campfire key or registered name (`cf://agents/implementer`)
- Rudi automaton: namespaced identity (`atlas/worker-3`)
- Unassigned: omit `by` arg or set to empty string

**CLI invocation:**
```bash
# Fire-and-forget delegation:
cf work delegate --target <create-msg-id> --to atlas/worker-3

# Delegation as future — block until claimed:
msg_id=$(cf work delegate --target <create-msg-id> --to atlas/worker-3 \
  --future --json | jq -r .id)
cf await <campfire-id> "$msg_id" --timeout 10m
```

### 4.6 `work:block`

Wire a dependency between two items. The blocked item cannot enter the `ready` view until the blocker is closed or the dependency is removed.

```json
{
  "convention": "work",
  "version": "0.3",
  "operation": "block",
  "description": "Wire a dependency between items",
  "args": [
    {"name": "blocker_id", "type": "string", "required": true, "max_length": 64},
    {"name": "blocked_id", "type": "string", "required": true, "max_length": 64},
    {"name": "blocker_msg", "type": "message_id", "required": true},
    {"name": "blocked_msg", "type": "message_id", "required": true}
  ],
  "produces_tags": [
    {"tag": "work:block", "cardinality": "exactly_one"}
  ],
  "antecedents": "none",
  "signing": "member_key"
}
```

The `blocker_msg` and `blocked_msg` args carry the `work:create` message IDs for both items. Implementations MUST include both as antecedents on the sent message (the executor places them via the payload; the conformance checker verifies the causal links). `antecedents` is `"none"` in the declaration because the convention system's `exactly_one(target)` mode only supports a single antecedent — the two-antecedent requirement is enforced by the conformance checker (§8).

**CLI invocation:**
```bash
cf work block \
  --blocker-id ready-t01 --blocker-msg <t01-create-msg-id> \
  --blocked-id ready-t02 --blocked-msg <t02-create-msg-id>
```

### 4.7 `work:unblock`

Remove a dependency while the blocker is still open. Not needed when the blocker closes — implicit unblock handles that (§6 rule 5).

```json
{
  "convention": "work",
  "version": "0.3",
  "operation": "unblock",
  "description": "Remove a dependency between items",
  "args": [
    {"name": "target", "type": "message_id", "required": true},
    {"name": "reason", "type": "string", "max_length": 1024}
  ],
  "produces_tags": [
    {"tag": "work:unblock", "cardinality": "exactly_one"}
  ],
  "antecedents": "exactly_one(target)",
  "signing": "member_key"
}
```

The `target` is the `work:block` message being removed.

**CLI invocation:**
```bash
cf work unblock --target <block-msg-id> --reason "Dependency no longer needed"
```

### 4.8 `work:gate`

Request human escalation. This is a **future** — the agent sends the gate and can `cf await` the resolution without polling. The item implicitly transitions to `waiting` with `waiting_type: "gate"`.

```json
{
  "convention": "work",
  "version": "0.3",
  "operation": "gate",
  "description": "Request human escalation (sends future)",
  "args": [
    {"name": "target", "type": "message_id", "required": true},
    {"name": "gate_type", "type": "enum", "required": true, "values": ["budget", "design", "scope", "review", "human", "stall", "periodic"]},
    {"name": "description", "type": "string", "max_length": 4096}
  ],
  "produces_tags": [
    {"tag": "work:gate", "cardinality": "exactly_one"},
    {"tag": "work:gate-type:*", "cardinality": "exactly_one"}
  ],
  "antecedents": "exactly_one(target)",
  "signing": "member_key"
}
```

The gate message is always sent with `--future`. The agent then calls `cf await` to block until the human resolves it. No polling, no state machine — the agent's session suspends and resumes with the decision in hand.

**CLI invocation:**
```bash
# Agent sends gate and blocks:
gate_id=$(cf work gate --target <create-msg-id> \
  --gate-type design \
  --description "Flatten arrays for perf or keep nested for fidelity?" \
  --future --json | jq -r .id)

resolution=$(cf await <campfire-id> "$gate_id" --timeout 1h --json)
# resolution contains the gate-resolve payload
```

### 4.9 `work:gate-resolve`

Human approves or rejects a gate. Sent with `--fulfills` to complete the gate future.

```json
{
  "convention": "work",
  "version": "0.3",
  "operation": "gate-resolve",
  "description": "Resolve a human escalation gate",
  "args": [
    {"name": "target", "type": "message_id", "required": true},
    {"name": "resolution", "type": "enum", "required": true, "values": ["approved", "rejected"]},
    {"name": "reason", "type": "string", "max_length": 4096}
  ],
  "produces_tags": [
    {"tag": "work:gate-resolve", "cardinality": "exactly_one"},
    {"tag": "work:resolution:*", "cardinality": "exactly_one"}
  ],
  "antecedents": "exactly_one(target)",
  "signing": "member_key"
}
```

**Status side-effects:**
- `approved`: Item transitions back to `active`. The `by` party resumes work.
- `rejected`: Item remains in `waiting`. The `by` party SHOULD revise approach and either resume (`work:status` → active) or re-gate with a new question.

The `target` is the `work:gate` message. Because `--fulfills` implies `--reply-to`, the causal link is automatic.

**CLI invocation:**
```bash
cf work gate-resolve --target <gate-msg-id> \
  --resolution approved \
  --reason "Flatten — query perf wins" \
  --fulfills <gate-msg-id>
```

### 4.10 `work:update`

Modify mutable fields on an existing item.

```json
{
  "convention": "work",
  "version": "0.3",
  "operation": "update",
  "description": "Modify fields on a work item",
  "args": [
    {"name": "target", "type": "message_id", "required": true},
    {"name": "fields", "type": "json", "required": true}
  ],
  "produces_tags": [
    {"tag": "work:update", "cardinality": "exactly_one"}
  ],
  "antecedents": "exactly_one(target)",
  "signing": "member_key"
}
```

The `fields` arg is a JSON object with field names → new values. Mutable fields: `eta`, `due`, `priority`, `context`, `title`, `level`.

**CLI invocation:**
```bash
cf work update --target <create-msg-id> \
  --fields '{"eta":"2026-03-26T12:00:00Z","priority":"p1"}'
```

### 4.11 `work:close`

Close an item with a terminal resolution.

```json
{
  "convention": "work",
  "version": "0.3",
  "operation": "close",
  "description": "Close a work item",
  "args": [
    {"name": "target", "type": "message_id", "required": true},
    {"name": "resolution", "type": "enum", "required": true, "values": ["done", "cancelled", "failed"]},
    {"name": "reason", "type": "string", "max_length": 4096}
  ],
  "produces_tags": [
    {"tag": "work:close", "cardinality": "exactly_one"},
    {"tag": "work:resolution:*", "cardinality": "exactly_one"}
  ],
  "antecedents": "exactly_one(target)",
  "signing": "member_key"
}
```

**CLI invocation:**
```bash
cf work close --target <create-msg-id> \
  --resolution done \
  --reason "Widget parser implemented, tests passing"
```

---

## 5. Futures Integration

The campfire futures system (`--future`, `--fulfills`, `cf await`) replaces polling and state-machine coordination. Two operations in this convention are designed around futures.

### 5.1 Gate as Future

The gate/gate-resolve pair is a **synchronous escalation**: the agent needs a human decision before it can proceed.

```
Agent                          Human
  |                              |
  |-- work:gate --future ------->|  (agent blocks on cf await)
  |                              |
  |                              |  (human reviews)
  |                              |
  |<-- work:gate-resolve --------|  (--fulfills <gate-msg-id>)
  |    --fulfills                |
  |                              |
  |  (cf await returns)          |
  |  (agent reads resolution     |
  |   and resumes)               |
```

The agent's session suspends at `cf await`. When the human sends `gate-resolve` with `--fulfills`, the await returns the fulfillment message. The agent reads `resolution` and `reason` from the payload and continues — no cursor management, no polling loop, no intermediate state.

**Timeout**: If `cf await` times out, the agent SHOULD re-post the gate (the prior future is stale) or escalate to a higher gate type.

### 5.2 Delegation as Future (optional)

Delegation can optionally use futures for synchronous assignment — the delegator blocks until the delegate claims.

```
Baron                          Automaton
  |                              |
  |-- work:delegate --future --->|  (Baron blocks on cf await)
  |                              |
  |                              |  (automaton sees delegation)
  |                              |
  |<-- work:claim ---------------|  (--fulfills <delegate-msg-id>)
  |    --fulfills                |
  |                              |
  |  (cf await returns)          |
  |  (Baron sees claim)          |
```

This is optional — fire-and-forget delegation (without `--future`) is valid when the delegator doesn't need synchronous confirmation. The `delegated:<identity>` view provides async visibility.

### 5.3 Await Patterns for Agents

Agents using `cf await` in their session can chain operations without losing context:

```bash
# Send gate, block, resume with decision
gate_id=$(cf work gate --target "$item_msg" \
  --gate-type design --description "..." \
  --future --json | jq -r .id)

decision=$(cf await "$campfire" "$gate_id" --timeout 1h --json)
resolution=$(echo "$decision" | jq -r .payload.resolution)

if [ "$resolution" = "approved" ]; then
  # Continue implementation
  ...
  cf work close --target "$item_msg" --resolution done --reason "..."
else
  # Read rejection reason, revise
  reason=$(echo "$decision" | jq -r .payload.reason)
  ...
fi
```

---

## 6. State Derivation

Current item state is derived by replaying the message log:

1. `work:create` establishes the initial state.
2. Subsequent operations modify derived state. The most recent message for each field wins.
3. `blocked` status is derived: if any open blocker exists (via `work:block` without a corresponding `work:unblock` or blocker `work:close`), the item is blocked regardless of explicit status.
4. **Implicit unblock on close**: When a blocker item is closed (`work:close`), all items it blocks are automatically unblocked — no explicit `work:unblock` needed. Explicit `work:unblock` is only for removing dependencies while the blocker is still open.
5. `work:gate` implicitly transitions the item to `waiting` with `waiting_type: "gate"`.
6. `work:gate-resolve` with `approved` implicitly transitions the item to `active`.
7. `work:claim` implicitly transitions the item to `active` and sets `by` to the sender if not already set.

### 6.1 Hierarchy

Items form a strict three-level hierarchy: Epic → Task → Subtask.

Parent-child relationships are established by including `parent_id` in the `work:create` args. Parents are containers, not gates — parent status is independent of child status. Epics do not auto-close when all children complete; the `for` party sends an explicit `work:close`. Implementations MAY surface a notification when all children of an epic reach terminal status.

---

## 7. Named Views (Attention Engine)

Views are `campfire:view` messages with S-expression predicates. They are registered once on the work campfire and materialized on read.

### 7.1 View Definitions

**`ready`** — What needs attention now:
```bash
cf view create <campfire-id> ready --predicate '
  (and
    (tag "work:create")
    (not (tag "work:status:done"))
    (not (tag "work:status:cancelled"))
    (not (tag "work:status:failed"))
    (not (tag "work:status:waiting"))
    (not (tag "work:status:scheduled"))
  )
' --ordering "timestamp asc"
```

Note: The `ready` view is further refined by the state derivation engine — items with open blockers (derived from `work:block`/`work:unblock`/blocker close) are excluded even if they pass the tag predicate. The ETA window (`eta < now + 4h OR eta IS NULL`) is applied at query time. Items where the `work:create` has a subsequent `work:close` are excluded.

**`work`** — Actively being worked on:
```bash
cf view create <campfire-id> work --predicate '(tag "work:status:active")'
```

**`pending`** — Parked items:
```bash
cf view create <campfire-id> pending --predicate '
  (or
    (tag "work:status:waiting")
    (tag "work:status:scheduled")
  )
'
```

**`overdue`** — Past-due items:
```bash
cf view create <campfire-id> overdue --predicate '
  (and
    (tag "work:create")
    (not (tag "work:status:done"))
    (not (tag "work:status:cancelled"))
    (not (tag "work:status:failed"))
    (lt (field "payload.eta") (timestamp))
  )
'
```

**`gates`** — Pending human escalations:
```bash
cf view create <campfire-id> gates --predicate '
  (and
    (tag "work:gate")
    (not (tag "work:gate-resolve"))
  )
'
```

Note: This predicate finds gate messages that have no corresponding fulfillment. The futures system tracks this — an unfulfilled `work:gate` future is a pending gate.

**Identity-scoped views** are parameterized at query time:

| View | Filter | Purpose |
|------|--------|---------|
| `for:<identity>` | `(tag "work:for:<identity>")` + open | Everything for a person |
| `by:<identity>` | `(tag "work:by:<identity>")` + open | Everything assigned to a performer |
| `delegated:<identity>` | `for=<identity> AND by!=<identity>` + open | Delegated work, in progress |

### 7.2 Sort Order

All views sort by ETA ascending (nearest first), with priority as tiebreaker (P0 before P1) for items with equal or null ETA. Specified via `--ordering "timestamp asc"` on creation; ETA-based sort is applied by the state derivation layer.

### 7.3 View Materialization

```bash
# Read a view:
cf view read <campfire-id> ready

# Read with JSON output for programmatic use:
cf view read <campfire-id> ready --json

# Read with field projection:
cf view read <campfire-id> ready --fields id,tags,payload
```

`rd ready` resolves to the `ready` view filtered by `for:<me> OR by:<me>`.

### 7.4 Follow Mode for Real-Time Queues

Agents and routing services can watch views in real time:

```bash
# Stream new items as they become ready:
cf read <campfire-id> --tag work:create --follow

# Watch for gates needing resolution:
cf read <campfire-id> --tag work:gate --follow
```

Combined with `--json`, this enables routing agents that auto-assign items based on type, skill tags, priority, and capacity — built entirely on convention primitives.

---

## 8. Delegation Model

Delegation is the core routing mechanism.

### 8.1 Delegation Chain

An item starts with `for: baron` and `by: unassigned`. Baron delegates:

```bash
cf work delegate --target <msg> --to alice@3dl.dev
cf work delegate --target <msg> --to atlas/worker-3
cf work delegate --target <msg> --to cf://agents/implementer
```

The `for` field does not change on delegation — Baron still needs the outcome. Multiple levels of delegation are supported: Alice can delegate to her own agent. Each level is a `work:delegate` message with the previous delegate as sender.

### 8.2 Delegation Visibility

The `delegated:<identity>` view shows items where the identity is the `for` party and someone else is the `by` party. This is Baron's "things I'm waiting on from my delegates" view.

For synchronous delegation visibility, use the futures pattern (§5.2) — delegate with `--future`, await the claim.

### 8.3 Intelligent Routing

When `by` is unassigned, the item sits in the `for` party's ready queue. Routing agents can watch the ready view via `--follow` (§7.4) and auto-assign items. This is not part of the convention — it's an implementation built on delegation primitives.

---

## 9. Compaction Policy

### 9.1 Never Compact (structural)

- `work:create` — item existence
- `work:close` — final state
- `work:gate` — escalation record (future)
- `work:gate-resolve` — escalation resolution (fulfillment)
- `work:block` / `work:unblock` — dependency structure

### 9.2 Compact After Close (operational)

Once an item reaches terminal status (done / cancelled / failed), the following messages for that item MAY be compacted:

- `work:status` — intermediate transitions
- `work:claim` — who worked on it (captured in close reason)
- `work:update` — field changes (final state captured in create + close)
- `work:delegate` — delegation chain (final assignment captured in close)

### 9.3 Compact-to-Archive

Compacted messages MUST be moved to an archive campfire, not deleted. The archive campfire ID is referenced from the work campfire's metadata. Causal chain is preserved — archived messages keep their IDs and antecedent links.

```bash
cf compact <campfire-id> --summary "Archived operational messages for closed items ready-t01, ready-t02, ready-t03"
```

---

## 10. Conformance

### 10.1 Declaration-Enforced (automatic)

The convention executor enforces these via the declarations in §4:

- **Arg validation**: Required args present, types match, max_length/pattern/enum constraints pass.
- **Tag composition**: Correct tags produced with correct cardinality.
- **Antecedent resolution**: `exactly_one(target)` ensures causal link to the target item.
- **Rate limiting**: `work:create` limited to 50/sender/hour. Prevents attention queue flooding.
- **Signing**: All operations require `member_key` — sender must be a campfire member.

### 10.2 Conformance Checker (application-level)

Beyond what the executor validates, a conformance checker SHOULD verify:

- **Sender authority for `work:close`**: Sender is the `by` party, the `for` party, or an authorized operator.
- **Sender authority for `work:delegate`**: Sender is the `for` party or a delegate in the chain.
- **Block antecedents**: `work:block` messages have antecedents to both items' `work:create` messages (the declaration uses `antecedents: "none"` because the convention system only supports single-target antecedent resolution; the two-antecedent rule is checker-enforced via the `blocker_msg` and `blocked_msg` payload args).
- **Status transition validity**: The `from` field (if provided) matches the item's current derived status.
- **ETA bounds**: P0 items SHOULD have ETA ≤ now; implementations MAY reject artificially low ETAs on non-P0 items.
- **Hierarchy depth**: `parent_id` chains must not exceed 3 levels (epic → task → subtask).

### 10.3 Extension Points

- Implementations MAY add custom auxiliary tags with a project-specific prefix (e.g., `rudi:skill:<name>`).
- Implementations MAY extend the compaction policy for project-specific operational messages.
- Implementations MAY register additional named views beyond those in §7.

---

## 11. Security Considerations

- **Item injection**: Mitigated by campfire membership controls, trust convention verification, and `work:create` rate limiting (50/sender/hour per §4.2).
- **Status spoofing**: Conformance checker (§10.2) verifies sender authority for `work:close` and `work:delegate`.
- **Delegation hijacking**: Conformance checker verifies sender has delegation authority.
- **ETA manipulation**: Conformance checker MAY enforce ETA bounds based on priority.
- **Payload injection**: Item context is markdown and TAINTED. Rendering surfaces MUST sanitize.
- **Gate spoofing**: `work:gate-resolve` requires antecedent to the `work:gate` message, and the futures system verifies fulfillment links. A spoofed resolution without the causal link will not complete the agent's `cf await`.

---

## 12. Examples

Examples show the actual `cf` CLI invocations. The convention executor handles tag composition, payload construction, and antecedent wiring from the args.

### 12.1 Create → Delegate → Claim → Complete

```bash
# Step 1 — Baron creates a task
msg_create=$(cf work create \
  --id ready-a1b \
  --title "Implement widget parser" \
  --context "Parse incoming widget payloads per spec v2" \
  --type task --for baron@3dl.dev --priority p1 --level task \
  --json | jq -r .id)

# Step 2 — Baron delegates to automaton (as future, to await claim)
msg_delegate=$(cf work delegate \
  --target "$msg_create" --to atlas/worker-3 \
  --reason "Delegating to automaton for implementation" \
  --future --json | jq -r .id)

# Step 3 — Automaton claims (fulfills the delegation future)
cf work claim --target "$msg_create" \
  --reason "Accepting delegation" \
  --fulfills "$msg_delegate"
# Baron's cf await returns — delegation confirmed.

# Step 4 — Automaton completes
cf work close --target "$msg_create" \
  --resolution done --reason "Widget parser implemented, tests passing"
```

### 12.2 Epic with Sequential Children

```bash
# Create the epic
msg_epic=$(cf work create --id ready-e01 \
  --title "Ship widget feature" --context "Full pipeline: parse, validate, store" \
  --type task --for baron@3dl.dev --priority p1 --level epic \
  --json | jq -r .id)

# Create children
msg_t01=$(cf work create --id ready-t01 --title "Widget parser" \
  --parent-id ready-e01 --context "Parse widget payloads per spec v2" \
  --type task --for baron@3dl.dev --priority p1 --level task \
  --json | jq -r .id)

msg_t02=$(cf work create --id ready-t02 --title "Widget validator" \
  --parent-id ready-e01 --context "Validate parsed widgets against schema" \
  --type task --for baron@3dl.dev --priority p1 --level task \
  --json | jq -r .id)

msg_t03=$(cf work create --id ready-t03 --title "Widget storage" \
  --parent-id ready-e01 --context "Persist validated widgets to store" \
  --type task --for baron@3dl.dev --priority p1 --level task \
  --json | jq -r .id)

# Wire sequential dependencies
cf work block --blocker-id ready-t01 --blocker-msg "$msg_t01" \
  --blocked-id ready-t02 --blocked-msg "$msg_t02"
cf work block --blocker-id ready-t02 --blocker-msg "$msg_t02" \
  --blocked-id ready-t03 --blocked-msg "$msg_t03"

# At this point: ready-t01 is ready. ready-t02, ready-t03 are blocked.

# Children complete in sequence — each close implicitly unblocks the next (§6 rule 4)
cf work close --target "$msg_t01" --resolution done --reason "Parser implemented"
# → ready-t02 unblocked

cf work close --target "$msg_t02" --resolution done --reason "Validator implemented"
# → ready-t03 unblocked

cf work close --target "$msg_t03" --resolution done --reason "Storage implemented"

# Close the epic (explicit — does not auto-close)
cf work close --target "$msg_epic" --resolution done \
  --reason "All children complete — widget feature shipped"
```

### 12.3 Gate Operation (futures)

An agent hits a design question, sends a gate future, blocks until Baron resolves it.

```bash
# Agent sends gate future and blocks
gate_id=$(cf work gate --target "$msg_create" \
  --gate-type design \
  --description "Widget schema: flatten arrays for query perf or keep nested for fidelity?" \
  --future --json | jq -r .id)

# Item is now waiting (gate side-effect). Appears in 'gates' view.

# Agent blocks:
resolution=$(cf await "$campfire" "$gate_id" --timeout 1h --json)

# Meanwhile, Baron sees it in the gates view:
cf view read "$campfire" gates

# Baron approves (fulfills the future):
cf work gate-resolve --target "$gate_id" \
  --resolution approved \
  --reason "Flatten — query performance wins. Fidelity recoverable from raw payload." \
  --fulfills "$gate_id"

# Agent's cf await returns with Baron's resolution payload.
# Item transitions back to active. Agent reads reason and continues.
```

**If Baron rejects:**
```bash
cf work gate-resolve --target "$gate_id" \
  --resolution rejected \
  --reason "Neither option. Explore hybrid: flat index + nested storage." \
  --fulfills "$gate_id"

# Agent reads rejection, revises approach, may re-gate or resume.
```

### 12.4 Waiting on External Vendor

```bash
# Transition to waiting
cf work status --target "$msg_create" --to waiting \
  --reason "Need vendor quote for hardware pricing" \
  --waiting-on "vendor quote from Raytheon" \
  --waiting-type vendor

# waiting_since derived from message timestamp.
# Item moves from ready to pending view.

# Vendor responds — resume
cf work status --target "$msg_create" --to active \
  --from waiting \
  --reason "Raytheon quote received — \$45k for 100 units"
```

### 12.5 P0 Override

```bash
# P0 arrives — ETA defaults to now, appears at top of ready view
msg_p0=$(cf work create --id ready-p0x \
  --title "URGENT: Production widget pipeline down" \
  --context "500 errors on /api/widgets since 14:32 UTC. All ingest halted." \
  --type task --for baron@3dl.dev --priority p0 --level task \
  --json | jq -r .id)

# Claim immediately
cf work claim --target "$msg_p0" --reason "Taking P0 immediately"

# Optionally pause current work (preemption is policy, not primitive)
cf work status --target "$msg_t01" --to scheduled \
  --reason "Paused for P0 ready-p0x"

# Resolve P0
cf work close --target "$msg_p0" --resolution done \
  --reason "Root cause: stale cache after deploy. Flushed and verified."

# Resume paused work
cf work status --target "$msg_t01" --to active \
  --from scheduled --reason "Resuming after P0 resolved"
```

### 12.6 Routing Agent (follow mode)

A routing agent watches for new unassigned items and auto-delegates based on type:

```bash
# Watch for new items in real time
cf read "$campfire" --tag work:create --follow --json | while read -r msg; do
  by=$(echo "$msg" | jq -r '.payload.by // empty')
  type=$(echo "$msg" | jq -r '.payload.type')
  msg_id=$(echo "$msg" | jq -r '.id')

  # Skip already-assigned items
  [ -n "$by" ] && continue

  # Route by type
  case "$type" in
    task)     delegate_to="atlas/worker-3" ;;
    review)   delegate_to="reviewer@atlas" ;;
    decision) continue ;;  # humans only
    *)        continue ;;
  esac

  cf work delegate --target "$msg_id" --to "$delegate_to" \
    --reason "Auto-routed by type=$type"
done
```

This is not part of the convention — it's an implementation built entirely on convention primitives (`--follow`, `--json`, `work:delegate`).
