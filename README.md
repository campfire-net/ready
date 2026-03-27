# rd — work management as a campfire convention

`rd` surfaces what needs attention. For humans, for agents, for anyone in the delegation chain.

No separate backend. Your [campfire](https://getcampfire.dev) is the database, the audit trail, and the coordination layer.

## Install

```bash
curl -fsSL https://ready.getcampfire.dev/install.sh | sh
```

Or via Homebrew:

```bash
brew install campfire-net/tap/ready
```

Or build from source:

```bash
go install github.com/campfire-net/ready/cmd/rd@latest
```

Self-contained binary — the [campfire](https://getcampfire.dev) protocol is built in. For agent access via MCP: `npx @campfire-net/campfire-mcp`.

## Quick start

```bash
# Create a work campfire for your project
rd init --name myproject

# Create work items
rd create --title "User auth returns 403 on valid tokens" --type task --priority p0

# What needs attention?
rd ready

# Claim and work
rd claim myproject-a1b
rd close myproject-a1b --reason "Token validation was checking issuer, not audience"
```

## How it works

Ready is a **convention**, not an application. It defines structured operations (`work:create`, `work:claim`, `work:close`, etc.) as campfire messages. The `rd` CLI is a thin wrapper around `cf` that speaks this convention.

**WHO is first-class.** Every item has explicit `for` (who needs this outcome) and `by` (who's doing the work) fields. Delegation is an explicit act.

**Attention engine.** Named views filter by who you are. `rd ready` shows what's actionable for you. An agent's view shows what's assigned to it.

**Convention-declared.** When you `rd init`, the campfire publishes what operations it supports. Agents discover capabilities from the campfire itself.

## Commands

| Command | What it does |
|---------|-------------|
| `rd init --name <project>` | Create a work campfire with convention declarations |
| `rd create --title "..." --type task` | Create a work item |
| `rd ready` | What needs attention now |
| `rd list` | All open items |
| `rd list --all` | All items including done/cancelled |
| `rd claim <id>` | Claim an item (set yourself as `by`) |
| `rd close <id> --reason "..."` | Close with reason |
| `rd update <id> --status <status>` | Change status |
| `rd show <id>` | Item details |
| `rd delegate <id> --to <identity>` | Assign to someone else |

## Item fields

- **`for`** — who needs this outcome
- **`by`** — who's doing the work
- **`type`** — task, decision, review, reminder, deadline, prep, message, directive
- **`priority`** — p0, p1, p2, p3
- **`status`** — inbox, active, scheduled, waiting, blocked, done, cancelled, failed
- **`eta`** — when this status should change (the attention engine)
- **`due`** — hard external deadline

## Named views

| View | Shows |
|------|-------|
| `ready` | Items needing attention now (not done, not blocked, eta < 4h) |
| `work` | Actively being worked on |
| `pending` | Waiting, scheduled, or blocked |
| `overdue` | Past-due items |
| `delegated` | Work I delegated, in progress |
| `my-work` | Work assigned to me |

## Part of the campfire ecosystem

Ready is built on the [campfire protocol](https://getcampfire.dev). It works with any campfire — local or hosted.

- [campfire](https://github.com/campfire-net/campfire) — the protocol
- [agentic internet](https://aietf.getcampfire.dev) — conventions for agent coordination

## License

MIT
