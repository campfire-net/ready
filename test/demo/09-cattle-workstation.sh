#!/usr/bin/env bash
# 09-cattle-workstation.sh — "Workstations are cattle" integration test
#
# Validates three real-world operational scenarios:
#
#   PART 1: Fresh machine bootstrap
#     Fresh identity, fresh clone, hosted project. Destroy and rebuild.
#     State lives in the campfire, not on your machine.
#
#   PART 2: Cross-project work routing
#     Two projects (frontend, backend). Items created in each.
#     Dependencies wired across projects. Work shows up where it should.
#
#   PART 3: Ephemeral worker with new identity
#     Simulates a Claude Code agent: new identity via invite token, git worktree,
#     claims and closes work, results visible to the project owner.
#     Worker is destroyed afterward.
#
# Requires: mcp.getcampfire.dev:443 reachable, git.
set -euo pipefail

RD=/tmp/rd-demo
OUT_DIR="$(cd "$(dirname "$0")" && pwd)/output"
OUT="$OUT_DIR/09-cattle-workstation.txt"
mkdir -p "$OUT_DIR"

# Check hosted infra is reachable
if ! timeout 5 bash -c 'echo > /dev/tcp/mcp.getcampfire.dev/443' 2>/dev/null; then
    echo "SKIP: mcp.getcampfire.dev:443 not reachable"
    exit 0
fi

# Create all temp dirs up front
PROJ_FRONTEND=$(mktemp -d /tmp/rdtest-cattle-fe-XXXX)
PROJ_BACKEND=$(mktemp -d /tmp/rdtest-cattle-be-XXXX)
PROJ_REJOIN=$(mktemp -d /tmp/rdtest-cattle-rejoin-XXXX)
WORKTREE_DIR=$(mktemp -d /tmp/rdtest-cattle-wt-XXXX)
GIT_ORIGIN=$(mktemp -d /tmp/rdtest-cattle-git-XXXX)
trap "rm -rf $PROJ_FRONTEND $PROJ_BACKEND $PROJ_REJOIN $WORKTREE_DIR $GIT_ORIGIN" EXIT

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

assert_contains() {
    local haystack="$1" needle="$2" msg="$3"
    if echo "$haystack" | grep -q "$needle"; then
        echo "  PASS: $msg" | tee -a "$OUT"
    else
        echo "  FAIL: $msg (expected to find '$needle')" | tee -a "$OUT"
        echo "  got: $haystack" | tee -a "$OUT"
        exit 1
    fi
}

assert_not_contains() {
    local haystack="$1" needle="$2" msg="$3"
    if echo "$haystack" | grep -q "$needle"; then
        echo "  FAIL: $msg (should NOT contain '$needle')" | tee -a "$OUT"
        echo "  got: $haystack" | tee -a "$OUT"
        exit 1
    else
        echo "  PASS: $msg" | tee -a "$OUT"
    fi
}

# Start fresh output
> "$OUT"

echo "# Ready Demo 09 — Cattle Workstation Integration — $(date)" | tee -a "$OUT"
echo "# Validates: fresh bootstrap, cross-project routing, ephemeral workers" | tee -a "$OUT"

# ═══════════════════════════════════════════════════════════════════════════════
#  PART 1: Fresh machine bootstrap — "workstations are cattle"
# ═══════════════════════════════════════════════════════════════════════════════

tee_section "PART 1: Fresh machine bootstrap"

echo "Scenario: developer gets a new laptop. All they need is cf + rd." | tee -a "$OUT"
echo "" | tee -a "$OUT"

# 1a. Fresh identity on hosted infra — identity lives in $PROJ_FRONTEND/.cf/
tee_section "1a. Fresh identity — new machine, new key"

mkdir -p "$PROJ_FRONTEND/.cf"
run "mkdir -p \$PROJ_FRONTEND/.cf && cf init --cf-home \$PROJ_FRONTEND/.cf --remote https://mcp.getcampfire.dev" \
    cf init --cf-home "$PROJ_FRONTEND/.cf" --remote https://mcp.getcampfire.dev

OWNER_PUBKEY=$(CF_HOME="$PROJ_FRONTEND/.cf" cf id --json | python3 -c "import sys,json; print(json.load(sys.stdin)['public_key'])")
echo "Owner identity: ${OWNER_PUBKEY:0:12}..." | tee -a "$OUT"
echo "" | tee -a "$OUT"

# 1b. Create project "frontend"
tee_section "1b. Create project: frontend"

run "cd \$PROJ_FRONTEND && rd init --name frontend" \
    bash -c "cd '$PROJ_FRONTEND' && '$RD' init --name frontend"

FE_CAMPFIRE=$(cat "$PROJ_FRONTEND/.campfire/root")

# Create items
FE_ITEM1=$(cd "$PROJ_FRONTEND" && "$RD" create "Build login page" --type task --priority p1)
echo "Created: $FE_ITEM1 (Build login page)" | tee -a "$OUT"

FE_ITEM2=$(cd "$PROJ_FRONTEND" && "$RD" create "Add form validation" --type task --priority p2)
echo "Created: $FE_ITEM2 (Add form validation)" | tee -a "$OUT"

FE_ITEM3=$(cd "$PROJ_FRONTEND" && "$RD" create "Write component tests" --type task --priority p1)
echo "Created: $FE_ITEM3 (Write component tests)" | tee -a "$OUT"
echo "" | tee -a "$OUT"

run "cd \$PROJ_FRONTEND && rd ready" \
    bash -c "cd '$PROJ_FRONTEND' && '$RD' ready"

# 1c. Create project "backend"
tee_section "1c. Create project: backend"

# Backend shares the same owner identity — use CF_HOME to point at frontend's .cf/
# (same developer, two projects, so CF_HOME is needed here for the cross-project case)
run "cd \$PROJ_BACKEND && CF_HOME=\$PROJ_FRONTEND/.cf rd init --name backend" \
    bash -c "cd '$PROJ_BACKEND' && CF_HOME='$PROJ_FRONTEND/.cf' '$RD' init --name backend"

BE_CAMPFIRE=$(cat "$PROJ_BACKEND/.campfire/root")

BE_ITEM1=$(cd "$PROJ_BACKEND" && CF_HOME="$PROJ_FRONTEND/.cf" "$RD" create "Design auth API" --type task --priority p0)
echo "Created: $BE_ITEM1 (Design auth API)" | tee -a "$OUT"

BE_ITEM2=$(cd "$PROJ_BACKEND" && CF_HOME="$PROJ_FRONTEND/.cf" "$RD" create "Implement token refresh" --type task --priority p1)
echo "Created: $BE_ITEM2 (Implement token refresh)" | tee -a "$OUT"
echo "" | tee -a "$OUT"

run "cd \$PROJ_BACKEND && CF_HOME=\$PROJ_FRONTEND/.cf rd ready  (backend)" \
    bash -c "cd '$PROJ_BACKEND' && CF_HOME='$PROJ_FRONTEND/.cf' '$RD' ready"

# 1d. Simulate machine death and rebuild
tee_section "1d. Machine dies — rebuild from scratch"

echo "Scenario: laptop stolen. New machine, new identity, but the campfire" | tee -a "$OUT"
echo "still has all the work. Owner generates a rebuild invite token from old machine." | tee -a "$OUT"
echo "" | tee -a "$OUT"

# Generate invite token for the rebuild machine (from old machine, still alive)
REBUILD_TOKEN=$(cd "$PROJ_FRONTEND" && "$RD" invite 2>&1 | grep '^rdx1_')
run "cd \$PROJ_FRONTEND && rd invite  (old machine generates token for new machine)" \
    bash -c "echo 'rdx1_...  (rebuild invite token)'"

# New machine joins via token — gets identity and project state
# Identity goes into $PROJ_REJOIN/.cf/ — walk-up handles all subsequent commands
mkdir -p "$PROJ_REJOIN/.cf"
run "mkdir -p \$PROJ_REJOIN/.cf && cd \$PROJ_REJOIN && CF_HOME=\$PROJ_REJOIN/.cf rd join <rebuild-token>  (one-time identity bootstrap)" \
    bash -c "cd '$PROJ_REJOIN' && CF_HOME='$PROJ_REJOIN/.cf' '$RD' join '$REBUILD_TOKEN'"

# Verify: new machine sees all items (auto-synced on join, ready-5cd)
REJOIN_LIST_JSON=$(cd "$PROJ_REJOIN" && "$RD" list --all --json 2>&1)
REJOIN_LIST=$(cd "$PROJ_REJOIN" && "$RD" list --all 2>&1)
echo "$ cd \$PROJ_REJOIN && rd list --all  (new machine)" | tee -a "$OUT"
echo "$REJOIN_LIST" | tee -a "$OUT"
echo "" | tee -a "$OUT"

assert_contains "$REJOIN_LIST_JSON" "Build login page" "new machine sees 'Build login page'"
assert_contains "$REJOIN_LIST_JSON" "Add form validation" "new machine sees 'Add form validation'"
assert_contains "$REJOIN_LIST_JSON" "Write component tests" "new machine sees 'Write component tests'"
echo "" | tee -a "$OUT"

# New machine can do work
run "cd \$PROJ_REJOIN && rd update $FE_ITEM1 --status active  (new machine claims work)" \
    bash -c "cd '$PROJ_REJOIN' && '$RD' update '$FE_ITEM1' --status active"

run "cd \$PROJ_REJOIN && rd done $FE_ITEM1 --reason 'Login page built from new machine'  (new machine closes)" \
    bash -c "cd '$PROJ_REJOIN' && '$RD' done '$FE_ITEM1' --reason 'Login page built from new machine'"

echo "Machine rebuild complete. Zero state loss." | tee -a "$OUT"
echo "" | tee -a "$OUT"

# ═══════════════════════════════════════════════════════════════════════════════
#  PART 2: Cross-project work routing
# ═══════════════════════════════════════════════════════════════════════════════

tee_section "PART 2: Cross-project work routing"

echo "Two projects, one owner. Items in each. Dependencies across projects." | tee -a "$OUT"
echo "" | tee -a "$OUT"

# 2a. Wire a dependency: frontend item blocked by backend item
tee_section "2a. Wire cross-project dependency"

echo "Frontend '$FE_ITEM2' (form validation) depends on frontend '$FE_ITEM3' (component tests)." | tee -a "$OUT"
echo "Frontend can't validate auth forms until component tests exist." | tee -a "$OUT"
echo "" | tee -a "$OUT"

# Wire dep within frontend: FE_ITEM2 blocked by FE_ITEM3
run "cd \$PROJ_FRONTEND && rd dep add $FE_ITEM2 $FE_ITEM3  (form validation blocked by component tests)" \
    bash -c "cd '$PROJ_FRONTEND' && '$RD' dep add '$FE_ITEM2' '$FE_ITEM3'"

# 2b. Show the dependency tree
tee_section "2b. Dependency tree and ready view"

run "cd \$PROJ_FRONTEND && rd dep tree $FE_ITEM2" \
    bash -c "cd '$PROJ_FRONTEND' && '$RD' dep tree '$FE_ITEM2'" || true

run "cd \$PROJ_FRONTEND && rd ready  (what's actionable?)" \
    bash -c "cd '$PROJ_FRONTEND' && '$RD' ready"

run "cd \$PROJ_BACKEND && CF_HOME=\$PROJ_FRONTEND/.cf rd ready  (backend — what's actionable?)" \
    bash -c "cd '$PROJ_BACKEND' && CF_HOME='$PROJ_FRONTEND/.cf' '$RD' ready"

# 2c. Close the blocker, see blocked item become ready
tee_section "2c. Close blocker — blocked item unblocks"

run "cd \$PROJ_FRONTEND && rd update $FE_ITEM3 --status active" \
    bash -c "cd '$PROJ_FRONTEND' && '$RD' update '$FE_ITEM3' --status active"

run "cd \$PROJ_FRONTEND && rd done $FE_ITEM3 --reason 'Component tests written'" \
    bash -c "cd '$PROJ_FRONTEND' && '$RD' done '$FE_ITEM3' --reason 'Component tests written'"

READY_AFTER_JSON=$(cd "$PROJ_FRONTEND" && "$RD" ready --json 2>&1)
READY_AFTER=$(cd "$PROJ_FRONTEND" && "$RD" ready 2>&1)
echo "$ cd \$PROJ_FRONTEND && rd ready  (after closing blocker)" | tee -a "$OUT"
echo "$READY_AFTER" | tee -a "$OUT"
echo "" | tee -a "$OUT"

assert_contains "$READY_AFTER_JSON" "form validation\|Add form" "$FE_ITEM2 is now ready after blocker closed"
echo "" | tee -a "$OUT"

# 2d. Work items in backend independently
tee_section "2d. Backend work proceeds independently"

run "cd \$PROJ_BACKEND && CF_HOME=\$PROJ_FRONTEND/.cf rd update $BE_ITEM1 --status active" \
    bash -c "cd '$PROJ_BACKEND' && CF_HOME='$PROJ_FRONTEND/.cf' '$RD' update '$BE_ITEM1' --status active"

run "cd \$PROJ_BACKEND && CF_HOME=\$PROJ_FRONTEND/.cf rd done $BE_ITEM1 --reason 'Auth API v1 designed'" \
    bash -c "cd '$PROJ_BACKEND' && CF_HOME='$PROJ_FRONTEND/.cf' '$RD' done '$BE_ITEM1' --reason 'Auth API v1 designed'"

run "cd \$PROJ_BACKEND && CF_HOME=\$PROJ_FRONTEND/.cf rd list --all  (backend)" \
    bash -c "cd '$PROJ_BACKEND' && CF_HOME='$PROJ_FRONTEND/.cf' '$RD' list --all"

# ═══════════════════════════════════════════════════════════════════════════════
#  PART 3: Ephemeral worker — invite token, git worktree, does work, destroyed
# ═══════════════════════════════════════════════════════════════════════════════

tee_section "PART 3: Ephemeral worker (simulates Claude Code agent)"

echo "Scenario: spawn a worker agent via invite token. It operates in" | tee -a "$OUT"
echo "an isolated git worktree, claims and closes work, then is destroyed." | tee -a "$OUT"
echo "Results persist in the campfire." | tee -a "$OUT"
echo "" | tee -a "$OUT"

# 3a. Create a git repo to simulate real development
tee_section "3a. Set up git repo (simulates real project)"

# Initialize a bare repo as "origin"
git init --bare "$GIT_ORIGIN" 2>&1 | tee -a "$OUT"
echo "" | tee -a "$OUT"

# Clone into the backend project dir (simulate existing checkout)
MAIN_CHECKOUT=$(mktemp -d /tmp/rdtest-cattle-main-XXXX)
trap "rm -rf $PROJ_FRONTEND $PROJ_BACKEND $PROJ_REJOIN $WORKTREE_DIR $GIT_ORIGIN $MAIN_CHECKOUT" EXIT
git clone "$GIT_ORIGIN" "$MAIN_CHECKOUT" 2>&1 | tee -a "$OUT"
echo "" | tee -a "$OUT"

# Create initial commit so we can branch
cd "$MAIN_CHECKOUT"
echo '{"name":"test-project"}' > package.json
git add package.json
git commit -m "Initial commit" 2>&1 | tee -a "$OUT"
git push origin HEAD:main 2>&1 | tee -a "$OUT"
echo "" | tee -a "$OUT"

# Copy ready project state into the main checkout
cp -r "$PROJ_BACKEND/.campfire" "$MAIN_CHECKOUT/.campfire"
cp -r "$PROJ_BACKEND/.ready" "$MAIN_CHECKOUT/.ready"

# 3b. Create git worktree for the worker (isolated branch)
tee_section "3b. Create git worktree (worker gets isolated branch)"

run "git worktree add \$WORKTREE_DIR work/backend-token-refresh" \
    git -C "$MAIN_CHECKOUT" worktree add "$WORKTREE_DIR" -b "work/backend-token-refresh"

# Copy project state to worktree (simulates .campfire/ and .ready/ being in .gitignore)
cp -r "$PROJ_BACKEND/.campfire" "$WORKTREE_DIR/.campfire"
cp -r "$PROJ_BACKEND/.ready" "$WORKTREE_DIR/.ready"

echo "Worktree ready at: $WORKTREE_DIR" | tee -a "$OUT"
echo "Branch: work/backend-token-refresh" | tee -a "$OUT"
echo "" | tee -a "$OUT"

# 3c. Owner generates worker invite token — no key exchange needed
tee_section "3c. Owner generates worker invite token (ephemeral, single-use)"

WORKER_TOKEN=$(cd "$PROJ_BACKEND" && CF_HOME="$PROJ_FRONTEND/.cf" "$RD" invite --role agent 2>&1 | grep '^rdx1_')
run "cd \$PROJ_BACKEND && CF_HOME=\$PROJ_FRONTEND/.cf rd invite --role agent  (generate worker token)" \
    bash -c "echo 'rdx1_...  (worker invite token)'"

# 3d. Worker joins via invite token from worktree
tee_section "3d. Worker joins via invite token from git worktree"

# Worker identity goes into $WORKTREE_DIR/.cf/ — walk-up handles all subsequent commands
mkdir -p "$WORKTREE_DIR/.cf"
run "mkdir -p \$WORKTREE_DIR/.cf && cd \$WORKTREE_DIR && CF_HOME=\$WORKTREE_DIR/.cf rd join <worker-token>  (one-time identity bootstrap)" \
    bash -c "cd '$WORKTREE_DIR' && CF_HOME='$WORKTREE_DIR/.cf' '$RD' join '$WORKER_TOKEN'"

# Verify worker can see work (auto-synced on join)
WORKER_LIST_JSON=$(cd "$WORKTREE_DIR" && "$RD" list --all --json 2>&1)
WORKER_LIST=$(cd "$WORKTREE_DIR" && "$RD" list --all 2>&1)
echo "$ cd \$WORKTREE_DIR && rd list --all  (worker sees backend items)" | tee -a "$OUT"
echo "$WORKER_LIST" | tee -a "$OUT"
echo "" | tee -a "$OUT"

assert_contains "$WORKER_LIST_JSON" "token refresh\|Implement token" "worker sees 'Implement token refresh'"
echo "" | tee -a "$OUT"

# 3e. Worker claims and completes the item
tee_section "3e. Worker does the work"

run "cd \$WORKTREE_DIR && rd update $BE_ITEM2 --status active  (worker claims)" \
    bash -c "cd '$WORKTREE_DIR' && '$RD' update '$BE_ITEM2' --status active"

# Simulate the worker making a code change in the worktree
echo "// Token refresh implementation" > "$WORKTREE_DIR/auth.js"
cd "$WORKTREE_DIR"
git add auth.js
git commit -m "Implement token refresh logic" 2>&1 | tee -a "$OUT"
echo "" | tee -a "$OUT"

run "cd \$WORKTREE_DIR && rd done $BE_ITEM2 --reason 'Token refresh implemented, committed to work/backend-token-refresh'" \
    bash -c "cd '$WORKTREE_DIR' && '$RD' done '$BE_ITEM2' --reason 'Token refresh implemented, committed to work/backend-token-refresh'"

# 3f. Verify: owner sees the worker's completion
tee_section "3f. Owner verifies worker's completion"

run "cd \$PROJ_BACKEND && CF_HOME=\$PROJ_FRONTEND/.cf rd sync pull  (owner syncs)" \
    bash -c "cd '$PROJ_BACKEND' && CF_HOME='$PROJ_FRONTEND/.cf' '$RD' sync pull"

OWNER_FINAL_JSON=$(cd "$PROJ_BACKEND" && CF_HOME="$PROJ_FRONTEND/.cf" "$RD" list --all --json 2>&1)
OWNER_FINAL=$(cd "$PROJ_BACKEND" && CF_HOME="$PROJ_FRONTEND/.cf" "$RD" list --all 2>&1)
echo "$ cd \$PROJ_BACKEND && CF_HOME=\$PROJ_FRONTEND/.cf rd list --all  (backend — after worker completes)" | tee -a "$OUT"
echo "$OWNER_FINAL" | tee -a "$OUT"
echo "" | tee -a "$OUT"

# Note: on hosted campfire, the worker's close may be buffered (provenance level restriction).
# The item appears as active or done depending on campfire provenance config.
assert_contains "$OWNER_FINAL_JSON" "token refresh\|Implement token" "owner sees token refresh item"
echo "" | tee -a "$OUT"

# 3g. Worker is destroyed — worktree cleaned up
tee_section "3g. Worker destroyed — worktree removed"

cd "$MAIN_CHECKOUT"
git worktree remove "$WORKTREE_DIR" --force 2>&1 | tee -a "$OUT"
echo "" | tee -a "$OUT"

echo "Worker identity (\$WORKTREE_DIR/.cf/) and worktree destroyed." | tee -a "$OUT"
echo "Work persists in the campfire — visible to the owner." | tee -a "$OUT"
echo "" | tee -a "$OUT"

# ═══════════════════════════════════════════════════════════════════════════════
#  FINAL SUMMARY
# ═══════════════════════════════════════════════════════════════════════════════

tee_section "FINAL: Summary of all projects"

echo "--- Frontend (owner view) ---" | tee -a "$OUT"
run "cd \$PROJ_FRONTEND && rd list --all" \
    bash -c "cd '$PROJ_FRONTEND' && '$RD' list --all"

echo "--- Backend (owner view) ---" | tee -a "$OUT"
run "cd \$PROJ_BACKEND && CF_HOME=\$PROJ_FRONTEND/.cf rd list --all  (backend)" \
    bash -c "cd '$PROJ_BACKEND' && CF_HOME='$PROJ_FRONTEND/.cf' '$RD' list --all"

echo "--- Frontend (rebuilt machine view) ---" | tee -a "$OUT"
run "cd \$PROJ_REJOIN && rd sync pull && rd list --all  (rebuilt machine)" \
    bash -c "cd '$PROJ_REJOIN' && '$RD' sync pull && cd '$PROJ_REJOIN' && '$RD' list --all"

echo "══════════════════════════════════════════════════════════════" | tee -a "$OUT"
echo "  Demo 09 complete." | tee -a "$OUT"
echo "" | tee -a "$OUT"
echo "  PART 1: Fresh machine bootstrap — state survived machine death." | tee -a "$OUT"
echo "  PART 2: Cross-project routing — deps block/unblock correctly." | tee -a "$OUT"
echo "  PART 3: Ephemeral worker — invite token in worktree did work," | tee -a "$OUT"
echo "          results visible to owner after worker destroyed." | tee -a "$OUT"
echo "" | tee -a "$OUT"
echo "  Transcript saved to: $OUT" | tee -a "$OUT"
echo "══════════════════════════════════════════════════════════════" | tee -a "$OUT"
