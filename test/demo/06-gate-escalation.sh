#!/usr/bin/env bash
# Demo: gate/escalation workflow
# An agent hits a decision point and gates an item for human review.
# The human approves or rejects. Item status updates accordingly.
set -euo pipefail

RD=/tmp/rd-demo
OUT_DIR="$(cd "$(dirname "$0")" && pwd)/output"
OUT="$OUT_DIR/06-gate-escalation.txt"
mkdir -p "$OUT_DIR"

AGENT_CF=$(mktemp -d /tmp/rdtest-gate-agent-XXXX)
HUMAN_CF=$(mktemp -d /tmp/rdtest-gate-human-XXXX)
PROJECT=$(mktemp -d /tmp/rdtest-gate-proj-XXXX)
trap "rm -rf $AGENT_CF $HUMAN_CF $PROJECT" EXIT

export PATH="/tmp/go/bin:$PATH"

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

echo "# Initializing two identities: human (project owner) and agent" | tee -a "$OUT"
echo "" | tee -a "$OUT"

run "cf init --cf-home \$HUMAN_CF  (human/owner)" \
    cf init --cf-home "$HUMAN_CF"

run "cf init --cf-home \$AGENT_CF  (agent)" \
    cf init --cf-home "$AGENT_CF"

run "CF_HOME=\$HUMAN_CF rd init --name gate-demo --confirm  (human creates project)" \
    bash -c "cd '$PROJECT' && CF_HOME='$HUMAN_CF' '$RD' init --name gate-demo --confirm"

CAMPFIRE_ID=$(cat "$PROJECT/.campfire/root")
echo "Project campfire ID: $CAMPFIRE_ID" | tee -a "$OUT"
echo "" | tee -a "$OUT"

# Get agent public key from identity.json — base64 string, convert to hex
AGENT_PUBKEY=$(python3 -c "
import json, base64
with open('$AGENT_CF/identity.json') as f:
    d = json.load(f)
pk = d['public_key']
if isinstance(pk, list):
    print(bytes(pk).hex())
elif isinstance(pk, str):
    # base64-encoded bytes
    print(base64.b64decode(pk).hex())
")
echo "Agent public key: $AGENT_PUBKEY" | tee -a "$OUT"
echo "" | tee -a "$OUT"

run "CF_HOME=\$HUMAN_CF rd admit <agent-pubkey>  (human pre-admits agent)" \
    bash -c "cd '$PROJECT' && CF_HOME='$HUMAN_CF' '$RD' admit '$AGENT_PUBKEY'"

run "CF_HOME=\$AGENT_CF rd join <campfire-id>  (agent joins project)" \
    bash -c "cd '$PROJECT' && CF_HOME='$AGENT_CF' '$RD' join '$CAMPFIRE_ID'"

# ── SECTION: agent-claims-work ────────────────────────────────────────────────
tee_section "=== SECTION: agent-claims-work ==="

echo "# Agent creates and claims a work item" | tee -a "$OUT"
echo "" | tee -a "$OUT"

ITEM_JSON=$(cd "$PROJECT" && CF_HOME="$AGENT_CF" "$RD" create \
    --title "Migrate auth layer to new token format" \
    --type task \
    --priority p1 \
    --json)
echo "$ CF_HOME=\$AGENT_CF rd create --title 'Migrate auth layer...' --type task --priority p1 --json" | tee -a "$OUT"
echo "$ITEM_JSON" | tee -a "$OUT"
echo "" | tee -a "$OUT"

ITEM_ID=$(echo "$ITEM_JSON" | python3 -c "import sys,json; print(json.load(sys.stdin)['id'])")
echo "Created item: $ITEM_ID" | tee -a "$OUT"
echo "" | tee -a "$OUT"

run "CF_HOME=\$AGENT_CF rd update $ITEM_ID --status active  (agent claims item)" \
    bash -c "cd '$PROJECT' && CF_HOME='$AGENT_CF' '$RD' update '$ITEM_ID' --status active"

run "CF_HOME=\$AGENT_CF rd show $ITEM_ID  (confirm active)" \
    bash -c "cd '$PROJECT' && CF_HOME='$AGENT_CF' '$RD' show '$ITEM_ID'"

# ── SECTION: agent-gates ──────────────────────────────────────────────────────
tee_section "=== SECTION: agent-gates ==="

echo "# Agent hits a decision point: two viable approaches, needs direction." | tee -a "$OUT"
echo "# Gate type 'design' signals an architectural decision is needed." | tee -a "$OUT"
echo "" | tee -a "$OUT"

GATE_JSON=$(cd "$PROJECT" && CF_HOME="$AGENT_CF" "$RD" gate "$ITEM_ID" \
    --gate-type design \
    --description "Two viable approaches: option A saves 2ms but breaks caching, option B is safe. Need direction." \
    --json)
echo "$ CF_HOME=\$AGENT_CF rd gate $ITEM_ID --gate-type design --description '...' --json" | tee -a "$OUT"
echo "$GATE_JSON" | tee -a "$OUT"
echo "" | tee -a "$OUT"

GATE_MSG_ID=$(echo "$GATE_JSON" | python3 -c "import sys,json; print(json.load(sys.stdin)['msg_id'])")
echo "Gate message ID: $GATE_MSG_ID" | tee -a "$OUT"
echo "" | tee -a "$OUT"

run "CF_HOME=\$AGENT_CF rd show $ITEM_ID  (item is now waiting/gate)" \
    bash -c "cd '$PROJECT' && CF_HOME='$AGENT_CF' '$RD' show '$ITEM_ID'"

# ── SECTION: human-sees-gate ──────────────────────────────────────────────────
tee_section "=== SECTION: human-sees-gate ==="

echo "# Human runs 'rd gates' to see pending escalations" | tee -a "$OUT"
echo "" | tee -a "$OUT"

run "CF_HOME=\$HUMAN_CF rd gates  (human sees pending gates)" \
    bash -c "cd '$PROJECT' && CF_HOME='$HUMAN_CF' '$RD' gates"

run "CF_HOME=\$HUMAN_CF rd gates --json  (machine-readable)" \
    bash -c "cd '$PROJECT' && CF_HOME='$HUMAN_CF' '$RD' gates --json"

# ── SECTION: human-approves ───────────────────────────────────────────────────
tee_section "=== SECTION: human-approves ==="

echo "# Human reviews the gate and approves — go with option B (safe approach)" | tee -a "$OUT"
echo "" | tee -a "$OUT"

run "CF_HOME=\$HUMAN_CF rd approve $ITEM_ID --reason 'Use option B. Safety over 2ms gain.' --json" \
    bash -c "cd '$PROJECT' && CF_HOME='$HUMAN_CF' '$RD' approve '$ITEM_ID' --reason 'Use option B. Safety over 2ms gain.' --json"

# ── SECTION: verify-approved ──────────────────────────────────────────────────
tee_section "=== SECTION: verify-approved ==="

echo "# Agent checks — item should be back to active" | tee -a "$OUT"
echo "" | tee -a "$OUT"

run "CF_HOME=\$AGENT_CF rd show $ITEM_ID  (item is active again)" \
    bash -c "cd '$PROJECT' && CF_HOME='$AGENT_CF' '$RD' show '$ITEM_ID'"

run "CF_HOME=\$AGENT_CF rd gates --json  (no pending gates)" \
    bash -c "cd '$PROJECT' && CF_HOME='$AGENT_CF' '$RD' gates --json"

# ── SECTION: reject-scenario ──────────────────────────────────────────────────
tee_section "=== SECTION: reject-scenario ==="

echo "# Second scenario: agent gates a new item, human rejects it." | tee -a "$OUT"
echo "# After rejection, item stays in waiting (gate unresolved until approved)." | tee -a "$OUT"
echo "" | tee -a "$OUT"

ITEM2_JSON=$(cd "$PROJECT" && CF_HOME="$AGENT_CF" "$RD" create \
    --title "Refactor payment processor" \
    --type task \
    --priority p2 \
    --json)
echo "$ CF_HOME=\$AGENT_CF rd create --title 'Refactor payment processor' --type task --priority p2 --json" | tee -a "$OUT"
echo "$ITEM2_JSON" | tee -a "$OUT"
echo "" | tee -a "$OUT"

ITEM2_ID=$(echo "$ITEM2_JSON" | python3 -c "import sys,json; print(json.load(sys.stdin)['id'])")
echo "Created item: $ITEM2_ID" | tee -a "$OUT"
echo "" | tee -a "$OUT"

run "CF_HOME=\$AGENT_CF rd update $ITEM2_ID --status active  (agent claims item)" \
    bash -c "cd '$PROJECT' && CF_HOME='$AGENT_CF' '$RD' update '$ITEM2_ID' --status active"

run "CF_HOME=\$AGENT_CF rd gate $ITEM2_ID --gate-type scope --description 'Scope too broad — touches 6 modules. Needs decomposition or explicit sign-off to proceed.' --json" \
    bash -c "cd '$PROJECT' && CF_HOME='$AGENT_CF' '$RD' gate '$ITEM2_ID' \
        --gate-type scope \
        --description 'Scope too broad — touches 6 modules. Needs decomposition or explicit sign-off to proceed.' \
        --json"

run "CF_HOME=\$HUMAN_CF rd gates  (human sees new gate)" \
    bash -c "cd '$PROJECT' && CF_HOME='$HUMAN_CF' '$RD' gates"

run "CF_HOME=\$HUMAN_CF rd reject $ITEM2_ID --reason 'Split into smaller items first. One module per item.' --json" \
    bash -c "cd '$PROJECT' && CF_HOME='$HUMAN_CF' '$RD' reject '$ITEM2_ID' \
        --reason 'Split into smaller items first. One module per item.' \
        --json"

run "CF_HOME=\$AGENT_CF rd show $ITEM2_ID  (item stays waiting after rejection — gate unresolved)" \
    bash -c "cd '$PROJECT' && CF_HOME='$AGENT_CF' '$RD' show '$ITEM2_ID'"

run "CF_HOME=\$AGENT_CF rd gates  (rejected item still in gates list)" \
    bash -c "cd '$PROJECT' && CF_HOME='$AGENT_CF' '$RD' gates"

echo "══════════════════════════════════════════════════════════════" | tee -a "$OUT"
echo "  Demo complete. Transcript saved to: $OUT" | tee -a "$OUT"
echo "══════════════════════════════════════════════════════════════" | tee -a "$OUT"
