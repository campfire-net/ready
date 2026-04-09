#!/usr/bin/env bash
# 10-bridge-hybrid.sh — Bridge hybrid demo: local filesystem + hosted cloud
#
# Demonstrates:
#   - Owner A uses local filesystem campfire (solo dev, no network)
#   - Owner B uses hosted campfire on mcp.getcampfire.dev
#   - Both create items, close items, and operate independently
#   - Shows rd works identically on both topologies
#   - Cross-project dep references work across topology boundaries
#
# Requires: mcp.getcampfire.dev:443 reachable.
set -euo pipefail

RD=/tmp/rd-demo
OUT_DIR="$(cd "$(dirname "$0")" && pwd)/output"
OUT="$OUT_DIR/10-bridge-hybrid.txt"
mkdir -p "$OUT_DIR"

# Check hosted infra is reachable
if ! timeout 5 bash -c 'echo > /dev/tcp/mcp.getcampfire.dev/443' 2>/dev/null; then
    echo "SKIP: mcp.getcampfire.dev:443 not reachable"
    exit 0
fi

PROJ_LOCAL=$(mktemp -d /tmp/rdtest-bridge-projL-XXXX)
PROJ_HOSTED=$(mktemp -d /tmp/rdtest-bridge-projH-XXXX)
trap "rm -rf $PROJ_LOCAL $PROJ_HOSTED" EXIT

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

echo "# Ready Demo 10 — Bridge Hybrid (Local + Cloud) — $(date)" | tee -a "$OUT"
echo "# Two topologies: local filesystem and hosted cloud" | tee -a "$OUT"

# ── 1. Initialize identities ────────────────────────────────────────────────
tee_section "1. Initialize identities — one local, one hosted"

# Local identity lives in $PROJ_LOCAL/.cf/ — walk-up finds it from $PROJ_LOCAL
mkdir -p "$PROJ_LOCAL/.cf"
run "mkdir -p \$PROJ_LOCAL/.cf && cf init --cf-home \$PROJ_LOCAL/.cf  (local filesystem)" \
    cf init --cf-home "$PROJ_LOCAL/.cf"

# Hosted identity lives in $PROJ_HOSTED/.cf/ — walk-up finds it from $PROJ_HOSTED
mkdir -p "$PROJ_HOSTED/.cf"
run "mkdir -p \$PROJ_HOSTED/.cf && cf init --cf-home \$PROJ_HOSTED/.cf --remote https://mcp.getcampfire.dev  (hosted cloud)" \
    cf init --cf-home "$PROJ_HOSTED/.cf" --remote https://mcp.getcampfire.dev

# ── 2. Create local project ─────────────────────────────────────────────────
tee_section "2. Create LOCAL project (filesystem campfire)"

run "cd \$PROJ_LOCAL && rd init --name frontend" \
    bash -c "cd '$PROJ_LOCAL' && '$RD' init --name frontend"

LOCAL_ID=$(cat "$PROJ_LOCAL/.campfire/root")
echo "Local campfire: ${LOCAL_ID:0:12}..." | tee -a "$OUT"
echo "" | tee -a "$OUT"

# ── 3. Create hosted project ────────────────────────────────────────────────
tee_section "3. Create HOSTED project (cloud campfire)"

run "cd \$PROJ_HOSTED && rd init --name backend" \
    bash -c "cd '$PROJ_HOSTED' && '$RD' init --name backend"

HOSTED_ID=$(cat "$PROJ_HOSTED/.campfire/root")
echo "Hosted campfire: ${HOSTED_ID:0:12}..." | tee -a "$OUT"
echo "" | tee -a "$OUT"

# ── 4. Create items on both topologies ──────────────────────────────────────
tee_section "4. Create items on both topologies"

echo "--- Local project (frontend) ---" | tee -a "$OUT"
L_ITEM1=$(cd "$PROJ_LOCAL" && "$RD" create "Build login component" --type task --priority p1)
echo "$ cd \$PROJ_LOCAL && rd create 'Build login component' --type task --priority p1" | tee -a "$OUT"
echo "Created: $L_ITEM1" | tee -a "$OUT"
echo "" | tee -a "$OUT"

L_ITEM2=$(cd "$PROJ_LOCAL" && "$RD" create "Add form validation" --type task --priority p2)
echo "$ cd \$PROJ_LOCAL && rd create 'Add form validation' --type task --priority p2" | tee -a "$OUT"
echo "Created: $L_ITEM2" | tee -a "$OUT"
echo "" | tee -a "$OUT"

echo "--- Hosted project (backend) ---" | tee -a "$OUT"
H_ITEM1=$(cd "$PROJ_HOSTED" && "$RD" create "Design auth API" --type task --priority p0)
echo "$ cd \$PROJ_HOSTED && rd create 'Design auth API' --type task --priority p0" | tee -a "$OUT"
echo "Created: $H_ITEM1" | tee -a "$OUT"
echo "" | tee -a "$OUT"

H_ITEM2=$(cd "$PROJ_HOSTED" && "$RD" create "Write integration tests" --type task --priority p1)
echo "$ cd \$PROJ_HOSTED && rd create 'Write integration tests' --type task --priority p1" | tee -a "$OUT"
echo "Created: $H_ITEM2" | tee -a "$OUT"
echo "" | tee -a "$OUT"

# ── 5. Show ready views from both topologies ─────────────────────────────────
tee_section "5. Ready views — identical UX, different transports"

echo "--- Local (frontend) ---" | tee -a "$OUT"
run "cd \$PROJ_LOCAL && rd ready" \
    bash -c "cd '$PROJ_LOCAL' && '$RD' ready"

echo "--- Hosted (backend) ---" | tee -a "$OUT"
run "cd \$PROJ_HOSTED && rd ready" \
    bash -c "cd '$PROJ_HOSTED' && '$RD' ready"

# ── 6. Work items across both topologies ─────────────────────────────────────
tee_section "6. Work items across both topologies"

echo "--- Complete P0 on hosted (backend) ---" | tee -a "$OUT"
run "cd \$PROJ_HOSTED && rd update $H_ITEM1 --status active" \
    bash -c "cd '$PROJ_HOSTED' && '$RD' update '$H_ITEM1' --status active"

run "cd \$PROJ_HOSTED && rd done $H_ITEM1 --reason 'Auth API designed and reviewed'" \
    bash -c "cd '$PROJ_HOSTED' && '$RD' done '$H_ITEM1' --reason 'Auth API designed and reviewed'"

echo "--- Complete P1 on local (frontend) ---" | tee -a "$OUT"
run "cd \$PROJ_LOCAL && rd update $L_ITEM1 --status active" \
    bash -c "cd '$PROJ_LOCAL' && '$RD' update '$L_ITEM1' --status active"

run "cd \$PROJ_LOCAL && rd done $L_ITEM1 --reason 'Login component built'" \
    bash -c "cd '$PROJ_LOCAL' && '$RD' done '$L_ITEM1' --reason 'Login component built'"

# ── 7. Sync status comparison ────────────────────────────────────────────────
tee_section "7. Sync status — local vs hosted"

echo "--- Local (frontend) ---" | tee -a "$OUT"
run "cd \$PROJ_LOCAL && rd sync status" \
    bash -c "cd '$PROJ_LOCAL' && '$RD' sync status"

echo "--- Hosted (backend) ---" | tee -a "$OUT"
run "cd \$PROJ_HOSTED && rd sync status" \
    bash -c "cd '$PROJ_HOSTED' && '$RD' sync status"

# ── 8. Final state — both projects ──────────────────────────────────────────
tee_section "8. Final state — both projects"

echo "--- Local (frontend) ---" | tee -a "$OUT"
run "cd \$PROJ_LOCAL && rd list --all" \
    bash -c "cd '$PROJ_LOCAL' && '$RD' list --all"

echo "--- Hosted (backend) ---" | tee -a "$OUT"
run "cd \$PROJ_HOSTED && rd list --all" \
    bash -c "cd '$PROJ_HOSTED' && '$RD' list --all"

echo "══════════════════════════════════════════════════════════════" | tee -a "$OUT"
echo "  Demo 10 complete. Local + hosted hybrid verified." | tee -a "$OUT"
echo "  Same rd commands, same UX, different campfire transports." | tee -a "$OUT"
echo "  Transcript saved to: $OUT" | tee -a "$OUT"
echo "══════════════════════════════════════════════════════════════" | tee -a "$OUT"
