#!/usr/bin/env bash
# 05-agent-workflow.sh — Agent/programmatic workflow demo
# An automated agent (CI bot, Claude session, automaton) that:
#   1. Joins a project campfire programmatically
#   2. Queries the ready queue for work assigned to it
#   3. Claims an item
#   4. Posts progress notes
#   5. Closes the item with a result
#
# Key differentiator from human solo workflow: uses --json throughout for
# programmatic parsing, demonstrates the claim/progress/done cycle as a
# script would run it.
set -euo pipefail

RD=/tmp/rd-demo
OUT_DIR="$(cd "$(dirname "$0")" && pwd)/output"
OUT="$OUT_DIR/05-agent-workflow.txt"
mkdir -p "$OUT_DIR"

AGENT_CF=$(mktemp -d /tmp/rdtest-agent-XXXX)
OWNER_CF=$(mktemp -d /tmp/rdtest-agent-owner-XXXX)
PROJECT=$(mktemp -d /tmp/rdtest-agent-proj-XXXX)
trap "rm -rf $AGENT_CF $OWNER_CF $PROJECT" EXIT
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

echo "# Ready Agent Workflow Demo — $(date)" | tee -a "$OUT"
echo "# Demonstrates programmatic agent integration: JSON parsing, claim/progress/done cycle" | tee -a "$OUT"
echo "" | tee -a "$OUT"

echo "=== SECTION: setup ===" | tee -a "$OUT"
echo "" | tee -a "$OUT"

# Initialize owner and agent identities
run "cf init --cf-home \$OWNER_CF  (owner)" \
    cf init --cf-home "$OWNER_CF"

run "cf init --cf-home \$AGENT_CF  (agent)" \
    cf init --cf-home "$AGENT_CF"

# Owner creates project
run "CF_HOME=\$OWNER_CF rd init --name ci-project --confirm  (in \$PROJECT)" \
    bash -c "cd '$PROJECT' && CF_HOME='$OWNER_CF' '$RD' init --name ci-project --confirm"

CAMPFIRE_ID=$(cat "$PROJECT/.campfire/root")
echo "Project campfire ID: $CAMPFIRE_ID" | tee -a "$OUT"
echo "" | tee -a "$OUT"

# Get agent pubkey
AGENT_PUBKEY=$(CF_HOME="$AGENT_CF" cf id --json | python3 -c "import sys,json; print(json.load(sys.stdin)['public_key'])")
run "CF_HOME=\$AGENT_CF cf id --json  (agent identity)" \
    bash -c "CF_HOME='$AGENT_CF' cf id --json"
echo "Agent pubkey: $AGENT_PUBKEY" | tee -a "$OUT"
echo "" | tee -a "$OUT"

# Owner admits agent
run "CF_HOME=\$OWNER_CF rd admit <agent-pubkey>" \
    bash -c "cd '$PROJECT' && CF_HOME='$OWNER_CF' '$RD' admit '$AGENT_PUBKEY'"

# Agent joins the project campfire
run "CF_HOME=\$AGENT_CF rd join <campfire-id>" \
    bash -c "cd '$PROJECT' && CF_HOME='$AGENT_CF' '$RD' join '$CAMPFIRE_ID'"

echo "=== SECTION: create-work ===" | tee -a "$OUT"
echo "" | tee -a "$OUT"

# Owner creates two items
echo "# Owner creates two work items" | tee -a "$OUT"
echo "" | tee -a "$OUT"

ITEM1_JSON=$(cd "$PROJECT" && CF_HOME="$OWNER_CF" "$RD" create "Reindex search corpus" \
    --type task --priority p1 --json)
echo "$ CF_HOME=\$OWNER_CF rd create \"Reindex search corpus\" --type task --priority p1 --json" | tee -a "$OUT"
echo "$ITEM1_JSON" | tee -a "$OUT"
echo "" | tee -a "$OUT"

ITEM2_JSON=$(cd "$PROJECT" && CF_HOME="$OWNER_CF" "$RD" create "Update dependency manifest" \
    --type task --priority p2 --json)
echo "$ CF_HOME=\$OWNER_CF rd create \"Update dependency manifest\" --type task --priority p2 --json" | tee -a "$OUT"
echo "$ITEM2_JSON" | tee -a "$OUT"
echo "" | tee -a "$OUT"

ITEM1_ID=$(echo "$ITEM1_JSON" | python3 -c "import sys,json; print(json.load(sys.stdin)['id'])")
ITEM2_ID=$(echo "$ITEM2_JSON" | python3 -c "import sys,json; print(json.load(sys.stdin)['id'])")
echo "# Created items: $ITEM1_ID, $ITEM2_ID" | tee -a "$OUT"
echo "" | tee -a "$OUT"

# Owner delegates one item to the agent
echo "# Owner delegates $ITEM1_ID to the agent" | tee -a "$OUT"
echo "" | tee -a "$OUT"

run "CF_HOME=\$OWNER_CF rd delegate $ITEM1_ID --to <agent-pubkey> --reason \"Route to indexer bot\"" \
    bash -c "cd '$PROJECT' && CF_HOME='$OWNER_CF' '$RD' delegate '$ITEM1_ID' \
        --to '$AGENT_PUBKEY' \
        --reason 'Route to indexer bot'"

echo "=== SECTION: agent-query ===" | tee -a "$OUT"
echo "" | tee -a "$OUT"

echo "# Agent uses --json to query its assigned work programmatically" | tee -a "$OUT"
echo "" | tee -a "$OUT"

run "CF_HOME=\$AGENT_CF rd ready --view my-work --json" \
    bash -c "cd '$PROJECT' && CF_HOME='$AGENT_CF' '$RD' ready --view my-work --json"

# Capture for parsing
MY_WORK_JSON=$(cd "$PROJECT" && CF_HOME="$AGENT_CF" "$RD" ready --view my-work --json)

echo "# Parse item ID from JSON output" | tee -a "$OUT"
AGENT_ITEM_ID=$(echo "$MY_WORK_JSON" | python3 -c "
import sys, json
items = json.load(sys.stdin)
if isinstance(items, list) and items:
    print(items[0]['id'])
elif isinstance(items, dict):
    print(items.get('id', ''))
")
echo "# Agent's assigned item: $AGENT_ITEM_ID" | tee -a "$OUT"
echo "" | tee -a "$OUT"

echo "=== SECTION: agent-claim ===" | tee -a "$OUT"
echo "" | tee -a "$OUT"

echo "# Agent claims the item — accepts delegation, transitions to active" | tee -a "$OUT"
echo "" | tee -a "$OUT"

run "CF_HOME=\$AGENT_CF rd claim $AGENT_ITEM_ID --reason \"Starting batch reindex job\"" \
    bash -c "cd '$PROJECT' && CF_HOME='$AGENT_CF' '$RD' claim '$AGENT_ITEM_ID' \
        --reason 'Starting batch reindex job'"

echo "# Verify status is now active" | tee -a "$OUT"
run "CF_HOME=\$AGENT_CF rd ready --view work --json" \
    bash -c "cd '$PROJECT' && CF_HOME='$AGENT_CF' '$RD' ready --view work --json"

echo "=== SECTION: agent-progress ===" | tee -a "$OUT"
echo "" | tee -a "$OUT"

echo "# Agent posts incremental progress notes as work proceeds" | tee -a "$OUT"
echo "" | tee -a "$OUT"

run "CF_HOME=\$AGENT_CF rd progress $AGENT_ITEM_ID --notes \"Processed 47/142 records, 0 errors\"" \
    bash -c "cd '$PROJECT' && CF_HOME='$AGENT_CF' '$RD' progress '$AGENT_ITEM_ID' \
        --notes 'Processed 47/142 records, 0 errors'"

run "CF_HOME=\$AGENT_CF rd progress $AGENT_ITEM_ID --notes \"Processed 142/142 records, 0 errors — indexing complete\"" \
    bash -c "cd '$PROJECT' && CF_HOME='$AGENT_CF' '$RD' progress '$AGENT_ITEM_ID' \
        --notes 'Processed 142/142 records, 0 errors — indexing complete'"

echo "=== SECTION: agent-done ===" | tee -a "$OUT"
echo "" | tee -a "$OUT"

echo "# Agent closes the item with a structured result" | tee -a "$OUT"
echo "" | tee -a "$OUT"

run "CF_HOME=\$AGENT_CF rd done $AGENT_ITEM_ID --reason \"Batch complete: 142 records processed, 0 errors\"" \
    bash -c "cd '$PROJECT' && CF_HOME='$AGENT_CF' '$RD' done '$AGENT_ITEM_ID' \
        --reason 'Batch complete: 142 records processed, 0 errors'"

echo "=== SECTION: verify ===" | tee -a "$OUT"
echo "" | tee -a "$OUT"

echo "# Owner queries all items as JSON to confirm completion" | tee -a "$OUT"
echo "" | tee -a "$OUT"

run "CF_HOME=\$OWNER_CF rd list --all --json" \
    bash -c "cd '$PROJECT' && CF_HOME='$OWNER_CF' '$RD' list --all --json"

# Show human-readable summary too
run "CF_HOME=\$OWNER_CF rd list --all" \
    bash -c "cd '$PROJECT' && CF_HOME='$OWNER_CF' '$RD' list --all"

echo "══════════════════════════════════════════════════════════════" | tee -a "$OUT"
echo "  Demo complete. Transcript saved to: $OUT" | tee -a "$OUT"
echo "══════════════════════════════════════════════════════════════" | tee -a "$OUT"
