#!/usr/bin/env bash
# Demo: two-identity team workflow
# Owner creates a project, invites a member via token, creates and delegates work,
# member claims and completes it, owner verifies.
set -euo pipefail

RD=/tmp/rd-demo
OUT_DIR="$(cd "$(dirname "$0")" && pwd)/output"
OUT="$OUT_DIR/02-team.txt"
mkdir -p "$OUT_DIR"

PROJECT=$(mktemp -d /tmp/rdtest-team-proj-XXXX)
trap "rm -rf $PROJECT" EXIT

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

# Owner identity lives in $PROJECT/.cf/ — walk-up finds it from anywhere in $PROJECT
mkdir -p "$PROJECT/.cf"
run "mkdir -p \$PROJECT/.cf && cf init --cf-home \$PROJECT/.cf  (owner)" \
    cf init --cf-home "$PROJECT/.cf"

# ── 2. Owner creates project ──────────────────────────────────────────────────
tee_section "2. Owner initializes project"

run "cd \$PROJECT && rd init --name teamproject" \
    bash -c "cd '$PROJECT' && '$RD' init --name teamproject"

# ── 3. Owner generates invite token ──────────────────────────────────────────
tee_section "3. Owner generates invite token"

INVITE_TOKEN=$(cd "$PROJECT" && "$RD" invite 2>&1 | grep '^rdx1_')
echo "$ cd \$PROJECT && rd invite" | tee -a "$OUT"
echo "rdx1_...  (invite token — treat as secret)" | tee -a "$OUT"
echo "" | tee -a "$OUT"

# ── 4. Member joins via token ────────────────────────────────────────────────
tee_section "4. Member joins via invite token"

# Member identity goes into $PROJECT/member/.cf/ — join runs from the project root
# so that .campfire/root and .ready/ are shared (same project directory).
# After join, member cd into their subdir and walk-up finds:
#   .cf/identity.json  at $PROJECT/member/.cf/   (member identity)
#   .campfire/root     at $PROJECT/              (shared project)
mkdir -p "$PROJECT/member/.cf"
run "mkdir -p \$PROJECT/member/.cf && cd \$PROJECT && CF_HOME=\$PROJECT/member/.cf rd join <invite-token>  (one-time identity bootstrap)" \
    bash -c "cd '$PROJECT' && CF_HOME='$PROJECT/member/.cf' '$RD' join '$INVITE_TOKEN'"

# ── 5. Owner creates a work item ──────────────────────────────────────────────
tee_section "5. Owner creates work item"

ITEM_ID=$(cd "$PROJECT" && "$RD" create "Build API" --type task --priority p1)
echo "$ cd \$PROJECT && rd create 'Build API' --type task --priority p1" | tee -a "$OUT"
echo "Created item: $ITEM_ID" | tee -a "$OUT"
echo "" | tee -a "$OUT"

# ── 6. Owner delegates to member ─────────────────────────────────────────────
tee_section "6. Owner delegates item to member"

MEMBER_PUBKEY=$(CF_HOME="$PROJECT/member/.cf" cf id --json | python3 -c "import sys,json; print(json.load(sys.stdin)['public_key'])")
echo "Delegating to member identity: ${MEMBER_PUBKEY:0:12}..." | tee -a "$OUT"
echo "" | tee -a "$OUT"

run "cd \$PROJECT && rd delegate $ITEM_ID --to <member-identity>" \
    bash -c "cd '$PROJECT' && '$RD' delegate '$ITEM_ID' --to '$MEMBER_PUBKEY'"

# ── 7. Member views their work ────────────────────────────────────────────────
tee_section "7. Member views ready items"

run "cd \$PROJECT/member && rd ready" \
    bash -c "cd '$PROJECT/member' && '$RD' ready"

# ── 8. Member claims item ────────────────────────────────────────────────────
tee_section "8. Member claims item (sets active)"

run "cd \$PROJECT/member && rd update $ITEM_ID --status active" \
    bash -c "cd '$PROJECT/member' && '$RD' update '$ITEM_ID' --status active"

# ── 9. Member completes item ─────────────────────────────────────────────────
tee_section "9. Member completes item"

run "cd \$PROJECT/member && rd done $ITEM_ID --reason 'API complete'" \
    bash -c "cd '$PROJECT/member' && '$RD' done '$ITEM_ID' --reason 'API complete'"

# ── 10. Owner verifies ────────────────────────────────────────────────────────
tee_section "10. Owner verifies — list all items"

run "cd \$PROJECT && rd list --all" \
    bash -c "cd '$PROJECT' && '$RD' list --all"

echo "══════════════════════════════════════════════════════════════" | tee -a "$OUT"
echo "  Demo complete. Transcript saved to: $OUT" | tee -a "$OUT"
echo "══════════════════════════════════════════════════════════════" | tee -a "$OUT"
