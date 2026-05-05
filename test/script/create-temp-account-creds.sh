#!/usr/bin/env bash

set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
source "$SCRIPT_DIR/e2e-debug.sh"

if [ "$#" -ne 2 ]; then
  echo "usage: $0 <namespace> <account-name>" >&2
  exit 1
fi

NAMESPACE="$1"
ACCOUNT_NAME="$2"

log "resolve account signing material for $NAMESPACE/$ACCOUNT_NAME"
# Read account id from account metadata; generated account secrets are labelled with this id.
ACCOUNT_ID="$(kubectl get accounts.nauth.io "$ACCOUNT_NAME" -n "$NAMESPACE" -o jsonpath='{.metadata.labels.account\.nauth\.io/id}')"
if [ -z "$ACCOUNT_ID" ]; then
  echo "failed to resolve account id for $NAMESPACE/$ACCOUNT_NAME" >&2
  exit 2
fi

# Resolve the account root seed secret so the helper can mint temporary user creds.
ROOT_SEED_B64="$(kubectl get secret -n "$NAMESPACE" -l account.nauth.io/id="$ACCOUNT_ID",nauth.io/secret-type=account-root -o jsonpath='{.items[0].data.default}')"
if [ -z "$ROOT_SEED_B64" ]; then
  echo "failed to resolve account root seed secret for $NAMESPACE/$ACCOUNT_NAME" >&2
  exit 3
fi

# Decode to a shell variable only; never log the root seed.
ROOT_SEED="$(printf '%s' "$ROOT_SEED_B64" | base64 -d)"

log "generate temporary account creds for $NAMESPACE/$ACCOUNT_NAME"
go run "$SCRIPT_DIR/create-temp-account-creds.go" "$ACCOUNT_ID" "$ROOT_SEED"
