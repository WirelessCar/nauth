#!/bin/bash
# Usage: nauth-secret-assert-count.sh <namespace> <secret-type> <expected-count>
# Example: ./nauth-secret-assert-count.sh my-namespace account-root 2
# Example: ./nauth-secret-assert-count.sh my-namespace account-sign 0
#
# This script asserts that the number of Secrets with a specific nauth.io/secret-type label
# matches the expected count.

set -euo pipefail

if [ "$#" -ne 3 ]; then
  echo "[$0] Usage: $0 <namespace> <secret-type> <expected-count>" >&2
  echo "[$0] Example: $0 my-namespace account-root 2" >&2
  echo "[$0] Example: $0 my-namespace account-sign 0" >&2
  exit 1
fi

NAMESPACE="$1"
SECRET_TYPE="$2"
EXPECTED_COUNT="$3"

# Validate the expected-count parameter is a non-negative integer
if ! [[ "$EXPECTED_COUNT" =~ ^[0-9]+$ ]]; then
  echo "[$0] ERROR: expected-count must be a non-negative integer, got: $EXPECTED_COUNT" >&2
  exit 1
fi

LABEL_SELECTOR="nauth.io/secret-type=$SECRET_TYPE"

# Count secrets matching the label selector
ACTUAL_COUNT=$(kubectl get secrets -n "$NAMESPACE" -l "$LABEL_SELECTOR" --no-headers 2>/dev/null | wc -l | tr -d ' ')

# Evaluate the assertion
if [ "$ACTUAL_COUNT" -eq "$EXPECTED_COUNT" ]; then
  exit 0
else
  echo "[$0] ERROR: Found $ACTUAL_COUNT Secret(s), expected $EXPECTED_COUNT" >&2
  if [ "$ACTUAL_COUNT" -gt 0 ]; then
    echo "[$0] Listing found Secrets:" >&2
    kubectl get secrets -n "$NAMESPACE" -l "$LABEL_SELECTOR" >&2
  fi
  exit 1
fi

