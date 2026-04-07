#!/usr/bin/env bash
# 03-multiproject.sh — Multi-project cross-dependency demo
# Shows rd init for two separate projects, item creation in each, and the
# clear error messages when cross-project dep wiring is attempted.
# Cross-project deps are not supported — rd dep operates within a single
# project's campfire. This demo documents what works and what doesn't.
# Produces a real terminal transcript for documentation.
set -euo pipefail

RD=/tmp/rd-demo
OUTPUT_DIR="$(cd "$(dirname "$0")" && pwd)/output"
OUTPUT_FILE="$OUTPUT_DIR/03-multiproject.txt"

mkdir -p "$OUTPUT_DIR"

# Isolated environment — both projects share the same CF_HOME (same identity)
CF_HOME=$(mktemp -d /tmp/rdtest-multi-XXXX)
FRONTEND=$(mktemp -d /tmp/rdtest-frontend-XXXX)
BACKEND=$(mktemp -d /tmp/rdtest-backend-XXXX)
trap "rm -rf $CF_HOME $FRONTEND $BACKEND" EXIT
export CF_HOME

# Tee all output to the transcript file
exec > >(tee "$OUTPUT_FILE") 2>&1

echo "=== SECTION: setup ==="
echo "$ cf init --cf-home \"$CF_HOME\""
cf init --cf-home "$CF_HOME"

echo ""
echo "=== SECTION: init-frontend ==="
echo "$ cd FRONTEND && rd init --name \"frontend\" --confirm"
cd "$FRONTEND"
"$RD" init --name "frontend" --confirm --cf-home "$CF_HOME"

echo ""
echo "=== SECTION: init-backend ==="
echo "$ cd BACKEND && rd init --name \"backend\" --confirm"
cd "$BACKEND"
"$RD" init --name "backend" --confirm --cf-home "$CF_HOME"

echo ""
echo "=== SECTION: create-items ==="
echo "$ cd BACKEND && rd create --title \"Expose /api/v1/users endpoint\" --priority p1 --type task --json"
cd "$BACKEND"
BACKEND_JSON=$("$RD" create --title "Expose /api/v1/users endpoint" --priority p1 --type task --json --cf-home "$CF_HOME")
echo "$BACKEND_JSON"
BACKEND_ID=$(echo "$BACKEND_JSON" | python3 -c "import sys,json; print(json.load(sys.stdin)['id'])")
echo "# backend item ID: $BACKEND_ID"

echo ""
echo "$ cd FRONTEND && rd create --title \"Build user list page\" --priority p1 --type task --json"
cd "$FRONTEND"
FRONTEND_JSON=$("$RD" create --title "Build user list page" --priority p1 --type task --json --cf-home "$CF_HOME")
echo "$FRONTEND_JSON"
FRONTEND_ID=$(echo "$FRONTEND_JSON" | python3 -c "import sys,json; print(json.load(sys.stdin)['id'])")
echo "# frontend item ID: $FRONTEND_ID"

echo ""
echo "=== SECTION: wire-dep ==="
echo "# Cross-project dep attempt 1: dotted syntax (project.item-id)"
echo "$ rd dep add $FRONTEND_ID backend.$BACKEND_ID"
cd "$FRONTEND"
"$RD" dep add "$FRONTEND_ID" "backend.$BACKEND_ID" --cf-home "$CF_HOME" 2>&1 || true

echo ""
echo "# Cross-project dep attempt 2: plain IDs across project dirs"
echo "# (items from different campfires are not visible to each other)"
echo "$ cd FRONTEND && rd dep add $FRONTEND_ID $BACKEND_ID"
cd "$FRONTEND"
"$RD" dep add "$FRONTEND_ID" "$BACKEND_ID" --cf-home "$CF_HOME" 2>&1 || true

echo ""
echo "# NOTE: Cross-project deps are not supported."
echo "# rd dep operates within a single project's campfire."
echo "# Wire dependencies within one project, or track cross-project"
echo "# coordination manually (e.g. item notes or campfire messages)."

echo ""
echo "# --- Within-project dep wiring (supported) ---"
echo "# Create a local placeholder tracking the backend dependency:"
echo "$ cd FRONTEND && rd create --title \"[backend] API endpoint ready\" --type task --priority p1 --json"
cd "$FRONTEND"
PLACEHOLDER_JSON=$("$RD" create --title "[backend] API endpoint ready" --type task --priority p1 --json --cf-home "$CF_HOME")
echo "$PLACEHOLDER_JSON"
PLACEHOLDER_ID=$(echo "$PLACEHOLDER_JSON" | python3 -c "import sys,json; print(json.load(sys.stdin)['id'])")
echo "# placeholder item ID: $PLACEHOLDER_ID"

echo ""
echo "=== SECTION: show-blocked ==="
echo "$ cd FRONTEND && rd dep add $FRONTEND_ID $PLACEHOLDER_ID"
cd "$FRONTEND"
"$RD" dep add "$FRONTEND_ID" "$PLACEHOLDER_ID" --cf-home "$CF_HOME"

echo ""
echo "$ rd dep tree $FRONTEND_ID"
"$RD" dep tree "$FRONTEND_ID" --cf-home "$CF_HOME"

echo ""
echo "$ rd ready"
"$RD" ready --cf-home "$CF_HOME"
echo "# (frontend item is blocked by placeholder — not shown in ready)"

echo ""
echo "=== SECTION: close-blocker ==="
echo "# Backend team ships the endpoint; mark the placeholder done:"
echo "$ rd done $PLACEHOLDER_ID --reason \"Backend confirmed /api/v1/users deployed\""
"$RD" done "$PLACEHOLDER_ID" --reason "Backend confirmed /api/v1/users deployed" --cf-home "$CF_HOME"

echo ""
echo "=== SECTION: verify-unblocked ==="
echo "$ rd ready"
"$RD" ready --cf-home "$CF_HOME"
echo "# frontend item is now unblocked and ready"

echo ""
echo "$ rd dep tree $FRONTEND_ID"
"$RD" dep tree "$FRONTEND_ID" --cf-home "$CF_HOME"

echo ""
echo "# Demo complete. Transcript written to: $OUTPUT_FILE"
