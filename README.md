# rd — work management as a campfire convention

`rd` surfaces what needs attention. No separate backend — your [campfire](https://getcampfire.dev) is the database, audit trail, and coordination layer. Primary users are AI agents; humans use it too.

## Install

```bash
curl -fsSL https://ready.getcampfire.dev/install.sh | sh
# or: brew install campfire-net/tap/ready
# or: go install github.com/campfire-net/ready/cmd/rd@latest
```

Self-contained binary. For MCP access: `npx @campfire-net/campfire-mcp`.

---

## For agents

### Drop into CLAUDE.md

```markdown
## Work Management
rd is available on PATH. It auto-detects the project from your working directory.
Run `rd ready` at session start. Claim before working. Close with a reason.
After context loss: `rd ready --view work` to see what you were doing, `rd show <id>` to reload context.
```

### Install the skill

```bash
curl -fsSL https://ready.getcampfire.dev/claude-skill.sh | sh
```

### The work loop

```bash
rd ready                                     # what's actionable right now?
rd update <id> --status in_progress          # claim it
# ... do the work ...
rd done <id> --reason "fixed: was checking issuer not audience"
rd ready                                     # what unlocked?
```

### Pipe-friendly patterns

When stdout is a pipe, `rd create`, `rd ready`, and `rd list` print bare IDs — no decoration.

```bash
# Capture a new item ID
ITEM=$(rd create "Auth returns 403 on valid tokens" --type task --priority p0)

# Work every ready item in a loop
for id in $(rd ready); do
  rd update "$id" --status in_progress
  # ... work it ...
  rd done "$id" --reason "done"
done

# JSON for richer queries
rd list --json | python3 -c "import sys,json; [print(i['id']) for i in json.load(sys.stdin) if i['priority']=='p0']"
```

### After context loss

```bash
rd ready --view work    # items you had in_progress
rd show <id>            # full description + audit trail
```

---

## For teams

### Invite / join — no key exchange

```bash
# One person creates the campfire and issues a token:
rd invite                     # prints a single-use token with TTL

# Teammate joins with one command:
rd join <token>               # configures identity, joins campfire, ready to go
```

Tokens are single-use and server-enforced. Failed joins roll back identity automatically.

### Walk-up identity

`.cf/identity.json` is found by walking up from your working directory. No env vars required.

Put a project-level identity in your repo root. Agents running in worktrees automatically get isolated identities — the filesystem is your config.

```
myproject/
  .cf/identity.json        ← project identity (shared via git or per-machine)
  worktrees/
    feature-x/
      .cf/identity.json    ← isolated identity for this worktree (auto-used)
```

### Gate escalation — agent blocks on human decision

```bash
# Agent encounters a decision it can't make:
rd gate <item-id> --question "Use pessimistic or optimistic locking?" --context "Observed concurrent requests in prod"

# Human sees the gate in rd ready, approves:
rd approve <gate-id> --ruling "Pessimistic. Reason: concurrent request rate too high for optimistic."

# Agent resumes — rd show <item-id> now includes the ruling
```

---

## Quick reference

| Command | What it does |
|---------|-------------|
| `rd init --name <project>` | Create a work campfire |
| `rd invite` | Issue a single-use join token |
| `rd join <token>` | Join a campfire from a token |
| `rd create "..." [--type task] [--priority p0]` | Create a work item |
| `rd ready` | What's actionable now (auto-synced) |
| `rd ready --view work` | Items currently in_progress |
| `rd list` | All open items |
| `rd show <id>` | Item details + audit trail |
| `rd update <id> --status in_progress` | Claim an item |
| `rd done <id> --reason "..."` | Close with reason |
| `rd update <id> --note "..."` | Add a progress note |
| `rd dep add <child> <blocker>` | Wire a dependency (cross-campfire OK) |
| `rd dep tree <id>` | View dependency hierarchy |
| `rd gate <id> --question "..."` | Block item on human decision |
| `rd approve <gate-id> --ruling "..."` | Fulfill a gate |

**Item fields:** `type` (task, decision, review, reminder, deadline), `priority` (p0–p3), `status` (inbox, in_progress, waiting, blocked, done, cancelled, failed), `due`, `eta`

---

## How it works

Ready is a **convention**, not an application. It defines structured operations (`work:create`, `work:claim`, `work:close`, etc.) as campfire messages. `rd` is a thin CLI wrapper that speaks this convention.

`rd list`, `rd ready`, and `rd show` auto-pull from campfire on every call — no manual sync step.

**WHO is first-class.** Every item has `for` (who needs the outcome) and `by` (who's doing the work). Delegation is an explicit act.

**Attention engine.** `rd ready` filters to what's actionable for your identity right now. An agent's view shows what's assigned to it.

---

- [campfire](https://getcampfire.dev) — the protocol
- [agentic internet](https://aietf.getcampfire.dev) — conventions for agent coordination

MIT License
