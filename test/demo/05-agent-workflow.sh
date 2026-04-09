#!/usr/bin/env bash
# 05-agent-workflow.sh — Agent/programmatic workflow demo
# An automated agent (CI bot, Claude session, automaton) that:
#   1. Joins a project campfire via invite token
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

PROJECT=$(mktemp -d /tmp/rdtest-agent-proj-XXXX)
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

echo "# Ready Agent Workflow Demo — $(date)" | tee -a "$OUT"
echo "# Demonstrates programmatic agent integration: JSON parsing, claim/progress/done cycle" | tee -a "$OUT"
echo "" | tee -a "$OUT"

echo "=== SECTION: setup ===" | tee -a "$OUT"
echo "" | tee -a "$OUT"

# Owner identity lives in $PROJECT/.cf/ — walk-up finds it from anywhere in $PROJECT
mkdir -p "$PROJECT/.cf"
run "mkdir -p \$PROJECT/.cf && cf init --cf-home \$PROJECT/.cf  (owner)" \
    cf init --cf-home "$PROJECT/.cf"

run "cd \$PROJECT && rd init --name ci-project" \
    bash -c "cd '$PROJECT' && '$RD' init --name ci-project"

CAMPFIRE_ID=$(cat "$PROJECT/.campfire/root")
echo "Project campfire ID: $CAMPFIRE_ID" | tee -a "$OUT"
echo "" | tee -a "$OUT"

# Owner generates agent invite token
echo "# Owner generates an invite token for the agent (agent role)" | tee -a "$OUT"
echo "" | tee -a "$OUT"

AGENT_TOKEN=$(cd "$PROJECT" && "$RD" invite --role agent 2>&1 | grep '^rdx1_')
run "cd \$PROJECT && rd invite --role agent  (owner generates agent token)" \
    bash -c "echo 'rdx1_...  (agent invite token)'"

# Agent joins from the project root so .campfire/root and .ready/ are shared.
# Identity goes into $PROJECT/agent/.cf/ — walk-up finds it from $PROJECT/agent/.
mkdir -p "$PROJECT/agent/.cf"
run "mkdir -p \$PROJECT/agent/.cf && cd \$PROJECT && CF_HOME=\$PROJECT/agent/.cf rd join <agent-token>  (one-time identity bootstrap)" \
    bash -c "cd '$PROJECT' && CF_HOME='$PROJECT/agent/.cf' '$RD' join '$AGENT_TOKEN'"

echo "=== SECTION: create-work ===" | tee -a "$OUT"
echo "" | tee -a "$OUT"

# Owner creates two items
echo "# Owner creates two work items" | tee -a "$OUT"
echo "" | tee -a "$OUT"

ITEM1_ID=$(cd "$PROJECT" && "$RD" create "Reindex search corpus" \
    --type task --priority p1)
echo "$ cd \$PROJECT && rd create \"Reindex search corpus\" --type task --priority p1" | tee -a "$OUT"
echo "Created: $ITEM1_ID" | tee -a "$OUT"
echo "" | tee -a "$OUT"

ITEM2_ID=$(cd "$PROJECT" && "$RD" create "Update dependency manifest" \
    --type task --priority p2)
echo "$ cd \$PROJECT && rd create \"Update dependency manifest\" --type task --priority p2" | tee -a "$OUT"
echo "Created: $ITEM2_ID" | tee -a "$OUT"
echo "" | tee -a "$OUT"

# Owner delegates one item to the agent
AGENT_PUBKEY=$(CF_HOME="$PROJECT/agent/.cf" cf id --json | python3 -c "import sys,json; print(json.load(sys.stdin)['public_key'])")
echo "# Owner delegates $ITEM1_ID to the agent" | tee -a "$OUT"
echo "" | tee -a "$OUT"

run "cd \$PROJECT && rd delegate $ITEM1_ID --to <agent-pubkey> --reason \"Route to indexer bot\"" \
    bash -c "cd '$PROJECT' && '$RD' delegate '$ITEM1_ID' \
        --to '$AGENT_PUBKEY' \
        --reason 'Route to indexer bot'"

echo "=== SECTION: agent-query ===" | tee -a "$OUT"
echo "" | tee -a "$OUT"

echo "# Agent uses --json to query its assigned work programmatically" | tee -a "$OUT"
echo "" | tee -a "$OUT"

run "cd \$PROJECT/agent && rd ready --view my-work --json" \
    bash -c "cd '$PROJECT/agent' && '$RD' ready --view my-work --json"

# Capture for parsing
MY_WORK_JSON=$(cd "$PROJECT/agent" && "$RD" ready --view my-work --json)

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

run "cd \$PROJECT/agent && rd claim $AGENT_ITEM_ID --reason \"Starting batch reindex job\"" \
    bash -c "cd '$PROJECT/agent' && '$RD' claim '$AGENT_ITEM_ID' \
        --reason 'Starting batch reindex job'"

echo "# Verify status is now active" | tee -a "$OUT"
run "cd \$PROJECT/agent && rd ready --view work --json" \
    bash -c "cd '$PROJECT/agent' && '$RD' ready --view work --json"

echo "=== SECTION: agent-progress ===" | tee -a "$OUT"
echo "" | tee -a "$OUT"

echo "# Agent posts incremental progress notes as work proceeds" | tee -a "$OUT"
echo "" | tee -a "$OUT"

run "cd \$PROJECT/agent && rd progress $AGENT_ITEM_ID --notes \"Processed 47/142 records, 0 errors\"" \
    bash -c "cd '$PROJECT/agent' && '$RD' progress '$AGENT_ITEM_ID' \
        --notes 'Processed 47/142 records, 0 errors'"

run "cd \$PROJECT/agent && rd progress $AGENT_ITEM_ID --notes \"Processed 142/142 records, 0 errors — indexing complete\"" \
    bash -c "cd '$PROJECT/agent' && '$RD' progress '$AGENT_ITEM_ID' \
        --notes 'Processed 142/142 records, 0 errors — indexing complete'"

echo "=== SECTION: agent-done ===" | tee -a "$OUT"
echo "" | tee -a "$OUT"

echo "# Agent closes the item with a structured result" | tee -a "$OUT"
echo "" | tee -a "$OUT"

run "cd \$PROJECT/agent && rd done $AGENT_ITEM_ID --reason \"Batch complete: 142 records processed, 0 errors\"" \
    bash -c "cd '$PROJECT/agent' && '$RD' done '$AGENT_ITEM_ID' \
        --reason 'Batch complete: 142 records processed, 0 errors'"

echo "=== SECTION: verify ===" | tee -a "$OUT"
echo "" | tee -a "$OUT"

echo "# Owner queries all items as JSON to confirm completion" | tee -a "$OUT"
echo "" | tee -a "$OUT"

run "cd \$PROJECT && rd list --all --json" \
    bash -c "cd '$PROJECT' && '$RD' list --all --json"

# Show human-readable summary too
run "cd \$PROJECT && rd list --all" \
    bash -c "cd '$PROJECT' && '$RD' list --all"

echo "══════════════════════════════════════════════════════════════" | tee -a "$OUT"
echo "  Demo complete. Transcript saved to: $OUT" | tee -a "$OUT"
echo "══════════════════════════════════════════════════════════════" | tee -a "$OUT"
