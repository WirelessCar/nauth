#!/bin/bash
# Usage: nauth-account-resource-absent.sh <namespace> <account-name>
# Example: ./nauth-account-resource-absent.sh my-namespace example-account
#
# This script verifies that an Account resource does NOT exist in the specified namespace.
# Returns success (exit 0) if the resource is absent, failure (exit 1) if it still exists.

set -euo pipefail

if [ "$#" -ne 2 ]; then
  echo "[$0] Usage: $0 <namespace> <account-name>" >&2
  echo "[$0] Example: $0 my-namespace example-account" >&2
  exit 1
fi

NAMESPACE="$1"
ACCOUNT_NAME="$2"

# Try to get the Account resource
if kubectl get account "$ACCOUNT_NAME" -n "$NAMESPACE" &>/dev/null; then
  echo "[$0] ERROR: Account '$ACCOUNT_NAME' still exists in namespace '$NAMESPACE' (expected to be deleted)" >&2
  echo "[$0] Showing Account resource:" >&2
  kubectl get account "$ACCOUNT_NAME" -n "$NAMESPACE" -o yaml >&2
  exit 1
else
  exit 0
fi

