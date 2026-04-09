#!/usr/bin/env bash
# 03-multiproject.sh — Multi-project cross-dependency demo
# Shows rd init for two separate projects, item creation in each, and real
# cross-project dep wiring via local placeholder items.
# Cross-project deps are not natively supported — rd dep operates within a
# single project's campfire. This demo documents the recommended pattern.
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
echo "$ cd FRONTEND && rd init --name \"frontend\""
cd "$FRONTEND"
"$RD" init --name "frontend" --cf-home "$CF_HOME"

echo ""
echo "=== SECTION: init-backend ==="
echo "$ cd BACKEND && rd init --name \"backend\""
cd "$BACKEND"
"$RD" init --name "backend" --cf-home "$CF_HOME"

echo ""
echo "=== SECTION: create-items ==="
echo '$ cd BACKEND && rd create "Expose /api/v1/users endpoint" --priority p1 --type task'
cd "$BACKEND"
BACKEND_ID=$("$RD" create "Expose /api/v1/users endpoint" --priority p1 --type task --cf-home "$CF_HOME")
echo "# backend item ID: $BACKEND_ID"

echo ""
echo '$ cd FRONTEND && rd create "Build user list page" --priority p1 --type task'
cd "$FRONTEND"
FRONTEND_ID=$("$RD" create "Build user list page" --priority p1 --type task --cf-home "$CF_HOME")
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
echo "# --- Real cross-project dep wiring (supported pattern) ---"
echo "# Create a local placeholder in the frontend project tracking the backend dep:"
echo '$ cd FRONTEND && rd create "[backend] API endpoint ready" --type task --priority p1'
cd "$FRONTEND"
PLACEHOLDER_ID=$("$RD" create "[backend] API endpoint ready" --type task --priority p1 --cf-home "$CF_HOME")
echo "# placeholder item ID: $PLACEHOLDER_ID"

echo ""
echo "# Wire: frontend page blocked by placeholder"
echo "$ cd FRONTEND && rd dep add $FRONTEND_ID $PLACEHOLDER_ID"
cd "$FRONTEND"
"$RD" dep add "$FRONTEND_ID" "$PLACEHOLDER_ID" --cf-home "$CF_HOME"

echo ""
echo "=== SECTION: show-blocked ==="
echo "$ rd dep tree $FRONTEND_ID"
"$RD" dep tree "$FRONTEND_ID" --cf-home "$CF_HOME"

echo ""
echo "$ rd ready"
"$RD" ready --cf-home "$CF_HOME"
echo "# (frontend item is blocked by placeholder — not shown in ready)"

echo ""
echo "=== SECTION: close-blocker ==="
echo "# Backend team ships the endpoint; close the placeholder in the backend project:"
echo "$ cd BACKEND && rd update $BACKEND_ID --status active"
cd "$BACKEND"
"$RD" update "$BACKEND_ID" --status active --cf-home "$CF_HOME"

echo ""
echo "$ cd BACKEND && rd done $BACKEND_ID --reason \"API endpoint /api/v1/users deployed\""
"$RD" done "$BACKEND_ID" --reason "API endpoint /api/v1/users deployed" --cf-home "$CF_HOME"

echo ""
echo "# Mark the placeholder done (signal to frontend project that backend shipped):"
echo "$ cd FRONTEND && rd done $PLACEHOLDER_ID --reason \"Backend confirmed /api/v1/users deployed\""
cd "$FRONTEND"
"$RD" done "$PLACEHOLDER_ID" --reason "Backend confirmed /api/v1/users deployed" --cf-home "$CF_HOME"

echo ""
echo "=== SECTION: verify-unblocked ==="
echo "$ cd FRONTEND && rd ready"
"$RD" ready --cf-home "$CF_HOME"
echo "# frontend item is now unblocked and ready"

echo ""
echo "$ rd dep tree $FRONTEND_ID"
"$RD" dep tree "$FRONTEND_ID" --cf-home "$CF_HOME"

echo ""
echo "# Demo complete. Transcript written to: $OUTPUT_FILE"
