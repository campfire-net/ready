#!/usr/bin/env bash
# 04-org-observer.sh — Org-observer demo: restricted read-only access to a project campfire
#
# Demonstrates the org-observer role:
#   - Owner creates a project and populates items
#   - Observer identity is admitted with --role org-observer via invite token
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

PROJECT=$(mktemp -d /tmp/rdtest-observer-proj-XXXX)
trap "rm -rf $PROJECT" EXIT

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

# ── 1. Setup: initialize owner identity ──────────────────────────────────────
section "=== SECTION: setup ==="

# Owner identity lives in $PROJECT/.cf/ — walk-up finds it from anywhere in $PROJECT
mkdir -p "$PROJECT/.cf"
run "mkdir -p \$PROJECT/.cf && cf init --cf-home \$PROJECT/.cf  (owner identity)" \
    cf init --cf-home "$PROJECT/.cf"

# ── 2. Owner initializes project ─────────────────────────────────────────────
section "=== SECTION: init-project ==="

run "cd \$PROJECT && rd init --name acme-platform" \
    bash -c "cd '$PROJECT' && '$RD' init --name acme-platform"

CAMPFIRE_ID=$(cat "$PROJECT/.campfire/root")
SUMMARY_CAMPFIRE_ID=$(python3 -c "import json; d=json.load(open('$PROJECT/.ready/config.json')); print(d['summary_campfire_id'])")
echo "Project campfire ID:  $CAMPFIRE_ID" | tee -a "$OUT"
echo "Summary campfire ID:  $SUMMARY_CAMPFIRE_ID" | tee -a "$OUT"
echo "" | tee -a "$OUT"

# Owner creates several items at different priorities
run "cd \$PROJECT && rd create \"Migrate auth to OAuth2\" --type task --priority p0" \
    bash -c "cd '$PROJECT' && '$RD' create 'Migrate auth to OAuth2' --type task --priority p0"

run "cd \$PROJECT && rd create \"Refactor billing module\" --type task --priority p1" \
    bash -c "cd '$PROJECT' && '$RD' create 'Refactor billing module' --type task --priority p1"

run "cd \$PROJECT && rd create \"Update API docs\" --type task --priority p2" \
    bash -c "cd '$PROJECT' && '$RD' create 'Update API docs' --type task --priority p2"

echo "# Current project state (owner view):" | tee -a "$OUT"
run "cd \$PROJECT && rd list" \
    bash -c "cd '$PROJECT' && '$RD' list"

# ── 3. Admit observer via invite token with org-observer role ─────────────────
section "=== SECTION: admit-observer ==="

# --role org-observer admits to the shadow summary campfire only.
# The summary campfire receives work:item-summary projections.
# Main campfire content is NOT accessible to the observer.
echo "# Owner generates an invite token with org-observer role." | tee -a "$OUT"
echo "# The token admits to the summary campfire — not the main project campfire." | tee -a "$OUT"
echo "" | tee -a "$OUT"

INVITE_TOKEN=$(cd "$PROJECT" && "$RD" invite --role org-observer 2>&1 | grep '^rdx1_')
run "cd \$PROJECT && rd invite --role org-observer" \
    bash -c "echo 'rdx1_...  (observer invite token)'"

# ── 4. Observer joins via invite token ────────────────────────────────────────
section "=== SECTION: observer-join ==="

# Observer gets their own subdirectory with its own .cf/ — walk-up anchors there
# NOTE: org-observers join the SUMMARY campfire via the invite token.
# The token is pre-authorized for the summary campfire only.
echo "# Observer joins using the invite token (no cf init needed)." | tee -a "$OUT"
echo "" | tee -a "$OUT"

mkdir -p "$PROJECT/observer/.cf"
# Observer joins from the project root so .campfire/root and .ready/ are shared.
# Identity goes into $PROJECT/observer/.cf/ — walk-up finds it from $PROJECT/observer/.
run "mkdir -p \$PROJECT/observer/.cf && cd \$PROJECT && CF_HOME=\$PROJECT/observer/.cf rd join <observer-invite-token>  (one-time identity bootstrap)" \
    bash -c "cd '$PROJECT' && CF_HOME='$PROJECT/observer/.cf' '$RD' join '$INVITE_TOKEN'"

echo "# For comparison: joining the main campfire is rejected for org-observers." | tee -a "$OUT"
run_expect_fail \
    "cd \$PROJECT/observer && rd join <main-campfire-id>  (expect rejection)" \
    bash -c "cd '$PROJECT/observer' && '$RD' join '$CAMPFIRE_ID'"

# ── 5. Observer reads ─────────────────────────────────────────────────────────
section "=== SECTION: observer-reads ==="

echo "# Observer runs rd list — sees item summary projections:" | tee -a "$OUT"
run "cd \$PROJECT/observer && rd list" \
    bash -c "cd '$PROJECT/observer' && '$RD' list"

echo "# Observer runs rd ready — sees actionable items:" | tee -a "$OUT"
run "cd \$PROJECT/observer && rd ready" \
    bash -c "cd '$PROJECT/observer' && '$RD' ready"

# ── 6. Observer attempts to create an item (should fail) ──────────────────────
section "=== SECTION: observer-write-attempt ==="

echo "# Observer attempts to create a work item." | tee -a "$OUT"
echo "# BEHAVIOR: rd create succeeds locally but campfire send is blocked." | tee -a "$OUT"
echo "# The item is buffered to pending.jsonl — not propagated to other members." | tee -a "$OUT"
echo "# The warning 'campfire send failed (buffered to pending.jsonl): not a member'" | tee -a "$OUT"
echo "# is the enforcement signal. Write isolation is at the campfire layer." | tee -a "$OUT"
echo "" | tee -a "$OUT"

run "cd \$PROJECT/observer && rd create 'Sneaky item' --type task --priority p3  (blocked at campfire layer)" \
    bash -c "cd '$PROJECT/observer' && '$RD' create 'Sneaky item' --type task --priority p3"

echo "# NOTE: The item was written to the project's local mutations.jsonl (local-first" | tee -a "$OUT"
echo "# architecture). Because the observer shares the project directory, the item" | tee -a "$OUT"
echo "# is visible to anyone reading that directory. The campfire send was blocked —" | tee -a "$OUT"
echo "# the item was buffered to pending.jsonl and was never delivered to remote members." | tee -a "$OUT"
echo "# Write isolation is enforced at the campfire layer, not the local JSONL layer." | tee -a "$OUT"
echo "" | tee -a "$OUT"

# ── 7. Owner adds another item — verify observer sees it ──────────────────────
section "=== SECTION: verify ==="

echo "# Owner creates a new high-priority item:" | tee -a "$OUT"
run "cd \$PROJECT && rd create \"Deploy v2 to production\" --type task --priority p0" \
    bash -c "cd '$PROJECT' && '$RD' create 'Deploy v2 to production' --type task --priority p0"

echo "# Observer list — new owner item appears as a summary projection." | tee -a "$OUT"
echo "# (The earlier 'Sneaky item' also appears because it was written to the shared" | tee -a "$OUT"
echo "#  project directory's local mutations.jsonl — visible locally but not synced.)" | tee -a "$OUT"
run "cd \$PROJECT/observer && rd list" \
    bash -c "cd '$PROJECT/observer' && '$RD' list"

echo "# Owner list (full view for comparison — same four items plus the local-only sneaky item):" | tee -a "$OUT"
run "cd \$PROJECT && rd list" \
    bash -c "cd '$PROJECT' && '$RD' list"

echo "══════════════════════════════════════════════════════════════" | tee -a "$OUT"
echo "  Demo complete. Transcript saved to: $OUT" | tee -a "$OUT"
echo "══════════════════════════════════════════════════════════════" | tee -a "$OUT"
