---
name: rd
description: "Work management on campfire. Use when the user asks about work items, tasks, bugs, or what needs attention. Commands: rd ready, rd create, rd claim, rd close, rd list, rd delegate."
argument-hint: "<command> [args]"
---

Running rd $ARGUMENTS.

## Commands

| Command | What it does |
|---------|-------------|
| `rd ready` | Show items needing attention now |
| `rd list [--status ...] [--all] [--json]` | List items with filters |
| `rd show <id>` | Item details and audit trail |
| `rd create "..." --type task --priority p1` | Create a work item |
| `rd claim <id>` | Accept work — become the performer |
| `rd close <id> --reason "..."` | Close with reason |
| `rd delegate <id> --to <identity>` | Assign to a person or agent |
| `rd update <id> --status <status>` | Change status, priority, ETA |
| `rd init --name <project>` | Create a work campfire |
| `rd register [--org <name>]` | Register in an org for multi-project |

## Usage

If $ARGUMENTS is empty, run `rd ready` to show what needs attention.

Otherwise, run the command as given:

```bash
rd $ARGUMENTS 2>&1
```

If the command fails with "rd: command not found", tell the user to install it:

```
curl -fsSL https://ready.getcampfire.dev/install.sh | sh
```

## Item lifecycle

1. **Create**: `rd create "..." --type task --priority p1`
2. **Claim**: `rd claim <id>` — sets you as performer, status → active
3. **Work**: do the work
4. **Close**: `rd close <id> --reason "why"` — done, cancelled, or failed

## Types and priorities

- **Types**: task, decision, review, reminder, deadline, prep, message, directive
- **Priorities**: p0 (now), p1 (+4h), p2 (+24h), p3 (+72h)
- **Statuses**: inbox, active, scheduled, waiting, blocked, done, cancelled, failed

## Views

`rd ready` defaults to the "ready" view. Other views:

```bash
rd ready --view overdue      # past-due items
rd ready --view delegated    # work I delegated, in progress
rd ready --view my-work      # work assigned to me
rd ready --view work         # actively being worked
rd ready --view pending      # waiting, scheduled, blocked
```

## JSON output

All commands support `--json` for structured output:

```bash
rd list --json | jq '.[] | select(.status == "active")'
rd create "..." --type task --priority p1 --json
```
