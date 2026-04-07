# Design Brief: Ready — Federation, Authorization, and Naming

**Date:** 2026-04-07  
**Status:** Draft — for adversarial review  
**Scope:** Architecture of ready's next major evolution: naming adoption, authorization model, and safe federation

---

## 1. Context and Problem Statement

Ready is work management built on campfire — a decentralized coordination protocol. The core model is sound: work items are convention messages on an append-only campfire log; state is derived by replaying those messages. There is no separate database, no API server. The campfire is the backend.

The current implementation (v0.6.1) works well for a single person with agents. It breaks down when extended to teams and federation, for two reasons:

**The abstraction is wrong.** Campfire primitives leak directly into the user interface. `.campfire/root` is a 64-character hex campfire ID written to disk. `rd join` takes a campfire ID. Team setup requires sharing campfire IDs out of band. This is the equivalent of passing IP addresses instead of domain names. Nobody using Jira thinks about HTTP or TCP/IP. Ready users should think about projects, teams, items, and queues — never about campfire IDs, beacons, conventions, or walk-up discovery.

**There is no authorization model.** Any campfire member can perform any operation on any item. A member can close items they didn't create, resolve gates they weren't escalated to, and delegate items they have no standing to reassign. For a two-person trusted project this is workable. For teams, open-source contributors, and federated organizations it is a griefing surface and a correctness problem.

This brief defines the design decisions that fix both problems.

---

## 2. Use Cases

### 2.1 One Human, Many Agents

A single person — Baron — works with multiple Claude Code agents across one or more projects. Agents claim items, do work, escalate gates. Baron sees what needs his attention, resolves gates, and closes the loop.

**Requirements:**
- Baron runs `rd ready` and sees exactly what needs him — nothing more
- Agents claim and close items without Baron's involvement
- Gates surface immediately when an agent can't proceed
- Nothing is lost if an agent or machine goes offline
- Zero configuration per agent beyond identity

### 2.2 Many Humans, Many Agents

Baron plus one or more contributors. Each contributor may also run agents. Items flow across machines. Some items are personal (for Baron, or for the contributor), some are shared pool (claimed by whoever picks them up first).

**Requirements:**
- Contributor joins with a single command — no campfire ID shared, no manual config
- Each person sees their personal queue and the shared pool
- A person cannot close or delegate items they have no standing on
- Contributor's agents operate within the contributor's authorization scope
- The full item history (who did what, when) is auditable from any member machine
- A hostile or compromised contributor cannot corrupt derived state visible to others

### 2.3 Many Teams, Many Agents

Multiple teams — backend, frontend, security — each with their own work queue. Agents may be shared across teams or scoped to one. Cross-team dependencies exist. Org-level humans need visibility across all teams without joining every project campfire.

**Requirements:**
- Teams are independently managed — joining backend doesn't grant access to frontend
- Cross-team dependencies can be expressed and tracked
- Org-level humans have a view across teams without per-team join ceremony
- Agent pools are scoped to their domain — a security agent cannot close billing items
- An org admin can revoke any member from any project without touching each project individually

---

## 3. Core Design Decisions

### 3.1 Names Replace Campfire IDs Everywhere

Campfire IDs are an implementation detail. They are never shown to users. Every project, team, and org has a name. Names are the stable, human-readable identity of a campfire.

**Naming is dot-notation, root-first:**
```
acme              ← org namespace campfire
acme.backend      ← backend project campfire
acme.backend      ← same name used everywhere: in commands, config, item references
```

**Naming campfires are separate from project campfires.** The `acme` campfire stores registrations for its children (`backend`, `frontend`, etc.) — it is the nameserver for the `acme` namespace. The `acme.backend` project campfire is where `work:*` messages live. These are different campfires. They may coalesce (the same campfire can serve both purposes) but the default is separation. Ready does not own or create naming campfires — those are independent infrastructure.

**Resolution is campfire traversal.** Resolving `acme.backend` means: read the root campfire for `acme` → get a campfire ID → read that campfire for `backend` → get another campfire ID → that is the project campfire. Each step reads from a campfire. To read from a campfire you must be a member. Auto-join fires at each level for open-protocol naming campfires. The campfire is simultaneously the namespace, the nameserver, and the communication channel.

**`.campfire/root` is eliminated.** The campfire ID is cached locally in `.cf/config.toml` after first resolution. Users never touch it. Config that surfaces a campfire ID to a user is a bug.

### 3.2 Naming-Aware Commands

| Today | After |
|---|---|
| `rd init` (local campfire, no name) | `rd init` (local) or `rd init acme.backend` (registers under name) |
| `rd join <campfire-id>` | `rd join acme.backend` |
| cross-project ref: string convention | `acme.frontend.item-abc` resolved through naming |
| `.campfire/root` file with hex ID | `.cf/config.toml` with `project = "acme.backend"` |

`rd init` and `rd join` are distinct operations:
- `rd init` creates a new project campfire. Optionally registers it under a name.
- `rd join acme.backend` resolves an existing named project, traverses the naming hierarchy, and joins the project campfire.

They happen to use a name as an argument in the named case, but they are fundamentally different acts: create vs. discover.

### 3.3 Authorization Model

**Principle:** Authorization is enforced at the convention server level, not only at the client's state derivation level. The convention server's fulfillments and rejections are campfire messages — signed, timestamped, immutable, visible to all members. State derivation uses fulfillments as the canonical source of authorization truth. Clients that differ in their local authorization logic still agree on derived state because they see the same server fulfillments.

**Membership roles:**

| Role | Can do |
|---|---|
| `maintainer` | All operations on all items; can invite, revoke, grant roles |
| `contributor` | Create, claim, close own items; gate-resolve items `for` them |
| `agent` | Create, claim, close items within scope; cannot gate-resolve |
| `observer` | Read only |

**Operation authorization matrix:**

| Operation | Authorized senders |
|---|---|
| `work:create` | Any member |
| `work:claim` | Any member (unclaimed items); existing `by` person (re-claim) |
| `work:update` | Creator or current `by` (claimer) |
| `work:delegate` | Creator, current `by`, or `for` person, or maintainer |
| `work:close` | Current `by`, creator, `for` person, or maintainer |
| `work:gate` | Current `by` or creator |
| `work:gate-resolve` | The `for` person, or maintainer |

**Convention server enforcement:** Operations that fail authorization are received by the campfire (the log is append-only — the message is recorded) but the convention server posts a rejection fulfillment. State derivation ignores operations without a valid fulfillment from the convention server. An attacker can pollute the log; they cannot change the canonical derived state.

**Provenance levels for external contributors:** For projects with external contributors, `min_operator_level` on write operations requires verified identity beyond a bare keypair:
- `work:create`: Level 1 (self-asserted identity sufficient)
- `work:close`, `work:gate-resolve`: Level 2 (challenge/response verified)

This prevents anonymous bots from performing consequential operations while allowing issue filing.

### 3.4 Invite Workflow Through Ready Itself

The join workflow is a work management workflow. It is not an out-of-band ceremony.

**Join request flow:**

The naming campfire for a project (e.g., the `acme` campfire that holds the `acme.backend` registration) accepts a `work:join-request` convention message from non-members. This is the anteroom — publicly writable for this one operation, nothing else.

A join request surfaces in the project maintainer's `rd ready` as an item:
```
ready-x7f  inbox  p1  Join request: alice@contributor.dev → acme.backend
```

The maintainer reviews and runs:
```bash
rd approve ready-x7f
```

This closes the item and issues a signed join grant — an Ed25519 signature over Alice's specific pubkey, bound to this campfire, with an assigned role. Alice's client presents the grant to the campfire; the campfire validates it against the join grant convention and admits her as a member with the specified role.

**Properties of the join grant:**
- Bound to Alice's specific pubkey — not a shared secret, not reusable by another identity
- Signed by a member with invite authority (maintainer role)
- Assigned role is embedded in the grant
- Time-bounded or single-use depending on project policy
- Revocable: a revocation message invalidates the grant even if not yet used

**Invite delegation:** A maintainer can grant a contributor invite authority for a specific role level. The contributor can then admit their own agents without involving the maintainer. The delegation chain is itself a campfire message, auditable.

**Revocation flow:** Same pattern — `rd revoke alice@contributor.dev` creates an item, maintainer confirms, revocation message posts to campfire. State derivation stops accepting operations from the revoked key. Existing items remain in the log (append-only) but the key can no longer post new operations that receive valid fulfillments.

### 3.5 Agent Identity and Scoping

Agents operating on behalf of a human inherit that human's authorization scope. An agent that Baron runs carries Baron's identity (or a delegated context key). An agent the contributor runs carries the contributor's identity.

**Agent scoping via config.** The `[scope]` section in `.cf/config.toml` restricts which campfires an agent can access and which operation classes it can use. A security agent might be scoped to `acme.security` and operation class `write`. It cannot read or write `acme.backend` items. This is configured once at deployment time — the agent itself has no visibility into the scope constraint.

**Agent roles.** Agents get the `agent` role by default, which cannot gate-resolve or perform maintainer operations. This is appropriate — gates are human escalation points. An agent hitting a gate cannot self-resolve it regardless of which human's identity it runs under.

---

## 4. Campfire Primitives Being Leveraged

| Primitive | How Ready Uses It |
|---|---|
| Naming (dot-notation, root-first) | All project/team/org references; `rd join` resolution |
| Naming campfires as nameservers | Separate from project campfires; ready doesn't create them |
| `invite-only` join protocol | Default for all project campfires |
| Signed join grant | Issued by `rd approve`, consumed by `rd join` |
| Convention Tier 2 (HTTP handler) | Convention server worker: authorization oracle |
| Convention fulfillment/rejection | Canonical authorization signal in append-only log |
| `min_operator_level` | Provenance gate on consequential operations |
| Membership roles | maintainer, contributor, agent, observer |
| Provenance levels 0–3 | Anonymous → verified → present |
| Config cascade (`~/.cf/config.toml` + `.cf/config.toml`) | Project and global config; replaces `.campfire/root` |
| `behavior.auto_join` | Auto-join project campfire on first use after membership granted |
| `[scope]` | Agent operation and campfire restriction |
| Super-identity (`cf home be`) | Multi-machine identity for distributed contributors |
| `InitWithConfig()` | Primary SDK entry point for ready's protocol init |
| `WithWalkUp()` | Explicit opt-in for center campfire discovery (v0.15 change) |
| Append-only log + state derivation | Griefing-resistant: bad messages in log, correct derived state |
| JSONL local buffer | Offline-first; pending mutations sync on reconnect |

---

## 5. What Does Not Change

**The item model is correct.** `for`, `by`, status lifecycle, priority/ETA-driven ready view, gates, dependencies, history — these are right and stay.

**Offline-first is non-negotiable.** The JSONL local buffer is load-bearing. Named or unnamed, hosted or local, a user without network access must be able to create and claim items. They sync when connectivity returns.

**State derivation is the source of truth.** The campfire log is replayed to derive state. This is the right model — it gives auditability, survivability, and the ability to extend the derivation logic without migrating data.

**`rd ready` is the attention primitive.** The ETA-driven ready filter — not blocked, not terminal, ETA within 4 hours — surfaces exactly what needs attention. This is the core UX loop and it is correct.

---

## 6. Current Implementation Gaps

Ordered by severity:

| Gap | Severity | Notes |
|---|---|---|
| Home dir default `~/.campfire` (should be `~/.cf`) | P0 | v0.15 break; users with new campfire installs can't find identity |
| Walk-up not explicit (`WithWalkUp()` missing) | P0 | v0.15 break; center campfire discovery silently broken |
| No convention server | P0 | All authorization is currently unenforced |
| No naming at init or join | P1 | Campfire IDs still user-visible |
| No join request convention | P1 | Invite workflow is manual and out-of-band |
| No role model in membership | P1 | All members have identical authority |
| Gates not wired to futures | P1 | Agents can't block on gate resolution |
| Convention declarations at v0.3 | P2 | Should be v0.4; min_operator_level not declared |
| Compaction not implemented | P2 | Log grows unbounded; state derivation gets slower |
| Super-identity not surfaced in `rd init` | P2 | Multi-machine team setup has no guidance |

---

## 7. Build Order

Authorization and naming are coupled — you cannot safely open naming-based discovery without the authorization layer. The sequence:

**Wave 1 — Fix the breaks (unblock everything)**
- Home dir default: `~/.cf` with `~/.campfire` fallback
- Walk-up: add `WithWalkUp()` to `requireClient()`
- Switch to `InitWithConfig()` throughout

**Wave 2 — Authorization layer**
- Convention server: HTTP handler validating `work:close`, `work:gate-resolve`, `work:delegate` against item ownership
- Authorization matrix enforced in state derivation (client-side) AND server fulfillments (canonical)
- Role field on membership records
- `min_operator_level` on consequential declarations

**Wave 3 — Invite workflow**
- `work:join-request` convention on naming campfire anteroom
- `rd approve` issues signed join grant
- `rd join` resolves name and presents grant
- `rd revoke` posts revocation

**Wave 4 — Naming adoption**
- `rd init <name>` registers under naming hierarchy
- `rd join <name>` resolves and joins
- `.cf/config.toml` replaces `.campfire/root`
- All cross-project references use dot-notation names

**Wave 5 — Federation complete**
- Agent scoping via `[scope]` in config
- Super-identity surfaced in team onboarding
- Org/team namespace management
- Convention declarations updated to v0.4

---

## 8. Open Questions for Adversarial Review

1. **Convention server hosting.** Who runs the convention server? For solo projects it could run locally alongside `rd`. For teams it needs to be reachable from all member machines — which implies hosted infrastructure. Is there a path where the convention server is optional (client-side-only enforcement) and upgrades to server-side as a project scales? What are the correctness implications of the hybrid period?

2. **Naming campfire bootstrap.** When Baron runs `rd init acme.backend`, how does he register `acme.backend`? He needs write access to the `acme` naming campfire. How is the `acme` campfire created? Who runs it? What happens if no naming root is configured — should ready fall back to local-only with no registration, or require a naming root before allowing named init?

3. **Join request anteroom security.** The anteroom (naming campfire accepting `work:join-request` from non-members) is a public write surface for that one operation. What prevents DoS — flooding the maintainer's queue with fake join requests? Rate limiting per sender key? CAPTCHA-style proof of work? Does the provenance level of the requester affect how the request is surfaced (Level 2 requests get higher priority)?

4. **Authorization during the offline period.** If the convention server is unreachable, can items still be closed? If yes — the authorization guarantee breaks. If no — the system grinds to a halt on network partition. What is the right policy? Local-only optimistic close with server reconciliation on reconnect? How does state derivation handle conflicting fulfillments after a partition?

5. **Revocation propagation.** When a contributor's access is revoked, their agents may be mid-execution with claimed items. What happens to those items? Do they revert to unclaimed? Do they stay claimed but the agent can no longer post operations that receive fulfillments? Who sees the stranded items and how are they recovered?

6. **Cross-team dependency authorization.** When `acme.backend` creates an item that depends on `acme.frontend.item-xyz`, the backend team is asserting a dependency on a campfire they may not be members of. How does the dependency convention work across campfire boundaries? Does the convention server for `acme.frontend` need to validate the dependency claim?

7. **Observer role and privacy.** An observer can read all items in a campfire. Is this the right default for org-level humans who need cross-team visibility? Or should there be item-level visibility controls — private items visible only to `for` and `by` persons? What does campfire's encryption model give us here (the campfire can be E2E encrypted with CEK delivery only to non-observer members)?

8. **State derivation performance at scale.** State derivation replays every message in the log. For a project with years of history this becomes expensive. Compaction is the fix but requires the convention server to post snapshot messages that state derivation can use as a starting point. What is the compaction protocol — what does a snapshot message contain, who is authorized to post one, and how does a new member bootstrap from a snapshot?
