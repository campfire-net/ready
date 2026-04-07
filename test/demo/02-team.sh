#!/usr/bin/env bash
# Demo: two-identity team workflow
# Owner creates a project, admits a member, creates and delegates work,
# member claims and completes it, owner verifies.
set -euo pipefail

RD=/tmp/rd-demo
OUT_DIR="$(cd "$(dirname "$0")" && pwd)/output"
OUT="$OUT_DIR/02-team.txt"
mkdir -p "$OUT_DIR"

OWNER_CF=$(mktemp -d /tmp/rdtest-team-owner-XXXX)
MEMBER_CF=$(mktemp -d /tmp/rdtest-team-member-XXXX)
PROJECT=$(mktemp -d /tmp/rdtest-team-proj-XXXX)
trap "rm -rf $OWNER_CF $MEMBER_CF $PROJECT" EXIT

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

# Start fresh output
> "$OUT"

echo "# Ready Team Demo — $(date)" | tee -a "$OUT"
echo "# Two identities: owner and member" | tee -a "$OUT"

# ── 1. Initialize identities ─────────────────────────────────────────────────
tee_section "1. Initialize identities"

run "cf init --cf-home \$OWNER_CF  (owner)" \
    cf init --cf-home "$OWNER_CF"

run "cf init --cf-home \$MEMBER_CF  (member)" \
    cf init --cf-home "$MEMBER_CF"

# ── 2. Owner creates project ──────────────────────────────────────────────────
tee_section "2. Owner initializes project"

run "CF_HOME=\$OWNER_CF rd init --name teamproject --confirm  (in \$PROJECT)" \
    bash -c "cd '$PROJECT' && CF_HOME='$OWNER_CF' '$RD' init --name teamproject --confirm"

CAMPFIRE_ID=$(cat "$PROJECT/.campfire/root")
echo "Project campfire ID: $CAMPFIRE_ID" | tee -a "$OUT"
echo "" | tee -a "$OUT"

# ── 3. Get member pubkey ──────────────────────────────────────────────────────
tee_section "3. Capture member public key"

MEMBER_PUBKEY=$(CF_HOME="$MEMBER_CF" cf id --json | python3 -c "import sys,json; print(json.load(sys.stdin)['public_key'])")
run "CF_HOME=\$MEMBER_CF cf id --json  (member pubkey)" \
    bash -c "CF_HOME='$MEMBER_CF' cf id --json"
echo "Member pubkey: $MEMBER_PUBKEY" | tee -a "$OUT"
echo "" | tee -a "$OUT"

# ── 4. Owner admits member ────────────────────────────────────────────────────
tee_section "4. Owner admits member"

run "CF_HOME=\$OWNER_CF rd admit <member-pubkey>" \
    bash -c "cd '$PROJECT' && CF_HOME='$OWNER_CF' '$RD' admit '$MEMBER_PUBKEY'"

# ── 5. Member joins ───────────────────────────────────────────────────────────
tee_section "5. Member joins project campfire"

run "CF_HOME=\$MEMBER_CF rd join <campfire-id>" \
    bash -c "cd '$PROJECT' && CF_HOME='$MEMBER_CF' '$RD' join '$CAMPFIRE_ID'"

# ── 6. Owner creates a work item ──────────────────────────────────────────────
tee_section "6. Owner creates work item"

ITEM_JSON=$(cd "$PROJECT" && CF_HOME="$OWNER_CF" "$RD" create "Build API" --type task --priority p1 --json)
echo "$ITEM_JSON" | tee -a "$OUT"
echo "" | tee -a "$OUT"

ITEM_ID=$(echo "$ITEM_JSON" | python3 -c "import sys,json; print(json.load(sys.stdin)['id'])")
echo "Created item: $ITEM_ID" | tee -a "$OUT"
echo "" | tee -a "$OUT"

# ── 7. Owner delegates to member ─────────────────────────────────────────────
tee_section "7. Owner delegates item to member"

MEMBER_IDENTITY=$(CF_HOME="$MEMBER_CF" cf id --json | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('campfire_id') or d.get('public_key'))")
echo "Delegating to member identity: $MEMBER_IDENTITY" | tee -a "$OUT"
echo "" | tee -a "$OUT"

run "CF_HOME=\$OWNER_CF rd delegate $ITEM_ID --to <member-identity>" \
    bash -c "cd '$PROJECT' && CF_HOME='$OWNER_CF' '$RD' delegate '$ITEM_ID' --to '$MEMBER_PUBKEY'"

# ── 8. Member views their work ────────────────────────────────────────────────
tee_section "8. Member views ready items"

run "CF_HOME=\$MEMBER_CF rd ready" \
    bash -c "cd '$PROJECT' && CF_HOME='$MEMBER_CF' '$RD' ready"

# ── 9. Member claims item ────────────────────────────────────────────────────
tee_section "9. Member claims item (sets active)"

run "CF_HOME=\$MEMBER_CF rd update $ITEM_ID --status active" \
    bash -c "cd '$PROJECT' && CF_HOME='$MEMBER_CF' '$RD' update '$ITEM_ID' --status active"

# ── 10. Member completes item ─────────────────────────────────────────────────
tee_section "10. Member completes item"

run "CF_HOME=\$MEMBER_CF rd done $ITEM_ID --reason 'API complete'" \
    bash -c "cd '$PROJECT' && CF_HOME='$MEMBER_CF' '$RD' done '$ITEM_ID' --reason 'API complete'"

# ── 11. Owner verifies ────────────────────────────────────────────────────────
tee_section "11. Owner verifies — list all items"

run "CF_HOME=\$OWNER_CF rd list --all" \
    bash -c "cd '$PROJECT' && CF_HOME='$OWNER_CF' '$RD' list --all"

echo "══════════════════════════════════════════════════════════════" | tee -a "$OUT"
echo "  Demo complete. Transcript saved to: $OUT" | tee -a "$OUT"
echo "══════════════════════════════════════════════════════════════" | tee -a "$OUT"
