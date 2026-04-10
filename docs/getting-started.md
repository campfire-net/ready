# Getting Started with Ready

Ready is work management as a campfire convention. Items, dependencies, gates, and views are all convention-conforming messages on a campfire. No server backend — the campfire *is* the backend.

## Table of Contents

- [Concepts](#concepts)
- [Prerequisites](#prerequisites)
- [Part 1: Solo (5 minutes)](#part-1-solo-5-minutes)
- [Part 2: Team (invite tokens)](#part-2-team-invite-tokens)
- [Part 3: Multi-agent (walk-up config)](#part-3-multi-agent-walk-up-config)
- [Part 4: Dependencies](#part-4-dependencies)
- [Part 5: Playbooks (reusable work trees)](#part-5-playbooks-reusable-work-trees)
- [Part 6: Gate escalation](#part-6-gate-escalation)
- [Part 7: Resuming work (for agents)](#part-7-resuming-work-for-agents)
- [Part 8: Reference](#part-8-reference)

---

## Concepts

**Campfire** — an append-only message log with named views, compaction, and convention enforcement. `rd init` creates one campfire per project. The campfire is the work item store.

**Item** — a convention-conforming message. Fields: `id`, `title`, `type`, `priority`, `status`, `for`, `by`, `eta`, `due`. All state transitions are messages — the log is the audit trail.

**Views** — named filter predicates evaluated server-side. `rd ready` runs the `ready` view: items that are not done, not blocked, and need attention within 4 hours. `rd list` runs `my-work`. Auto-sync means every read pulls the latest state — no manual sync required.

---

## Prerequisites

Install `cf` (campfire CLI):

```bash
curl -fsSL https://getcampfire.dev/install.sh | sh
```

Install `rd`:

```bash
curl -fsSL https://ready.getcampfire.dev/install.sh | sh
```

Verify both are on your PATH:

```bash
cf version
rd version
```

---

## Part 1: Solo (5 minutes)

One person, one project.

### Initialize

```bash
# Create your identity (once per machine)
cf init --cf-home ~/.cf

# Initialize a work campfire in your project
cd ~/projects/myproject
rd init --name myproject
```

Output:

```
initialized myproject
  campfire: 7b0929f77f95...
  declarations: 16 operations published
```

### Daily workflow

```bash
# Create an item — capture the ID for scripting
ITEM=$(rd create "Ship login page" --priority p1 --type task)

# See what needs attention now
rd ready
# rdtestsoloproja-e1f

# Claim it — transitions to active
rd claim $ITEM

# Post progress as work proceeds
rd progress $ITEM --notes "Wired up auth middleware"

# Close when done (reason is required)
rd done $ITEM --reason "Login page ships with JWT auth"
```

When stdout is piped, `rd create` emits only the bare item ID — no decoration. This makes shell assignment reliable without parsing.

### Show item detail

```bash
rd show <id>
```

Output (from `test/demo/output/06-gate-escalation.txt`):

```
ID:       rdtestgateprojyar-dd6
Title:    Migrate auth layer to new token format
Status:   active
Type:     task
Priority: p1
ETA:      2026-04-09T06:19:08Z (3h)

History:
  [2026-04-09T02:19:08Z] inbox → inbox — created
  [2026-04-09T02:19:08Z] inbox → active
```

### Other item operations

```bash
rd list                              # all items in this project
rd list --all                        # include done/cancelled
rd list --status active              # filter by status
rd update <id> --priority p0         # change priority
rd update <id> --note "blocked on review from alice"
rd cancel <id> --reason "..."        # cancel with reason
```

---

## Part 2: Team (invite tokens)

Teammates join via single-use invite tokens. No pubkey exchange. Tokens expire server-side (default 2 hours).

### Owner: generate an invite token

```bash
cd ~/projects/myproject
rd invite
# rdx1_...  (invite token — treat as secret, share out of band)
```

For agents, include a role flag:

```bash
rd invite --role agent
# rdx1_...  (agent invite token)
```

### Teammate: join

```bash
# One-time identity bootstrap (cf init creates .cf/identity.json)
cf init --cf-home ~/projects/myproject/.cf

# Join the project campfire using the token
rd join rdx1_...
# joined 00d5716f0154... via invite token (expires in 1h59m0s)

# Items are auto-synced — no rd sync needed
rd ready
```

### Delegate work to a teammate

```bash
# Owner creates and delegates an item
rd create "Build API" --type task --priority p1
rd delegate <item-id> --to <member-identity>
# delegated <item-id> to <member-identity>

# Teammate claims it
rd update <item-id> --status active

# Teammate closes it
rd done <item-id> --reason "API complete"
```

Real transcript excerpt (`test/demo/output/02-team.txt`):

```
$ rd invite
rdx1_...  (invite token — treat as secret)

$ rd join <invite-token>
joined 00d5716f0154... via invite token (expires in 1h59m0s)

$ rd ready
rdtestteamproj-776

$ rd done rdtestteamproj-776 --reason 'API complete'
closed rdtestteamproj-776 (done)
```

---

## Part 3: Multi-agent (walk-up config)

Multiple agents on the same project each get their own identity. `rd` walks up from the current directory to find `.cf/identity.json` — no env vars needed at runtime.

### Filesystem layout

```
myproject/
  .campfire/root              ← project campfire pointer (committed to git)
  .cf/identity.json           ← owner identity
  worktree-a/
    .cf/identity.json         ← agent A identity
  worktree-b/
    .cf/identity.json         ← agent B identity
```

### Setup

```bash
# Owner initializes the project
cd ~/projects/myproject
cf init --cf-home .cf
rd init --name myproject

# Add the campfire pointer to git (how teammates find the campfire)
echo ".cf/" >> .gitignore          # don't commit identity keys
git add .campfire/ .gitignore
git commit -m "chore: add work campfire"

# Create worktrees for agents
git worktree add worktree-a
git worktree add worktree-b

# Bootstrap each agent identity and join
cf init --cf-home worktree-a/.cf
cd ~/projects/myproject && CF_HOME=worktree-a/.cf rd join rdx1_<token-for-agent-a>

cf init --cf-home worktree-b/.cf
cd ~/projects/myproject && CF_HOME=worktree-b/.cf rd join rdx1_<token-for-agent-b>
```

### Each agent works independently

```bash
# Agent A — walk-up finds worktree-a/.cf/identity.json automatically
cd ~/projects/myproject/worktree-a
rd ready                            # sees items assigned to agent A

# Agent B — walk-up finds worktree-b/.cf/identity.json
cd ~/projects/myproject/worktree-b
rd ready                            # sees items assigned to agent B
```

Walk-up resolution order: current directory → parent directories → `~/.cf/identity.json`. The first `.cf/identity.json` found wins. No `CF_HOME` needed after initial bootstrap.

---

## Part 4: Dependencies

### Within a project

```bash
cd ~/projects/myproject

rd create "Build backend API" --priority p1 --type task
# → myproject-001

rd create "Wire frontend to API" --priority p1 --type task
# → myproject-002

# frontend work blocks on backend
rd dep add myproject-002 myproject-001

# View the dep graph
rd dep tree myproject-002
# myproject-002  [inbox]  Wire frontend to API

# rd ready hides myproject-002 until myproject-001 is closed
rd ready
# myproject-001  p1  inbox  3h  Build backend API

rd done myproject-001 --reason "API endpoint deployed"
# closed myproject-001 (done)

rd ready
# myproject-002  p1  inbox  3h  Wire frontend to API
# ↑ unblocked
```

### Cross-project

Cross-project deps work when both projects use identities from the same `cf init`. The dep add resolves the blocker across campfires automatically.

Real transcript excerpt (`test/demo/output/03-multiproject.txt`):

```
$ cd FRONTEND && rd dep add rdtestfrontendtisl-a91 rdtestbackendn8e2-322
blocked: rdtestfrontendtisl-a91 is now blocked by db468d83b830....rdtestbackendn8e2-322 [cross]

$ cd FRONTEND && rd ready
# (frontend item blocked — not shown)

$ cd BACKEND && rd done rdtestbackendn8e2-322 --reason "API endpoint /api/v1/users deployed"
closed rdtestbackendn8e2-322 (done)

$ cd FRONTEND && rd ready
rdtestfrontendtisl-a91
# frontend item is now unblocked
```

---

## Part 5: Playbooks (reusable work trees)

A **playbook** is a template — a reusable pattern of work items with dependencies and variable substitution. `rd engage` stamps a playbook into concrete items, wires the deps, and records the engagement as an audit entry.

Reach for a playbook whenever you find yourself typing the same decomposition twice: incident runbook, feature rollout, release prep, migration, onboarding flow.

### Register a playbook

Create a JSON file describing the item tree:

```json
[
  {
    "title": "Triage {{env}} incident",
    "type": "task",
    "priority": "p0",
    "context": "Identify blast radius in {{env}}. Page on-call if >10% users affected.",
    "deps": []
  },
  {
    "title": "Root cause for {{env}} incident",
    "type": "task",
    "priority": "p0",
    "context": "Find the commit or config change. Link it in progress notes.",
    "deps": [0]
  },
  {
    "title": "Remediate {{env}}",
    "type": "task",
    "priority": "p0",
    "context": "Roll back or forward-fix. Verify metrics recover.",
    "deps": [1]
  },
  {
    "title": "Post-incident review for {{env}}",
    "type": "review",
    "priority": "p1",
    "context": "Write up timeline, contributing factors, action items.",
    "deps": [2]
  }
]
```

Per-item fields: `title`, `type`, `priority` (required); `level`, `context`, `deps` (optional). `deps` are 0-based indices into the items array. `{{variable}}` placeholders can appear in `title` and `context` and are substituted at engage time.

Register it:

```bash
rd playbook create "SRE Incident Response" \
  --id sre-incident \
  --description "Standard incident runbook" \
  --items-file sre-incident.json
# playbook sre-incident registered (4 items, msg: ...)
```

### List and inspect

```bash
rd playbook list
#   sre-incident   4 items   Standard incident runbook

rd playbook show sre-incident
# ID:          sre-incident
# Title:       SRE Incident Response
# Description: Standard incident runbook
# Items:       4
#
# Item tree:
#   [0] p0  task    Triage {{env}} incident
#   [1] p0  task    Root cause for {{env}} incident   (after: [0])
#   [2] p0  task    Remediate {{env}}                 (after: [1])
#   [3] p1  review  Post-incident review for {{env}}  (after: [2])
```

### Engage — instantiate into work items

```bash
rd engage sre-incident \
  --project myapp \
  --for oncall@myteam.dev \
  --var env=prod
# engaged playbook sre-incident → 4 items
#
#   myapp-a2f   p0  Triage prod incident
#   myapp-4x1   p0  Root cause for prod incident      (blocked by: myapp-a2f)
#   myapp-7b3   p0  Remediate prod                    (blocked by: myapp-4x1)
#   myapp-9c0   p1  Post-incident review for prod     (blocked by: myapp-7b3)
```

What engage does:

1. Finds the playbook by ID.
2. Generates item IDs (`<project>-<random-3-chars>` per template item).
3. Substitutes `{{variable}}` placeholders (unknown vars are left as-is).
4. Sends `work:create` for each item.
5. Sends `work:block` for each dependency edge.
6. Records a `work:engage` message linking every created ID — audit trail from engagement back to items.

### When agents should reach for playbooks

**Before decomposing work by hand**, run `rd playbook list`. If a registered playbook fits the shape of the task, `rd engage` it and edit the resulting items as needed. Faster than creating from scratch, and it preserves accumulated team knowledge about which steps matter.

**After producing a clean item tree for non-trivial work**, consider registering it as a playbook so the next engagement reuses the decomposition instead of re-deriving it.

Playbooks and the `work:engage` message are fully specified in `docs/convention/work-management.md` §4.12–4.13.

---

## Part 6: Gate escalation

Agents use `rd gate` when they hit a decision point that requires human judgment. The item transitions to `waiting`. The human runs `rd gates`, then `rd approve` or `rd reject`. Approval transitions the item back to `active`.

### Agent gates an item

```bash
rd gate <item-id> \
  --gate-type design \
  --description "Two approaches: option A saves 2ms but breaks caching, option B is safe. Need direction."
```

Gate types: `design`, `scope`, `risk`, `legal`, `other`.

### Human reviews and approves

```bash
# See all pending gates
rd gates

# Output:
#   rdtestgateprojyar-dd6  p1  Two viable approaches...  Migrate auth layer

# Approve — item returns to active
rd approve <item-id> --reason "Use option B. Safety over 2ms gain."

# Or reject — item stays in waiting for further discussion
rd reject <item-id> --reason "Split into smaller items first."
```

Real transcript excerpt (`test/demo/output/06-gate-escalation.txt`):

```
$ rd gate rdtestgateprojyar-dd6 --gate-type design --description '...'
{"gate_type":"design","id":"rdtestgateprojyar-dd6","msg_id":"396874de-..."}

$ rd gates
  rdtestgateprojyar-dd6  p1  Two viable approaches...  Migrate auth layer to new token format

$ rd approve rdtestgateprojyar-dd6 --reason 'Use option B. Safety over 2ms gain.'
{"id":"rdtestgateprojyar-dd6","resolution":"approved"}

$ rd show rdtestgateprojyar-dd6
Status:   active
```

After approval, the agent checks `rd gates` — no pending entries — and continues work.

---

## Part 7: Resuming work (for agents)

Agents resuming after a context reset (compaction, restart) follow this pattern:

```bash
# What's actionable right now?
rd ready

# What am I currently working?
rd ready --view work

# Load the spec for the active item
rd show <id>

# Continue from where the spec says
```

The `work` view surfaces items in `active` status. `rd show` includes the full history and any progress notes posted during previous sessions.

### Programmatic agent loop

Agents can query JSON directly — no parsing wrapper needed:

```bash
# Get assigned work as JSON
rd ready --view my-work --json

# Claim the first item
ITEM_ID=$(rd ready --view my-work --json | python3 -c "import sys,json; print(json.load(sys.stdin)[0]['id'])")
rd claim $ITEM_ID --reason "Starting batch job"

# Post incremental progress
rd progress $ITEM_ID --notes "Processed 47/142 records"
rd progress $ITEM_ID --notes "Processed 142/142 records — complete"

# Close
rd done $ITEM_ID --reason "Batch complete: 142 records processed, 0 errors"
```

Real transcript excerpt (`test/demo/output/05-agent-workflow.txt`):

```
$ rd ready --view my-work --json
[{"id":"rdtestagentprojs-f79","title":"Reindex search corpus","status":"inbox",...}]

$ rd claim rdtestagentprojs-f79 --reason "Starting batch reindex job"
claimed rdtestagentprojs-f79

$ rd progress rdtestagentprojs-f79 --notes "Processed 47/142 records, 0 errors"
progress noted on rdtestagentprojs-f79

$ rd done rdtestagentprojs-f79 --reason "Batch complete: 142 records processed, 0 errors"
closed rdtestagentprojs-f79 (done)
```

---

## Part 8: Reference

### Status values

| Status | Meaning |
|--------|---------|
| `inbox` | Created, not yet claimed |
| `active` | Being worked now |
| `scheduled` | Planned for later |
| `waiting` | Blocked on a gate or external party |
| `blocked` | Blocked on another item (dep) |
| `done` | Completed |
| `cancelled` | Abandoned with reason |
| `failed` | Attempted and did not succeed |

### Priority and ETA

Priority drives the default ETA offset from creation time:

| Priority | Default ETA offset |
|----------|--------------------|
| P0 | +1 hour |
| P1 | +4 hours |
| P2 | +24 hours |
| P3 | +72 hours |

The `ready` view surfaces items where `eta < now + 4h`. Override with `--eta`:

```bash
rd create "Quarterly review" --priority p2 --eta "2026-04-15T09:00"
```

### Item types

`task`, `decision`, `review`, `reminder`, `deadline`, `prep`, `message`, `directive`

### Views

| View | `rd` command | Shows |
|------|-------------|-------|
| `ready` | `rd ready` | Unblocked, not done, ETA within 4h |
| `work` | `rd ready --view work` | Items you have active |
| `my-work` | `rd ready --view my-work` | Items assigned to you |
| `delegated` | `rd ready --view delegated` | Items you delegated, still open |
| `pending` | `rd list --view pending` | Scheduled for later |
| `overdue` | `rd list --view overdue` | Past ETA, not done |

### Common flags

| Flag | Works with | Effect |
|------|-----------|--------|
| `--json` | `rd ready`, `rd list`, `rd gates`, `rd show`, `rd gate`, `rd approve` | Machine-readable output |
| `--all` | `rd list` | Include done and cancelled items |
| `--view <name>` | `rd ready`, `rd list` | Use a named view predicate |
| `--role agent` | `rd invite` | Generate an agent-scoped token |

### Further reading

- Convention spec: `docs/convention/work-management.md` — full operation declarations, field validation, compaction policy
- Named view predicates: `pkg/views/` — S-expression predicates for each built-in view
- Campfire protocol: https://getcampfire.dev/docs
