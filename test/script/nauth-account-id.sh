#!/bin/bash
# Usage: nauth-account-id.sh <action> <namespace> <account-name>
# Example: ./nauth-account-id.sh RESOLVE my-namespace example-account
# Example: ./nauth-account-id.sh SAVE my-namespace example-account
# Example: ./nauth-account-id.sh READ my-namespace example-account
#
# This script manages account IDs from Account resources.
#
# Actions:
#   RESOLVE - Extract account ID from the Account resource and output to stdout
#   SAVE    - Extract account ID from the Account resource and save to temp file
#   READ    - Read account ID from temp file and output to stdout
#
# Temp file format: /tmp/<namespace>_<account-name>_accountId

set -euo pipefail

if [ "$#" -ne 3 ]; then
  echo "[$0] Usage: $0 <action> <namespace> <account-name>" >&2
  echo "[$0] Example: $0 RESOLVE my-namespace example-account" >&2
  echo "[$0] Example: $0 SAVE my-namespace example-account" >&2
  echo "[$0] Example: $0 READ my-namespace example-account" >&2
  echo "[$0]" >&2
  echo "[$0] Actions:" >&2
  echo "[$0]   RESOLVE - Extract account ID from resource and output to stdout" >&2
  echo "[$0]   SAVE    - Extract account ID from resource and save to temp file" >&2
  echo "[$0]   READ    - Read account ID from temp file and output to stdout" >&2
  exit 1
fi

ACTION="$1"
NAMESPACE="$2"
ACCOUNT_NAME="$3"

# Validate action parameter
if [ "$ACTION" != "RESOLVE" ] && [ "$ACTION" != "SAVE" ] && [ "$ACTION" != "READ" ]; then
  echo "[$0] ERROR: Action must be RESOLVE, SAVE, or READ, got: $ACTION" >&2
  exit 1
fi

# Define temp file path
TEMP_FILE="/tmp/${NAMESPACE}_${ACCOUNT_NAME}_accountId"

case "$ACTION" in
  RESOLVE)
    # Extract the account ID from the label
    ACCOUNT_ID=$(kubectl get account.nauth.io "$ACCOUNT_NAME" -n "$NAMESPACE" -o jsonpath='{.metadata.labels.account\.nauth\.io/id}' 2>&1)

    if [ -z "$ACCOUNT_ID" ]; then
      echo "[$0] ERROR: Account ID not found for account '$ACCOUNT_NAME' in namespace '$NAMESPACE'" >&2
      exit 1
    fi

    # Output the account ID to stdout
    echo "$ACCOUNT_ID"
    ;;

  SAVE)
    # Extract the account ID from the label

    # Check if the Account resource exists
    if ! kubectl get account.nauth.io "$ACCOUNT_NAME" -n "$NAMESPACE" &>/dev/null; then
      echo "[$0] ERROR: Account resource '$ACCOUNT_NAME' not found in namespace '$NAMESPACE'" >&2
      kubectl get accounts -n "$NAMESPACE" >&2
      exit 1
    fi

    # Extract the account ID from the label
    ACCOUNT_ID=$(kubectl get account.nauth.io "$ACCOUNT_NAME" -n "$NAMESPACE" -o jsonpath='{.metadata.labels.account\.nauth\.io/id}' 2>&1)

    if [ -z "$ACCOUNT_ID" ] || echo "$ACCOUNT_ID" | grep -q "error:"; then
      echo "[$0] ERROR: Account ID not found or extraction failed for account '$ACCOUNT_NAME' in namespace '$NAMESPACE'" >&2
      echo "[$0] Showing Account resource:" >&2
      kubectl get account.nauth.io "$ACCOUNT_NAME" -n "$NAMESPACE" -o yaml >&2
      exit 1
    fi

    # Save to temp file
    echo "$ACCOUNT_ID" > "$TEMP_FILE"
    echo "[$0] Saved Account ID to $TEMP_FILE: $ACCOUNT_ID" >&2

    # Also output to stdout
    echo "$ACCOUNT_ID"
    ;;

  READ)
    # Read from temp file
    if [ ! -f "$TEMP_FILE" ]; then
      echo "[$0] ERROR: Temp file not found: $TEMP_FILE" >&2
      echo "[$0] Make sure to run with SAVE action first" >&2
      exit 1
    fi

    ACCOUNT_ID=$(cat "$TEMP_FILE")

    if [ -z "$ACCOUNT_ID" ]; then
      echo "[$0] ERROR: Account ID is empty in temp file: $TEMP_FILE" >&2
      exit 1
    fi

    # Output to stdout
    echo "$ACCOUNT_ID"
    ;;
esac

