#!/usr/bin/env bash
# 04-org-observer.sh — Org-observer demo: restricted read-only access to a project campfire
#
# Demonstrates the org-observer role:
#   - Owner creates a project and populates items
#   - Observer identity is admitted with --role org-observer
#   - Observer joins the summary campfire (not the main project campfire)
#   - Observer sees work:item-summary projections (title, status, priority, assignee, eta)
#   - Observer CANNOT create items (no write access to main campfire)
#   - Owner adds another item — it appears in observer's summary view
#
# IMPLEMENTATION NOTE: --role org-observer admits the observer to a shadow
# "summary campfire" (separate from the main project campfire). The summary
# campfire receives work:item-summary projections. The observer joins that
# summary campfire ID directly, not the main campfire ID.
#
# Produces a real terminal transcript for documentation.
set -euo pipefail

RD=/tmp/rd-demo
OUT_DIR="$(cd "$(dirname "$0")" && pwd)/output"
OUT="$OUT_DIR/04-org-observer.txt"
mkdir -p "$OUT_DIR"

OWNER_CF=$(mktemp -d /tmp/rdtest-observer-owner-XXXX)
OBS_CF=$(mktemp -d /tmp/rdtest-observer-obs-XXXX)
PROJECT=$(mktemp -d /tmp/rdtest-observer-proj-XXXX)
trap "rm -rf $OWNER_CF $OBS_CF $PROJECT" EXIT

export PATH="/tmp/go/bin:$PATH"

# ── helpers ──────────────────────────────────────────────────────────────────

section() {
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

# Run a command that is EXPECTED to fail; capture exit code without aborting.
run_expect_fail() {
    local label="$1"; shift
    echo "$ $label" | tee -a "$OUT"
    set +e
    "$@" 2>&1 | tee -a "$OUT"
    local rc=$?
    set -e
    echo "(exit code: $rc)" | tee -a "$OUT"
    echo "" | tee -a "$OUT"
    return 0
}

# Start fresh
> "$OUT"
echo "# Ready Org-Observer Demo — $(date)" | tee -a "$OUT"
echo "# Owner grants read-only summary access to an external observer." | tee -a "$OUT"

# ── 1. Setup: initialize identities ──────────────────────────────────────────
section "=== SECTION: setup ==="

run "cf init --cf-home \$OWNER_CF  (owner identity)" \
    cf init --cf-home "$OWNER_CF"

run "cf init --cf-home \$OBS_CF  (observer identity)" \
    cf init --cf-home "$OBS_CF"

# ── 2. Owner initializes project ─────────────────────────────────────────────
section "=== SECTION: init-project ==="

run "CF_HOME=\$OWNER_CF  rd init --name acme-platform --confirm  (in \$PROJECT)" \
    bash -c "cd '$PROJECT' && CF_HOME='$OWNER_CF' '$RD' init --name acme-platform --confirm"

CAMPFIRE_ID=$(cat "$PROJECT/.campfire/root")
SUMMARY_CAMPFIRE_ID=$(python3 -c "import json; d=json.load(open('$PROJECT/.ready/config.json')); print(d['summary_campfire_id'])")
echo "Project campfire ID:  $CAMPFIRE_ID" | tee -a "$OUT"
echo "Summary campfire ID:  $SUMMARY_CAMPFIRE_ID" | tee -a "$OUT"
echo "" | tee -a "$OUT"

# Owner creates several items at different priorities
run "rd create \"Migrate auth to OAuth2\" --type task --priority p0" \
    bash -c "cd '$PROJECT' && CF_HOME='$OWNER_CF' '$RD' create 'Migrate auth to OAuth2' --type task --priority p0"

run "rd create \"Refactor billing module\" --type task --priority p1" \
    bash -c "cd '$PROJECT' && CF_HOME='$OWNER_CF' '$RD' create 'Refactor billing module' --type task --priority p1"

run "rd create \"Update API docs\" --type task --priority p2" \
    bash -c "cd '$PROJECT' && CF_HOME='$OWNER_CF' '$RD' create 'Update API docs' --type task --priority p2"

echo "# Current project state (owner view):" | tee -a "$OUT"
run "CF_HOME=\$OWNER_CF  rd list" \
    bash -c "cd '$PROJECT' && CF_HOME='$OWNER_CF' '$RD' list"

# ── 3. Admit observer ─────────────────────────────────────────────────────────
section "=== SECTION: admit-observer ==="

OBS_PUBKEY=$(CF_HOME="$OBS_CF" cf id --json | python3 -c "import sys,json; print(json.load(sys.stdin)['public_key'])")
echo "Observer public key: $OBS_PUBKEY" | tee -a "$OUT"
echo "" | tee -a "$OUT"

# --role org-observer admits to the shadow summary campfire only.
# The summary campfire receives work:item-summary projections.
# Main campfire content is NOT accessible to the observer.
run "CF_HOME=\$OWNER_CF  rd admit <observer-pubkey> --role org-observer" \
    bash -c "cd '$PROJECT' && CF_HOME='$OWNER_CF' '$RD' admit '$OBS_PUBKEY' --role org-observer"

# ── 4. Observer joins the summary campfire ────────────────────────────────────
section "=== SECTION: observer-join ==="

# NOTE: org-observers join the SUMMARY campfire (not the main project campfire).
# The summary campfire ID is in .ready/config.json as "summary_campfire_id".
# Joining the main campfire would be rejected — the observer was not admitted there.
echo "# Observer joins the summary campfire (not the main project campfire)." | tee -a "$OUT"
echo "# Summary campfire ID: $SUMMARY_CAMPFIRE_ID" | tee -a "$OUT"
echo "" | tee -a "$OUT"

run "CF_HOME=\$OBS_CF  rd join <summary-campfire-id>" \
    bash -c "cd '$PROJECT' && CF_HOME='$OBS_CF' '$RD' join '$SUMMARY_CAMPFIRE_ID'"

echo "# For comparison: joining the main campfire is rejected for org-observers." | tee -a "$OUT"
run_expect_fail \
    "CF_HOME=\$OBS_CF  rd join <main-campfire-id>  (expect rejection)" \
    bash -c "cd '$PROJECT' && CF_HOME='$OBS_CF' '$RD' join '$CAMPFIRE_ID'"

# ── 5. Observer reads ─────────────────────────────────────────────────────────
section "=== SECTION: observer-reads ==="

echo "# Observer runs rd list — sees item summary projections:" | tee -a "$OUT"
run "CF_HOME=\$OBS_CF  rd list" \
    bash -c "cd '$PROJECT' && CF_HOME='$OBS_CF' '$RD' list"

echo "# Observer runs rd ready — sees actionable items:" | tee -a "$OUT"
run "CF_HOME=\$OBS_CF  rd ready" \
    bash -c "cd '$PROJECT' && CF_HOME='$OBS_CF' '$RD' ready"

# ── 6. Observer attempts to create an item (should fail) ──────────────────────
section "=== SECTION: observer-write-attempt ==="

echo "# Observer attempts to create a work item." | tee -a "$OUT"
echo "# BEHAVIOR: rd create succeeds locally but campfire send is blocked." | tee -a "$OUT"
echo "# The item is buffered to pending.jsonl — not propagated to other members." | tee -a "$OUT"
echo "# The warning 'campfire send failed (buffered to pending.jsonl): not a member'" | tee -a "$OUT"
echo "# is the enforcement signal. Write isolation is at the campfire layer." | tee -a "$OUT"
echo "" | tee -a "$OUT"

run "CF_HOME=\$OBS_CF  rd create 'Sneaky item' --type task --priority p3  (blocked at campfire layer)" \
    bash -c "cd '$PROJECT' && CF_HOME='$OBS_CF' '$RD' create 'Sneaky item' --type task --priority p3"

echo "# NOTE: The item was written to the project's local mutations.jsonl (local-first" | tee -a "$OUT"
echo "# architecture). Because the observer shares the project directory, the item" | tee -a "$OUT"
echo "# is visible to anyone reading that directory. The campfire send was blocked —" | tee -a "$OUT"
echo "# the item was buffered to pending.jsonl and was never delivered to remote members." | tee -a "$OUT"
echo "# Write isolation is enforced at the campfire layer, not the local JSONL layer." | tee -a "$OUT"
echo "" | tee -a "$OUT"

# ── 7. Owner adds another item — verify observer sees it ──────────────────────
section "=== SECTION: verify ==="

echo "# Owner creates a new high-priority item:" | tee -a "$OUT"
run "CF_HOME=\$OWNER_CF  rd create \"Deploy v2 to production\" --type task --priority p0" \
    bash -c "cd '$PROJECT' && CF_HOME='$OWNER_CF' '$RD' create 'Deploy v2 to production' --type task --priority p0"

echo "# Observer list — new owner item appears as a summary projection." | tee -a "$OUT"
echo "# (The earlier 'Sneaky item' also appears because it was written to the shared" | tee -a "$OUT"
echo "#  project directory's local mutations.jsonl — visible locally but not synced.)" | tee -a "$OUT"
run "CF_HOME=\$OBS_CF  rd list" \
    bash -c "cd '$PROJECT' && CF_HOME='$OBS_CF' '$RD' list"

echo "# Owner list (full view for comparison — same four items plus the local-only sneaky item):" | tee -a "$OUT"
run "CF_HOME=\$OWNER_CF  rd list" \
    bash -c "cd '$PROJECT' && CF_HOME='$OWNER_CF' '$RD' list"

echo "══════════════════════════════════════════════════════════════" | tee -a "$OUT"
echo "  Demo complete. Transcript saved to: $OUT" | tee -a "$OUT"
echo "══════════════════════════════════════════════════════════════" | tee -a "$OUT"
