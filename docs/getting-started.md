# Getting Started with Ready

Ready is work management as a campfire convention. Items, dependencies, gates, and views are all convention-conforming messages on a campfire. No server backend — the campfire *is* the backend.

## Table of Contents

- [Concepts](#concepts)
- [Prerequisites](#prerequisites)
- [Which topology?](#which-topology)
- [Topology 1: Single Project (Quick Start)](#topology-1-single-project-quick-start)
- [Topology 2: Multiple Projects](#topology-2-multiple-projects)
- [Topology 3: Git-Backed Project (Team Collaboration)](#topology-3-git-backed-project-team-collaboration)
- [Topology 4: Hosted Persistent Campfire](#topology-4-hosted-persistent-campfire)

---

## Concepts

**Center** — your identity anchor. Created once per operator via `cf init`. Every campfire you create or join is rooted in your center. Centers can live on the filesystem (local dev) or on a hosted instance (cloud).

**Campfire** — an append-only message log with named views, compaction, and convention enforcement. Each `rd init` creates one campfire per project. The campfire is the work item store.

**Convention** — a typed schema registered on a campfire. The work management convention defines `work:create`, `work:close`, `work:status`, and so on. The campfire runtime validates every message against the convention before accepting it. `rd` is a thin wrapper that calls these convention operations.

**Item** — a convention-conforming message on the campfire. Fields: `id`, `title`, `context`, `type`, `priority`, `status`, `for`, `by`, `eta`, `due`, `parent_id`, `blocks`. All state transitions are messages — the message log is the audit trail.

**Views** — named filter predicates registered on the campfire. `rd ready` runs the `ready` view: items that are not done, not blocked, have no parent, and need attention within 4 hours. `rd list` runs `my-work`. Views are evaluated server-side.

**Context key** — a file (`cf.key` or `.campfire/key`) that `cf` walks up from the current directory to find. This is how project campfires are discovered automatically when you run `rd` in a project directory.

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

If `~/.local/bin` is not in your PATH, the installers will print the exact lines to add to your shell profile.

---

## Which topology?

| Situation | Use |
|-----------|-----|
| One person, one project, trying it out | [Topology 1: Single Project](#topology-1-single-project-quick-start) |
| One person, several projects | [Topology 2: Multiple Projects](#topology-2-multiple-projects) |
| A team sharing work state via git | [Topology 3: Git-Backed Project](#topology-3-git-backed-project-team-collaboration) |
| A team that wants always-on, no git sync | [Topology 4: Hosted Persistent Campfire](#topology-4-hosted-persistent-campfire) |

---

## Topology 1: Single Project (Quick Start)

One person, one project. The simplest path.

### Setup

```bash
# Create your identity (once per machine)
cf init

# Initialize a work campfire in your project
cd ~/projects/myproject
rd init --name myproject
```

Expected output from `rd init`:

```
Created campfire: myproject-a1b2c3
Registered work convention v0.3
Named views: ready, work, pending, overdue, my-work, delegated
Context key written to .campfire/root
```

### Daily workflow

```bash
# Create an item
rd create "Design the API" --priority p1 --type task

# Output:
# Created myproject-001
# Priority: P1  ETA: +4h  Status: inbox

# See what needs attention now
rd ready

# Output:
# myproject-001  Design the API  P1  inbox

# Claim and start working it
rd update myproject-001 --status active

# Show item detail and audit trail
rd show myproject-001

# Close it when done
rd done myproject-001 --reason "API design finalized in docs/api.md"
```

### Dependencies

```bash
# Create two items
rd create "Write tests" --priority p1 --type task
# → myproject-002

rd create "Deploy to staging" --priority p1 --type task
# → myproject-003

# Wire the dependency: deploy blocks on tests
rd dep add myproject-003 myproject-002

# View the dependency graph
rd dep tree myproject-003

# Output:
# myproject-003  Deploy to staging  [blocked]
# └── myproject-002  Write tests  [active]

# rd ready will not surface myproject-003 until myproject-002 is closed
```

### Useful commands

```bash
rd list                            # all items in this project
rd list --status active            # filter by status
rd show <id>                       # detail + audit trail
rd update <id> --priority p0       # change priority
rd update <id> --note "blocked on review from alice"  # add context
rd done <id> --reason "..."        # close with reason (required)
rd cancel <id> --reason "..."      # cancel with reason
```

---

## Topology 2: Multiple Projects

One identity, many projects. Each project gets its own campfire. `rd` auto-detects which campfire to talk to based on the current directory.

### Setup

```bash
# Create your identity once
cf init

# Initialize each project separately
cd ~/projects/backend
rd init --name backend

cd ~/projects/frontend
rd init --name frontend

cd ~/projects/infra
rd init --name infra
```

### Project-scoped vs. cross-project queries

```bash
# In ~/projects/backend — shows only backend items
cd ~/projects/backend
rd ready

# In ~/projects/frontend — shows only frontend items
cd ~/projects/frontend
rd ready

# Cross-project: list items in another project by name
rd list --project backend

# Cross-project: ready items across all your projects
rd ready --all
```

### How project detection works

When you run `rd` in a directory, it walks up looking for `.campfire/root` (written by `rd init`). The first one it finds is the active project. This means:

```bash
cd ~/projects/backend/src/api
rd ready        # still sees backend project — walked up and found .campfire/root
```

If no `.campfire/root` is found in any parent, `rd` falls back to your center's default project (if configured) or returns an error.

### Cross-project dependencies

```bash
# Wire a dep across projects using full item IDs
rd dep add frontend-042 backend-017

# View it
rd dep tree frontend-042
# frontend-042  Ship login UI  [blocked]
# └── backend-017  Auth endpoint  [active]  (backend)
```

---

## Topology 3: Git-Backed Project (Team Collaboration)

For teams using git. The campfire context key is committed to the repo. Teammates clone the repo, join the campfire, and share the same work state. No separate infrastructure needed.

### Lead developer: initialize

```bash
# Create identity (once per machine)
cf init

cd ~/projects/myrepo

# Initialize the work campfire
rd init --name myrepo

# Output includes:
# Context key written to .campfire/root

# Exclude local mutation state from git (not the context key)
echo ".ready/" >> .gitignore

# Commit the campfire context key — this is how teammates find the campfire
git add .campfire/ .gitignore
git commit -m "chore: add work campfire"
git push
```

The `.campfire/root` file contains the campfire ID. Anyone who clones the repo can resolve the campfire from it.

### Teammate: join

```bash
# Create identity if you don't have one
cf init

# Clone the repo as usual
git clone git@github.com:org/myrepo.git
cd myrepo

# Join the campfire (reads .campfire/root automatically)
rd join

# Output:
# Joined campfire: myrepo-a1b2c3
# Syncing...  47 messages pulled
# Named views: ready, work, pending, overdue, my-work, delegated

# Now you're in
rd ready
```

### Assigning work to teammates

```bash
# Create and assign an item to a teammate
rd create "Implement rate limiting" --priority p1 --type task --by alice@example.com

# Alice runs rd ready and sees items assigned to her
# Baron runs rd delegated and sees items he delegated that are in progress

# Alice claims it
rd update myrepo-007 --status active

# Alice closes it
rd done myrepo-007 --reason "Rate limiting implemented, PR #42 merged"
```

### Sync behavior

The campfire protocol handles sync. Messages are append-only and replicated. When Alice posts a `work:close` message, anyone running `rd ready` next will see the updated state — no explicit pull needed for reads.

For filesystem-based campfires, `rd sync` forces an immediate pull if you want to guarantee you're current:

```bash
rd sync          # pull latest from campfire
rd ready         # now reflects latest state
```

---

## Topology 4: Hosted Persistent Campfire

For teams that want always-on work management with no filesystem state to manage. Uses `mcp.getcampfire.dev` as the campfire host. Items persist in the cloud. Agents and humans connect via campfire ID.

### Setup

```bash
# Create identity anchored to the hosted instance
cf init --remote https://mcp.getcampfire.dev

# Output:
# Center created: https://mcp.getcampfire.dev/c/your-center-id
# Identity: your-key@mcp.getcampfire.dev

# Initialize a project campfire (also lives on the hosted instance)
cd ~/projects/myproject
rd init --name myproject --remote https://mcp.getcampfire.dev

# Output:
# Created campfire: https://mcp.getcampfire.dev/c/myproject-a1b2c3
# Registered work convention v0.3
# Named views: ready, work, pending, overdue, my-work, delegated
```

### Daily workflow

Same commands as Topology 1 — the campfire URL is transparent:

```bash
rd create "Ship the feature" --priority p0 --type task
rd ready
rd update <id> --status active
rd done <id> --reason "Feature shipped"
```

### Sharing with teammates and agents

The campfire ID is the sharing primitive. Give it to anyone who needs access:

```bash
# Get your campfire ID
rd info
# Campfire: https://mcp.getcampfire.dev/c/myproject-a1b2c3

# Teammate joins using the ID
cf join https://mcp.getcampfire.dev/c/myproject-a1b2c3
rd ready   # sees the same items, same views

# An agent (Claude Code, rudi automaton, etc.) joins the same way
# The campfire is the coordination bus — no polling, no webhooks
```

### Gate operations (human escalation)

Hosted campfires make gate operations practical across timezones:

```bash
# Agent flags a decision needed
rd gate <id> --type design --note "Two viable approaches, need direction before proceeding"

# Human sees it in their ready view (gate items surface immediately)
rd ready
# myproject-019  Design the caching layer  P0  [GATE: design]

# Human resolves it
rd gate-resolve myproject-019 --resolution approved --note "Go with approach A"

# Agent's await unblocks, work continues
```

### Local state

With hosted campfires, `.ready/` holds only local cache and credentials — no campfire state. Safe to delete if you need to reset:

```bash
echo ".ready/" >> .gitignore   # standard practice — don't commit local cache
```

---

## Reference

### Status values

| Status | Meaning |
|--------|---------|
| `inbox` | Created, not yet claimed |
| `active` | Being worked now |
| `scheduled` | Planned for later |
| `waiting` | Blocked on an external party |
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

### Further reading

- Convention spec: `docs/convention/work-management.md` — full operation declarations, field validation, compaction policy
- Named view predicates: `pkg/views/` — S-expression predicates for each built-in view
- Campfire protocol: https://getcampfire.dev/docs
