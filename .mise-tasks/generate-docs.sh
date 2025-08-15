#!/usr/bin/env zsh
#MISE description="Generate CRD & Helm API Specs"
#MISE alias="docs"

crd-ref-docs --source-path=$MISE_PROJECT_ROOT/api --config=$MISE_PROJECT_ROOT/api/config.yaml --renderer=markdown --output-path=$MISE_PROJECT_ROOT/docs/crds.md

helm-docs
