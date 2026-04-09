#!/usr/bin/env bash
# 11-filesystem-isolation.sh — Filesystem hierarchy as isolation primitive
#
# Demonstrates walk-up config resolution: agents in sibling worktrees get isolated
# identities automatically, while sharing the same project campfire via .campfire/root
# walk-up. No CF_HOME env vars needed after identity setup.
#
# Three scenarios:
#   1. Inheritance  — subdirectory inherits project campfire from parent
#   2. Isolation    — sibling worktrees get different identities, same campfire
#   3. Coordination — two agents claim disjoint work; owner sees all claims
set -euo pipefail

RD=/tmp/rd-demo
OUT_DIR="$(cd "$(dirname "$0")" && pwd)/output"
OUT="$OUT_DIR/11-filesystem-isolation.txt"
mkdir -p "$OUT_DIR"

# Shared temp dirs — all cleaned up on exit
OWNER_CF=$(mktemp -d /tmp/rdtest-iso-owner-XXXX)
PROJECT=$(mktemp -d /tmp/rdtest-iso-proj-XXXX)
trap "rm -rf $OWNER_CF $PROJECT" EXIT

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

# Parse JSON item list and show human-readable table
show_items() {
    # accepts JSON on stdin, prints id+status+title
    python3 -c "
import sys, json
items = json.load(sys.stdin)
if not isinstance(items, list):
    items = [items]
for i in items:
    print(f'  {i[\"id\"]:20s}  [{i[\"status\"]:8s}]  {i[\"title\"]}')
"
}

assert_contains() {
    local text="$1" pattern="$2" desc="$3"
    if echo "$text" | grep -q "$pattern"; then
        echo "✓ assert: $desc" | tee -a "$OUT"
    else
        echo "✗ FAIL: $desc" | tee -a "$OUT"
        echo "  expected pattern: $pattern" | tee -a "$OUT"
        echo "  actual output:    $text" | tee -a "$OUT"
        exit 1
    fi
    echo "" | tee -a "$OUT"
}

# Start fresh output
> "$OUT"

echo "# Ready Filesystem Isolation Demo — $(date)" | tee -a "$OUT"
echo "# Walk-up config: .cf/identity.json anchors identity; .campfire/root anchors project" | tee -a "$OUT"
echo "" | tee -a "$OUT"

# ════════════════════════════════════════════════════════════════════════════════
tee_section "Setup: Owner initializes project"
# ════════════════════════════════════════════════════════════════════════════════

# Owner sets up their identity in OWNER_CF
run "cf init --cf-home \$OWNER_CF" \
    cf init --cf-home "$OWNER_CF"

# Owner creates the project. This writes .campfire/root and .ready/ in $PROJECT.
run "CF_HOME=\$OWNER_CF rd init --name iso-project  (in \$PROJECT)" \
    bash -c "cd '$PROJECT' && CF_HOME='$OWNER_CF' '$RD' init --name iso-project"

CAMPFIRE_ID=$(cat "$PROJECT/.campfire/root")
echo "# Project campfire ID: $CAMPFIRE_ID" | tee -a "$OUT"
echo "# Stored at: \$PROJECT/.campfire/root" | tee -a "$OUT"
echo "" | tee -a "$OUT"

# ════════════════════════════════════════════════════════════════════════════════
tee_section "Scenario 1: Inheritance — subdirectory sees project campfire"
# ════════════════════════════════════════════════════════════════════════════════
#
# A subdirectory of the project has no .cf/ of its own. Walk-up finds the
# project's .campfire/root. rd commands just work — no flags, no env vars.
# This is the common case: the owner's shell is inside the project tree.

echo "# Directory layout:" | tee -a "$OUT"
echo "#   \$PROJECT/                ← rd init wrote .campfire/root here" | tee -a "$OUT"
echo "#   \$PROJECT/src/            ← no .cf/ here; walk-up finds parent's .campfire/root" | tee -a "$OUT"
echo "" | tee -a "$OUT"

mkdir -p "$PROJECT/src"

# Create a work item from the project root (CF_HOME needed — owner identity lives there)
ITEM_A=$(cd "$PROJECT" && CF_HOME="$OWNER_CF" "$RD" create "Implement login" --type task --priority p1)
echo "$ CF_HOME=\$OWNER_CF rd create 'Implement login' --type task --priority p1" | tee -a "$OUT"
echo "Created: $ITEM_A" | tee -a "$OUT"
echo "" | tee -a "$OUT"

# From src/ subdirectory — owner still needs CF_HOME for their identity,
# but the project campfire resolves automatically via .campfire/root walk-up
echo "# rd ready from subdirectory — campfire resolves via walk-up to parent's .campfire/root" | tee -a "$OUT"
READY_IDS=$(cd "$PROJECT/src" && CF_HOME="$OWNER_CF" "$RD" ready 2>&1)
READY_JSON=$(cd "$PROJECT/src" && CF_HOME="$OWNER_CF" "$RD" ready --json 2>&1)
echo "$ (from \$PROJECT/src) CF_HOME=\$OWNER_CF rd ready" | tee -a "$OUT"
echo "$READY_IDS" | tee -a "$OUT"
echo "" | tee -a "$OUT"

echo "# Human-readable (via --json):" | tee -a "$OUT"
echo "$READY_JSON" | show_items | tee -a "$OUT"
echo "" | tee -a "$OUT"

assert_contains "$READY_JSON" "Implement login" \
    "item visible from subdirectory via .campfire/root walk-up"

# ════════════════════════════════════════════════════════════════════════════════
tee_section "Scenario 2: Isolation — sibling worktrees, different identities"
# ════════════════════════════════════════════════════════════════════════════════
#
# Two agent worktrees share the same project (same .campfire/root above) but each
# has its own .cf/identity.json. CFHome() walk-up finds the local .cf/ first,
# giving each agent its own identity without any CF_HOME env var.
#
# Key: pre-create .cf/ in each worktree BEFORE running rd join, so walk-up
# anchors the identity write to the right directory.

echo "# Directory layout:" | tee -a "$OUT"
echo "#   \$PROJECT/                ← .campfire/root (project anchor)" | tee -a "$OUT"
echo "#   \$PROJECT/worktree-a/.cf/ ← agent-A identity (walk-up stops here)" | tee -a "$OUT"
echo "#   \$PROJECT/worktree-b/.cf/ ← agent-B identity (walk-up stops here)" | tee -a "$OUT"
echo "" | tee -a "$OUT"

# Pre-create .cf/ dirs so walk-up anchors the identity writes here.
# Without this, walk-up would traverse up past the worktrees and write
# into the project root's .cf/ (or ~/.cf), losing per-worktree isolation.
mkdir -p "$PROJECT/worktree-a/.cf"
mkdir -p "$PROJECT/worktree-b/.cf"

# Owner generates invite tokens for each agent
echo "# Owner generates two invite tokens" | tee -a "$OUT"
TOKEN_A=$(cd "$PROJECT" && CF_HOME="$OWNER_CF" "$RD" invite --cf-home "$OWNER_CF" 2>&1 | grep '^rdx1_')
TOKEN_B=$(cd "$PROJECT" && CF_HOME="$OWNER_CF" "$RD" invite --cf-home "$OWNER_CF" 2>&1 | grep '^rdx1_')
echo "$ CF_HOME=\$OWNER_CF rd invite  →  rdx1_...  (token for agent-A)" | tee -a "$OUT"
echo "$ CF_HOME=\$OWNER_CF rd invite  →  rdx1_...  (token for agent-B)" | tee -a "$OUT"
echo "" | tee -a "$OUT"

# Agent A joins from worktree-a/
# Walk-up requires identity.json to exist before it stops at a .cf/ dir.
# So we pass CF_HOME once for the join command to write the identity,
# then all subsequent commands use walk-up automatically.
echo "# Agent A: CF_HOME=worktree-a/.cf rd join <token>  (one-time identity bootstrap)" | tee -a "$OUT"
run "cd \$PROJECT/worktree-a && CF_HOME=\$PROJECT/worktree-a/.cf rd join <token-a>" \
    bash -c "cd '$PROJECT/worktree-a' && CF_HOME='$PROJECT/worktree-a/.cf' '$RD' join '$TOKEN_A'"

# Agent B joins from worktree-b/ — same one-time CF_HOME for the join
echo "# Agent B: CF_HOME=worktree-b/.cf rd join <token>  (one-time identity bootstrap)" | tee -a "$OUT"
run "cd \$PROJECT/worktree-b && CF_HOME=\$PROJECT/worktree-b/.cf rd join <token-b>" \
    bash -c "cd '$PROJECT/worktree-b' && CF_HOME='$PROJECT/worktree-b/.cf' '$RD' join '$TOKEN_B'"

# Verify each worktree has its own identity.json
echo "# Each worktree now has its own identity — no CF_HOME needed:" | tee -a "$OUT"
run "ls \$PROJECT/worktree-a/.cf/" \
    ls "$PROJECT/worktree-a/.cf/"
run "ls \$PROJECT/worktree-b/.cf/" \
    ls "$PROJECT/worktree-b/.cf/"

# Show identities are different — cf id reads from the walked-up CF_HOME
PUBKEY_A=$(CF_HOME="$PROJECT/worktree-a/.cf" cf id --json 2>/dev/null | python3 -c "import sys,json; print(json.load(sys.stdin)['public_key'])")
PUBKEY_B=$(CF_HOME="$PROJECT/worktree-b/.cf" cf id --json 2>/dev/null | python3 -c "import sys,json; print(json.load(sys.stdin)['public_key'])")
echo "$ (worktree-a identity) cf id --json | .public_key" | tee -a "$OUT"
echo "${PUBKEY_A:0:16}...  ← agent-A" | tee -a "$OUT"
echo "" | tee -a "$OUT"
echo "$ (worktree-b identity) cf id --json | .public_key" | tee -a "$OUT"
echo "${PUBKEY_B:0:16}...  ← agent-B" | tee -a "$OUT"
echo "" | tee -a "$OUT"

if [ "$PUBKEY_A" != "$PUBKEY_B" ]; then
    echo "✓ assert: agent-A and agent-B have different public keys" | tee -a "$OUT"
else
    echo "✗ FAIL: identities are the same — isolation broken" | tee -a "$OUT"
    exit 1
fi
echo "" | tee -a "$OUT"

# Both agents can see all project items — campfire resolves via .campfire/root walk-up.
# rd list shows all items regardless of assignment; rd ready shows items assigned to you.
echo "# Both agents see all project items via walk-up to parent's .campfire/root:" | tee -a "$OUT"
LIST_JSON_A=$(cd "$PROJECT/worktree-a" && "$RD" list --json 2>&1)
LIST_JSON_B=$(cd "$PROJECT/worktree-b" && "$RD" list --json 2>&1)

echo "$ (from worktree-a) rd list --json  # no CF_HOME" | tee -a "$OUT"
echo "$LIST_JSON_A" | show_items | tee -a "$OUT"
echo "" | tee -a "$OUT"

echo "$ (from worktree-b) rd list --json  # no CF_HOME" | tee -a "$OUT"
echo "$LIST_JSON_B" | show_items | tee -a "$OUT"
echo "" | tee -a "$OUT"

assert_contains "$LIST_JSON_A" "Implement login" \
    "agent-A sees project items from worktree-a/ (no CF_HOME)"
assert_contains "$LIST_JSON_B" "Implement login" \
    "agent-B sees project items from worktree-b/ (no CF_HOME)"

# ════════════════════════════════════════════════════════════════════════════════
tee_section "Scenario 3: Coordination — two agents claim disjoint work"
# ════════════════════════════════════════════════════════════════════════════════
#
# Five items. Two agents each claim different items from their isolated worktrees.
# The filesystem IS the orchestrator's config: no CF_HOME, no env vars, no flags.
# The directory you're in determines who you are.

echo "# Owner creates five work items" | tee -a "$OUT"
ITEM_1=$(cd "$PROJECT" && CF_HOME="$OWNER_CF" "$RD" create "Set up database schema" --type task --priority p1)
ITEM_2=$(cd "$PROJECT" && CF_HOME="$OWNER_CF" "$RD" create "Build REST endpoints" --type task --priority p1)
ITEM_3=$(cd "$PROJECT" && CF_HOME="$OWNER_CF" "$RD" create "Write integration tests" --type task --priority p2)
ITEM_4=$(cd "$PROJECT" && CF_HOME="$OWNER_CF" "$RD" create "Add rate limiting" --type task --priority p2)
ITEM_5=$(cd "$PROJECT" && CF_HOME="$OWNER_CF" "$RD" create "Deploy to staging" --type task --priority p3)
for item in "$ITEM_1" "$ITEM_2" "$ITEM_3" "$ITEM_4" "$ITEM_5"; do
    echo "Created: $item" | tee -a "$OUT"
done
echo "" | tee -a "$OUT"

# Show all items from the owner's view
echo "$ CF_HOME=\$OWNER_CF rd list --json  (all items)" | tee -a "$OUT"
cd "$PROJECT" && CF_HOME="$OWNER_CF" "$RD" list --json 2>&1 | show_items | tee -a "$OUT"
echo "" | tee -a "$OUT"

# Agents sync to pick up new items created since they joined
echo "# Agents sync to pick up new items" | tee -a "$OUT"
run "cd \$PROJECT/worktree-a && rd sync pull  # no CF_HOME" \
    bash -c "cd '$PROJECT/worktree-a' && '$RD' sync pull"
run "cd \$PROJECT/worktree-b && rd sync pull  # no CF_HOME" \
    bash -c "cd '$PROJECT/worktree-b' && '$RD' sync pull"

# Agent A claims items 1 and 3 — just cd, no CF_HOME
echo "# Agent A claims two items — identity from worktree-a/.cf/ (walk-up), no CF_HOME" | tee -a "$OUT"
run "cd \$PROJECT/worktree-a && rd claim $ITEM_1  # no CF_HOME" \
    bash -c "cd '$PROJECT/worktree-a' && '$RD' claim '$ITEM_1'"
run "cd \$PROJECT/worktree-a && rd claim $ITEM_3  # no CF_HOME" \
    bash -c "cd '$PROJECT/worktree-a' && '$RD' claim '$ITEM_3'"

# Agent B claims items 2 and 4 — just cd, no CF_HOME
echo "# Agent B claims two items — identity from worktree-b/.cf/ (walk-up), no CF_HOME" | tee -a "$OUT"
run "cd \$PROJECT/worktree-b && rd claim $ITEM_2  # no CF_HOME" \
    bash -c "cd '$PROJECT/worktree-b' && '$RD' claim '$ITEM_2'"
run "cd \$PROJECT/worktree-b && rd claim $ITEM_4  # no CF_HOME" \
    bash -c "cd '$PROJECT/worktree-b' && '$RD' claim '$ITEM_4'"

# Agent A posts progress on its items from worktree-a/ — no CF_HOME
run "cd \$PROJECT/worktree-a && rd progress $ITEM_1 --notes 'Schema migrations written'  # no CF_HOME" \
    bash -c "cd '$PROJECT/worktree-a' && '$RD' progress '$ITEM_1' --notes 'Schema migrations written'"
run "cd \$PROJECT/worktree-a && rd progress $ITEM_3 --notes 'Tests at 80% coverage'  # no CF_HOME" \
    bash -c "cd '$PROJECT/worktree-a' && '$RD' progress '$ITEM_3' --notes 'Tests at 80% coverage'"

# Agent B posts progress on its items from worktree-b/ — no CF_HOME
run "cd \$PROJECT/worktree-b && rd progress $ITEM_2 --notes 'Auth endpoint complete'  # no CF_HOME" \
    bash -c "cd '$PROJECT/worktree-b' && '$RD' progress '$ITEM_2' --notes 'Auth endpoint complete'"
run "cd \$PROJECT/worktree-b && rd progress $ITEM_4 --notes 'Token bucket implemented'  # no CF_HOME" \
    bash -c "cd '$PROJECT/worktree-b' && '$RD' progress '$ITEM_4' --notes 'Token bucket implemented'"

# Owner syncs and sees all agent activity — from the project root
run "CF_HOME=\$OWNER_CF rd sync pull  (owner pulls latest state)" \
    bash -c "cd '$PROJECT' && CF_HOME='$OWNER_CF' '$RD' sync pull"

LIST_JSON=$(cd "$PROJECT" && CF_HOME="$OWNER_CF" "$RD" list --all --json 2>&1)
echo "$ CF_HOME=\$OWNER_CF rd list --all --json  (owner views agent activity)" | tee -a "$OUT"
echo "$LIST_JSON" | show_items | tee -a "$OUT"
echo "" | tee -a "$OUT"

# Count active items (claimed and progressed by agents)
ACTIVE_COUNT=$(echo "$LIST_JSON" | python3 -c "import sys,json; items=json.load(sys.stdin); print(sum(1 for i in items if i['status']=='active'))")
echo "# $ACTIVE_COUNT items active — claimed and progressed by two isolated agents" | tee -a "$OUT"
echo "" | tee -a "$OUT"

assert_contains "$LIST_JSON" "\"status\": \"active\"" \
    "owner sees active items claimed by isolated agents"

# Show that different agents claimed different items (distinct 'by' fields)
echo "# The 'by' field shows which agent claimed each item:" | tee -a "$OUT"
echo "$LIST_JSON" | python3 -c "
import sys, json
items = json.load(sys.stdin)
for i in items:
    by = i.get('by', '')
    if by:
        print(f'  {i[\"id\"]:24s}  [{i[\"status\"]:8s}]  by={by[:12]}...  {i[\"title\"]}')
    else:
        print(f'  {i[\"id\"]:24s}  [{i[\"status\"]:8s}]  (unassigned)        {i[\"title\"]}')
" | tee -a "$OUT"
echo "" | tee -a "$OUT"

# ════════════════════════════════════════════════════════════════════════════════
tee_section "Summary: What the filesystem hierarchy did"
# ════════════════════════════════════════════════════════════════════════════════

cat <<'EOF' | tee -a "$OUT"
# Walk-up resolution gives you zero-config agent isolation:
#
#   project/                    ← rd init: .campfire/root, .ready/
#   project/worktree-a/
#   project/worktree-a/.cf/     ← agent-A identity (walk-up stops here)
#   project/worktree-b/
#   project/worktree-b/.cf/     ← agent-B identity (walk-up stops here)
#
# When agent-A runs `rd` from worktree-a/:
#   CFHome()   → walks up → finds worktree-a/.cf/identity.json  → agent-A identity
#   campfire   → walks up → finds project/.campfire/root         → shared project
#
# When agent-B runs `rd` from worktree-b/:
#   CFHome()   → walks up → finds worktree-b/.cf/identity.json  → agent-B identity
#   campfire   → walks up → finds project/.campfire/root         → shared project
#
# No CF_HOME=. No --cf-home flags. No env var juggling.
# The directory you cd into IS your identity configuration.
EOF

echo "" | tee -a "$OUT"
echo "══════════════════════════════════════════════════════════════" | tee -a "$OUT"
echo "  Demo complete. Transcript saved to: $OUT" | tee -a "$OUT"
echo "══════════════════════════════════════════════════════════════" | tee -a "$OUT"
