# Ready Federation, Authorization, and Naming — Architecture v2

**Date:** 2026-04-07
**Status:** Architecture (post-adversarial synthesis)
**Supersedes:** `federation-auth-naming.md` (v1 design brief)
**Audience:** Implementers building Waves 1–5

---

## 0. Source of Truth and Change Log

This document is the canonical architecture for ready's federation, authorization, and naming evolution. Where v1 (the design brief) and v2 (this document) disagree, **v2 wins**. v1 remains in the repo as historical context for the questions that drove the design.

**What changed from v1:**

1. **Anteroom rewritten.** The "publicly writable for one operation" anteroom does not exist as a campfire primitive. v2 uses the `consult` join policy already implemented in `pkg/naming/join_policy.go`. The maintainer's `rd ready` queue is the consult callback's destination. (Domain purist V3.)
2. **Convention server identity binding added.** v1 assumed clients could distinguish convention-server fulfillments from any other `fulfills` message. They cannot. v2 introduces a `convention:server-binding` campfire-key-signed declaration that names `(campfire, convention, operation) → server pubkey`. State derivation only honors fulfillments whose sender matches the binding. (Domain purist V2, Adversary A2.)
3. **`.campfire/root` retained as SDK contract.** The SDK's `pkg/naming/fswalk.go:30` reads `.campfire/root`. Ready does not have unilateral authority to retire it. v2 keeps `.campfire/root` as the SDK boundary artifact, and adds a `[ready]` section to `.cf/config.toml` for ready-specific project metadata (project name, etc.). No SDK PR is on ready's critical path. (Domain purist V1.)
4. **Offline authorization: capability tokens (C5 adopted).** Short-TTL Ed25519 capability tokens issued by the convention server, verified offline against the bound server pubkey. Resolves the optimistic-vs-halt false binary in v1 §8.4. (Adversary A3.)
5. **Authorization-as-projection (C3 adopted).** Member roles materialize as a Class 1 named filter projection keyed by pubkey. Role check is O(1), works offline, identical between client and server. (Adversary A1.)
6. **Wave 1.5 inserted (C6 adopted).** Local name binding (`rd init <name>` writes config) ships before authorization, so the convention server itself can be addressed by name in Wave 2. (Adversary A7.)
7. **Policy-as-campfire (C1) deferred, not adopted.** See §3.2 for the trade-off analysis. v2 ships an in-process Tier 1 enforcement model in Wave 2, with a clear path to a policy campfire later. The HTTP Tier 2 model is **not** adopted; it would require campfire-hosting infrastructure ready does not have on the critical path.
8. **C2 (projection-relay anteroom) discarded** because the anteroom itself is discarded. The consult join policy makes the relay unnecessary. Projections re-enter the design as the materialization mechanism for §3.4 (member-roles view).
9. **C4 (anteroom-as-campfire) folded into the consult join policy.** The consult callback can include an optional pointer to a "join-conversation" campfire created by the requester. Adopted as an option, not a requirement.
10. **A11 (in-memory provenance store) acknowledged as a campfire-hosting bug**, not ready's. Tracked as an external dependency.

---

## 1. Core Model (Confirmed Unchanged from Brief §1)

The following remain from v1 and are not relitigated:

- Work items are convention messages on append-only campfire logs.
- State is derived by replaying the log (`pkg/state/state.go:Derive`).
- There is no separate database, no API server, no migrations.
- Offline-first is non-negotiable: a user without network access can create and claim items; they sync on reconnect.
- `rd ready` is the attention primitive — not blocked, not terminal, ETA within 4 hours.
- The item model (`for`, `by`, status, priority/ETA, gates, dependencies) is correct.
- Naming uses dot notation, root-first (`acme.backend`), and resolution is campfire traversal with auto-join at each segment (confirmed in `pkg/naming/resolve.go:246-263`).
- Locality is the default: a local naming root is just as authoritative as AIETF's, beacon root is configurable, TOFU pinning applies.

---

## 2. Naming Architecture

### 2.1 Resolution Model

Resolution of `acme.backend` walks the naming hierarchy: join (or already-be-member-of) the root → resolve `acme` to a campfire ID → join `acme` → resolve `backend` within `acme` → that ID is the project campfire.

This is implemented by `pkg/naming/resolve.go`. Ready uses it via `protocol.Client` once `WithNamingResolver` is exported (see §2.2). The walk is semi-automatic: `AutoJoinFunc` fires for every intermediate naming campfire, but **the root is not auto-joined** — the caller must already be a member of the root campfire (`resolve.go:122-125`). This is the bootstrap step: configure the beacon root, join it once, everything below is automatic.

**TOFU pinning of beacon root** is required after first use (per `design-locality.md`). Ready surfaces a one-time warning when a non-default beacon root is configured.

### 2.2 Config: Two Files, One Boundary

**Decision:** Ready maintains the SDK's `.campfire/root` file as the on-disk per-directory root pointer (the SDK contract). Ready additionally writes a `[ready]` section into `.cf/config.toml` for ready-specific project metadata.

**Why:** `pkg/naming/fswalk.go:30` reads `.campfire/root` directly. Ready cannot retire that file without an upstream SDK change, and ready's critical path does not include SDK changes. TOML's unknown-section tolerance means ready can colocate its config in `.cf/config.toml` without an SDK schema change.

**Layout after Wave 4:**

```
project-root/
├── .campfire/
│   └── root              # 64-char hex campfire ID — SDK contract, never user-edited
├── .cf/
│   └── config.toml       # SDK config + [ready] section
└── ...
```

`.cf/config.toml`:

```toml
[identity]
# SDK-managed

[behavior]
auto_join = true
walk_up = true

[ready]
project = "acme.backend"     # human name; resolved to campfire ID via naming
convention_server = "acme.auth"  # name, not pubkey hex
```

**Both files are written by `rd init`.** The campfire ID in `.campfire/root` is treated as the cached resolution of the `[ready].project` name. If they ever disagree (manual edit, corrupted state), `[ready].project` is authoritative — `rd` re-resolves it and rewrites `.campfire/root`.

**Open SDK-extension item (deferred, not blocking):** rd-XXX "Propose `[ready]` (or generic `[project]`) section to campfire SDK config schema" — when the SDK accepts a `project = "name"` field natively, ready drops `.campfire/root`. Until then, both files coexist.

### 2.3 Locality and Beacon Roots

**COMPLIANT — no change from brief.** Beacon root is configurable via `CF_BEACON_ROOT` or `--beacon-root`. The default AIETF root is one option among many. Ready surfaces this in `rd init` help text. TOFU pinning applies to the beacon root after first use; warning required for non-default roots.

---

## 3. Authorization Architecture

### 3.1 Server-Identity Binding (Resolves Domain Purist V2 + Adversary A2)

`dispatcher.go:548-566`'s `sendFulfillment` posts a `fulfills`-tagged message via `client.Send`. The signature on the message is whatever pubkey the dispatcher's `protocol.Client` carries. **There is no protocol-level "I am the convention server" attribute.**

**Decision:** Ready introduces a new convention declaration, `convention:server-binding`, posted into the project campfire by the campfire owner (signing tier: `campfire_key`). The binding declares:

```json
{
  "convention": "convention:server-binding",
  "version": "0.1",
  "operation": "bind",
  "args": [
    {"name": "convention", "type": "string", "required": true},
    {"name": "operation", "type": "string", "required": true},
    {"name": "server_pubkey", "type": "pubkey", "required": true},
    {"name": "valid_from", "type": "timestamp", "required": true},
    {"name": "valid_until", "type": "timestamp", "required": false}
  ],
  "produces_tags": [{"tag": "convention:server-binding", "cardinality": "at_most_one_per_op"}],
  "signing": "campfire_key"
}
```

A binding message in the project campfire log says: "for the `work-management` convention's `close` operation, fulfillments must be signed by `<server_pubkey>`." State derivation only honors `fulfills` messages whose sender pubkey matches the active binding.

**Key rotation** is just posting a new binding with a later `valid_from`. Old fulfillments before the rotation remain valid (they were signed by the old key, which the historical binding still authorized).

**Bootstrap:** The first binding for a project is posted by the campfire owner at project creation time, in the same atomic step that deploys (or registers) the convention server. For solo workflow this is a no-op — see §3.2 bypass mode.

### 3.2 Convention Server: HTTP vs Policy Campfire (C1 Evaluation)

**Trade-off:**

| | HTTP Tier 2 (v1 brief) | Policy Campfire (C1) |
|---|---|---|
| Discovery | Hardcoded URL or DNS | Campfire address (cf:// URI, name-resolvable) |
| Auth state visibility | Black-box service | First-class campfire log, `cf read` inspectable |
| Federation | Per-tenant URL | Single policy campfire serves many projects |
| Latency | One HTTP RTT | Two campfire round-trips (write request, read fulfillment) |
| Hosting | Requires HTTP service infra (campfire-hosting ACA) | Requires only a campfire (anywhere) |
| Solo workflow | Needs local HTTP server | Needs only a local in-process loop |
| Implementation cost | Existing dispatcher.go path | New "policy campfire" pattern, untested in SDK |
| Cold start | ACA scale-from-zero | None (campfire is always-on storage) |

**Decision: ship Tier 1 in-process enforcement in Wave 2; defer policy campfire to Wave 5+ as a possible federation option.**

Reasoning:
- The HTTP Tier 2 model has a hard external dependency on campfire-hosting infrastructure that is not on ready's critical path. Adopting it forces ready to wait for ACA.
- The policy campfire model is architecturally elegant but is an unproven pattern. Building it for Wave 2 would block ready on a campfire SDK feature that doesn't yet exist.
- **In-process Tier 1** — the convention server is a goroutine inside the `rd` process for solo work, and the same code can be packaged as a sidecar for team work — is the only model that ships in Wave 2 without external dependencies. It still produces signed `fulfills` messages in the campfire log. The signing key is the rd process's identity. The server-binding declaration (§3.1) names that identity as authoritative.
- For multi-machine teams, the convention server runs on one designated machine (typically the maintainer's), addressed by name (`acme.auth`) via Wave 1.5 naming. The fact that it speaks campfire-write-and-fulfill rather than HTTP is incidental.
- A future migration to policy-campfire-as-service is non-breaking: the binding declaration already abstracts the server's pubkey, and the campfire log is the same in either case.

**Bypass mode (mandatory for Wave 2 ship):** If no `convention:server-binding` is present in a project campfire, state derivation accepts all messages from members (Wave 1 behavior). This is the solo path: no convention server, no binding, no enforcement. The moment a binding is posted, enforcement engages. This solves Adversary A3's pessimistic-halt failure for solo users.

### 3.3 Role Model: Ready Overlay, Not SDK Extension

**Decision:** Roles (`maintainer`, `contributor`, `agent`, `observer`) are a ready-specific overlay implemented as campfire messages, not an SDK primitive. The role assignment is itself a signed convention message, similar to a join grant.

A `work:role-grant` message:
- Tag: `work:role-grant`
- Sender: a `maintainer` (or self, for the bootstrap maintainer of a freshly-created campfire)
- Payload: `{pubkey, role, granted_at, expires_at?}`
- Signing: member key, with min_operator_level=2

The bootstrap rule: the campfire creator (whoever's pubkey first appears as the campfire owner in the membership log) is implicitly `maintainer`. All other members are implicitly `contributor` until a role-grant message says otherwise. This avoids a chicken-and-egg "who grants the first maintainer role."

### 3.4 Authorization as Class 1 Projection (C3 Adopted)

**Decision:** Member roles materialize as a named filter projection on the project campfire:

```
name:        member-roles
predicate:   (tag "work:role-grant")
entity_key:  payload.pubkey
refresh:     on-write
```

Latest `work:role-grant` for a pubkey is the active record. A revocation is `work:role-grant` with `role = "revoked"`. Authorization check is `GetProjectionEntry(campfireID, "member-roles", senderPubkey)` — O(1).

**Why this matters for A1 (non-monotonic state):** Role state is derivable from the log alone, with no fulfillment dependency. Two clients replaying the log at any timestamp see the same role assignments. The non-monotonic problem in v1 was that *operation fulfillment* was Class 3 (depends on a separate fulfillment message arriving). By splitting authorization into two layers — *role state* (Class 1, deterministic) and *operation fulfillment* (Class 3, used for policy decisions the server makes about specific operations like close/delegate) — we eliminate the non-monotonic problem for the role layer entirely.

Operation fulfillment remains Class 3 for now. The fulfillment window is the convention server's responsibility to bound; see §3.5.

### 3.5 Offline Authorization: Capability Tokens (C5 Adopted)

**Decision:** Adopt capability tokens for offline operation.

The convention server issues short-TTL Ed25519-signed tokens to members on demand:

```json
{
  "subject": "<member_pubkey>",
  "campfire": "<campfire_id>",
  "role": "contributor",
  "operations": ["work:close", "work:claim", "work:gate-resolve"],
  "issued_at": 1712534400,
  "expires_at": 1712548800,
  "binding_msg_id": "<server_binding_msg_id>"
}
```

Signed by the convention server's private key (the one the binding declaration names).

**Verification (offline):** A client receiving an operation accompanied by a capability token validates:
1. Token signature against the bound server pubkey (read from the local `member-roles` projection or the binding cache).
2. `expires_at > now`.
3. Operation is in the token's `operations` list.
4. Subject matches the operation's sender.

If valid, the operation is applied to derived state immediately, no server round-trip.

**TTLs by operation severity:**
- Bulk safe ops (`work:create`, `work:claim`, `work:update`): 48h
- Consequential ops (`work:close`, `work:delegate`): 24h
- High-stakes (`work:gate-resolve`): 4h

**Reconciliation on reconnect:** When the client reconnects, the convention server reads the log, sees the operations, verifies them against the same token logic, and posts retroactive `fulfills` messages. State derivation now sees both the token-authorized op and the fulfillment — they agree.

**Revocation during a token's TTL:** A `work:role-grant` with `role = "revoked"` invalidates *future* operations from the revoked key, but operations already accepted under a still-valid token cannot be retroactively undone. This is the explicit trade-off for offline-first: revocation has token-TTL latency. For high-stakes ops, the 4h window bounds the harm. Maintainers can shorten TTLs in `.cf/config.toml` if their threat model requires it.

**Resolves A3** completely. **Resolves A1** for token-authorized operations (no fulfillment-window race). The remaining A1 surface is operations performed *without* a token (because the client never fetched one) — those still require server fulfillment and inherit the Class 3 fallback.

### 3.6 `min_operator_level` Integration

Already supported by `pkg/convention/parser.go:57` and enforced at `convention/server.go:209-216`. Ready adds `min_operator_level` to the v0.4 declarations:

| Operation | min_operator_level |
|---|---|
| `work:create` | 1 (self-asserted) |
| `work:claim`, `work:update` | 1 |
| `work:close`, `work:delegate` | 2 (challenge/response) |
| `work:gate-resolve` | 2 |
| `work:role-grant` | 2 |

Client-side enforcement is `executor.go:243-255`. The convention server re-checks on dispatch (so a malicious client cannot bypass).

---

## 4. Join and Invite Workflow

### 4.1 Anteroom Replaced with Consult Join Policy (Resolves Domain Purist V3)

**v1's "publicly writable for one operation" anteroom does not exist as a primitive.** The closest SDK-supported pattern is the `consult` join policy in `pkg/naming/join_policy.go`: a would-be member sends a join request *via the campfire join protocol*, the campfire's join policy forwards the request to a consult campfire, and a member of the consult campfire approves or denies.

**Decision:** Project campfires use `Policy = consult`. The consult campfire is the project's own maintainer-inbox campfire (which can be the same campfire or a separate one — implementation choice). The consult callback's payload — the join request — is materialized as a `work:join-request` item visible in the maintainer's `rd ready`.

**Flow:**
1. Alice runs `rd join acme.backend`. Her client resolves the name, attempts to join the project campfire.
2. The project campfire's join policy is `consult`. The campfire's join handler forwards the request to the consult callback.
3. The consult callback (running inside the convention server, or inside `rd` for solo mode) materializes a `work:join-request` work item with payload `{pubkey, requested_role, optional_attestations, optional_join_conversation_campfire}`.
4. Alice's client blocks (or polls, or returns and lets her come back later) waiting for admission.
5. The maintainer sees the item in `rd ready` and runs `rd admit <item-id>` (see §4.3).

This is **the SDK's existing pattern**. No anteroom invention. No cross-campfire monitoring. No relay convention.

### 4.2 C4 Evaluation: Optional Join-Conversation Campfire

C4 (the requester creates a campfire to hold their join-request context) is **adopted as an option**, not a requirement. The `work:join-request` payload may include `join_conversation_campfire`. If present, the maintainer can `cf read` it to see the requester's full provenance, attestations, and a back-and-forth conversation. If absent, the join request is just the basic payload.

This eliminates the DoS amplification concern (A8): the bulk of the join request lives in the requester's own campfire, which the requester pays for. The naming campfire only sees a single pointer.

### 4.3 `rd admit` (Not `rd approve`)

**Decision:** New command `rd admit`. Existing `rd approve` handles gate resolution and is not overloaded.

```
rd admit <item-id>                  # admit with default role (contributor)
rd admit <item-id> --role agent     # admit with specific role
rd admit <item-id> --deny "reason"  # deny the request
```

`rd admit` does two things atomically:
1. Posts a `work:role-grant` for the requester's pubkey with the chosen role.
2. Triggers admission via the convention server (which calls the SDK's join-grant primitive — see §4.4).

The work item closes on success.

### 4.4 JoinGrant Type (Ready-Defined, SDK-Independent)

**Pragmatist finding:** There is no `JoinGrant` type in the SDK. `protocol.Admit` writes to the filesystem transport directory and is unusable for remote members.

**Decision:** Ready defines its own `JoinGrant` type and admission flow. A grant is an Ed25519-signed structure:

```json
{
  "subject": "<requester_pubkey>",
  "campfire": "<campfire_id>",
  "role": "contributor",
  "issued_by": "<maintainer_pubkey>",
  "issued_at": 1712534400,
  "expires_at": 1712620800,
  "single_use": true,
  "nonce": "<random_hex>"
}
```

Signed by the issuer's member key. The grant is delivered to the requester out-of-band (or via the convention server's response to the join-request). The requester's `rd join` presents the grant to the campfire, which validates it via the join policy callback.

For Wave 3 ship, the grant is presented through the campfire's join handler — implemented as part of the `consult` callback wired through the convention server. The convention server is the bridge that holds the campfire-write capability.

**Open item (rd-XXX, deferred):** Propose a portable `JoinGrant` primitive to campfire SDK so the convention server is not the only path. Until then, ready's flow requires a convention server for any non-filesystem transport.

### 4.5 Revocation and TOCTOU (Resolves A5)

**Decision:**
- **Grant revocation before use** is a `work:role-grant` with `role = "revoked"` for the subject pubkey. Synchronous: the join handler reads the `member-roles` projection before accepting any grant. The Class 1 projection guarantees this is O(1) and current.
- **Grant revocation after use** removes the member's ability to receive future fulfillments / capability tokens. Existing items remain in the log. Stranded claimed items go through the **revocation reclaim flow**: a `work:role-grant role=revoked` automatically marks all in-progress items claimed `by` that pubkey as eligible for re-claim. Other members see them in `rd ready`. (Implementation: a state-derivation rule that flips claimed→ready on revocation. Estimated 30 LOC in `state.go`.)
- **Compromised maintainer key:** A `work:role-grant role=revoked` for a maintainer pubkey takes effect immediately for *future* grants signed by that key (state derivation rejects them). Past grants remain valid by default — retroactive invalidation is opt-in (`rd revoke --retroactive <key>`), which posts a `work:role-grant role=revoked` for every subject that key admitted. This is a manual nuclear option, not the default, because retroactive de-membership shreds audit trails for legitimate work the admitted members did.

---

## 5. Agent Identity and Scoping

### 5.1 A6 Resolution: Scope Enforcement at Convention Server Level

**v1's `[scope]` config was client-side only.** The convention server had no visibility, so a misconfigured agent (or one whose scope file was tampered with) had full role-level authority on any campfire it was admitted to.

**Decision:** Agent scope is published, not local. When an agent is admitted, its `work:role-grant` includes a `scope` field:

```json
{
  "subject": "<agent_pubkey>",
  "role": "agent",
  "scope": {
    "campfires": ["acme.security"],
    "operations": ["work:create", "work:update", "work:claim", "work:close"]
  }
}
```

The scope is part of the role grant — a signed campfire message. The convention server reads it from the `member-roles` projection and rejects out-of-scope operations. The local `[scope]` section in `.cf/config.toml` is now an *advisory* — it tells the agent's client not to bother sending operations the server will reject. The authoritative copy is on the campfire.

This makes scope first-class authorization state, derivable from the log, identical client-side and server-side.

### 5.2 Agent Role Cannot Gate-Resolve

Confirmed and unchanged from v1. The `agent` role's authorization matrix entry omits `work:gate-resolve`. Gates are human escalation points — an agent hitting one cannot self-resolve regardless of which human's identity it runs under. Enforced in §3.4's projection lookup + the matrix in `state.go`.

---

## 6. Federation

### 6.1 Cross-Team Dependencies (A9)

**v1 had no implementation path.** A dependency like `acme.backend.item-X` blocked-by `acme.frontend.item-Y` was a dangling string. Reading `acme.frontend` requires membership.

**Decision: defer with concrete implementation path.**

Cross-campfire dependencies are valuable but require either (a) the dependent client to be a member of both campfires (which works today for org-level humans with broad membership), or (b) a directory-service convention that publishes item state summaries for cross-campfire query (the AIETF directory-service convention provides the primitive — see `agentic-internet/docs/conventions/directory-service.md`).

**Wave 5 (deferred) item:** rd-XXX "Cross-campfire dependency resolution via directory-service convention." Until Wave 5 ships:
- Cross-campfire deps are expressible (`acme.frontend.item-Y` is a valid string).
- State derivation treats unresolvable cross-campfire deps as **non-blocking** with an explicit warning in `rd show`. Items are not silently blocked nor silently unblocked; the user sees that the dependency is cross-campfire and unresolvable from their membership.
- When the user is a member of both campfires, resolution works automatically via local replay.

### 6.2 Observer Role and E2E Encryption (A10)

**v1 had observers + E2E encryption in fundamental tension.** A late-joining observer cannot decrypt historical messages encrypted under CEKs they never received.

**Decision: project campfires are NOT E2E encrypted by default.** Privacy of work items is enforced by membership and roles, not encryption. Observer = member with read-only role; can read everything in the log from before and after their admission.

**Why:** The org-level visibility use case (§2.3 of v1) is the primary motivator for observers. E2E encryption with retroactive read access is incompatible with that use case. Forcing every campfire to choose between "private items" and "org visibility" is worse than choosing one and being clear about it.

**Future option (deferred):** Item-level visibility tags (`private:for+by`) enforced by the convention server's authorization matrix on read-projection delivery. This is a Wave 6+ feature requiring server-side filtered reads, which campfire does not currently support as a primitive.

**For projects that need E2E encryption** (regulated environments, secrets), use a separate campfire and accept that observer joinability is limited. This is a deployment choice, not a default.

### 6.3 Org/Team Namespace Management

Wave 5. Mechanism is a `work:role-grant` at the org-namespace level (e.g., a grant on the `acme` naming campfire) which propagates a convention to child campfires via federated server-binding declarations. Detailed design deferred to a Wave 5 sub-brief.

---

## 7. Revised Build Order

### Wave 1 — Fix the Breaks (P0, ~50 LOC)

**Goal:** Existing users' commands work; new users get the right defaults.

- `cmd/rd/root.go:80` `CFHome()`: check `~/.cf` first, fall back to `~/.campfire`, default new installs to `~/.cf`. (~8 LOC)
- `cmd/rd/root.go:64` flag default text: update to "(default: ~/.cf)".
- `cmd/rd/init.go:297` `localCampfireBaseDir()`: use updated `CFHome()`.
- `cmd/rd/root.go:140` `requireClient()`: replace `protocol.Init(CFHome(), ...)` with `protocol.InitWithConfig(protocol.WithWalkUp(), protocol.WithAuthorizeFunc(centerAuthorize))`. (~10 LOC)
- **Test gate:** existing-user smoke test (identity in `~/.campfire`) and new-user smoke test (no `~/.cf` exists) before merge. CFHome migration is the highest-risk change in this wave.

### Wave 1.5 — Local Name Binding (~120 LOC)

**Goal:** Project has a name, written to disk; convention server (Wave 2) can be addressed by name.

- New: `rd init <name>` syntax. Resolves the name through the configured beacon root, registers the project under it, writes both `.campfire/root` (SDK contract) and `.cf/config.toml` `[ready] project = "<name>"`.
- Fallback: if no beacon root configured, `rd init <name>` proceeds local-only with a warning. The name is written to `.cf/config.toml` for future re-registration.
- `cmd/rd/send.go:401` `projectRoot()`: read `[ready].project` from `.cf/config.toml` first, fall back to `.campfire/root` walk. Both files coexist in the transition.
- New: `rd init` (no name) still works — creates a local-only campfire, no naming.
- **Verify** `WithNamingResolver` is exported from the SDK before this wave starts. If not, file a campfire SDK PR as wave-blocker.

### Wave 2 — Authorization Layer (~400 LOC)

**Goal:** Convention server enforces authorization; bypass mode for solo workflow.

- New convention declaration: `convention:server-binding` (§3.1). v0.4 of the work-management convention.
- New convention declaration: `work:role-grant` with `min_operator_level = 2`. The bootstrap rule (campfire creator = implicit maintainer) is encoded in state derivation, not as a message.
- Add `min_operator_level` to all 12 declarations per §3.6.
- `pkg/state/state.go:245` `Derive`: add fulfillment gating (~80-120 LOC). For each consequential operation, look up the active server-binding, find a `fulfills` message from the bound pubkey or a valid capability token. If neither present and no binding exists in this campfire (bypass mode), accept the operation (Wave 1 behavior).
- New: in-process convention server (goroutine) inside `rd` for solo workflow. Same authorization matrix code path as a sidecar deployment. Signs fulfillments with the local member key. Posts a server-binding on first use that names itself as authoritative.
- New: `member-roles` named filter projection declared at convention server startup.
- New: capability token issuance and verification (§3.5).
- **Bypass mode** (mandatory): no binding present → trust all messages. Solo users keep working.
- **Backward compatibility:** items closed before any server-binding existed have no fulfillment. Treat all pre-binding messages as implicitly authorized (timestamp comparison against the first binding's `valid_from`).

### Wave 3 — Invite Workflow (~250 LOC)

**Goal:** `rd join` and `rd admit` work end-to-end.

- New convention declaration: `work:join-request` with `min_operator_level = 0` (anonymous bootstrap permitted; the request is just a request).
- Project campfires set `Policy = consult` in the join policy. The consult callback (running in the convention server from Wave 2) materializes the join request as a `work:join-request` work item in the project's `rd ready`.
- New command: `rd join <name>`. Resolves the name, attempts to join, posts a `work:join-request` if consult policy bounces it, blocks/polls for admission.
- New command: `rd admit <item-id> [--role <role>] [--deny "reason"]`. Posts a `work:role-grant` and triggers admission via the convention server.
- New command: `rd revoke <pubkey-or-name> [--retroactive]`. Posts a `work:role-grant role=revoked`.
- `JoinGrant` type defined in ready (`pkg/grant/`), Ed25519 signed.
- Stranded-item reclaim: state derivation rule on `role=revoked`.
- DoS defense: `work:join-request` rate-limited per source pubkey at the convention server (10/h default, configurable). Anonymous floods cost the attacker key generation but bound the maintainer's queue.

### Wave 4 — Naming Adoption Complete (~200 LOC)

**Goal:** All cross-project references use names; `.campfire/root` remains as SDK contract but is invisible to users.

- `cmd/rd/send.go:401` `projectRoot()`: full migration to `[ready].project` resolution. `.campfire/root` is a fallback / cache.
- Cross-project item references (`acme.frontend.item-Y`) resolved via `cf://` URI through `WithNamingResolver`.
- All status output uses names, never campfire IDs. Hex IDs only appear in `--debug` output.
- `rd join` works for non-bootstrap members (Wave 3 covered the basic flow; Wave 4 polishes edge cases like name-resolution failures, multi-root resolution, TOFU pinning of beacon root).

### Wave 5 — Federation (~deferred sizing)

- Agent scope publication via signed `work:role-grant` (§5.1).
- Super-identity (`cf home be`) surfaced in team onboarding docs.
- Org/team namespace management via federated server-binding.
- Cross-team dependency resolution via directory-service convention (§6.1).
- Convention declarations updated to v0.4 final.
- (Optional) Migrate convention server from in-process to policy-campfire model if the SDK matures the pattern.

---

## 8. Attack Disposition Table

| ID | Attack | Status | Resolution |
|---|---|---|---|
| A1 | Non-monotonic state during fulfillment window (~15min) | **PARTIALLY RESOLVED** | Role state moved to Class 1 projection (deterministic, no window). Operation fulfillment uses capability tokens (§3.5) which validate offline with no window. Residual A1 surface — operations performed without a token — falls back to Class 3 lazy-delta. The 15-minute window is documented as a known constraint for non-tokenized ops; clients SHOULD always fetch a token. |
| A2 | Convention server pubkey not discoverable | **RESOLVED** | `convention:server-binding` declaration (§3.1). Key rotation = post a new binding. |
| A3 | Offline/auth binary | **RESOLVED** | Capability tokens (§3.5) + bypass mode (§3.2). Solo users never see authorization. Team users get offline-first via tokens with TTL-bounded revocation latency, explicitly documented as the trade-off. |
| A4 | Anteroom bridging race conditions | **RESOLVED** | Anteroom dissolved into the SDK's `consult` join policy (§4.1). No bridging, no cross-campfire monitor, no race. The consult callback is synchronous from the SDK's perspective. |
| A5 | Join grant TOCTOU + compromised maintainer | **RESOLVED** | Synchronous projection-lookup of revocation state on every grant validation (§4.5). Compromised key revocation has explicit retroactive opt-in; default is forward-only with documented rationale. |
| A6 | Agent scope client-side only | **RESOLVED** | Scope is a field in the published `work:role-grant`, server-enforced (§5.1). |
| A7 | Wave 1 ships without auth, larger attack surface | **RESOLVED** | Wave 1 does NOT ship naming-based join. Wave 1.5 ships local name binding only (no public discovery). Wave 2 ships authorization. Wave 3 ships join workflow. Public name-based discoverability does not exist until Wave 3, by which time auth is in place. |
| A8 | Anteroom DoS | **RESOLVED** | Per-source-pubkey rate limit on `work:join-request` at the convention server (§Wave 3). Optional join-conversation campfire (§4.2) shifts payload cost to requester. Maintainer queue is bounded. |
| A9 | Cross-team dependencies dangling | **DEFERRED** | Wave 5 + AIETF directory-service convention. Until then, cross-campfire deps are expressible but treated as non-blocking with a visible warning. Tracked as rd-XXX. |
| A10 | E2E encryption + observer tension | **PERMANENT CONSTRAINT** | Default project campfires are NOT E2E encrypted. Observer use case wins. Privacy is membership-enforced. Documented explicitly so users in regulated environments choose a different deployment (separate encrypted campfire, no observers). |
| A11 | Provenance store in-memory, wiped on cold start | **EXTERNAL DEPENDENCY** | This is a campfire-hosting bug, not ready's. Tracked as a cross-project dependency. Ready's in-process convention server (§3.2) avoids this entirely for solo users. For hosted teams, ready's correctness depends on campfire-hosting persisting the attestation store. File a campfire-hosting issue. |

---

## 9. Open Questions (Unresolved)

These are deferred but tracked. Each becomes an rd item in the project tracker.

1. **SDK extension for `[ready]` (or generic `[project]`) section in `.cf/config.toml`.** Until upstream, both `.campfire/root` and `.cf/config.toml [ready]` coexist. Cosmetic, not blocking. (rd-XXX)
2. **Portable `JoinGrant` SDK primitive.** Today `protocol.Admit` requires filesystem write to the transport dir. Ready's flow requires a convention server as the bridge. SDK PR would let admission work without a convention server intermediary. (rd-XXX)
3. **Cross-campfire dependency resolution via directory-service.** Wave 5. Concrete design and AIETF directory-service spec must mature. (rd-XXX)
4. **Convention server as policy-campfire migration.** Wave 5+, optional, only if the campfire SDK matures the pattern. Architecturally clean but not on the critical path. (rd-XXX)
5. **Item-level visibility (private items).** Requires server-side filtered reads, which campfire does not have as a primitive. Wave 6+ if ever. (rd-XXX)
6. **`WithNamingResolver` export status.** Verify before Wave 1.5 starts. If unexported, file an SDK PR as wave-blocker. (rd-XXX)
7. **Token TTL tuning by deployment.** Defaults proposed in §3.5 are starting points. Real-world threat models may need shorter (high-stakes) or longer (low-bandwidth field operations). Configurable in `[ready]` section. (rd-XXX, post-Wave 3)
8. **Compaction.** v1 P2 gap. Log grows unbounded; state derivation gets slower. Defer to Wave 5 or later, after the projection middleware proves out for ready's actual workload. (rd-XXX)
9. **Super-identity (`cf home be`) in team onboarding.** Wave 5. (rd-XXX)
10. **Long-lived key compromise audit trail.** When `--retroactive` revocation is used, the audit trail of the compromised key's actions must be preserved alongside the revocation. UX for inspecting this is undesigned. (rd-XXX)

---

## 10. Implementation Notes (Pragmatist Wave Summary)

### Wave 1 Files

- `cmd/rd/root.go:75-81` — `CFHome()` migration. ~8 LOC. **Test gate: existing-user identity in `~/.campfire` MUST resolve.**
- `cmd/rd/root.go:64` — flag default text update.
- `cmd/rd/root.go:140-150` — `requireClient()` replaced with `protocol.InitWithConfig(WithWalkUp(), WithAuthorizeFunc(centerAuthorize))`. ~10 LOC.
- `cmd/rd/init.go:297` — `localCampfireBaseDir()` consumes updated `CFHome()`.

### Wave 1.5 Files

- `cmd/rd/init.go:111` — write `.cf/config.toml [ready] project = "<name>"` alongside `.campfire/root`.
- `cmd/rd/init.go` — new `rd init <name>` arg parsing, beacon-root resolution, fallback-with-warning path.
- `cmd/rd/send.go:401` `projectRoot()` — read `[ready].project` first, fall back to `.campfire/root`. **Load-bearing — every command depends on this.**
- New: `pkg/rdconfig/` extended (or new `pkg/projectconfig/`) for `[ready]` section read/write.
- **Verify before starting:** `WithNamingResolver` exported from `pkg/protocol`.

### Wave 2 Files

- `pkg/declarations/ops/server-binding.json` — new declaration.
- `pkg/declarations/ops/role-grant.json` — new declaration with `min_operator_level = 2`.
- `pkg/declarations/ops/*.json` — add `min_operator_level` to all 12 existing declarations.
- `pkg/state/state.go:245` `Derive()` — fulfillment gating + role projection lookup. ~80-120 LOC. **Bypass mode mandatory.**
- New: `pkg/conventionserver/` — in-process convention server. Authorization matrix, capability token issuance/verification, server-binding bootstrap. ~200 LOC.
- New: `pkg/captoken/` — Ed25519 capability token format, sign/verify. ~60 LOC.
- `cmd/rd/root.go` — `requireClient()` boots the in-process convention server when bypass is disengaged.

### Wave 3 Files

- `pkg/declarations/ops/join-request.json` — new declaration.
- `cmd/rd/admit.go` — new command (avoid `approve.go` collision). ~80 LOC.
- `cmd/rd/join.go` — new command. ~100 LOC.
- `cmd/rd/revoke.go` — new command. ~40 LOC.
- `pkg/grant/` — `JoinGrant` type, sign/verify. ~80 LOC.
- `pkg/state/state.go` — stranded-item reclaim rule on `role=revoked`. ~30 LOC.
- `pkg/conventionserver/` — consult callback materializes `work:join-request` items; rate limiter on join requests.

### Wave 4 Files

- `cmd/rd/send.go:401` `projectRoot()` — full migration; `.campfire/root` becomes cache.
- `pkg/state/state.go` — cross-campfire item reference resolution (membership-gated, warn on unresolvable).
- `cmd/rd/*.go` — replace all hex ID display with name display; hex behind `--debug`.
- `cmd/rd/join.go` — TOFU pinning, multi-root, beacon failover.

### Wave 5 Files

Sized in a Wave 5 sub-brief written when Waves 1–4 ship.

---

**End of Architecture v2.**
