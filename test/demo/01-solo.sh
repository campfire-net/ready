#!/usr/bin/env bash
# 01-solo.sh — Solo developer demo: create → claim → progress → done
# Produces a real terminal transcript for documentation.
set -euo pipefail

RD=/tmp/rd-demo
OUTPUT_DIR="$(cd "$(dirname "$0")" && pwd)/output"
OUTPUT_FILE="$OUTPUT_DIR/01-solo.txt"

mkdir -p "$OUTPUT_DIR"

# Isolated environment — identity lives inside the project directory
PROJECT=$(mktemp -d /tmp/rdtest-solo-proj-XXXX)
trap "rm -rf $PROJECT" EXIT

# Tee all output to the transcript file
exec > >(tee "$OUTPUT_FILE") 2>&1

# One-time: create project-local .cf/ and initialize identity there
mkdir -p "$PROJECT/.cf"
echo "=== SECTION: init ==="
echo "$ mkdir -p \$PROJECT/.cf && cf init --cf-home \$PROJECT/.cf"
cf init --cf-home "$PROJECT/.cf"

# From now on: cd into the project and run — walk-up finds .cf/identity.json
cd "$PROJECT"

echo ""
echo "=== SECTION: project ==="
echo "$ cd PROJECT && rd init --name \"myproject\""
"$RD" init --name "myproject"

echo ""
echo "=== SECTION: create ==="
echo '$ rd create "Ship login page" --priority p1 --type task'
ITEM_ID=$("$RD" create "Ship login page" --priority p1 --type task)
echo "# item ID: $ITEM_ID"

echo ""
echo "=== SECTION: ready ==="
echo "$ rd ready"
"$RD" ready

echo ""
echo "=== SECTION: claim ==="
echo "$ rd claim $ITEM_ID"
"$RD" claim "$ITEM_ID"

echo ""
echo "=== SECTION: progress ==="
echo "$ rd progress $ITEM_ID --notes \"Wired up auth middleware\""
"$RD" progress "$ITEM_ID" --notes "Wired up auth middleware"

echo ""
echo "=== SECTION: done ==="
echo "$ rd done $ITEM_ID --reason \"Login page ships with JWT auth\""
"$RD" done "$ITEM_ID" --reason "Login page ships with JWT auth"

echo ""
echo "=== SECTION: verify ==="
echo "$ rd list --all"
"$RD" list --all

echo ""
echo "# Demo complete. Transcript written to: $OUTPUT_FILE"
