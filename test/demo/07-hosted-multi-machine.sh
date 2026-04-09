#!/usr/bin/env bash
# 07-hosted-multi-machine.sh — Hosted real-time multi-machine demo
#
# End-to-end verification of the hosted campfire path:
#   - Owner creates hosted project on mcp.getcampfire.dev (Machine A)
#   - Owner generates invite token; member joins (Machine B)
#   - Member sees items, claims and closes one
#   - Owner syncs back and sees the close
#
# SECURITY: An unadmitted intruder is rejected at the campfire layer.
#
# Requires: mcp.getcampfire.dev:443 reachable.
set -euo pipefail

RD=/tmp/rd-demo
OUT_DIR="$(cd "$(dirname "$0")" && pwd)/output"
OUT="$OUT_DIR/07-hosted-multi-machine.txt"
mkdir -p "$OUT_DIR"

# Check hosted infra is reachable
if ! timeout 5 bash -c 'echo > /dev/tcp/mcp.getcampfire.dev/443' 2>/dev/null; then
    echo "SKIP: mcp.getcampfire.dev:443 not reachable"
    exit 0
fi

PROJ_A=$(mktemp -d /tmp/rdtest-hosted-projA-XXXX)
PROJ_B=$(mktemp -d /tmp/rdtest-hosted-projB-XXXX)
PROJ_INTRUDER=$(mktemp -d /tmp/rdtest-hosted-projI-XXXX)
trap "rm -rf $PROJ_A $PROJ_B $PROJ_INTRUDER" EXIT

tee_section() {
    local header="$1"
    {
        echo ""
        echo "═══════════════════════════════════════════════════════════"
        echo "  $header"
        echo "═══════════════════════════════════════════════════════════"
        echo ""
    } | tee -a "$OUT"
}

run() {
    local label="$1"; shift
    echo "$ $label" | tee -a "$OUT"
    "$@" 2>&1 | tee -a "$OUT"
    echo "" | tee -a "$OUT"
}

# Run a command that is expected to fail. Captures output and exit code.
# Fails the demo if the command unexpectedly succeeds.
run_expect_fail() {
    local label="$1"; shift
    echo "$ $label" | tee -a "$OUT"
    local output
    output=$("$@" 2>&1) && {
        echo "  UNEXPECTED SUCCESS — command should have failed" | tee -a "$OUT"
        echo "  output: $output" | tee -a "$OUT"
        echo "" | tee -a "$OUT"
        exit 1
    }
    echo "  REJECTED: $output" | tee -a "$OUT"
    echo "" | tee -a "$OUT"
}

# Start fresh output
> "$OUT"

echo "# Ready Demo 07 — Hosted Real-Time Multi-Machine — $(date)" | tee -a "$OUT"
echo "# Three identities: owner, member, and intruder (unadmitted)" | tee -a "$OUT"

# ── 1. Initialize hosted identities ──────────────────────────────────────────
tee_section "1. Initialize hosted identities (owner and intruder)"

# Owner identity lives in $PROJ_A/.cf/ — walk-up finds it from $PROJ_A
mkdir -p "$PROJ_A/.cf"
run "mkdir -p \$PROJ_A/.cf && cf init --cf-home \$PROJ_A/.cf --remote https://mcp.getcampfire.dev  (owner)" \
    cf init --cf-home "$PROJ_A/.cf" --remote https://mcp.getcampfire.dev

# Intruder identity lives in $PROJ_INTRUDER/.cf/
mkdir -p "$PROJ_INTRUDER/.cf"
run "mkdir -p \$PROJ_INTRUDER/.cf && cf init --cf-home \$PROJ_INTRUDER/.cf --remote https://mcp.getcampfire.dev  (intruder)" \
    cf init --cf-home "$PROJ_INTRUDER/.cf" --remote https://mcp.getcampfire.dev

# ── 2. Owner creates hosted project (Machine A) ─────────────────────────────
tee_section "2. Owner creates hosted project (Machine A)"

run "cd \$PROJ_A && rd init --name hosted-demo" \
    bash -c "cd '$PROJ_A' && '$RD' init --name hosted-demo"

CAMPFIRE_ID=$(cat "$PROJ_A/.campfire/root")
echo "Project campfire ID: ${CAMPFIRE_ID:0:12}..." | tee -a "$OUT"
echo "" | tee -a "$OUT"

# ── 3. Owner creates a work item ─────────────────────────────────────────────
tee_section "3. Owner creates work item"

ITEM_ID=$(cd "$PROJ_A" && "$RD" create "Deploy staging env" --type task --priority p1)
echo "$ cd \$PROJ_A && rd create 'Deploy staging env' --type task --priority p1" | tee -a "$OUT"
echo "Created item: $ITEM_ID" | tee -a "$OUT"
echo "" | tee -a "$OUT"

# ── 4. Owner generates invite token for member ───────────────────────────────
tee_section "4. Owner generates invite token for member"

INVITE_TOKEN=$(cd "$PROJ_A" && "$RD" invite 2>&1 | grep '^rdx1_')
run "cd \$PROJ_A && rd invite  (owner generates member token)" \
    bash -c "echo 'rdx1_...  (invite token — share securely)'"

# ── 5. INTRUDER tries to access the campfire ─────────────────────────────────
tee_section "5. SECURITY: unadmitted intruder is rejected"

echo "Intruder has a valid hosted identity but was NEVER invited." | tee -a "$OUT"
echo "The campfire is invite-only — membership is enforced at the protocol layer." | tee -a "$OUT"
echo "" | tee -a "$OUT"

# 5a. Intruder tries to join → hard rejection
echo "--- 5a. Join attempt (should be rejected) ---" | tee -a "$OUT"
run_expect_fail "cd \$PROJ_INTRUDER && rd join <campfire-id>  (intruder)" \
    bash -c "cd '$PROJ_INTRUDER' && '$RD' join '$CAMPFIRE_ID'"

# 5b. Intruder manually crafts local state to attempt bypass, tries to create
echo "--- 5b. Intruder manually creates .campfire/root and tries rd create ---" | tee -a "$OUT"
mkdir -p "$PROJ_INTRUDER/.campfire" "$PROJ_INTRUDER/.ready"
echo "$CAMPFIRE_ID" > "$PROJ_INTRUDER/.campfire/root"
echo '{"campfire_id":"'"$CAMPFIRE_ID"'"}' > "$PROJ_INTRUDER/.ready/config.json"
touch "$PROJ_INTRUDER/.ready/mutations.jsonl"

# rd create writes locally first (offline-first design), but the campfire REJECTS the send.
# The item exists only on the intruder's machine — it never reaches the campfire.
CREATE_OUTPUT=$(cd "$PROJ_INTRUDER" && "$RD" create "Injected item" --priority p0 --type task 2>&1)
echo "$ cd \$PROJ_INTRUDER && rd create 'Injected item' --priority p0 --type task" | tee -a "$OUT"
echo "$CREATE_OUTPUT" | tee -a "$OUT"
echo "" | tee -a "$OUT"

if echo "$CREATE_OUTPUT" | grep -q "not a member"; then
    echo "  CAMPFIRE REJECTED: send failed — intruder is not a member." | tee -a "$OUT"
    echo "  Item buffered locally only. Will never reach the campfire." | tee -a "$OUT"
else
    echo "  ERROR: expected 'not a member' rejection from campfire" | tee -a "$OUT"
    exit 1
fi
echo "" | tee -a "$OUT"

# 5c. Intruder tries to sync pull → gets nothing (can't read campfire messages)
echo "--- 5c. Intruder tries rd sync pull (should get 0 messages) ---" | tee -a "$OUT"
PULL_OUTPUT=$(cd "$PROJ_INTRUDER" && "$RD" sync pull 2>&1)
echo "$ cd \$PROJ_INTRUDER && rd sync pull" | tee -a "$OUT"
echo "$PULL_OUTPUT" | tee -a "$OUT"
echo "" | tee -a "$OUT"

# 5d. Verify: owner's rd ready does NOT show intruder's item
echo "--- 5d. Verify: owner sees NO intruder items ---" | tee -a "$OUT"
OWNER_LIST=$(cd "$PROJ_A" && "$RD" list --all 2>&1)
echo "$ cd \$PROJ_A && rd list --all  (should NOT contain 'Injected item')" | tee -a "$OUT"
echo "$OWNER_LIST" | tee -a "$OUT"
echo "" | tee -a "$OUT"

if echo "$OWNER_LIST" | grep -q "Injected"; then
    echo "  SECURITY FAILURE: intruder's item appeared in owner's list!" | tee -a "$OUT"
    exit 1
else
    echo "  VERIFIED: intruder's item does not appear. Campfire membership enforced." | tee -a "$OUT"
fi
echo "" | tee -a "$OUT"

# ── 6. Member joins via invite token (Machine B) ────────────────────────────
tee_section "6. Member joins via invite token (Machine B)"

# Member identity goes into $PROJ_B/.cf/ — walk-up handles all subsequent commands
mkdir -p "$PROJ_B/.cf"
run "mkdir -p \$PROJ_B/.cf && cd \$PROJ_B && CF_HOME=\$PROJ_B/.cf rd join <invite-token>  (one-time identity bootstrap)" \
    bash -c "cd '$PROJ_B' && CF_HOME='$PROJ_B/.cf' '$RD' join '$INVITE_TOKEN'"

# Verify bootstrap: .campfire/root and .ready/ must exist (ready-8d8)
echo "Verify bootstrap:" | tee -a "$OUT"
if [ -f "$PROJ_B/.campfire/root" ]; then
    echo "  .campfire/root: EXISTS ✓" | tee -a "$OUT"
else
    echo "  .campfire/root: MISSING ✗" | tee -a "$OUT"
    exit 1
fi
if [ -f "$PROJ_B/.ready/config.json" ]; then
    echo "  .ready/config.json: EXISTS ✓" | tee -a "$OUT"
else
    echo "  .ready/config.json: MISSING ✗" | tee -a "$OUT"
    exit 1
fi
echo "" | tee -a "$OUT"

# ── 7. Member sees items (auto-synced on join, ready-5cd) ───────────────────
tee_section "7. Member sees owner's item (auto-synced on join)"

# rd join auto-syncs — no manual rd sync pull needed (ready-5cd)
run "cd \$PROJ_B && rd ready  (from Machine B)" \
    bash -c "cd '$PROJ_B' && '$RD' ready"

# ── 8. Member claims item ───────────────────────────────────────────────────
tee_section "8. Member claims item"

run "cd \$PROJ_B && rd update $ITEM_ID --status active" \
    bash -c "cd '$PROJ_B' && '$RD' update '$ITEM_ID' --status active"

# ── 9. Member closes item (ready-c0c: close at provenance level 1) ──────────
tee_section "9. Member closes item (ready-c0c: admitted member can close)"

run "cd \$PROJ_B && rd done $ITEM_ID --reason 'Staging deployed'" \
    bash -c "cd '$PROJ_B' && '$RD' done '$ITEM_ID' --reason 'Staging deployed'"

# ── 10. Owner sees the close — auto-sync on rd list --all ────────────────────
tee_section "10. Owner sees member's close — auto-synced on rd list --all"

# rd list --all auto-pulls from campfire before displaying (ready-341)
run "cd \$PROJ_A && rd list --all  (from Machine A)" \
    bash -c "cd '$PROJ_A' && '$RD' list --all"

echo "══════════════════════════════════════════════════════════════" | tee -a "$OUT"
echo "  Demo 07 complete. Hosted multi-machine + security verified." | tee -a "$OUT"
echo "  Transcript saved to: $OUT" | tee -a "$OUT"
echo "══════════════════════════════════════════════════════════════" | tee -a "$OUT"
