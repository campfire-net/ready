#!/usr/bin/env bash
# 01-solo.sh — Solo developer demo: create → claim → progress → done
# Produces a real terminal transcript for documentation.
set -euo pipefail

RD=/tmp/rd-demo
OUTPUT_DIR="$(cd "$(dirname "$0")" && pwd)/output"
OUTPUT_FILE="$OUTPUT_DIR/01-solo.txt"

mkdir -p "$OUTPUT_DIR"

# Isolated environment
CF_HOME=$(mktemp -d /tmp/rdtest-solo-XXXX)
PROJECT=$(mktemp -d /tmp/rdtest-solo-proj-XXXX)
trap "rm -rf $CF_HOME $PROJECT" EXIT
export CF_HOME

# Tee all output to the transcript file
exec > >(tee "$OUTPUT_FILE") 2>&1

echo "=== SECTION: init ==="
echo "$ cf init --cf-home \"$CF_HOME\""
cf init --cf-home "$CF_HOME"

echo ""
echo "=== SECTION: project ==="
echo "$ cd PROJECT && rd init --name \"myproject\""
cd "$PROJECT"
"$RD" init --name "myproject" --cf-home "$CF_HOME"

echo ""
echo "=== SECTION: create ==="
echo '$ rd create "Ship login page" --priority p1 --type task'
ITEM_ID=$("$RD" create "Ship login page" --priority p1 --type task --cf-home "$CF_HOME")
echo "# item ID: $ITEM_ID"

echo ""
echo "=== SECTION: ready ==="
echo "$ rd ready"
"$RD" ready --cf-home "$CF_HOME"

echo ""
echo "=== SECTION: claim ==="
echo "$ rd claim $ITEM_ID"
"$RD" claim "$ITEM_ID" --cf-home "$CF_HOME"

echo ""
echo "=== SECTION: progress ==="
echo "$ rd progress $ITEM_ID --notes \"Wired up auth middleware\""
"$RD" progress "$ITEM_ID" --notes "Wired up auth middleware" --cf-home "$CF_HOME"

echo ""
echo "=== SECTION: done ==="
echo "$ rd done $ITEM_ID --reason \"Login page ships with JWT auth\""
"$RD" done "$ITEM_ID" --reason "Login page ships with JWT auth" --cf-home "$CF_HOME"

echo ""
echo "=== SECTION: verify ==="
echo "$ rd list --all"
"$RD" list --all --cf-home "$CF_HOME"

echo ""
echo "# Demo complete. Transcript written to: $OUTPUT_FILE"
