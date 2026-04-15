#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

go_version="$(awk '/^go / {print $2; exit}' go.mod)"
toolchain_version="$(awk '/^toolchain / {sub(/^go/, "", $2); print $2; exit}' go.mod)"
mise_go_version="$(awk -F'"' '/^go = / {print $2; exit}' mise.toml)"
docker_go_version="$(sed -n 's/^FROM golang:\([0-9.]*\)-alpine.*/\1/p' Dockerfile | head -n1)"

if [[ -z "$go_version" || -z "$mise_go_version" || -z "$docker_go_version" ]]; then
	echo "failed to parse one or more Go versions" >&2
	exit 1
fi

if [[ -n "$toolchain_version" && "$go_version" != "$toolchain_version" ]]; then
	echo "go.mod mismatch: go=$go_version toolchain=$toolchain_version" >&2
	exit 1
fi

if [[ "$mise_go_version" != "$docker_go_version" ]]; then
	echo "preferred toolchain mismatch: mise.toml=$mise_go_version Dockerfile=$docker_go_version" >&2
	exit 1
fi

version_ge() {
	[[ "$(printf '%s\n%s\n' "$1" "$2" | sort -V | head -n1)" == "$2" ]]
}

if ! version_ge "$mise_go_version" "$go_version"; then
	echo "mise.toml toolchain too old: minimum=$go_version mise.toml=$mise_go_version" >&2
	exit 1
fi

if ! version_ge "$docker_go_version" "$go_version"; then
	echo "Dockerfile toolchain too old: minimum=$go_version Dockerfile=$docker_go_version" >&2
	exit 1
fi

echo "Go versions are valid: minimum=$go_version preferred=$mise_go_version"
