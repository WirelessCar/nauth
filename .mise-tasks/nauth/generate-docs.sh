#!/usr/bin/env zsh
#MISE description="Generate CRD & Helm API Specs"
#MISE alias="docs"

crd-ref-docs --source-path=$MISE_PROJECT_ROOT/api --config=$MISE_PROJECT_ROOT/api/config.yaml --renderer=markdown --output-path=$MISE_PROJECT_ROOT/www/src/content/docs/crds.md

# Add the required frontmatter for Starlight
sed -i '1i\
---\
title: API Reference\
description: Complete API reference for nauth CRDs\
---\
' "$MISE_PROJECT_ROOT/www/src/content/docs/crds.md"

# Remove the first H1 header since Starlight will generate it from frontmatter
sed -i '/^# API Reference$/d' "$MISE_PROJECT_ROOT/www/src/content/docs/crds.md"

helm-docs
