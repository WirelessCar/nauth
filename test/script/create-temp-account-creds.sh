#!/bin/bash

set -euo pipefail

if [ "$#" -ne 2 ]; then
  echo "usage: $0 <namespace> <account-name>" >&2
  exit 1
fi

NAMESPACE="$1"
ACCOUNT_NAME="$2"
SCRIPT_DIR="$(cd "$(dirname "$(realpath "${BASH_SOURCE[0]}")")" && pwd)"

ACCOUNT_ID="$(kubectl get accounts.nauth.io "$ACCOUNT_NAME" -n "$NAMESPACE" -o jsonpath='{.metadata.labels.account\.nauth\.io/id}')"
if [ -z "$ACCOUNT_ID" ]; then
  echo "failed to resolve account id for $NAMESPACE/$ACCOUNT_NAME" >&2
  exit 2
fi

ROOT_SEED_B64="$(kubectl get secret -n "$NAMESPACE" -l account.nauth.io/id="$ACCOUNT_ID",nauth.io/secret-type=account-root -o jsonpath='{.items[0].data.default}')"
if [ -z "$ROOT_SEED_B64" ]; then
  echo "failed to resolve account root seed secret for $NAMESPACE/$ACCOUNT_NAME" >&2
  exit 3
fi

ROOT_SEED="$(printf '%s' "$ROOT_SEED_B64" | base64 -d)"

go run "$SCRIPT_DIR/create-temp-account-creds.go" "$ACCOUNT_ID" "$ROOT_SEED"
