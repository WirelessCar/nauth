#!/usr/bin/env bash
#MISE description="Generate CRD & Helm API Specs"
#MISE alias="docs"

set -euo pipefail

: "${MISE_PROJECT_ROOT:?MISE_PROJECT_ROOT must be set}"

crd_docs="$MISE_PROJECT_ROOT/www/src/content/docs/crds.md"

crd-ref-docs \
    --source-path="$MISE_PROJECT_ROOT/api" \
    --config="$MISE_PROJECT_ROOT/api/config.yaml" \
    --renderer=markdown \
    --output-path="$crd_docs"

# Add the required frontmatter for Starlight
tmp_docs="$(mktemp)"
trap 'rm -f "$tmp_docs"' EXIT

{
    cat <<'EOF'
---
title: API Reference
description: API reference for nauth CRDs
---

EOF
    # Remove the first H1 header since Starlight will generate it from frontmatter.
    # Also trim trailing blank lines so generated docs pass whitespace checks.
    awk '
        NR == 1 && $0 == "# API Reference" { next }
        { lines[++n] = $0 }
        END {
            while (n > 0 && lines[n] == "") {
                n--
            }
            for (i = 1; i <= n; i++) {
                print lines[i]
            }
        }
    ' "$crd_docs"
} > "$tmp_docs"

mv "$tmp_docs" "$crd_docs"

helm-docs --chart-search-root "$MISE_PROJECT_ROOT/charts"
