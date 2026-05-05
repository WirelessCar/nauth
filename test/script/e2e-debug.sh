#!/usr/bin/env bash

set -Eeuo pipefail

_e2e_debug_err() {
  local status=$?
  local line="${BASH_LINENO[0]:-${LINENO}}"
  local cmd="${BASH_COMMAND}"
  echo "e2e-debug: ERROR line ${line}: ${cmd} exited with ${status}" >&2
  exit "$status"
}

trap _e2e_debug_err ERR

log() {
  echo "e2e-debug: $*" >&2
}

run() {
  printf 'e2e-debug: running:' >&2
  printf ' %q' "$@" >&2
  printf '\n' >&2
  "$@"
}
