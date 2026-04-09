#!/usr/bin/env bash
# Demo: gate/escalation workflow
# An agent hits a decision point and gates an item for human review.
# The human approves or rejects. Item status updates accordingly.
set -euo pipefail

RD=/tmp/rd-demo
OUT_DIR="$(cd "$(dirname "$0")" && pwd)/output"
OUT="$OUT_DIR/06-gate-escalation.txt"
mkdir -p "$OUT_DIR"

PROJECT=$(mktemp -d /tmp/rdtest-gate-proj-XXXX)
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

echo "# Ready Gate/Escalation Demo — $(date)" | tee -a "$OUT"
echo "# Agent hits a decision point, gates for human review." | tee -a "$OUT"
echo "# Human approves or rejects. Item status updates accordingly." | tee -a "$OUT"

# ── SECTION: setup ────────────────────────────────────────────────────────────
tee_section "=== SECTION: setup ==="

echo "# Initializing human identity and project" | tee -a "$OUT"
echo "" | tee -a "$OUT"

# Human/owner identity lives in $PROJECT/.cf/ — walk-up finds it from project root
mkdir -p "$PROJECT/.cf"
run "mkdir -p \$PROJECT/.cf && cf init --cf-home \$PROJECT/.cf  (human/owner)" \
    cf init --cf-home "$PROJECT/.cf"

run "cd \$PROJECT && rd init --name gate-demo  (human creates project)" \
    bash -c "cd '$PROJECT' && '$RD' init --name gate-demo"

CAMPFIRE_ID=$(cat "$PROJECT/.campfire/root")
echo "Project campfire ID: $CAMPFIRE_ID" | tee -a "$OUT"
echo "" | tee -a "$OUT"

# Owner generates agent invite token
AGENT_TOKEN=$(cd "$PROJECT" && "$RD" invite --role agent 2>&1 | grep '^rdx1_')
# (No pubkey lookup needed for gate/escalation demo)
run "cd \$PROJECT && rd invite --role agent  (human generates agent token)" \
    bash -c "echo 'rdx1_...  (agent invite token)'"

# Agent joins from the project root so .campfire/root and .ready/ are shared.
# Identity goes into $PROJECT/agent/.cf/ — walk-up finds it from $PROJECT/agent/.
mkdir -p "$PROJECT/agent/.cf"
run "mkdir -p \$PROJECT/agent/.cf && cd \$PROJECT && CF_HOME=\$PROJECT/agent/.cf rd join <agent-token>  (one-time identity bootstrap)" \
    bash -c "cd '$PROJECT' && CF_HOME='$PROJECT/agent/.cf' '$RD' join '$AGENT_TOKEN'"

# ── SECTION: agent-claims-work ────────────────────────────────────────────────
tee_section "=== SECTION: agent-claims-work ==="

echo "# Agent creates and claims a work item" | tee -a "$OUT"
echo "" | tee -a "$OUT"

ITEM_ID=$(cd "$PROJECT/agent" && "$RD" create \
    "Migrate auth layer to new token format" \
    --type task \
    --priority p1)
echo "$ cd \$PROJECT/agent && rd create 'Migrate auth layer...' --type task --priority p1" | tee -a "$OUT"
echo "Created item: $ITEM_ID" | tee -a "$OUT"
echo "" | tee -a "$OUT"

run "cd \$PROJECT/agent && rd update $ITEM_ID --status active  (agent claims item)" \
    bash -c "cd '$PROJECT/agent' && '$RD' update '$ITEM_ID' --status active"

run "cd \$PROJECT/agent && rd show $ITEM_ID  (confirm active)" \
    bash -c "cd '$PROJECT/agent' && '$RD' show '$ITEM_ID'"

# ── SECTION: agent-gates ──────────────────────────────────────────────────────
tee_section "=== SECTION: agent-gates ==="

echo "# Agent hits a decision point: two viable approaches, needs direction." | tee -a "$OUT"
echo "# Gate type 'design' signals an architectural decision is needed." | tee -a "$OUT"
echo "" | tee -a "$OUT"

GATE_JSON=$(cd "$PROJECT/agent" && "$RD" gate "$ITEM_ID" \
    --gate-type design \
    --description "Two viable approaches: option A saves 2ms but breaks caching, option B is safe. Need direction." \
    --json)
echo "$ cd \$PROJECT/agent && rd gate $ITEM_ID --gate-type design --description '...' --json" | tee -a "$OUT"
echo "$GATE_JSON" | tee -a "$OUT"
echo "" | tee -a "$OUT"

GATE_MSG_ID=$(echo "$GATE_JSON" | python3 -c "import sys,json; print(json.load(sys.stdin)['msg_id'])")
echo "Gate message ID: $GATE_MSG_ID" | tee -a "$OUT"
echo "" | tee -a "$OUT"

run "cd \$PROJECT/agent && rd show $ITEM_ID  (item is now waiting/gate)" \
    bash -c "cd '$PROJECT/agent' && '$RD' show '$ITEM_ID'"

# ── SECTION: human-sees-gate ──────────────────────────────────────────────────
tee_section "=== SECTION: human-sees-gate ==="

echo "# Human runs 'rd gates' to see pending escalations" | tee -a "$OUT"
echo "" | tee -a "$OUT"

run "cd \$PROJECT && rd gates  (human sees pending gates)" \
    bash -c "cd '$PROJECT' && '$RD' gates"

run "cd \$PROJECT && rd gates --json  (machine-readable)" \
    bash -c "cd '$PROJECT' && '$RD' gates --json"

# ── SECTION: human-approves ───────────────────────────────────────────────────
tee_section "=== SECTION: human-approves ==="

echo "# Human reviews the gate and approves — go with option B (safe approach)" | tee -a "$OUT"
echo "" | tee -a "$OUT"

run "cd \$PROJECT && rd approve $ITEM_ID --reason 'Use option B. Safety over 2ms gain.' --json" \
    bash -c "cd '$PROJECT' && '$RD' approve '$ITEM_ID' --reason 'Use option B. Safety over 2ms gain.' --json"

# ── SECTION: verify-approved ──────────────────────────────────────────────────
tee_section "=== SECTION: verify-approved ==="

echo "# Agent checks — item should be back to active" | tee -a "$OUT"
echo "" | tee -a "$OUT"

run "cd \$PROJECT/agent && rd show $ITEM_ID  (item is active again)" \
    bash -c "cd '$PROJECT/agent' && '$RD' show '$ITEM_ID'"

run "cd \$PROJECT/agent && rd gates --json  (no pending gates)" \
    bash -c "cd '$PROJECT/agent' && '$RD' gates --json"

# ── SECTION: reject-scenario ──────────────────────────────────────────────────
tee_section "=== SECTION: reject-scenario ==="

echo "# Second scenario: agent gates a new item, human rejects it." | tee -a "$OUT"
echo "# After rejection, item stays in waiting (gate unresolved until approved)." | tee -a "$OUT"
echo "" | tee -a "$OUT"

ITEM2_ID=$(cd "$PROJECT/agent" && "$RD" create \
    "Refactor payment processor" \
    --type task \
    --priority p2)
echo "$ cd \$PROJECT/agent && rd create 'Refactor payment processor' --type task --priority p2" | tee -a "$OUT"
echo "Created item: $ITEM2_ID" | tee -a "$OUT"
echo "" | tee -a "$OUT"

run "cd \$PROJECT/agent && rd update $ITEM2_ID --status active  (agent claims item)" \
    bash -c "cd '$PROJECT/agent' && '$RD' update '$ITEM2_ID' --status active"

run "cd \$PROJECT/agent && rd gate $ITEM2_ID --gate-type scope --description 'Scope too broad — touches 6 modules. Needs decomposition or explicit sign-off to proceed.' --json" \
    bash -c "cd '$PROJECT/agent' && '$RD' gate '$ITEM2_ID' \
        --gate-type scope \
        --description 'Scope too broad — touches 6 modules. Needs decomposition or explicit sign-off to proceed.' \
        --json"

run "cd \$PROJECT && rd gates  (human sees new gate)" \
    bash -c "cd '$PROJECT' && '$RD' gates"

run "cd \$PROJECT && rd reject $ITEM2_ID --reason 'Split into smaller items first. One module per item.' --json" \
    bash -c "cd '$PROJECT' && '$RD' reject '$ITEM2_ID' \
        --reason 'Split into smaller items first. One module per item.' \
        --json"

run "cd \$PROJECT/agent && rd show $ITEM2_ID  (item stays waiting after rejection — gate unresolved)" \
    bash -c "cd '$PROJECT/agent' && '$RD' show '$ITEM2_ID'"

run "cd \$PROJECT/agent && rd gates  (rejected item still in gates list)" \
    bash -c "cd '$PROJECT/agent' && '$RD' gates"

echo "══════════════════════════════════════════════════════════════" | tee -a "$OUT"
echo "  Demo complete. Transcript saved to: $OUT" | tee -a "$OUT"
echo "══════════════════════════════════════════════════════════════" | tee -a "$OUT"
