#!/usr/bin/env bash
# Demo: two-identity team workflow
# Owner creates a project, invites a member via token, creates and delegates work,
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

# ── 1. Initialize owner identity ─────────────────────────────────────────────
tee_section "1. Initialize owner identity"

run "cf init --cf-home \$OWNER_CF  (owner)" \
    cf init --cf-home "$OWNER_CF"

# ── 2. Owner creates project ──────────────────────────────────────────────────
tee_section "2. Owner initializes project"

run "CF_HOME=\$OWNER_CF rd init --name teamproject  (in \$PROJECT)" \
    bash -c "cd '$PROJECT' && CF_HOME='$OWNER_CF' '$RD' init --name teamproject"

# ── 3. Owner generates invite token ──────────────────────────────────────────
tee_section "3. Owner generates invite token"

INVITE_TOKEN=$(cd "$PROJECT" && CF_HOME="$OWNER_CF" "$RD" invite --cf-home "$OWNER_CF" 2>&1 | grep '^rdx1_')
echo "$ CF_HOME=\$OWNER_CF rd invite  (owner)" | tee -a "$OUT"
echo "rdx1_...  (invite token — treat as secret)" | tee -a "$OUT"
echo "" | tee -a "$OUT"

# ── 4. Member joins via token ────────────────────────────────────────────────
tee_section "4. Member joins via invite token"

# The invite token contains a pre-provisioned identity; --force overwrites the
# auto-generated identity that protocol.Init creates on first run.
run "CF_HOME=\$MEMBER_CF rd join <invite-token> --force  (member)" \
    bash -c "cd '$PROJECT' && CF_HOME='$MEMBER_CF' '$RD' join '$INVITE_TOKEN' --force"

# ── 5. Owner creates a work item ──────────────────────────────────────────────
tee_section "5. Owner creates work item"

ITEM_ID=$(cd "$PROJECT" && CF_HOME="$OWNER_CF" "$RD" create "Build API" --type task --priority p1)
echo "$ CF_HOME=\$OWNER_CF rd create 'Build API' --type task --priority p1" | tee -a "$OUT"
echo "Created item: $ITEM_ID" | tee -a "$OUT"
echo "" | tee -a "$OUT"

# ── 6. Owner delegates to member ─────────────────────────────────────────────
tee_section "6. Owner delegates item to member"

MEMBER_PUBKEY=$(CF_HOME="$MEMBER_CF" cf id --json | python3 -c "import sys,json; print(json.load(sys.stdin)['public_key'])")
echo "Delegating to member identity: ${MEMBER_PUBKEY:0:12}..." | tee -a "$OUT"
echo "" | tee -a "$OUT"

run "CF_HOME=\$OWNER_CF rd delegate $ITEM_ID --to <member-identity>" \
    bash -c "cd '$PROJECT' && CF_HOME='$OWNER_CF' '$RD' delegate '$ITEM_ID' --to '$MEMBER_PUBKEY'"

# ── 7. Member views their work ────────────────────────────────────────────────
tee_section "7. Member views ready items"

run "CF_HOME=\$MEMBER_CF rd ready" \
    bash -c "cd '$PROJECT' && CF_HOME='$MEMBER_CF' '$RD' ready"

# ── 8. Member claims item ────────────────────────────────────────────────────
tee_section "8. Member claims item (sets active)"

run "CF_HOME=\$MEMBER_CF rd update $ITEM_ID --status active" \
    bash -c "cd '$PROJECT' && CF_HOME='$MEMBER_CF' '$RD' update '$ITEM_ID' --status active"

# ── 9. Member completes item ─────────────────────────────────────────────────
tee_section "9. Member completes item"

run "CF_HOME=\$MEMBER_CF rd done $ITEM_ID --reason 'API complete'" \
    bash -c "cd '$PROJECT' && CF_HOME='$MEMBER_CF' '$RD' done '$ITEM_ID' --reason 'API complete'"

# ── 10. Owner verifies ────────────────────────────────────────────────────────
tee_section "10. Owner verifies — list all items"

run "CF_HOME=\$OWNER_CF rd list --all" \
    bash -c "cd '$PROJECT' && CF_HOME='$OWNER_CF' '$RD' list --all"

echo "══════════════════════════════════════════════════════════════" | tee -a "$OUT"
echo "  Demo complete. Transcript saved to: $OUT" | tee -a "$OUT"
echo "══════════════════════════════════════════════════════════════" | tee -a "$OUT"
