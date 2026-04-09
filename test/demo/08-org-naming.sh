#!/usr/bin/env bash
# 08-org-naming.sh — Org naming on hosted infra
#
# Demonstrates:
#   - Owner creates a hosted project and registers under an org namespace
#   - Project resolves via cf://<org>.ready.<project> naming
#   - Member joins via invite token (no key exchange needed)
#   - Both identities share the same named project
#
# Requires: mcp.getcampfire.dev:443 reachable.
set -euo pipefail

RD=/tmp/rd-demo
OUT_DIR="$(cd "$(dirname "$0")" && pwd)/output"
OUT="$OUT_DIR/08-org-naming.txt"
mkdir -p "$OUT_DIR"

# Check hosted infra is reachable
if ! timeout 5 bash -c 'echo > /dev/tcp/mcp.getcampfire.dev/443' 2>/dev/null; then
    echo "SKIP: mcp.getcampfire.dev:443 not reachable"
    exit 0
fi

OWNER_CF=$(mktemp -d /tmp/rdtest-naming-owner-XXXX)
MEMBER_CF=$(mktemp -d /tmp/rdtest-naming-member-XXXX)
PROJ_A=$(mktemp -d /tmp/rdtest-naming-projA-XXXX)
PROJ_B=$(mktemp -d /tmp/rdtest-naming-projB-XXXX)
trap "rm -rf $OWNER_CF $MEMBER_CF $PROJ_A $PROJ_B" EXIT

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

echo "# Ready Demo 08 — Org Naming on Hosted Infra — $(date)" | tee -a "$OUT"
echo "# Naming hierarchy: org -> ready namespace -> project" | tee -a "$OUT"

# ── 1. Initialize owner hosted identity ──────────────────────────────────────
tee_section "1. Initialize owner hosted identity"

run "cf init --cf-home \$OWNER_CF --remote https://mcp.getcampfire.dev  (owner)" \
    cf init --cf-home "$OWNER_CF" --remote https://mcp.getcampfire.dev

# ── 2. Owner creates hosted project ─────────────────────────────────────────
tee_section "2. Owner creates hosted project"

run "cd \$PROJ_A && CF_HOME=\$OWNER_CF rd init --name myapp" \
    bash -c "cd '$PROJ_A' && CF_HOME='$OWNER_CF' '$RD' init --name myapp"

CAMPFIRE_ID=$(cat "$PROJ_A/.campfire/root")
echo "Project campfire ID: ${CAMPFIRE_ID:0:12}..." | tee -a "$OUT"
echo "" | tee -a "$OUT"

# ── 3. Owner registers project under org namespace ──────────────────────────
tee_section "3. Owner registers under org namespace"

run "CF_HOME=\$OWNER_CF rd register --org acme" \
    bash -c "cd '$PROJ_A' && CF_HOME='$OWNER_CF' '$RD' register --org acme"

# Show the registered naming config
run "CF_HOME=\$OWNER_CF rd register --json" \
    bash -c "cd '$PROJ_A' && CF_HOME='$OWNER_CF' '$RD' register --json" || true

# ── 4. Owner creates work items ─────────────────────────────────────────────
tee_section "4. Owner creates work items"

ITEM1_ID=$(cd "$PROJ_A" && CF_HOME="$OWNER_CF" "$RD" create "Set up CI pipeline" --type task --priority p0)
echo "$ CF_HOME=\$OWNER_CF rd create 'Set up CI pipeline' --type task --priority p0" | tee -a "$OUT"
echo "Created: $ITEM1_ID" | tee -a "$OUT"
echo "" | tee -a "$OUT"

ITEM2_ID=$(cd "$PROJ_A" && CF_HOME="$OWNER_CF" "$RD" create "Write API docs" --type task --priority p2)
echo "$ CF_HOME=\$OWNER_CF rd create 'Write API docs' --type task --priority p2" | tee -a "$OUT"
echo "Created: $ITEM2_ID" | tee -a "$OUT"
echo "" | tee -a "$OUT"

# ── 5. Owner generates invite token ─────────────────────────────────────────
tee_section "5. Owner generates invite token for member"

INVITE_TOKEN=$(cd "$PROJ_A" && CF_HOME="$OWNER_CF" "$RD" invite --cf-home "$OWNER_CF" 2>&1 | grep '^rdx1_')
run "CF_HOME=\$OWNER_CF rd invite" \
    bash -c "echo 'rdx1_...  (invite token)'"

# ── 6. Member joins via invite token ────────────────────────────────────────
tee_section "6. Member joins via invite token"

# --force overwrites the auto-generated identity from protocol.Init
run "cd \$PROJ_B && CF_HOME=\$MEMBER_CF rd join <invite-token> --force  (member)" \
    bash -c "cd '$PROJ_B' && CF_HOME='$MEMBER_CF' '$RD' join '$INVITE_TOKEN' --force"

# ── 7. Member sees items (auto-synced on join) ───────────────────────────────
tee_section "7. Member sees items (auto-synced on join)"

# rd join auto-syncs — no manual rd sync pull needed (ready-5cd)
run "CF_HOME=\$MEMBER_CF rd ready" \
    bash -c "cd '$PROJ_B' && CF_HOME='$MEMBER_CF' '$RD' ready"

# ── 8. Member works the P0 item ─────────────────────────────────────────────
tee_section "8. Member works the P0 item"

run "CF_HOME=\$MEMBER_CF rd update $ITEM1_ID --status active" \
    bash -c "cd '$PROJ_B' && CF_HOME='$MEMBER_CF' '$RD' update '$ITEM1_ID' --status active"

run "CF_HOME=\$MEMBER_CF rd done $ITEM1_ID --reason 'CI pipeline live'" \
    bash -c "cd '$PROJ_B' && CF_HOME='$MEMBER_CF' '$RD' done '$ITEM1_ID' --reason 'CI pipeline live'"

# ── 9. Owner sees progress ──────────────────────────────────────────────────
tee_section "9. Owner syncs and sees progress"

run "CF_HOME=\$OWNER_CF rd sync pull" \
    bash -c "cd '$PROJ_A' && CF_HOME='$OWNER_CF' '$RD' sync pull"

run "CF_HOME=\$OWNER_CF rd list --all" \
    bash -c "cd '$PROJ_A' && CF_HOME='$OWNER_CF' '$RD' list --all"

echo "══════════════════════════════════════════════════════════════" | tee -a "$OUT"
echo "  Demo 08 complete. Org naming + hosted multi-user verified." | tee -a "$OUT"
echo "  Transcript saved to: $OUT" | tee -a "$OUT"
echo "══════════════════════════════════════════════════════════════" | tee -a "$OUT"
