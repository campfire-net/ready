# CLI Agent Ergonomics — Design Brief

**Date:** 2026-04-08
**Status:** Approved (Baron, session 2026-04-07)
**Items:** ready-42c, ready-00f, ready-731, ready-519, ready-bd6, ready-d2b

## Problem

99.9% of rd invocations are by agents, not operators. The CLI defaults are human-biased:

1. **Output format**: `--json` is opt-in. Default is formatted tables. Agents must add `--json` to every call or parse columns.
2. **Warnings on stdout**: Durability, inbox membership, and provenance warnings go to stdout, contaminating machine-parseable output. An agent piping `rd ready` to `jq` gets garbage.
3. **No agent onboarding**: Getting-started guide is for humans. No `.well-known/agent.json` for A2A discovery. README leads with human workflow.

## Decisions (shipped this session)

- ReadyFilter no longer gates on ETA. ETA is for sort order. `scheduled` status is the gate for work that can't start yet. (commit bc1cb43)
- `rd ready` identity filter matches `for` OR `by`. Delegated work now visible to the performer. (commit bc1cb43)

## Remaining work

### ready-42c: JSON default output

**Change:** `root.go:68` — flip `jsonOutput` default to `true`. Add `--human` / `--pretty` flag that sets `jsonOutput = false`.

**Scope:**
- `cmd/rd/root.go` — default and flag
- Every command that checks `jsonOutput` — verify JSON path is the primary code path
- `printItemTable` and similar formatters become `--human`-only
- `rd create` should always return the created item JSON (currently silent without `--json`)

**Breaking:** Yes. Any script parsing table output breaks. Any script already using `--json` is unaffected.

**Test:** `go test ./cmd/rd/...` — existing tests that assert table output need updating to assert JSON or use `--human`.

### ready-00f: Warnings to stderr

**Change:** All `fmt.Printf("warning: ...")` and `log.Printf("warning: ...")` calls in `cmd/rd/` must write to `os.Stderr`, not `os.Stdout`.

**Scope:**
- `cmd/rd/root.go` — the `warn()` helper (if it exists) or all inline warning prints
- `cmd/rd/init.go` — durability warnings
- `cmd/rd/join.go` — inbox membership warnings
- `cmd/rd/send.go` — campfire send failure warnings
- Any command that prints "warning:" to stdout

**Pattern:** Create a `warnf(format, args...)` helper that writes to stderr. Replace all warning prints.

**Test:** Capture stderr separately from stdout in tests. Verify stdout is clean JSON.

### ready-731: docs/agent.md

Primary getting-started guide for agents. Content:

```markdown
# Agent Quickstart

## Join a project
cf init                    # once — creates identity at ~/.cf
rd join <campfire-id>      # join the project

## Work loop
rd ready --view my-work    # your queue (JSON by default)
rd claim <id>              # accept work
rd progress <id> --notes "status update"
rd done <id> --reason "what was accomplished"

## Escalate
rd gate <id> --gate-type design --description "question for human"
# human runs: rd approve <id> --reason "direction"

## Programmatic parsing
rd ready --view my-work    # returns JSON array
rd show <id>               # returns JSON object
# exit code 0 = success, non-zero = error
# warnings go to stderr, data goes to stdout
```

### ready-519: .well-known/agent.json

A2A discovery file. Format:

```json
{
  "name": "ready",
  "description": "Work management as a campfire convention",
  "protocol": "campfire",
  "mcp_endpoint": "npx @campfire-net/campfire-mcp",
  "docs": {
    "agent_quickstart": "https://ready.getcampfire.dev/docs/agent",
    "convention_schema": "https://ready.getcampfire.dev/docs/convention"
  },
  "operations": [
    "work:create", "work:claim", "work:close", "work:status",
    "work:delegate", "work:gate", "work:gate-resolve",
    "work:block", "work:unblock"
  ]
}
```

### ready-bd6: README restructure

Lead with agent workflow. Move operator setup to "Setting up a project" section below. Link to docs/agent.md as the primary guide.

### ready-d2b: Demo script refresh

After D1+D2 ship, re-run all 6 demo scripts (`test/demo/01-06.sh`). Output will change (JSON default, warnings to stderr). Update `test/demo/output/*.txt` and the transcript excerpts in `docs/getting-started.md` and `site/index.html`.
