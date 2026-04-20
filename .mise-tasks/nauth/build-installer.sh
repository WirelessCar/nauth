#!/usr/bin/env bash
# MISE description="Render a consolidated install manifest to dist/install.yaml"

set -euo pipefail

NAMESPACE="${NAMESPACE:-default}"
RELEASE_NAME="${RELEASE_NAME:-nauth}"
CHART_DIR="$MISE_PROJECT_ROOT/charts/nauth"
OUTPUT_PATH="$MISE_PROJECT_ROOT/dist/install.yaml"

mkdir -p "$(dirname "$OUTPUT_PATH")"
helm dependency update "$CHART_DIR"
helm template "$RELEASE_NAME" "$CHART_DIR" \
  --namespace "$NAMESPACE" \
  > "$OUTPUT_PATH"

echo "Wrote $OUTPUT_PATH"
