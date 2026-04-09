#!/usr/bin/env bash
# 03-multiproject.sh — Multi-project cross-dependency demo
# Shows real cross-project dep wiring: same identity, two projects,
# cross-campfire dep resolved automatically via shared CF_HOME.
# When the backend item closes, the frontend item becomes ready.
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
echo "$ cf init --cf-home \"\$CF_HOME\""
cf init --cf-home "$CF_HOME"

echo ""
echo "=== SECTION: init-backend ==="
echo "$ cd BACKEND && rd init --name \"backend\""
cd "$BACKEND"
"$RD" init --name "backend" --cf-home "$CF_HOME"

echo ""
echo "=== SECTION: init-frontend ==="
echo "$ cd FRONTEND && rd init --name \"frontend\""
cd "$FRONTEND"
"$RD" init --name "frontend" --cf-home "$CF_HOME"

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
echo "# Wire a real cross-project dep: frontend item blocked by backend item"
echo "# rd dep add resolves the blocker across all campfires in CF_HOME"
echo "$ cd FRONTEND && rd dep add $FRONTEND_ID $BACKEND_ID"
cd "$FRONTEND"
"$RD" dep add "$FRONTEND_ID" "$BACKEND_ID" --cf-home "$CF_HOME"

echo ""
echo "=== SECTION: show-blocked ==="
echo "$ cd FRONTEND && rd dep tree $FRONTEND_ID"
"$RD" dep tree "$FRONTEND_ID" --cf-home "$CF_HOME"

echo ""
echo "$ cd FRONTEND && rd ready"
"$RD" ready --cf-home "$CF_HOME"
echo "# (frontend item is blocked by backend item — not shown in ready)"

echo ""
echo "=== SECTION: close-blocker ==="
echo "# Backend team ships the endpoint:"
echo "$ cd BACKEND && rd update $BACKEND_ID --status active"
cd "$BACKEND"
"$RD" update "$BACKEND_ID" --status active --cf-home "$CF_HOME"

echo ""
echo "$ cd BACKEND && rd done $BACKEND_ID --reason \"API endpoint /api/v1/users deployed\""
"$RD" done "$BACKEND_ID" --reason "API endpoint /api/v1/users deployed" --cf-home "$CF_HOME"

echo ""
echo "=== SECTION: verify-unblocked ==="
echo "$ cd FRONTEND && rd ready"
cd "$FRONTEND"
"$RD" ready --cf-home "$CF_HOME"
echo "# frontend item is now unblocked and ready"

echo ""
echo "$ cd FRONTEND && rd dep tree $FRONTEND_ID"
"$RD" dep tree "$FRONTEND_ID" --cf-home "$CF_HOME"

echo ""
echo "# Demo complete. Transcript written to: $OUTPUT_FILE"
