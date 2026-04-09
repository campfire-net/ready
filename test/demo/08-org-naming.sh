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

PROJ_A=$(mktemp -d /tmp/rdtest-naming-projA-XXXX)
PROJ_B=$(mktemp -d /tmp/rdtest-naming-projB-XXXX)
trap "rm -rf $PROJ_A $PROJ_B" EXIT

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

# Owner identity lives in $PROJ_A/.cf/ — walk-up finds it from $PROJ_A
mkdir -p "$PROJ_A/.cf"
run "mkdir -p \$PROJ_A/.cf && cf init --cf-home \$PROJ_A/.cf --remote https://mcp.getcampfire.dev  (owner)" \
    cf init --cf-home "$PROJ_A/.cf" --remote https://mcp.getcampfire.dev

# ── 2. Owner creates hosted project ─────────────────────────────────────────
tee_section "2. Owner creates hosted project"

run "cd \$PROJ_A && rd init --name myapp" \
    bash -c "cd '$PROJ_A' && '$RD' init --name myapp"

CAMPFIRE_ID=$(cat "$PROJ_A/.campfire/root")
echo "Project campfire ID: ${CAMPFIRE_ID:0:12}..." | tee -a "$OUT"
echo "" | tee -a "$OUT"

# ── 3. Owner registers project under org namespace ──────────────────────────
tee_section "3. Owner registers under org namespace"

run "cd \$PROJ_A && rd register --org acme" \
    bash -c "cd '$PROJ_A' && '$RD' register --org acme"

# Show the registered naming config
run "cd \$PROJ_A && rd register --json" \
    bash -c "cd '$PROJ_A' && '$RD' register --json" || true

# ── 4. Owner creates work items ─────────────────────────────────────────────
tee_section "4. Owner creates work items"

ITEM1_ID=$(cd "$PROJ_A" && "$RD" create "Set up CI pipeline" --type task --priority p0)
echo "$ cd \$PROJ_A && rd create 'Set up CI pipeline' --type task --priority p0" | tee -a "$OUT"
echo "Created: $ITEM1_ID" | tee -a "$OUT"
echo "" | tee -a "$OUT"

ITEM2_ID=$(cd "$PROJ_A" && "$RD" create "Write API docs" --type task --priority p2)
echo "$ cd \$PROJ_A && rd create 'Write API docs' --type task --priority p2" | tee -a "$OUT"
echo "Created: $ITEM2_ID" | tee -a "$OUT"
echo "" | tee -a "$OUT"

# ── 5. Owner generates invite token ─────────────────────────────────────────
tee_section "5. Owner generates invite token for member"

INVITE_TOKEN=$(cd "$PROJ_A" && "$RD" invite 2>&1 | grep '^rdx1_')
run "cd \$PROJ_A && rd invite" \
    bash -c "echo 'rdx1_...  (invite token)'"

# ── 6. Member joins via invite token ────────────────────────────────────────
tee_section "6. Member joins via invite token"

# Member identity goes into $PROJ_B/.cf/ — walk-up handles all subsequent commands
mkdir -p "$PROJ_B/.cf"
run "mkdir -p \$PROJ_B/.cf && cd \$PROJ_B && CF_HOME=\$PROJ_B/.cf rd join <invite-token>  (one-time identity bootstrap)" \
    bash -c "cd '$PROJ_B' && CF_HOME='$PROJ_B/.cf' '$RD' join '$INVITE_TOKEN'"

# ── 7. Member sees items (auto-synced on join) ───────────────────────────────
tee_section "7. Member sees items (auto-synced on join)"

# rd join auto-syncs — no manual rd sync pull needed (ready-5cd)
run "cd \$PROJ_B && rd ready" \
    bash -c "cd '$PROJ_B' && '$RD' ready"

# ── 8. Member works the P0 item ─────────────────────────────────────────────
tee_section "8. Member works the P0 item"

run "cd \$PROJ_B && rd update $ITEM1_ID --status active" \
    bash -c "cd '$PROJ_B' && '$RD' update '$ITEM1_ID' --status active"

run "cd \$PROJ_B && rd done $ITEM1_ID --reason 'CI pipeline live'" \
    bash -c "cd '$PROJ_B' && '$RD' done '$ITEM1_ID' --reason 'CI pipeline live'"

# ── 9. Owner sees progress ──────────────────────────────────────────────────
tee_section "9. Owner syncs and sees progress"

run "cd \$PROJ_A && rd sync pull" \
    bash -c "cd '$PROJ_A' && '$RD' sync pull"

run "cd \$PROJ_A && rd list --all" \
    bash -c "cd '$PROJ_A' && '$RD' list --all"

echo "══════════════════════════════════════════════════════════════" | tee -a "$OUT"
echo "  Demo 08 complete. Org naming + hosted multi-user verified." | tee -a "$OUT"
echo "  Transcript saved to: $OUT" | tee -a "$OUT"
echo "══════════════════════════════════════════════════════════════" | tee -a "$OUT"
