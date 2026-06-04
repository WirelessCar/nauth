#!/usr/bin/env bash
#MISE description="Update datreeio CRDs-catalog schemas for NAuth"
#USAGE arg "<catalog_dir>" help="Local datreeio/CRDs-catalog checkout"

set -euo pipefail

: "${MISE_PROJECT_ROOT:?MISE_PROJECT_ROOT must be set}"

catalog_dir="${usage_catalog_dir:?catalog_dir is required}"
crd_dir="$MISE_PROJECT_ROOT/charts/nauth-crds/crds"

if [[ ! -d "$catalog_dir" ]]; then
    echo "CRDs-catalog checkout does not exist: $catalog_dir" >&2
    exit 1
fi

catalog_dir="$(cd "$catalog_dir" && pwd -P)"
converter="$catalog_dir/Utilities/openapi2jsonschema.py"
if [[ ! -f "$converter" ]]; then
    echo "Missing catalog converter: $converter" >&2
    exit 1
fi

if [[ ! -d "$crd_dir" ]]; then
    echo "NAuth CRD directory does not exist: $crd_dir" >&2
    exit 1
fi

shopt -s nullglob
crds=("$crd_dir"/*.yaml)
shopt -u nullglob

if [[ ${#crds[@]} -eq 0 ]]; then
    echo "No NAuth CRDs found in $crd_dir" >&2
    exit 1
fi

tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT

(
    cd "$tmp_dir"
    FILENAME_FORMAT="{fullgroup}_{kind}_{version}" \
        uv run --no-project --with "pyyaml==6.0.2" python "$converter" "${crds[@]}"
)

shopt -s nullglob
generated_schemas=("$tmp_dir"/nauth.io_*.json)
shopt -u nullglob

if [[ ${#generated_schemas[@]} -eq 0 ]]; then
    echo "No nauth.io schemas were generated" >&2
    exit 1
fi

schema_dir="$catalog_dir/nauth.io"
mkdir -p "$schema_dir"
find "$schema_dir" -maxdepth 1 -type f -name '*.json' -delete

for schema in "${generated_schemas[@]}"; do
    schema_name="$(basename "$schema")"
    cp "$schema" "$schema_dir/${schema_name#nauth.io_}"
done

echo "Updated $schema_dir"
